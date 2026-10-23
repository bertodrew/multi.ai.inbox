package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/mcpserver"
	"github.com/bertodrew/multi.ai.inbox/internal/mx"
)

// serve starts another API server on the platform, with MCP mounted;
// configure adjusts it before it starts.
func (p *platform) serve(t *testing.T, configure func(*api.Server)) string {
	t.Helper()
	srv := &api.Server{Svc: p.svc, Log: p.log}
	srv.MCP = mcpserver.Handler(p.svc, func(ctx context.Context) mcpserver.Caller {
		return mcpserver.Caller{Principal: api.PrincipalFrom(ctx), IP: api.ClientIP(ctx), RequestID: api.RequestID(ctx)}
	}, p.log, "test")
	if configure != nil {
		configure(srv)
	}
	h := httptest.NewServer(srv.Handler())
	t.Cleanup(h.Close)
	return h.URL
}

// mcpTool calls one MCP tool and returns its structured result.
func mcpTool(t *testing.T, base, key, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	req, _ := http.NewRequest("POST", base+"/mcp", bytes.NewReader(body))
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Result struct {
			IsError           bool           `json:"isError"`
			StructuredContent map[string]any `json:"structuredContent"`
			Content           []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("mcp %s: %d %s", name, resp.StatusCode, raw)
	}
	if out.Result.IsError {
		msg := ""
		for _, c := range out.Result.Content {
			msg += c.Text
		}
		return map[string]any{"error": msg}, false
	}
	return out.Result.StructuredContent, true
}

func TestDrafts(t *testing.T) {
	p := newPlatform(t)
	ctx, a, b := p.ctx, p.a, p.b
	base := p.serve(t, nil)
	a.base = base
	b.base = base
	inbox := "agent@" + shared
	if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": "agent"}, nil); code != 201 {
		t.Fatalf("create inbox: %d %v", code, body)
	}
	drafts := "/v0/inboxes/" + inbox + "/drafts"

	// Create, read, list, update.
	code, d := a.do("POST", drafts, map[string]any{"to": []string{"Bob@Ext.test"}, "subject": "Quote", "text": "first",
		"labels": []string{"approval"}, "attachments": []map[string]any{{"filename": "q.txt", "content_type": "text/plain", "content": "aGVsbG8="}}}, nil)
	if code != 201 || d["status"] != "draft" || fmt.Sprint(d["to"]) != "[bob@ext.test]" {
		t.Fatalf("create draft: %d %v", code, d)
	}
	if att := d["attachments"].([]any)[0].(map[string]any); att["size"] != float64(5) || att["content"] != nil {
		t.Fatalf("draft attachment metadata: %v", att)
	}
	id := d["draft_id"].(string)
	if code, got := a.do("GET", drafts+"/"+id, nil, nil); code != 200 || got["subject"] != "Quote" {
		t.Fatalf("get draft: %d %v", code, got)
	}
	if code, list := a.do("GET", drafts, nil, nil); code != 200 || list["count"] != float64(1) {
		t.Fatalf("list drafts: %d %v", code, list)
	}
	code, d = a.do("PATCH", drafts+"/"+id, map[string]any{"text": "final", "cc": []string{"carol@ext.test"}}, nil)
	if code != 200 || d["text"] != "final" || d["subject"] != "Quote" || fmt.Sprint(d["cc"]) != "[carol@ext.test]" {
		t.Fatalf("patch draft: %d %v", code, d)
	}
	if code, body := a.do("PATCH", drafts+"/"+id, map[string]any{"to": []string{"not an address"}}, nil); code != 400 {
		t.Fatalf("patch with bad address: %d %v", code, body)
	}
	if code, body := a.do("PATCH", drafts+"/"+id, map[string]any{"send_at": time.Now().Add(-time.Hour)}, nil); code != 400 {
		t.Fatalf("send_at in the past: %d %v", code, body)
	}
	var created int
	if err := p.d.WithOrg(ctx, p.orgA.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from events where type = 'draft.created' and payload->'draft'->>'draft_id' = $1", id).Scan(&created)
	}); err != nil || created != 1 {
		t.Fatalf("draft.created events = %d, %v", created, err)
	}

	// Other organizations see nothing.
	if code, _ := b.do("GET", drafts+"/"+id, nil, nil); code != 404 {
		t.Fatalf("org B read org A draft: %d", code)
	}
	var visible int
	if err := p.d.WithOrg(ctx, mustOrgB(t, p), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from drafts").Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatalf("RLS: org B sees %d drafts (%v)", visible, err)
	}

	// Send with an idempotency key: the draft becomes a queued message and is gone.
	key := map[string]string{"Idempotency-Key": "draft-" + id}
	code, sent := a.do("POST", drafts+"/"+id+"/send", nil, key)
	if code != 200 || sent["message_id"] == nil {
		t.Fatalf("send draft: %d %v", code, sent)
	}
	if code, again := a.do("POST", drafts+"/"+id+"/send", nil, key); code != 200 || again["message_id"] != sent["message_id"] {
		t.Fatalf("idempotent draft send: %d %v", code, again)
	}
	if code, _ := a.do("GET", drafts+"/"+id, nil, nil); code != 404 {
		t.Fatalf("draft still there after send: %d", code)
	}
	if code, _ := a.do("POST", drafts+"/"+id+"/send", nil, nil); code != 404 {
		t.Fatalf("second send without key: %d", code)
	}
	code, m := a.do("GET", "/v0/inboxes/"+inbox+"/messages/"+sent["message_id"].(string), nil, nil)
	if code != 200 || m["text"] != "final" || m["status"] != "queued" || fmt.Sprint(m["cc"]) != "[carol@ext.test]" ||
		!strings.Contains(fmt.Sprint(m["labels"]), "approval") || len(m["attachments"].([]any)) != 1 {
		t.Fatalf("sent draft message: %d %v", code, m)
	}

	// A draft needs recipients and a body to send.
	_, empty := a.do("POST", drafts, map[string]any{"subject": "later"}, nil)
	if code, body := a.do("POST", drafts+"/"+empty["draft_id"].(string)+"/send", nil, nil); code != 400 || body["code"] != "validation_error" {
		t.Fatalf("send empty draft: %d %v", code, body)
	}
	if code, _ := a.do("DELETE", drafts+"/"+empty["draft_id"].(string), nil, nil); code != 204 {
		t.Fatalf("delete draft: %d", code)
	}
	if code, _ := a.do("DELETE", drafts+"/"+empty["draft_id"].(string), nil, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}

	// Reply draft: threading comes from the message replied to.
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 10 << 20}, "", "mx."+shared, nil))
	if err := deliver(t, mxAddr, "vendor@ext.test", inbox, "From: vendor@ext.test\r\nTo: "+inbox+
		"\r\nSubject: Pricing\r\nMessage-ID: <q1@ext.test>\r\n\r\nWhat is the price?\r\n"); err != nil {
		t.Fatal(err)
	}
	_, inbound := a.do("GET", "/v0/inboxes/"+inbox+"/messages/<q1@ext.test>", nil, nil)
	code, reply := a.do("POST", drafts, map[string]any{"in_reply_to": inbound["id"], "text": "It is 10 EUR."}, nil)
	if code != 201 || reply["subject"] != "Re: Pricing" || fmt.Sprint(reply["to"]) != "[vendor@ext.test]" ||
		reply["thread_id"] != inbound["thread_id"] || reply["in_reply_to"] != "<q1@ext.test>" {
		t.Fatalf("reply draft: %d %v", code, reply)
	}
	code, rs := a.do("POST", drafts+"/"+reply["draft_id"].(string)+"/send", nil, nil)
	if code != 200 || rs["thread_id"] != inbound["thread_id"] {
		t.Fatalf("send reply draft: %d %v", code, rs)
	}
	_, rm := a.do("GET", "/v0/inboxes/"+inbox+"/messages/"+rs["message_id"].(string), nil, nil)
	if rm["in_reply_to"] != "<q1@ext.test>" || fmt.Sprint(rm["references"]) != "[<q1@ext.test>]" {
		t.Fatalf("reply draft threading: %v", rm)
	}
	if code, body := a.do("POST", drafts, map[string]any{"in_reply_to": "<nope@ext.test>"}, nil); code != 404 {
		t.Fatalf("reply draft to unknown message: %d %v", code, body)
	}

	// Scheduled send: the scheduler sends due drafts.
	code, sched := a.do("POST", drafts, map[string]any{"to": []string{"x@ext.test"}, "subject": "Later", "text": "scheduled",
		"send_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, nil)
	if code != 201 || sched["status"] != "scheduled" {
		t.Fatalf("scheduled draft: %d %v", code, sched)
	}
	schedID := sched["draft_id"].(string)
	if n, _, err := p.svc.SendDueDrafts(ctx, 10); err != nil || n != 0 {
		t.Fatalf("nothing is due yet: %d %v", n, err)
	}
	// Cancel and reschedule through PATCH.
	if code, d := a.do("PATCH", drafts+"/"+schedID, map[string]any{"send_at": nil}, nil); code != 200 || d["status"] != "draft" || d["send_at"] != nil {
		t.Fatalf("cancel schedule: %d %v", code, d)
	}
	if code, d := a.do("PATCH", drafts+"/"+schedID, map[string]any{"send_at": time.Now().Add(time.Minute)}, nil); code != 200 || d["status"] != "scheduled" {
		t.Fatalf("reschedule: %d %v", code, d)
	}
	makeDue := func(id string) {
		p.system(t, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "update drafts set send_at = now() - interval '1 second' where id = $1", id)
			return err
		})
	}
	makeDue(schedID)
	if n, failed, err := p.svc.SendDueDrafts(ctx, 10); err != nil || n != 1 || failed != 0 {
		t.Fatalf("send due drafts: sent %d failed %d %v", n, failed, err)
	}
	if code, _ := a.do("GET", drafts+"/"+schedID, nil, nil); code != 404 {
		t.Fatalf("scheduled draft still there: %d", code)
	}
	var queued int
	_ = p.d.WithOrg(ctx, p.orgA.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from messages where subject = 'Later' and status = 'queued'").Scan(&queued)
	})
	if queued != 1 {
		t.Fatalf("scheduled message queued = %d", queued)
	}

	// A refused scheduled send (sending paused) fails with the reason;
	// a manual send is refused the same way.
	_, sched = a.do("POST", drafts, map[string]any{"to": []string{"x@ext.test"}, "text": "paused",
		"send_at": time.Now().Add(time.Hour)}, nil)
	schedID = sched["draft_id"].(string)
	p.system(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update inboxes set sending_paused_at = now(), sending_paused_reason = 'test' where address = $1", inbox)
		return err
	})
	makeDue(schedID)
	if n, failed, err := p.svc.SendDueDrafts(ctx, 10); err != nil || n != 0 || failed != 1 {
		t.Fatalf("paused scheduled draft: sent %d failed %d %v", n, failed, err)
	}
	_, d = a.do("GET", drafts+"/"+schedID, nil, nil)
	if d["status"] != "failed" || !strings.Contains(fmt.Sprint(d["last_error"]), "paused") {
		t.Fatalf("failed draft: %v", d)
	}
	if code, body := a.do("POST", drafts+"/"+schedID+"/send", nil, nil); code != 403 || body["code"] != "sending_paused" {
		t.Fatalf("manual send while paused: %d %v", code, body)
	}
	if code, d := a.do("PATCH", drafts+"/"+schedID, map[string]any{"text": "edited"}, nil); code != 200 || d["status"] != "draft" || d["last_error"] != nil {
		t.Fatalf("edit failed draft: %d %v", code, d)
	}
	p.system(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update inboxes set sending_paused_at = null, sending_paused_reason = null where address = $1", inbox)
		return err
	})

	// Permissions: message_read reads drafts, message_send is needed to write.
	_, ro := a.do("POST", "/v0/api-keys", map[string]any{"name": "reader", "permissions": []string{"message_read"}}, nil)
	reader := client{t: t, base: base, key: ro["api_key"].(string)}
	if code, _ := reader.do("GET", drafts, nil, nil); code != 200 {
		t.Fatalf("reader list drafts: %d", code)
	}
	for _, c := range []struct{ method, path string }{{"POST", drafts}, {"PATCH", drafts + "/" + schedID},
		{"DELETE", drafts + "/" + schedID}, {"POST", drafts + "/" + schedID + "/send"}} {
		if code, body := reader.do(c.method, c.path, map[string]any{}, nil); code != 403 || body["code"] != "missing_permission" {
			t.Fatalf("reader %s %s: %d %v", c.method, c.path, code, body)
		}
	}
	if out, ok := mcpTool(t, base, ro["api_key"].(string), "create_draft", map[string]any{"inbox": inbox, "text": "x"}); ok {
		t.Fatalf("reader create_draft over MCP: %v", out)
	}

	// MCP: create_draft, list_drafts, send_draft.
	out, ok := mcpTool(t, base, a.key, "create_draft", map[string]any{"inbox": inbox, "in_reply_to": inbound["id"], "text": "via MCP"})
	if !ok || out["subject"] != "Re: Pricing" || out["thread_id"] != inbound["thread_id"] {
		t.Fatalf("mcp create_draft: %v", out)
	}
	list, ok := mcpTool(t, base, a.key, "list_drafts", map[string]any{"inbox": inbox})
	if !ok || list["count"] != float64(2) {
		t.Fatalf("mcp list_drafts: %v", list)
	}
	res, ok := mcpTool(t, base, a.key, "send_draft", map[string]any{"inbox": inbox, "draft_id": out["draft_id"], "idempotency_key": "mcp-1"})
	if !ok || res["thread_id"] != inbound["thread_id"] {
		t.Fatalf("mcp send_draft: %v", res)
	}
	var audited int
	_ = p.d.WithOrg(ctx, p.orgA.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from audit_log where action in ('mcp.create_draft', 'mcp.send_draft')").Scan(&audited)
	})
	if audited != 2 {
		t.Fatalf("mcp draft audit entries = %d", audited)
	}
}

// mustOrgB returns the id of the second organization of newPlatform.
func mustOrgB(t *testing.T, p *platform) string {
	t.Helper()
	var id string
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(p.ctx, "select id from organizations where name = 'Other'").Scan(&id)
	})
	return id
}
