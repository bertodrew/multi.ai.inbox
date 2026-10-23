package mailauth

import (
	"bytes"
	"context"

	"github.com/emersion/go-msgauth/dkim"
)

// maxDKIMSignatures bounds the work a message can cause at the edge.
const maxDKIMSignatures = 5

// VerifyDKIM checks the DKIM signatures of raw and returns the d= domains
// of the ones that verified.
func VerifyDKIM(ctx context.Context, r Resolver, raw []byte) (passed []string, signed bool) {
	results, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{
		MaxVerifications: maxDKIMSignatures,
		LookupTXT:        func(name string) ([]string, error) { return r.LookupTXT(ctx, name) },
	})
	if err != nil || len(results) == 0 {
		return nil, false
	}
	for _, v := range results {
		if v.Err == nil {
			passed = append(passed, v.Domain)
		}
	}
	return passed, true
}
