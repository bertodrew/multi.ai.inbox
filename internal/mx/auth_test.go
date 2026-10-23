package mx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-smtp"
)

type authDNS struct {
	txt map[string][]string
	ip  map[string][]net.IP
}

func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *authDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f.txt[strings.TrimSuffix(name, ".")]; ok {
		return v, nil
	}
	return nil, notFound(name)
}

func (f *authDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return nil, notFound(name)
}

func (f *authDNS) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	if v, ok := f.ip[host]; ok {
		return v, nil
	}
	return nil, notFound(host)
}

func TestAuthenticate(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	dns := &authDNS{txt: map[string][]string{
		"example.com":                {"v=spf1 ip4:192.0.2.0/24 -all"},
		"_dmarc.example.com":         {"v=DMARC1; p=reject"},
		"quarantine.test":            {"v=spf1 -all"},
		"_dmarc.quarantine.test":     {"v=DMARC1; p=quarantine"},
		"sel._domainkey.example.com": {"v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pub)},
		"_dmarc.monitor.test":        {"v=DMARC1; p=none"},
		"monitor.test":               {"v=spf1 -all"},
	}}
	plain := func(from string) []byte {
		return []byte("From: <" + from + ">\r\nTo: <b@agents.test>\r\nSubject: hi\r\nMessage-ID: <1@x>\r\n\r\nhello\r\n")
	}
	var signed bytes.Buffer
	if err := dkim.Sign(&signed, bytes.NewReader(plain("a@example.com")), &dkim.SignOptions{Domain: "example.com", Selector: "sel", Signer: key}); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	run := func(enforce bool, remote, from string, raw []byte) (map[string]any, []string, error) {
		s := &session{b: &Backend{Log: log, DNS: dns, MailAuth: &MailAuth{Enforce: enforce}}, remote: remote, from: from, helo: "mx.sender.test"}
		return s.authenticate(context.Background(), raw)
	}

	// SPF fails (foreign IP) but the aligned DKIM signature passes DMARC.
	auth, labels, err := run(true, "203.0.113.9:4000", "bounce@example.com", signed.Bytes())
	if err != nil || len(labels) != 0 || auth["dkim"] != "pass" || auth["spf"] != "fail" || auth["dmarc"] != "pass" || auth["dmarc_policy"] != "reject" {
		t.Fatalf("dkim aligned: %v %v %v", auth, labels, err)
	}
	// SPF passes from the listed network, no signature.
	auth, labels, err = run(true, "192.0.2.7:4000", "bounce@example.com", plain("a@example.com"))
	if err != nil || len(labels) != 0 || auth["dkim"] != "none" || auth["spf"] != "pass" || auth["dmarc"] != "pass" {
		t.Fatalf("spf aligned: %v %v %v", auth, labels, err)
	}
	// Spoofed From under p=reject: labelled spam, or refused when enforcing.
	auth, labels, err = run(false, "203.0.113.9:4000", "x@spoof.test", plain("ceo@example.com"))
	if err != nil || auth["spf"] != "none" || auth["dmarc"] != "fail" || !reflect.DeepEqual(labels, []string{"dmarc-fail", "spam"}) {
		t.Fatalf("reject not enforced: %v %v %v", auth, labels, err)
	}
	var se *smtp.SMTPError
	if _, _, err = run(true, "203.0.113.9:4000", "x@spoof.test", plain("ceo@example.com")); !errors.As(err, &se) || se.Code != 550 || se.EnhancedCode != (smtp.EnhancedCode{5, 7, 1}) {
		t.Fatalf("reject enforced: %v", err)
	}
	// p=quarantine is never refused, only labelled.
	auth, labels, err = run(true, "203.0.113.9:4000", "a@quarantine.test", plain("a@quarantine.test"))
	if err != nil || auth["dmarc"] != "fail" || !reflect.DeepEqual(labels, []string{"dmarc-fail", "spam"}) {
		t.Fatalf("quarantine: %v %v %v", auth, labels, err)
	}
	// p=none only marks the failure.
	_, labels, err = run(true, "203.0.113.9:4000", "a@monitor.test", plain("a@monitor.test"))
	if err != nil || !reflect.DeepEqual(labels, []string{"dmarc-fail"}) {
		t.Fatalf("p=none: %v %v", labels, err)
	}
	// Without MailAuth only DKIM is recorded.
	s := &session{b: &Backend{Log: log, DNS: dns}, remote: "203.0.113.9:4000", from: "x@spoof.test"}
	auth, labels, err = s.authenticate(context.Background(), plain("ceo@example.com"))
	if err != nil || len(labels) != 0 || len(auth) != 1 || auth["dkim"] != "none" {
		t.Fatalf("disabled: %v %v %v", auth, labels, err)
	}
}
