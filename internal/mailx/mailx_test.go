package mailx

import (
	"strings"
	"testing"
	"time"
)

func TestBuildParseRoundTripKeepsThreadingHeaders(t *testing.T) {
	raw, err := Build(Outgoing{
		FromName: "Sales Agent", From: "sales@agents.test", To: []string{"vendor@ext.test"}, Cc: []string{"boss@ext.test"},
		Subject: "Re: Quote", Text: "Thursday works.", HTML: "<p>Thursday works.</p>",
		MessageID: "<out1@agents.test>", InReplyTo: "<ext1@ext.test>", References: []string{"<ext0@ext.test>", "<ext1@ext.test>"},
		Attachments: []Attachment{{Filename: "spec.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}},
		Date:        time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Bcc") {
		t.Fatal("Bcc must never be written")
	}
	p, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.MessageID != "<out1@agents.test>" || p.InReplyTo != "<ext1@ext.test>" {
		t.Fatalf("ids = %q %q", p.MessageID, p.InReplyTo)
	}
	if len(p.References) != 2 || p.References[1] != "<ext1@ext.test>" {
		t.Fatalf("references = %v", p.References)
	}
	if p.From != "sales@agents.test" || p.To[0] != "vendor@ext.test" || p.Cc[0] != "boss@ext.test" {
		t.Fatalf("addresses = %q %v %v", p.From, p.To, p.Cc)
	}
	if strings.TrimSpace(p.Text) != "Thursday works." || !strings.Contains(p.HTML, "<p>") {
		t.Fatalf("bodies = %q %q", p.Text, p.HTML)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Filename != "spec.pdf" || string(p.Attachments[0].Data) != "%PDF-1.4" {
		t.Fatalf("attachments = %+v", p.Attachments)
	}
}

func TestNormalizeSubject(t *testing.T) {
	for in, want := range map[string]string{
		"Re: Quote":          "Quote",
		"RE: Fwd: re: Quote": "Quote",
		"R: I: Preventivo":   "Preventivo",
		"AW: Angebot":        "Angebot",
		"Quote":              "Quote",
	} {
		if got := NormalizeSubject(in); got != want {
			t.Errorf("NormalizeSubject(%q) = %q, want %q", in, got, want)
		}
	}
	if ReplySubject("Re: Quote") != "Re: Quote" {
		t.Error("ReplySubject must not stack prefixes")
	}
}

func TestParseFallsBackWhenMessageIDMissing(t *testing.T) {
	raw := "From: a@x.test\r\nTo: b@y.test\r\nSubject: hi\r\n\r\nhello\r\n"
	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.MessageID, "<") || strings.TrimSpace(p.Text) != "hello" {
		t.Fatalf("parsed = %+v", p)
	}
}
