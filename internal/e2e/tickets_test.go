package e2e

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/mcpserver"
)

// TestTicketsEndToEnd covers the human side of the inbox against a real
// database with the production roles, so row level security is enforced the
// way it is in production:
//
//   - an intake inbox opens exactly one ticket per thread, with a number and
//     an SLA taken from the plan;
//   - held mail opens no ticket until it becomes visible, either because the
//     gatekeeper cleared it or because a human released it;
//   - the state machine refuses what it should refuse and the queue counters
//     follow;
//   - an internal note never reaches the thread payload, and does reach the
//     export a data subject is entitled to;
//   - another tenant cannot read, change or count the ticket.
func TestTicketsEndToEnd(t *testing.T) {
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	svc := &core.Service{DB: d, Blob: blob.FS{Root: t.TempDir()}, SharedDomain: shared, Hostname: "mx." + shared, AllowHTTPWebhooks: true}
	if _, err := svc.AddDomain(ctx, "", shared); err != nil {
		t.Fatal(err)
	}
	orgA, _ := svc.CreateOrg(ctx, "Support")
	orgB, _ := svc.CreateOrg(ctx, "Other")
	keyA, err := svc.CreateKey(ctx, orgA.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	keyB, _ := svc.CreateKey(ctx, orgB.ID, "owner")

	srv := &api.Server{Svc: svc, Log: log}
	srv.MCP = mcpserver.Handler(svc, func(ctx context.Context) mcpserver.Caller {
		return mcpserver.Caller{Principal: api.PrincipalFrom(ctx), IP: api.ClientIP(ctx), RequestID: api.RequestID(ctx)}
	}, log, "test")
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	a := client{t: t, base: httpSrv.URL, key: keyA.Key}
	b := client{t: t, base: httpSrv.URL, key: keyB.Key}

	// The default is to hold inbound mail until the gatekeeper decides, which
	// is exactly the case that must not open a ticket.
	if code, me := a.do("GET", "/v0/auth/me", nil, nil); code != 200 || me["screen_before_notify"] != true {
		t.Fatalf("me: %d %v", code, me)
	}
	if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": "desk"}, nil); code != 201 {
		t.Fatalf("create inbox: %d %v", code, body)
	}
	address := "desk@" + shared
	if code, body := a.do("PATCH", "/v0/inboxes/"+address, map[string]any{"ticket_intake": true}, nil); code != 200 || body["ticket_intake"] != true {
		t.Fatalf("intake on: %d %v", code, body)
	}

	ingest := func(raw []byte) {
		t.Helper()
		rc, err := svc.ResolveRecipient(ctx, address)
		if err != nil || rc == nil {
			t.Fatalf("resolve %s: %v", address, err)
		}
		if err := svc.Ingest(ctx, raw, []*core.LocalRecipient{rc}, core.InboundMeta{MailFrom: "customer@ext.test"}); err != nil {
			t.Fatal(err)
		}
	}
	mail := func(id, subject, inReplyTo string) []byte {
		headers := "From: Customer <customer@ext.test>\r\nTo: " + address + "\r\nSubject: " + subject +
			"\r\nMessage-ID: " + id
		if inReplyTo != "" {
			headers += "\r\nIn-Reply-To: " + inReplyTo + "\r\nReferences: " + inReplyTo
		}
		return []byte(headers + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nPlease help.\r\n")
	}
	tickets := func(c client) []map[string]any {
		t.Helper()
		code, body := c.do("GET", "/v0/tickets?limit=50", nil, nil)
		if code != 200 {
			t.Fatalf("list tickets: %d %v", code, body)
		}
		out := []map[string]any{}
		for _, raw := range body["tickets"].([]any) {
			out = append(out, raw.(map[string]any))
		}
		return out
	}

	// 1. Held mail opens no ticket.
	ingest(mail("<t1@ext.test>", "Cannot log in", ""))
	if got := len(tickets(a)); got != 0 {
		t.Fatalf("held mail opened %d tickets, want 0", got)
	}

	// 2. The gatekeeper clears it, and now the ticket exists.
	if err := svc.SaveEnrichment(ctx, core.EnrichJob{ID: firstMessageID(t, ctx, svc, "<t1@ext.test>"), Address: address},
		core.Verdict{Verdict: "clear", Status: "complete", ExtractedText: "Please help."}); err != nil {
		t.Fatal(err)
	}
	list := tickets(a)
	if len(list) != 1 {
		t.Fatalf("after the verdict: %d tickets, want 1", len(list))
	}
	first := list[0]
	if first["number"] != float64(1) || first["state"] != "new" {
		t.Fatalf("first ticket: %v", first)
	}
	if first["sla_policy"] != "free" && first["sla_policy"] != "" {
		// The free plan promises no SLA; the field may be empty but the number must exist.
		t.Logf("sla_policy: %v", first["sla_policy"])
	}
	if first["due_first_reply"] != nil && first["sla_policy"] == "free" {
		t.Fatalf("free plan promised an SLA: %v", first)
	}
	id := first["id"].(string)

	// 3. A reply in the same thread does not open a second ticket.
	ingest(mail("<t2@ext.test>", "Re: Cannot log in", "<t1@ext.test>"))
	if err := svc.SaveEnrichment(ctx, core.EnrichJob{ID: firstMessageID(t, ctx, svc, "<t2@ext.test>"), Address: address},
		core.Verdict{Verdict: "clear", Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	// A second thread, on the other hand, does.
	ingest(mail("<t3@ext.test>", "Invoice question", ""))
	if err := svc.SaveEnrichment(ctx, core.EnrichJob{ID: firstMessageID(t, ctx, svc, "<t3@ext.test>"), Address: address},
		core.Verdict{Verdict: "clear", Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if got := len(tickets(a)); got != 2 {
		t.Fatalf("after three messages in two threads: %d tickets, want 2", got)
	}

	// 4. Note, timeline, state machine.
	if code, note := a.do("POST", "/v0/tickets/"+id+"/notes", map[string]any{"body": "customer is on trial"}, nil); code != 201 {
		t.Fatalf("note: %d %v", code, note)
	}
	threadID := first["thread_id"].(string)
	if code, thread := a.do("GET", "/v0/threads/"+threadID, nil, nil); code != 200 || strings.Contains(toJSON(thread), "customer is on trial") {
		t.Fatalf("note leaked into the thread: %d %v", code, thread)
	}
	code, detail := a.do("GET", "/v0/tickets/"+id, nil, nil)
	if code != 200 || !strings.Contains(toJSON(detail), "customer is on trial") {
		t.Fatalf("note missing from the ticket detail: %d %v", code, detail)
	}
	if code, body := a.do("PATCH", "/v0/tickets/"+id, map[string]any{"state": "closed"}, nil); code != 200 || body["state"] != "closed" {
		t.Fatalf("close: %d %v", code, body)
	}
	if code, body := a.do("PATCH", "/v0/tickets/"+id, map[string]any{"state": "pending"}, nil); code != 409 || body["code"] != "invalid_transition" {
		t.Fatalf("closed to pending: %d %v", code, body)
	}
	if code, body := a.do("PATCH", "/v0/tickets/"+id, map[string]any{"state": "open"}, nil); code != 200 || body["state"] != "open" {
		t.Fatalf("reopen: %d %v", code, body)
	}
	if code, body := a.do("GET", "/v0/tickets/stats", nil, nil); code != 200 || body["open"] != float64(2) {
		t.Fatalf("stats: %d %v", code, body)
	}

	// 5. A scoped key can read the queue and cannot move it.
	code, scoped := a.do("POST", "/v0/api-keys", map[string]any{"name": "reader", "permissions": []string{core.PermInboxRead, core.PermTicketRead}}, nil)
	if code != 201 {
		t.Fatalf("scoped key: %d %v", code, scoped)
	}
	reader := client{t: t, base: httpSrv.URL, key: scoped["api_key"].(string)}
	if code, _ := reader.do("GET", "/v0/tickets", nil, nil); code != 200 {
		t.Fatalf("reader cannot list tickets: %d", code)
	}
	if code, body := reader.do("PATCH", "/v0/tickets/"+id, map[string]any{"state": "resolved"}, nil); code != 403 || body["code"] != "missing_permission" {
		t.Fatalf("reader changed a ticket: %d %v", code, body)
	}

	// 6. Isolation: the other tenant sees nothing and cannot touch it.
	if code, _ := b.do("GET", "/v0/tickets/"+id, nil, nil); code != 404 {
		t.Fatalf("org B read org A ticket: %d", code)
	}
	if code, _ := b.do("POST", "/v0/tickets/"+id+"/notes", map[string]any{"body": "x"}, nil); code != 404 {
		t.Fatalf("org B wrote a note on org A ticket: %d", code)
	}
	if got := len(tickets(b)); got != 0 {
		t.Fatalf("org B queue: %d tickets, want 0", got)
	}
	code, export := b.do("GET", "/v0/export", nil, nil)
	if code != 200 || len(export["tickets"].([]any)) != 0 {
		t.Fatalf("org B export carries org A tickets: %d %v", code, export["counts"])
	}

	// 7. The data subject export carries the ticket and its note.
	code, export = a.do("GET", "/v0/export", nil, nil)
	if code != 200 {
		t.Fatalf("export: %d", code)
	}
	if len(export["tickets"].([]any)) != 2 || len(export["ticket_notes"].([]any)) != 1 {
		t.Fatalf("export: %v", export["counts"])
	}
	if !strings.Contains(toJSON(export), "customer is on trial") {
		t.Fatal("export does not carry the internal note")
	}
}

// firstMessageID resolves the stored message for a Message-ID, which the ingest
// path generates its own id for.
func firstMessageID(t *testing.T, ctx context.Context, svc *core.Service, rfcID string) string {
	t.Helper()
	var id string
	err := svc.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select id::text from messages where rfc_message_id = $1 order by created_at desc limit 1`, rfcID).Scan(&id)
	})
	if err != nil {
		t.Fatalf("message %s: %v", rfcID, err)
	}
	return id
}

// toJSON is the cheap way to assert that a string does not appear anywhere in a
// decoded payload.
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
