package mailauth

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"strings"

	"github.com/emersion/go-msgauth/dmarc"
)

// DMARC results recorded in authentication_results.dmarc.
const (
	DMARCPass      = "pass"
	DMARCFail      = "fail"
	DMARCNone      = "none" // no policy published, or no From domain
	DMARCTempError = "temperror"
	DMARCPermError = "permerror" // the published record is invalid
)

// DMARCInput is what the SMTP edge learned about one message.
type DMARCInput struct {
	FromDomain string    // RFC 5322 From domain
	DKIMPass   []string  // d= domains of signatures that verified
	SPF        SPFResult // MAIL FROM result
	SPFDomain  string    // MAIL FROM domain, or the HELO name for a null reverse path
}

// DMARCVerdict is the outcome of a DMARC evaluation.
type DMARCVerdict struct {
	Result string
	// Policy is the policy to apply on failure (none, quarantine, reject)
	// after sp= and pct= are taken into account; empty when no record exists.
	Policy string
}

// CheckDMARC looks up the DMARC record of the From domain (falling back to
// its organizational domain, RFC 7489 section 6.6.3) and checks that a
// passing DKIM signature or a passing SPF identity aligns with the From
// domain in relaxed or strict mode as the record asks. sample returns a value
// in [0, 100) for pct= sampling; nil uses math/rand.
func CheckDMARC(ctx context.Context, r Resolver, in DMARCInput, sample func() int) DMARCVerdict {
	from := strings.ToLower(strings.TrimSuffix(in.FromDomain, "."))
	if !validDomain(from) {
		return DMARCVerdict{Result: DMARCNone}
	}
	org := OrgDomain(from)
	rec, err := lookupDMARC(ctx, r, from)
	atOrg := false
	if errors.Is(err, dmarc.ErrNoPolicy) && org != from {
		rec, err = lookupDMARC(ctx, r, org)
		atOrg = true
	}
	switch {
	case errors.Is(err, dmarc.ErrNoPolicy):
		return DMARCVerdict{Result: DMARCNone}
	case dmarc.IsTempFail(err):
		return DMARCVerdict{Result: DMARCTempError}
	case err != nil:
		return DMARCVerdict{Result: DMARCPermError}
	}
	policy := string(rec.Policy)
	if atOrg && rec.SubdomainPolicy != "" {
		policy = string(rec.SubdomainPolicy)
	}
	if rec.Percent != nil && *rec.Percent < 100 {
		if sample == nil {
			sample = func() int { return rand.IntN(100) }
		}
		if sample() >= *rec.Percent {
			// Section 6.6.4: messages outside the sample get the next policy down.
			switch policy {
			case dmarc.PolicyReject:
				policy = dmarc.PolicyQuarantine
			case dmarc.PolicyQuarantine:
				policy = string(dmarc.PolicyNone)
			}
		}
	}
	v := DMARCVerdict{Result: DMARCFail, Policy: policy}
	for _, d := range in.DKIMPass {
		if Aligned(d, from, rec.DKIMAlignment == dmarc.AlignmentStrict) {
			v.Result = DMARCPass
			return v
		}
	}
	if in.SPF == SPFPass && Aligned(in.SPFDomain, from, rec.SPFAlignment == dmarc.AlignmentStrict) {
		v.Result = DMARCPass
	}
	return v
}

// Aligned reports identifier alignment (RFC 7489 section 3.1): strict needs
// the same domain, relaxed the same organizational domain.
func Aligned(domain, fromDomain string, strict bool) bool {
	a := strings.ToLower(strings.TrimSuffix(domain, "."))
	b := strings.ToLower(strings.TrimSuffix(fromDomain, "."))
	if a == "" || b == "" {
		return false
	}
	if strict {
		return a == b
	}
	return OrgDomain(a) == OrgDomain(b)
}

func lookupDMARC(ctx context.Context, r Resolver, domain string) (*dmarc.Record, error) {
	return dmarc.LookupWithOptions(domain, &dmarc.LookupOptions{LookupTXT: func(name string) ([]string, error) {
		txts, err := r.LookupTXT(ctx, name)
		if err != nil && !notFound(err) {
			return nil, tempDNSError{err} // go-msgauth reports net.Error.Temporary as a temporary failure
		}
		return txts, err
	}})
}

type tempDNSError struct{ err error }

func (e tempDNSError) Error() string   { return e.err.Error() }
func (e tempDNSError) Timeout() bool   { return false }
func (e tempDNSError) Temporary() bool { return true }

var _ net.Error = tempDNSError{}
