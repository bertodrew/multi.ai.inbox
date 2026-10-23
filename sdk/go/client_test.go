package aimulti_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	aimulti "github.com/bertodrew/multi.ai.inbox/sdk/go"
)

// recorder is a stub API that records the last request and answers with a fixed
// status and body.
type recorder struct {
	method string
	path   string
	query  string
	base   string
	header http.Header
	body   []byte
}

func stub(t *testing.T, status int, response string, headers map[string]string) (*recorder, *aimulti.Client, func()) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.method, rec.path, rec.query = r.Method, r.URL.Path, r.URL.RawQuery
		rec.base = "http://" + r.Host
		rec.header, rec.body = r.Header.Clone(), raw
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		if response != "" && w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	c := aimulti.NewClient("mai_live_test", aimulti.WithBaseURL(srv.URL), aimulti.WithUserAgent("suite/1"))
	return rec, c, srv.Close
}

func TestMeUsesTheVersionedPathAndBearerToken(t *testing.T) {
	rec, c, done := stub(t, 200, `{"auth":"api_key","organization_id":"org_1","plan":"team"}`, nil)
	defer done()

	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Plan != "team" || me.OrganizationID != "org_1" {
		t.Fatalf("decoded %+v", me)
	}
	if rec.path != "/v1/me" {
		t.Fatalf("path = %q, want /v1/me", rec.path)
	}
	if got := rec.header.Get("Authorization"); got != "Bearer mai_live_test" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := rec.header.Get("User-Agent"); got != "suite/1" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := rec.header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q", got)
	}
}

// WithAPIVersion is the only way to reach the deprecated prefix, for a
// deployment that predates /v1.
func TestWithAPIVersionReachesV0(t *testing.T) {
	rec, c, done := stub(t, 200, `{"plan":"free"}`, nil)
	defer done()
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.path != "/v1/me" {
		t.Fatalf("default path = %q, want /v1/me", rec.path)
	}

	base := rec.base
	legacy := aimulti.NewClient("k", aimulti.WithBaseURL(base), aimulti.WithAPIVersion("v0"))
	if _, err := legacy.Me(context.Background()); err != nil {
		t.Fatalf("Me on v0: %v", err)
	}
	if rec.path != "/v0/me" {
		t.Fatalf("v0 path = %q, want /v0/me", rec.path)
	}
}

// A trailing slash on the base URL must not produce a doubled path separator.
func TestBaseURLOverrideTrimsSlash(t *testing.T) {
	rec, c, done := stub(t, 200, `{"plan":"free"}`, nil)
	defer done()
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatalf("Me: %v", err)
	}
	slashed := aimulti.NewClient("k", aimulti.WithBaseURL(rec.base+"/"))
	if _, err := slashed.Me(context.Background()); err != nil {
		t.Fatalf("Me with a slashed base: %v", err)
	}
	if rec.path != "/v1/me" {
		t.Fatalf("path = %q, want /v1/me", rec.path)
	}
}

func TestErrorCarriesCodeAndRequestID(t *testing.T) {
	body := `{"name":"Conflict","code":"already_exists","message":"the address is taken"}`
	_, c, done := stub(t, 409, body, map[string]string{"X-Request-Id": "req_abc", "Retry-After": "7"})
	defer done()

	_, err := c.CreateInbox(context.Background(), aimulti.CreateInboxRequest{Username: "sales"})
	if err == nil {
		t.Fatal("want an error")
	}
	var apiErr *aimulti.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T, want *aimulti.Error", err)
	}
	if apiErr.Status != 409 || apiErr.Code != "already_exists" || apiErr.RequestID != "req_abc" {
		t.Fatalf("error = %+v", apiErr)
	}
	if apiErr.RetryAfter != 7 {
		t.Fatalf("RetryAfter = %d", apiErr.RetryAfter)
	}
	if !aimulti.IsCode(err, "already_exists") {
		t.Fatal("IsCode did not match")
	}
	if aimulti.StatusCode(err) != 409 {
		t.Fatalf("StatusCode = %d", aimulti.StatusCode(err))
	}
	if aimulti.RetryAfter(err) != 7*time.Second {
		t.Fatalf("RetryAfter = %v", aimulti.RetryAfter(err))
	}
	if !strings.Contains(err.Error(), "req_abc") {
		t.Fatalf("message does not name the request: %s", err)
	}
	if aimulti.IsCode(err, "not_found") {
		t.Fatal("IsCode matched the wrong code")
	}
}

func TestAttachmentIsSentAsBase64(t *testing.T) {
	rec, c, done := stub(t, 200, `{"message_id":"m1","thread_id":"t1"}`, nil)
	defer done()

	_, err := c.Send(context.Background(), "agent@example.eu", aimulti.SendRequest{
		To:      []string{"human@example.com"},
		Subject: "Report",
		Text:    "attached",
		Attachments: []aimulti.Attachment{{
			Filename:    "report.txt",
			ContentType: "text/plain",
			Data:        []byte("hello"),
		}},
	}, aimulti.WithIdempotencyKey("send-1"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rec.path != "/v1/inboxes/agent@example.eu/messages/send" {
		t.Fatalf("path = %q", rec.path)
	}
	if got := rec.header.Get("Idempotency-Key"); got != "send-1" {
		t.Fatalf("Idempotency-Key = %q", got)
	}
	var body struct {
		Attachments []struct {
			Filename string `json:"filename"`
			Content  string `json:"content"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(rec.body, &body); err != nil {
		t.Fatalf("body: %v (%s)", err, rec.body)
	}
	if len(body.Attachments) != 1 {
		t.Fatalf("attachments = %d", len(body.Attachments))
	}
	want := base64.StdEncoding.EncodeToString([]byte("hello"))
	if body.Attachments[0].Content != want {
		t.Fatalf("content = %q, want %q", body.Attachments[0].Content, want)
	}
}

func TestCancelSendAtSendsNull(t *testing.T) {
	rec, c, done := stub(t, 200, `{"draft_id":"d1"}`, nil)
	defer done()

	if _, err := c.UpdateDraft(context.Background(), "a@example.eu", "d1",
		aimulti.UpdateDraftRequest{CancelSendAt: true}); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if got, ok := body["send_at"]; !ok || string(got) != "null" {
		t.Fatalf("send_at = %s, want null", got)
	}
}

func TestListOptionsBecomeQueryParameters(t *testing.T) {
	rec, c, done := stub(t, 200, `{"count":0,"messages":[]}`, nil)
	defer done()

	if _, err := c.ListMessages(context.Background(), "a@example.eu",
		aimulti.ListOptions{Limit: 5, PageToken: "p2", Label: "spam", IncludeReview: true}); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, want := range []string{"limit=5", "page_token=p2", "label=spam", "include_review=true"} {
		if !strings.Contains(rec.query, want) {
			t.Fatalf("query %q is missing %q", rec.query, want)
		}
	}
}

func TestEventStreamParsesServerSentEvents(t *testing.T) {
	stream := ": ping\n\nid: 7\nevent: message.received\ndata: {\"type\":\"event\",\"event_type\":\"message.received\"}\n\n" +
		"id: 8\nevent: message.sent\ndata: {\"type\":\"event\"}\n\n"
	rec, c, done := stub(t, 200, stream, map[string]string{"Content-Type": "text/event-stream"})
	defer done()

	events, err := c.Events(context.Background(), aimulti.EventOptions{Types: []string{"message.received"}, Inbox: "a@example.eu"})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer func() { _ = events.Close() }()
	if !strings.Contains(rec.query, "types=message.received") || !strings.Contains(rec.query, "inbox=a%40example.eu") {
		t.Fatalf("query = %q", rec.query)
	}
	if got := rec.header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q", got)
	}

	first, err := events.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if first.ID != "7" || first.Name != "message.received" {
		t.Fatalf("event = %+v", first)
	}
	envelope, err := first.Envelope()
	if err != nil || envelope.EventType != "message.received" {
		t.Fatalf("envelope = %+v, %v", envelope, err)
	}
	second, err := events.Next()
	if err != nil || second.ID != "8" || second.Name != "message.sent" {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if _, err := events.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream = %v, want io.EOF", err)
	}
}

func TestStreamErrorsBecomeAPIErrors(t *testing.T) {
	_, c, done := stub(t, 429, `{"code":"too_many_streams","message":"5 streams per key"}`, nil)
	defer done()
	_, err := c.Events(context.Background(), aimulti.EventOptions{})
	if !aimulti.IsCode(err, "too_many_streams") {
		t.Fatalf("err = %v", err)
	}
}

func TestAttachmentDownloadReadsHeaders(t *testing.T) {
	_, c, done := stub(t, 200, "file-bytes", map[string]string{
		"Content-Type":        "application/pdf",
		"Content-Disposition": `attachment; filename="invoice.pdf"`,
	})
	defer done()
	file, err := c.GetAttachment(context.Background(), "a@example.eu", "m1", "att1", false)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if file.Filename != "invoice.pdf" || file.ContentType != "application/pdf" || string(file.Data) != "file-bytes" {
		t.Fatalf("file = %+v", file)
	}
}

func TestDeleteWithNoContentIsNotAnError(t *testing.T) {
	rec, c, done := stub(t, 204, "", nil)
	defer done()
	if err := c.DeleteInbox(context.Background(), "a+tag@example.eu"); err != nil {
		t.Fatalf("DeleteInbox: %v", err)
	}
	if rec.path != "/v1/inboxes/a+tag@example.eu" {
		t.Fatalf("path = %q", rec.path)
	}
}

func TestContextIsRequired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, c, done := stub(t, 200, `{}`, nil)
	defer done()
	if _, err := c.Me(ctx); err == nil {
		t.Fatal("a cancelled context must fail the call")
	}
}
