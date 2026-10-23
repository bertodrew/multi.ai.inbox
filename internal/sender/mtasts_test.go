package sender

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

func testCert(t *testing.T, names ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

type txtDNS struct {
	mu  sync.Mutex
	txt map[string][]string
	mx  map[string][]*net.MX
}

func (f *txtDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *txtDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *txtDNS) set(name string, v ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v == nil {
		delete(f.txt, name)
	} else {
		f.txt[name] = v
	}
}

// policyServer serves mta-sts.<domain> over HTTPS; every name resolves to it.
type policyServer struct {
	mu     sync.Mutex
	body   string
	ctype  string
	status int
	hits   atomic.Int32
	client *http.Client
}

func newPolicyServer(t *testing.T, body string) *policyServer {
	t.Helper()
	p := &policyServer{body: body, ctype: "text/plain; charset=utf-8", status: 200}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		p.mu.Lock()
		defer p.mu.Unlock()
		if r.URL.Path != "/.well-known/mta-sts.txt" || !strings.HasPrefix(r.Host, "mta-sts.") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", p.ctype)
		w.WriteHeader(p.status)
		_, _ = io.WriteString(w, p.body)
	}))
	cert, pool := testCert(t, "mta-sts.example.com", "mta-sts.testing.example")
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	p.client = &http.Client{Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}}}
	return p
}

func (p *policyServer) set(body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.body = body
}

const enforcePolicy = "version: STSv1\r\nmode: enforce\r\nmx: *.example.com\r\nmx: backup.example.net\r\nmax_age: 86400\r\n"

func TestParseSTSPolicy(t *testing.T) {
	p, err := parseSTSPolicy(enforcePolicy)
	if err != nil || p.Mode != "enforce" || len(p.MX) != 2 || p.MaxAge != 24*time.Hour {
		t.Fatalf("parse: %+v %v", p, err)
	}
	if p, err := parseSTSPolicy("version: STSv1\nmode: none\nmax_age: 99999999999\n"); err == nil {
		t.Fatalf("over long max_age must fail: %+v", p)
	}
	if p, err := parseSTSPolicy("version: STSv1\nmode: testing\nmx: a.example\nmax_age: 40000000\nfuture: x\n"); err != nil || p.MaxAge != stsMaxAge*time.Second {
		t.Fatalf("max_age is capped: %+v %v", p, err)
	}
	for _, bad := range []string{
		"mode: enforce\nmx: a.example\nmax_age: 60\n",
		"version: STSv2\nmode: enforce\nmx: a.example\nmax_age: 60\n",
		"version: STSv1\nmode: strict\nmx: a.example\nmax_age: 60\n",
		"version: STSv1\nmode: enforce\nmax_age: 60\n",
		"version: STSv1\nmode: enforce\nmx: a.example\n",
		"version: STSv1\nmode: enforce\nmx: a.example\nmax_age: -1\n",
	} {
		if _, err := parseSTSPolicy(bad); err == nil {
			t.Errorf("parseSTSPolicy(%q) must fail", bad)
		}
	}
}

func TestSTSMatches(t *testing.T) {
	p := &STSPolicy{MX: []string{"*.example.com", "mx.example.net."}}
	for host, want := range map[string]bool{
		"mx1.example.com":   true,
		"MX1.Example.COM.":  true,
		"example.com":       false,
		"a.b.example.com":   false,
		"mx.example.net":    true,
		"mx2.example.net":   false,
		"mx1.example.com.x": false,
	} {
		if got := p.Matches(host); got != want {
			t.Errorf("Matches(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestMTASTSPolicyCache(t *testing.T) {
	ps := newPolicyServer(t, enforcePolicy)
	dns := &txtDNS{txt: map[string][]string{"_mta-sts.example.com": {"v=STSv1; id=20261001;"}}}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	m := &MTASTS{DNS: dns, HTTP: ps.client, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }}
	ctx := context.Background()

	p := m.Policy(ctx, "Example.com.")
	if p == nil || p.Mode != "enforce" || p.ID != "20261001" || ps.hits.Load() != 1 {
		t.Fatalf("first fetch: %+v hits=%d", p, ps.hits.Load())
	}
	if p := m.Policy(ctx, "example.com"); p == nil || ps.hits.Load() != 1 {
		t.Fatalf("same id must use the cache: hits=%d", ps.hits.Load())
	}
	// A new id refetches.
	ps.set(strings.Replace(enforcePolicy, "enforce", "testing", 1))
	dns.set("_mta-sts.example.com", "v=STSv1; id=20261002")
	if p := m.Policy(ctx, "example.com"); p == nil || p.Mode != "testing" || ps.hits.Load() != 2 {
		t.Fatalf("new id: %+v hits=%d", p, ps.hits.Load())
	}
	// The TXT record disappears: the cached policy still applies until max_age.
	dns.set("_mta-sts.example.com")
	if p := m.Policy(ctx, "example.com"); p == nil || p.Mode != "testing" {
		t.Fatalf("cached policy without TXT: %+v", p)
	}
	// A failed refetch keeps the cached policy too.
	ps.mu.Lock()
	ps.status = 500
	ps.mu.Unlock()
	dns.set("_mta-sts.example.com", "v=STSv1; id=20261003")
	if p := m.Policy(ctx, "example.com"); p == nil || p.ID != "20261002" {
		t.Fatalf("failed refetch: %+v", p)
	}
	// After max_age the cache is gone; with the server down there is no policy.
	now = now.Add(25 * time.Hour)
	if p := m.Policy(ctx, "example.com"); p != nil {
		t.Fatalf("expired: %+v", p)
	}
	// A wrong content type is refused.
	ps.mu.Lock()
	ps.status, ps.ctype = 200, "text/html"
	ps.mu.Unlock()
	if p := m.Policy(ctx, "example.com"); p != nil {
		t.Fatalf("text/html policy: %+v", p)
	}
	// Two STS records, or none, mean no policy.
	dns.set("_mta-sts.example.com", "v=STSv1; id=a", "v=STSv1; id=b")
	if p := m.Policy(ctx, "other.example.com"); p != nil {
		t.Fatalf("no record: %+v", p)
	}
	if id, _ := m.lookupID(ctx, "example.com"); id != "" {
		t.Fatalf("two records: id %q", id)
	}
}

// fakeMX is an SMTP server that records what it receives and whether the
// session was encrypted.
type fakeMX struct {
	mu   sync.Mutex
	got  []string
	tlsd []bool
}

func (f *fakeMX) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &fakeSession{f: f, c: c}, nil
}

type fakeSession struct {
	f *fakeMX
	c *smtp.Conn
}

func (s *fakeSession) Mail(string, *smtp.MailOptions) error { return nil }
func (s *fakeSession) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (s *fakeSession) Reset()                               {}
func (s *fakeSession) Logout() error                        { return nil }
func (s *fakeSession) Data(r io.Reader) error {
	b, _ := io.ReadAll(r)
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.f.got = append(s.f.got, string(b))
	_, isTLS := s.c.TLSConnectionState()
	s.f.tlsd = append(s.f.tlsd, isTLS)
	return nil
}

func (f *fakeMX) deliveries() (int, []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got), append([]bool(nil), f.tlsd...)
}

func startMX(t *testing.T, cfg *tls.Config) (*fakeMX, string) {
	t.Helper()
	f := &fakeMX{}
	srv := smtp.NewServer(f)
	srv.Domain = "mx.test"
	srv.TLSConfig = cfg
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { srv.Close() })
	return f, l.Addr().String()
}

func TestDirectMTASTS(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	good, roots := testCert(t, "mx1.example.com", "mx1.testing.example")
	wrongName, _ := testCert(t, "other.test")
	ps := newPolicyServer(t, enforcePolicy)
	raw := []byte("From: a@agents.test\r\nTo: b@example.com\r\nSubject: hi\r\n\r\nhello\r\n")

	run := func(t *testing.T, domain, policy string, mxs []*net.MX, hosts map[string]string) error {
		t.Helper()
		ps.set(policy)
		dns := &txtDNS{txt: map[string][]string{"_mta-sts." + domain: {"v=STSv1; id=1;"}},
			mx: map[string][]*net.MX{domain: mxs}}
		s := &Sender{HeloName: "mx.agents.test", Log: log, DNS: dns,
			TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
			MTASTS:    &MTASTS{DNS: dns, HTTP: ps.client, Log: log},
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, _ := net.SplitHostPort(addr)
				target, ok := hosts[host]
				if !ok {
					return nil, errors.New("no route to " + host)
				}
				return (&net.Dialer{}).DialContext(ctx, network, target)
			}}
		return s.direct(context.Background(), raw, "a@agents.test", []string{"b@" + domain})
	}
	mxList := []*net.MX{{Host: "evil.attacker.test.", Pref: 5}, {Host: "mx1.example.com.", Pref: 10}}

	t.Run("enforce skips unlisted MX and verifies TLS", func(t *testing.T) {
		evil, evilAddr := startMX(t, &tls.Config{Certificates: []tls.Certificate{good}})
		mx, addr := startMX(t, &tls.Config{Certificates: []tls.Certificate{good}})
		if err := run(t, "example.com", enforcePolicy, mxList, map[string]string{"evil.attacker.test": evilAddr, "mx1.example.com": addr}); err != nil {
			t.Fatal(err)
		}
		if n, _ := evil.deliveries(); n != 0 {
			t.Fatal("delivered to an MX outside the policy")
		}
		if n, tlsd := mx.deliveries(); n != 1 || !tlsd[0] {
			t.Fatalf("deliveries %d tls %v", n, tlsd)
		}
	})
	t.Run("enforce without STARTTLS is a temporary failure", func(t *testing.T) {
		mx, addr := startMX(t, nil)
		err := run(t, "example.com", enforcePolicy, mxList[1:], map[string]string{"mx1.example.com": addr})
		if err == nil || errors.Is(err, core.ErrPermanent) || !strings.Contains(err.Error(), "STARTTLS") {
			t.Fatalf("want temporary STARTTLS error, got %v", err)
		}
		if n, _ := mx.deliveries(); n != 0 {
			t.Fatal("delivered in plain text under enforce")
		}
	})
	t.Run("enforce with an invalid certificate fails", func(t *testing.T) {
		mx, addr := startMX(t, &tls.Config{Certificates: []tls.Certificate{wrongName}})
		err := run(t, "example.com", enforcePolicy, mxList[1:], map[string]string{"mx1.example.com": addr})
		if err == nil || errors.Is(err, core.ErrPermanent) {
			t.Fatalf("want temporary certificate error, got %v", err)
		}
		if n, _ := mx.deliveries(); n != 0 {
			t.Fatal("delivered despite an invalid certificate")
		}
	})
	t.Run("enforce with no matching MX fails", func(t *testing.T) {
		_, evilAddr := startMX(t, nil)
		err := run(t, "example.com", enforcePolicy, mxList[:1], map[string]string{"evil.attacker.test": evilAddr})
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("want policy mismatch, got %v", err)
		}
	})
	t.Run("testing mode only logs", func(t *testing.T) {
		testPolicy := strings.Replace(strings.Replace(enforcePolicy, "enforce", "testing", 1), "*.example.com", "mx1.testing.example", 1)
		evil, evilAddr := startMX(t, &tls.Config{Certificates: []tls.Certificate{wrongName}})
		err := run(t, "testing.example", testPolicy, []*net.MX{{Host: "evil.attacker.test.", Pref: 5}}, map[string]string{"evil.attacker.test": evilAddr})
		if err != nil {
			t.Fatal(err)
		}
		if n, tlsd := evil.deliveries(); n != 1 || !tlsd[0] {
			t.Fatalf("testing mode must still deliver over opportunistic TLS: %d %v", n, tlsd)
		}
		plain, plainAddr := startMX(t, nil)
		if err := run(t, "testing.example", testPolicy, []*net.MX{{Host: "mx1.testing.example.", Pref: 5}}, map[string]string{"mx1.testing.example": plainAddr}); err != nil {
			t.Fatal(err)
		}
		if n, _ := plain.deliveries(); n != 1 {
			t.Fatal("testing mode must deliver without STARTTLS")
		}
	})
	t.Run("no policy is opportunistic", func(t *testing.T) {
		mx, addr := startMX(t, nil)
		dns := &txtDNS{txt: map[string][]string{}, mx: map[string][]*net.MX{"plain.example": {{Host: "mx.plain.example."}}}}
		s := &Sender{HeloName: "mx.agents.test", Log: log, DNS: dns, MTASTS: &MTASTS{DNS: dns, HTTP: ps.client, Log: log},
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}}
		if err := s.direct(context.Background(), raw, "a@agents.test", []string{"b@plain.example"}); err != nil {
			t.Fatal(err)
		}
		if n, _ := mx.deliveries(); n != 1 {
			t.Fatal("not delivered")
		}
	})
}
