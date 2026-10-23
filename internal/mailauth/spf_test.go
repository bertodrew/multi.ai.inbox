package mailauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeDNS answers from maps; names are lower case without a trailing dot.
// Missing names return a not found error, names in fail a temporary one.
type fakeDNS struct {
	txt     map[string][]string
	mx      map[string][]*net.MX
	ip      map[string][]string
	fail    map[string]bool
	queries int
}

func (f *fakeDNS) lookup(name string) (string, error) {
	f.queries++
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if f.fail[name] {
		return "", errors.New("SERVFAIL")
	}
	return name, nil
}

func nx(name string) error { return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true} }

func (f *fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	n, err := f.lookup(name)
	if err != nil {
		return nil, err
	}
	if v, ok := f.txt[n]; ok {
		return v, nil
	}
	return nil, nx(n)
}

func (f *fakeDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	n, err := f.lookup(name)
	if err != nil {
		return nil, err
	}
	if v, ok := f.mx[n]; ok {
		return v, nil
	}
	return nil, nx(n)
}

func (f *fakeDNS) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	n, err := f.lookup(host)
	if err != nil {
		return nil, err
	}
	var out []net.IP
	for _, s := range f.ip[n] {
		ip := net.ParseIP(s)
		if (network == "ip4") == (ip.To4() != nil) {
			out = append(out, ip)
		}
	}
	if len(out) == 0 {
		return nil, nx(n)
	}
	return out, nil
}

func TestCheckSPF(t *testing.T) {
	dns := &fakeDNS{
		txt: map[string][]string{
			"example.com":       {"v=spf1 ip4:192.0.2.0/24 ip6:2001:db8::/32 include:_spf.example.net a:web.example.com mx -all"},
			"_spf.example.net":  {"some other txt", "v=spf1 ip4:198.51.100.7 ~all"},
			"soft.example":      {"v=spf1 ~all"},
			"neutral.example":   {"v=spf1 ?all"},
			"default.example":   {"v=spf1 ip4:10.9.9.9"},
			"redir.example":     {"v=spf1 redirect=example.com"},
			"redirnone.example": {"v=spf1 redirect=nothing.example"},
			"two.example":       {"v=spf1 -all", "v=spf1 +all"},
			"bad.example":       {"v=spf1 ip4:300.1.1.1 -all"},
			"unknown.example":   {"v=spf1 foo:bar -all"},
			"modifier.example":  {"v=spf1 future=thing +all"},
			"exists.example":    {"v=spf1 exists:%{ir}.%{l1r+-}._spf.%{d} -all"},
			"cidr.example":      {"v=spf1 a:host.cidr.example/24 -all"},
			"temp.example":      {"v=spf1 include:broken.example -all"},
			"incnone.example":   {"v=spf1 include:nothing.example -all"},
			"empty.example":     {"v=spf1"},
			"helo.example":      {"v=spf1 ip4:203.0.113.9 -all"},
		},
		mx: map[string][]*net.MX{
			"example.com": {{Host: "mx1.example.com.", Pref: 10}},
		},
		ip: map[string][]string{
			"web.example.com":   {"203.0.113.5"},
			"mx1.example.com":   {"203.0.113.25", "2001:db8:ffff::25"},
			"host.cidr.example": {"203.0.113.1"},
			"7.100.51.198.john.smith._spf.exists.example": {"127.0.0.2"},
		},
		fail: map[string]bool{"broken.example": true},
	}
	ctx := context.Background()
	for _, tc := range []struct {
		ip, from, helo string
		want           SPFResult
	}{
		{"192.0.2.44", "a@example.com", "", SPFPass},
		{"2001:db8:1::1", "a@example.com", "", SPFPass},
		{"198.51.100.7", "a@example.com", "", SPFPass}, // via include
		{"203.0.113.5", "a@example.com", "", SPFPass},  // a:web.example.com
		{"203.0.113.25", "a@EXAMPLE.com", "", SPFPass}, // mx
		{"2001:db8:ffff::25", "a@example.com", "", SPFPass},
		{"203.0.113.99", "a@example.com", "", SPFFail},
		{"203.0.113.99", "a@soft.example", "", SPFSoftFail},
		{"203.0.113.99", "a@neutral.example", "", SPFNeutral},
		{"203.0.113.99", "a@default.example", "", SPFNeutral},
		{"192.0.2.1", "a@redir.example", "", SPFPass},
		{"203.0.113.99", "a@redir.example", "", SPFFail},
		{"203.0.113.99", "a@redirnone.example", "", SPFPermError},
		{"203.0.113.99", "a@nothing.example", "", SPFNone},
		{"203.0.113.99", "a@two.example", "", SPFPermError},
		{"203.0.113.99", "a@bad.example", "", SPFPermError},
		{"203.0.113.99", "a@unknown.example", "", SPFPermError},
		{"203.0.113.99", "a@modifier.example", "", SPFPass},
		{"198.51.100.7", "john.smith@exists.example", "", SPFPass},
		{"198.51.100.8", "john.smith@exists.example", "", SPFFail},
		{"203.0.113.200", "a@cidr.example", "", SPFPass},
		{"203.0.114.1", "a@cidr.example", "", SPFFail},
		{"203.0.113.99", "a@temp.example", "", SPFTempError},
		{"203.0.113.99", "a@broken.example", "", SPFTempError},
		{"203.0.113.99", "a@incnone.example", "", SPFPermError},
		{"203.0.113.99", "a@empty.example", "", SPFNeutral},
		{"203.0.113.9", "", "helo.example", SPFPass}, // null reverse path: postmaster@helo
		{"203.0.113.8", "", "helo.example", SPFFail},
		{"203.0.113.8", "a@localhost", "", SPFNone}, // not a valid FQDN
	} {
		if got := CheckSPF(ctx, dns, net.ParseIP(tc.ip), tc.from, tc.helo); got != tc.want {
			t.Errorf("CheckSPF(%s, %q, %q) = %s, want %s", tc.ip, tc.from, tc.helo, got, tc.want)
		}
	}
}

func TestSPFLookupLimit(t *testing.T) {
	dns := &fakeDNS{txt: map[string][]string{}, ip: map[string][]string{}}
	// A chain of includes: 10 lookups are allowed, the 11th is a permerror.
	chain := func(prefix string, n int) {
		for i := range n {
			next := fmt.Sprintf("include:%s%d.example", prefix, i+1)
			if i == n-1 {
				next = "ip4:192.0.2.1"
			}
			dns.txt[fmt.Sprintf("%s%d.example", prefix, i)] = []string{"v=spf1 " + next + " -all"}
		}
	}
	chain("ok", 11)   // ok0 includes ok1 ... ok10: 10 include lookups
	chain("over", 12) // 11 include lookups
	ip := net.ParseIP("192.0.2.1")
	if got := CheckSPF(context.Background(), dns, ip, "a@ok0.example", ""); got != SPFPass {
		t.Fatalf("10 lookups: %s, want pass", got)
	}
	if got := CheckSPF(context.Background(), dns, ip, "a@over0.example", ""); got != SPFPermError {
		t.Fatalf("11 lookups: %s, want permerror", got)
	}

	// Mechanisms in one record count too, and so does redirect.
	dns.txt["many.example"] = []string{"v=spf1 a:x1.example a:x2.example a:x3.example a:x4.example a:x5.example " +
		"ptr:x6.example ptr mx:x8.example mx:x9.example a:x10.example redirect=x11.example"}
	for i := 1; i <= 11; i++ {
		dns.ip[fmt.Sprintf("x%d.example", i)] = []string{"198.51.100.1"} // present, so no void lookups
	}
	dns.mx = map[string][]*net.MX{"x8.example": {{Host: "x1.example."}}, "x9.example": {{Host: "x1.example."}}}
	dns.txt["x11.example"] = []string{"v=spf1 +all"}
	if got := CheckSPF(context.Background(), dns, ip, "a@many.example", ""); got != SPFPermError {
		t.Fatalf("10 mechanisms plus redirect: %s, want permerror", got)
	}
}

func TestSPFVoidLookups(t *testing.T) {
	dns := &fakeDNS{txt: map[string][]string{
		"void.example": {"v=spf1 a:n1.example a:n2.example a:n3.example +all"},
		"two.example":  {"v=spf1 a:n1.example mx:n2.example +all"},
	}}
	ip := net.ParseIP("192.0.2.1")
	if got := CheckSPF(context.Background(), dns, ip, "a@void.example", ""); got != SPFPermError {
		t.Fatalf("3 void lookups: %s, want permerror", got)
	}
	if got := CheckSPF(context.Background(), dns, ip, "a@two.example", ""); got != SPFPass {
		t.Fatalf("2 void lookups: %s, want pass", got)
	}
}

func TestSPFTooManyMX(t *testing.T) {
	var mxs []*net.MX
	for i := range 11 {
		mxs = append(mxs, &net.MX{Host: fmt.Sprintf("mx%d.example.", i)})
	}
	dns := &fakeDNS{txt: map[string][]string{"mx.example": {"v=spf1 mx -all"}}, mx: map[string][]*net.MX{"mx.example": mxs}}
	if got := CheckSPF(context.Background(), dns, net.ParseIP("192.0.2.1"), "a@mx.example", ""); got != SPFPermError {
		t.Fatalf("11 MX names: %s, want permerror", got)
	}
}

func TestSPFMacros(t *testing.T) {
	e := &spfEval{ip: net.ParseIP("192.0.2.3"), sender: "strong-bad@email.example.com", local: "strong-bad",
		senderDomain: "email.example.com", helo: "mx.example.org"}
	for spec, want := range map[string]string{
		"%{s}":                  "strong-bad@email.example.com",
		"%{o}":                  "email.example.com",
		"%{d}":                  "email.example.com",
		"%{d4}":                 "email.example.com",
		"%{d3}":                 "email.example.com",
		"%{d2}":                 "example.com",
		"%{d1}":                 "com",
		"%{dr}":                 "com.example.email",
		"%{d2r}":                "example.email",
		"%{l}":                  "strong-bad",
		"%{l-}":                 "strong.bad",
		"%{lr}":                 "strong-bad",
		"%{lr-}":                "bad.strong",
		"%{l1r-}":               "strong",
		"%{ir}.%{v}._spf.%{d2}": "3.2.0.192.in-addr._spf.example.com",
		"%{h}":                  "mx.example.org",
		"a%%b%_c%-d":            "a%b c%20d",
	} {
		got, err := e.expand(spec, "email.example.com")
		if err != nil || got != want {
			t.Errorf("expand(%q) = %q, %v; want %q", spec, got, err, want)
		}
	}
	for _, bad := range []string{"%{d0}", "%{c}", "%{x}", "%", "%a", "%{d"} {
		if _, err := e.expand(bad, "example.com"); err == nil {
			t.Errorf("expand(%q) must fail", bad)
		}
	}
	e.ip = net.ParseIP("2001:db8::cb01")
	got, _ := e.expand("%{ir}.%{v}", "x.example")
	if want := "1.0.b.c.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6"; got != want {
		t.Errorf("ipv6 %%{ir}: %q, want %q", got, want)
	}
}
