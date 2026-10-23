package mailauth

import (
	"context"
	"testing"
)

func TestOrgDomain(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":            "example.com",
		"mail.example.com":       "example.com",
		"a.b.mail.example.com.":  "example.com",
		"Mail.Example.CO.UK":     "example.co.uk",
		"example.co.uk":          "example.co.uk",
		"shop.example.com.au":    "example.com.au",
		"com":                    "com",
		"news.example.de":        "example.de",
		"x.y.example.com.br":     "example.com.br",
		"deep.sub.example.co.jp": "example.co.jp",
	} {
		if got := OrgDomain(in); got != want {
			t.Errorf("OrgDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAligned(t *testing.T) {
	for _, tc := range []struct {
		d, from string
		strict  bool
		want    bool
	}{
		{"example.com", "example.com", true, true},
		{"mail.example.com", "example.com", true, false},
		{"mail.example.com", "example.com", false, true},
		{"example.com", "news.example.com", false, true},
		{"other.com", "example.com", false, false},
		{"a.example.co.uk", "b.example.co.uk", false, true},
		{"example.co.uk", "other.co.uk", false, false},
		{"", "example.com", false, false},
	} {
		if got := Aligned(tc.d, tc.from, tc.strict); got != tc.want {
			t.Errorf("Aligned(%q, %q, strict=%v) = %v, want %v", tc.d, tc.from, tc.strict, got, tc.want)
		}
	}
}

func TestCheckDMARC(t *testing.T) {
	dns := &fakeDNS{
		txt: map[string][]string{
			"_dmarc.example.com":  {"v=DMARC1; p=reject; sp=quarantine; rua=mailto:d@example.com"},
			"_dmarc.strict.test":  {"v=DMARC1; p=reject; adkim=s; aspf=s"},
			"_dmarc.none.test":    {"v=DMARC1; p=none"},
			"_dmarc.pct.test":     {"v=DMARC1; p=reject; pct=20"},
			"_dmarc.invalid.test": {"v=DMARC1; p=sometimes"},
			"_dmarc.other.test":   {"v=spf1 -all"},
		},
		fail: map[string]bool{"_dmarc.broken.test": true},
	}
	ctx := context.Background()
	never := func() int { return 99 }
	for name, tc := range map[string]struct {
		in   DMARCInput
		want DMARCVerdict
	}{
		"dkim aligned":           {DMARCInput{FromDomain: "example.com", DKIMPass: []string{"example.com"}}, DMARCVerdict{DMARCPass, "reject"}},
		"dkim relaxed subdomain": {DMARCInput{FromDomain: "example.com", DKIMPass: []string{"mail.example.com"}}, DMARCVerdict{DMARCPass, "reject"}},
		"spf aligned":            {DMARCInput{FromDomain: "example.com", SPF: SPFPass, SPFDomain: "bounce.example.com"}, DMARCVerdict{DMARCPass, "reject"}},
		"spf pass not aligned":   {DMARCInput{FromDomain: "example.com", SPF: SPFPass, SPFDomain: "esp.test"}, DMARCVerdict{DMARCFail, "reject"}},
		"spf softfail aligned":   {DMARCInput{FromDomain: "example.com", SPF: SPFSoftFail, SPFDomain: "example.com"}, DMARCVerdict{DMARCFail, "reject"}},
		"dkim other domain":      {DMARCInput{FromDomain: "example.com", DKIMPass: []string{"esp.test"}}, DMARCVerdict{DMARCFail, "reject"}},
		"subdomain uses sp":      {DMARCInput{FromDomain: "news.example.com"}, DMARCVerdict{DMARCFail, "quarantine"}},
		"subdomain aligned":      {DMARCInput{FromDomain: "news.example.com", DKIMPass: []string{"example.com"}}, DMARCVerdict{DMARCPass, "quarantine"}},
		"strict dkim subdomain":  {DMARCInput{FromDomain: "strict.test", DKIMPass: []string{"m.strict.test"}}, DMARCVerdict{DMARCFail, "reject"}},
		"strict spf exact":       {DMARCInput{FromDomain: "strict.test", SPF: SPFPass, SPFDomain: "strict.test"}, DMARCVerdict{DMARCPass, "reject"}},
		"strict spf subdomain":   {DMARCInput{FromDomain: "strict.test", SPF: SPFPass, SPFDomain: "b.strict.test"}, DMARCVerdict{DMARCFail, "reject"}},
		"p none":                 {DMARCInput{FromDomain: "none.test"}, DMARCVerdict{DMARCFail, "none"}},
		"pct sampled out":        {DMARCInput{FromDomain: "pct.test"}, DMARCVerdict{DMARCFail, "quarantine"}},
		"no record":              {DMARCInput{FromDomain: "unknown.test", DKIMPass: []string{"unknown.test"}}, DMARCVerdict{Result: DMARCNone}},
		"non dmarc txt":          {DMARCInput{FromDomain: "other.test"}, DMARCVerdict{Result: DMARCNone}},
		"invalid record":         {DMARCInput{FromDomain: "invalid.test"}, DMARCVerdict{Result: DMARCPermError}},
		"dns failure":            {DMARCInput{FromDomain: "broken.test"}, DMARCVerdict{Result: DMARCTempError}},
		"no from domain":         {DMARCInput{}, DMARCVerdict{Result: DMARCNone}},
	} {
		if got := CheckDMARC(ctx, dns, tc.in, never); got != tc.want {
			t.Errorf("%s: %+v, want %+v", name, got, tc.want)
		}
	}
	always := func() int { return 0 }
	if got := CheckDMARC(ctx, dns, DMARCInput{FromDomain: "pct.test"}, always); got.Policy != "reject" {
		t.Errorf("pct sampled in: %+v, want reject", got)
	}
}
