package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/mx"
)

// TestSendBurstLimit: the free plan allows 10 sends per 5 minutes; the next
// one gets 429 rate_limited with a Retry-After until the window moves on.
func TestSendBurstLimit(t *testing.T) {
	p := newPlatform(t)
	b := p.b // free plan
	if code, body := b.do("POST", "/v0/inboxes", map[string]any{"username": "burst"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	sendPath := "/v0/inboxes/burst@" + shared + "/messages/send"
	send := func(i int) (*http.Response, map[string]any) {
		body, _ := json.Marshal(map[string]any{"to": []string{fmt.Sprintf("r%d@ext.test", i)}, "subject": "Hi", "text": "hello"})
		req, _ := http.NewRequest("POST", b.base+sendPath, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+b.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp, out
	}
	for i := range 10 {
		if resp, body := send(i); resp.StatusCode != 200 {
			t.Fatalf("send %d: %d %v", i, resp.StatusCode, body)
		}
	}
	resp, body := send(10)
	if resp.StatusCode != 429 || body["code"] != "rate_limited" {
		t.Fatalf("11th send: %d %v", resp.StatusCode, body)
	}
	if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err != nil || ra < 1 || ra > 300 {
		t.Fatalf("Retry-After %q", resp.Header.Get("Retry-After"))
	}
	// The paid organization has its own, larger window.
	if code, body := p.a.do("POST", "/v0/inboxes", map[string]any{"username": "paid"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	if code, body := p.a.do("POST", "/v0/inboxes/paid@"+shared+"/messages/send", map[string]any{"to": []string{"x@ext.test"}, "text": "hi"}, nil); code != 200 {
		t.Fatalf("other org send: %d %v", code, body)
	}
	// Five minutes later the window is free again.
	p.system(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(p.ctx, `update messages set created_at = created_at - interval '6 minutes'
			where inbox_id = (select id from inboxes where address = $1)`, "burst@"+shared)
		return err
	})
	if resp, body := send(11); resp.StatusCode != 200 {
		t.Fatalf("after the window: %d %v", resp.StatusCode, body)
	}
}

type hygieneDNS struct{ txt map[string][]string }

func (f hygieneDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}
func (f hygieneDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}
func (f hygieneDNS) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// TestInboundSPFDMARC: SPF and DMARC results are stored on the message;
// mail failing p=reject is labelled dmarc-fail and spam, or refused with 550
// when enforcement is on.
func TestInboundSPFDMARC(t *testing.T) {
	p := newPlatform(t)
	if code, body := p.a.do("POST", "/v0/inboxes", map[string]any{"username": "fin"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	dns := hygieneDNS{txt: map[string][]string{
		"bank.test":           {"v=spf1 ip4:192.0.2.0/24 -all"},
		"_dmarc.bank.test":    {"v=DMARC1; p=reject"},
		"partner.test":        {"v=spf1 ip4:127.0.0.0/8 -all"},
		"_dmarc.partner.test": {"v=DMARC1; p=quarantine"},
	}}
	backend := &mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 1 << 20, DNS: dns, MailAuth: &mx.MailAuth{}}
	mxAddr := listen(t, mx.NewServer(backend, "", "mx."+shared, nil))
	to := "fin@" + shared
	msg := func(from, id string) string {
		return "From: " + from + "\r\nTo: " + to + "\r\nSubject: wire\r\nMessage-ID: <" + id + ">\r\n\r\nplease pay\r\n"
	}
	get := func(id string) map[string]any {
		code, m := p.a.do("GET", "/v0/inboxes/"+to+"/messages/<"+id+">", nil, nil)
		if code != 200 {
			t.Fatalf("get %s: %d %v", id, code, m)
		}
		return m
	}

	// Spoofed From under p=reject, sent from a host outside the SPF record.
	if err := deliver(t, mxAddr, "ceo@bank.test", to, msg("ceo@bank.test", "spoof1@bank.test")); err != nil {
		t.Fatalf("not enforced: %v", err)
	}
	m := get("spoof1@bank.test")
	auth := m["authentication_results"].(map[string]any)
	labels := fmt.Sprint(m["labels"])
	if auth["spf"] != "fail" || auth["dmarc"] != "fail" || auth["dmarc_policy"] != "reject" ||
		!strings.Contains(labels, "dmarc-fail") || !strings.Contains(labels, "spam") {
		t.Fatalf("spoofed: %v %v", auth, labels)
	}

	// SPF aligned pass from the loopback network.
	if err := deliver(t, mxAddr, "billing@partner.test", to, msg("billing@partner.test", "ok1@partner.test")); err != nil {
		t.Fatal(err)
	}
	m = get("ok1@partner.test")
	auth = m["authentication_results"].(map[string]any)
	if auth["spf"] != "pass" || auth["dmarc"] != "pass" || strings.Contains(fmt.Sprint(m["labels"]), "spam") {
		t.Fatalf("aligned: %v %v", auth, m["labels"])
	}

	// With enforcement the spoofed message is refused at DATA.
	enforcing := *backend
	enforcing.MailAuth = &mx.MailAuth{Enforce: true}
	err := deliver(t, listen(t, mx.NewServer(&enforcing, "", "mx."+shared, nil)), "ceo@bank.test", to, msg("ceo@bank.test", "spoof2@bank.test"))
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("enforced: want 550, got %v", err)
	}
	if code, _ := p.a.do("GET", "/v0/inboxes/"+to+"/messages/<spoof2@bank.test>", nil, nil); code != 404 {
		t.Fatalf("refused message stored: %d", code)
	}
}
