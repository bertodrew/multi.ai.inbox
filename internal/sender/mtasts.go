package sender

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MTA-STS modes (RFC 8461 section 5).
const (
	stsEnforce = "enforce"
	stsTesting = "testing"
	stsNone    = "none"
)

const (
	stsMaxAge    = 31557600 // seconds, RFC 8461 section 3.2
	stsMaxPolicy = 64 << 10 // bytes, section 3.3 suggests a limit
)

// TXTResolver is the DNS lookup MTA-STS needs (net.Resolver satisfies it).
type TXTResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// MTASTS discovers, fetches and caches MTA-STS policies (RFC 8461) for
// direct delivery. Policies are cached per domain until max_age runs out; a
// new _mta-sts TXT id triggers a refetch. When the TXT record disappears or
// a refetch fails, a cached policy that has not expired keeps applying, as
// section 5.1 requires.
type MTASTS struct {
	DNS  TXTResolver  // nil: net.DefaultResolver
	HTTP *http.Client // must verify certificates and not follow redirects; nil: a default one
	Log  *slog.Logger
	Now  func() time.Time

	mu    sync.Mutex
	cache map[string]*STSPolicy
}

// STSPolicy is a parsed MTA-STS policy.
type STSPolicy struct {
	ID      string
	Mode    string
	MX      []string
	MaxAge  time.Duration
	Expires time.Time
}

// Matches reports whether an MX host name matches one of the policy's mx
// patterns; "*.example.com" matches exactly one extra label (section 4.1).
func (p *STSPolicy) Matches(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, pat := range p.MX {
		pat = strings.ToLower(strings.TrimSuffix(pat, "."))
		if rest, ok := strings.CutPrefix(pat, "*."); ok {
			if label, tail, found := strings.Cut(host, "."); found && label != "" && tail == rest {
				return true
			}
			continue
		}
		if host == pat {
			return true
		}
	}
	return false
}

func (m *MTASTS) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *MTASTS) log() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

// Policy returns the policy in force for domain, or nil when there is none.
func (m *MTASTS) Policy(ctx context.Context, domain string) *STSPolicy {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	now := m.now()
	m.mu.Lock()
	cached := m.cache[domain]
	if cached != nil && !now.Before(cached.Expires) {
		delete(m.cache, domain)
		cached = nil
	}
	m.mu.Unlock()

	id, err := m.lookupID(ctx, domain)
	if err != nil || id == "" {
		return cached
	}
	if cached != nil && cached.ID == id {
		return cached
	}
	p, err := m.fetch(ctx, domain)
	if err != nil {
		m.log().Warn("mta-sts policy fetch failed", "domain", domain, "err", err, "cached", cached != nil)
		return cached
	}
	p.ID, p.Expires = id, now.Add(p.MaxAge)
	m.mu.Lock()
	if m.cache == nil {
		m.cache = map[string]*STSPolicy{}
	}
	m.cache[domain] = p
	m.mu.Unlock()
	return p
}

var stsIDRe = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// lookupID reads the _mta-sts TXT record; "" means no (valid) record.
func (m *MTASTS) lookupID(ctx context.Context, domain string) (string, error) {
	r := m.DNS
	if r == nil {
		r = net.DefaultResolver
	}
	txts, err := r.LookupTXT(ctx, "_mta-sts."+domain)
	if err != nil {
		var de *net.DNSError
		if errors.As(err, &de) && de.IsNotFound {
			return "", nil
		}
		return "", err
	}
	id, n := "", 0
	for _, t := range txts {
		fields := strings.Split(t, ";")
		if strings.TrimSpace(fields[0]) != "v=STSv1" {
			continue
		}
		n++
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "id="); ok {
				id = v
			}
		}
	}
	if n != 1 || !stsIDRe.MatchString(id) {
		return "", nil // section 3.1: none or several records mean no policy
	}
	return id, nil
}

func (m *MTASTS) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// fetch downloads the policy over HTTPS with certificate validation.
func (m *MTASTS) fetch(ctx context.Context, domain string) (*STSPolicy, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://mta-sts."+domain+"/.well-known/mta-sts.txt", nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("policy returned HTTP %d", resp.StatusCode)
	}
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err != nil || mt != "text/plain" {
		return nil, fmt.Errorf("policy content type %q is not text/plain", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, stsMaxPolicy+1))
	if err != nil {
		return nil, err
	}
	if len(body) > stsMaxPolicy {
		return nil, errors.New("policy too large")
	}
	return parseSTSPolicy(string(body))
}

// parseSTSPolicy parses the key: value policy body (section 3.2).
func parseSTSPolicy(body string) (*STSPolicy, error) {
	p := &STSPolicy{}
	version, maxAge := "", -1
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "version":
			version = v
		case "mode":
			p.Mode = v
		case "mx":
			p.MX = append(p.MX, v)
		case "max_age":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || len(v) > 10 {
				return nil, fmt.Errorf("invalid max_age %q", v)
			}
			maxAge = min(n, stsMaxAge)
		}
	}
	switch {
	case version != "STSv1":
		return nil, fmt.Errorf("unsupported policy version %q", version)
	case p.Mode != stsEnforce && p.Mode != stsTesting && p.Mode != stsNone:
		return nil, fmt.Errorf("invalid mode %q", p.Mode)
	case maxAge < 0:
		return nil, errors.New("policy has no max_age")
	case p.Mode != stsNone && len(p.MX) == 0:
		return nil, errors.New("policy has no mx")
	}
	p.MaxAge = time.Duration(maxAge) * time.Second
	return p, nil
}
