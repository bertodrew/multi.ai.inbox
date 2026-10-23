package e2e

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-smtp"
	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/clamav/clamavtest"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/mx"
	"github.com/bertodrew/multi.ai.inbox/internal/sender"
)

// eventCounts returns the number of outbox events per type.
func (p *platform) eventCounts(t *testing.T) map[string]int {
	t.Helper()
	events := map[string]int{}
	p.system(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(p.ctx, "select type, count(*) from events group by type")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var typ string
			var n int
			if err := rows.Scan(&typ, &n); err != nil {
				return err
			}
			events[typ] = n
		}
		return rows.Err()
	})
	return events
}

// drain runs the sender until the queue is empty.
func drain(t *testing.T, snd *sender.Sender) {
	t.Helper()
	for {
		n, err := snd.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
}

func TestOrgDelete(t *testing.T) {
	p := newPlatform(t)
	ctx, a, b := p.ctx, p.a, p.b
	if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": "doomed"}, nil); code != 201 {
		t.Fatalf("create inbox: %d %v", code, body)
	}
	if code, body := a.do("POST", "/v0/domains", map[string]any{"domain": "acme.test"}, nil); code != 201 && code != 200 {
		t.Fatalf("create domain: %d %v", code, body)
	}
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 1 << 20}, "", "mx."+shared, nil))
	if err := deliver(t, mxAddr, "vendor@ext.test", "doomed@"+shared, "From: vendor@ext.test\r\nTo: doomed@"+shared+"\r\nSubject: Invoice\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b1\r\n\r\n--b1\r\nContent-Type: text/plain\r\n\r\nSee attached.\r\n"+
		"--b1\r\nContent-Type: text/csv\r\nContent-Disposition: attachment; filename=i.csv\r\n\r\na,b\r\n--b1--\r\n"); err != nil {
		t.Fatal(err)
	}
	if code, body := a.do("POST", "/v0/inboxes/doomed@"+shared+"/messages/send", map[string]any{
		"to": []string{"x@ext.test"}, "subject": "Out", "text": "outbound copy"}, nil); code != 200 {
		t.Fatalf("send: %d %v", code, body)
	}
	var keys []string
	p.system(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select raw_key from messages where org_id = $1
			union all select storage_key from attachments where org_id = $1`, p.orgA.ID)
		if err != nil {
			return err
		}
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	files := blobFiles(t, p.blobRoot)
	if len(keys) != 3 {
		t.Fatalf("blob keys: %v", keys)
	}
	for _, k := range keys {
		if !files[k] {
			t.Fatalf("blob %s missing before the delete", k)
		}
	}

	// A wrong confirmation changes nothing.
	export := func(*core.OrgDeletionSummary) error { t.Fatal("export called for a refused delete"); return nil }
	if _, err := p.svc.DeleteOrg(ctx, p.orgA.ID, "acme", export); err == nil || core.AsError(err).Code != "validation_error" {
		t.Fatalf("wrong confirmation: %v", err)
	}
	if _, err := p.svc.DeleteOrg(ctx, "00000000-0000-0000-0000-000000000000", "Acme", export); err == nil || core.AsError(err).Code != "not_found" {
		t.Fatalf("unknown org: %v", err)
	}
	if code, body := a.do("GET", "/v0/inboxes", nil, nil); code != 200 {
		t.Fatalf("key after refused delete: %d %v", code, body)
	}

	// A failed export stops before the delete; the keys stay revoked.
	if _, err := p.svc.DeleteOrg(ctx, p.orgA.ID, "Acme", func(*core.OrgDeletionSummary) error { return errors.New("disk full") }); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("a failed export must fail the delete: %v", err)
	}
	if code, body := a.do("GET", "/v0/inboxes", nil, nil); code != 401 {
		t.Fatalf("key after the revocation: %d %v", code, body)
	}
	var orgs int
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from organizations where id = $1", p.orgA.ID).Scan(&orgs)
	})
	if orgs != 1 {
		t.Fatal("organization deleted although the export failed")
	}

	var sum *core.OrgDeletionSummary
	res, err := p.svc.DeleteOrg(ctx, p.orgA.ID, "Acme", func(s *core.OrgDeletionSummary) error {
		// The summary is exported while everything still exists.
		var n int
		p.system(t, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from messages where org_id = $1", p.orgA.ID).Scan(&n)
		})
		if n != 2 {
			t.Errorf("messages at export time = %d", n)
		}
		sum = s
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil || sum.Organization.Name != "Acme" || fmt.Sprint(sum.Inboxes) != "[doomed@"+shared+"]" ||
		fmt.Sprint(sum.Domains) != "[acme.test]" || sum.Counts["messages"] != 2 || sum.Counts["attachments"] != 1 {
		t.Fatalf("summary: %+v", sum)
	}
	actions := []string{}
	for _, e := range sum.Audit {
		actions = append(actions, e.Action)
	}
	if !strings.Contains(strings.Join(actions, ","), "org.delete") {
		t.Fatalf("audit log in the summary: %v", actions)
	}
	if !res.Deleted || res.BlobsQueued != 3 || res.AddressesQuarantined != 1 || fmt.Sprint(res.DomainsReleased) != "[acme.test]" {
		t.Fatalf("result: %+v", res)
	}

	// Nothing of the organization is left but the blob queue and the quarantine.
	p.system(t, func(tx pgx.Tx) error {
		for _, table := range []string{"organizations", "inboxes", "messages", "attachments", "api_keys", "audit_log", "domains", "events"} {
			col := "org_id"
			if table == "organizations" {
				col = "id"
			}
			var n int
			if err := tx.QueryRow(ctx, fmt.Sprintf("select count(*) from %s where %s = $1", table, col), p.orgA.ID).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("%s still has %d rows of the deleted organization", table, n)
			}
		}
		return nil
	})
	if n, err := p.svc.ProcessBlobDeletions(ctx, 100); err != nil || n != 3 {
		t.Fatalf("blob deletions: %d %v", n, err)
	}
	files = blobFiles(t, p.blobRoot)
	for _, k := range keys {
		if files[k] {
			t.Errorf("blob %s still on disk", k)
		}
	}
	// The keys are rejected, the address is quarantined, the domain is free.
	if code, body := a.do("GET", "/v0/inboxes", nil, nil); code != 401 || body["code"] != "unknown_api_key" {
		t.Fatalf("deleted org key: %d %v", code, body)
	}
	if code, body := b.do("POST", "/v0/inboxes", map[string]any{"username": "doomed"}, nil); code != 409 || body["code"] != "address_quarantined" {
		t.Fatalf("recreate a deleted org's address: %d %v", code, body)
	}
	if code, body := b.do("POST", "/v0/domains", map[string]any{"domain": "acme.test"}, nil); code != 201 && code != 200 {
		t.Fatalf("released domain: %d %v", code, body)
	}
	if err := deliver(t, mxAddr, "vendor@ext.test", "doomed@"+shared, "Subject: x\r\n\r\nx\r\n"); err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("mail to a deleted org: %v", err)
	}
}

// Attachments uploaded through the API and agent to agent mail are scanned
// with the same clamd client and the same fail open or closed policy as the
// SMTP edge.
func TestAntivirusAPIAndInternal(t *testing.T) {
	p := newPlatform(t)
	ctx := p.ctx
	for _, u := range []string{"src", "dst"} {
		if code, body := p.a.do("POST", "/v0/inboxes", map[string]any{"username": u}, nil); code != 201 {
			t.Fatalf("create %s: %d %v", u, code, body)
		}
	}
	clamdAddr, _ := clamavtest.Fake(t, false)
	down := clamavtest.ClosedAddr(t)
	// withAV returns an API client and a sender backed by a copy of the
	// service with the given scanner policy.
	withAV := func(addr string, failClosed bool) (client, *sender.Sender) {
		svc := *p.svc
		svc.Antivirus = &clamav.Clamd{Addr: addr}
		svc.AntivirusFailClosed = failClosed
		srv := httptest.NewServer((&api.Server{Svc: &svc, Log: p.log}).Handler())
		t.Cleanup(srv.Close)
		relay := listen(t, smtp.NewServer(&rejectingRelay{}))
		return client{t: t, base: srv.URL, key: p.a.key}, &sender.Sender{Svc: &svc, Relay: "smtp+insecure://" + relay, HeloName: "mx." + shared, Log: p.log}
	}
	sendPath := "/v0/inboxes/src@" + shared + "/messages/send"
	att := func(name, content string) []map[string]any {
		return []map[string]any{{"filename": name, "content_type": "application/octet-stream",
			"content": base64.StdEncoding.EncodeToString([]byte(content))}}
	}

	c, snd := withAV(clamdAddr, false)
	code, body := c.do("POST", sendPath, map[string]any{"to": []string{"x@ext.test"}, "subject": "Invoice", "text": "see attached",
		"attachments": att("invoice.exe", "MZ"+clamavtest.EICAR)}, nil)
	if code != 422 || body["code"] != "attachment_infected" || !strings.Contains(fmt.Sprint(body["message"]), "invoice.exe") {
		t.Fatalf("infected attachment: %d %v", code, body)
	}
	if code, body := c.do("POST", sendPath, map[string]any{"to": []string{"x@ext.test"}, "subject": "Report", "text": "attached",
		"attachments": att("report.csv", "a,b\n1,2\n")}, nil); code != 200 {
		t.Fatalf("clean attachment: %d %v", code, body)
	}
	var queued int
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from messages where direction = 'out'").Scan(&queued)
	})
	if queued != 1 {
		t.Fatalf("queued outbound messages = %d, want 1 (the infected one must not be queued)", queued)
	}
	drain(t, snd)

	// Agent to agent: the API only scans attachments, the sender scans the
	// whole message before local delivery.
	if code, body := c.do("POST", sendPath, map[string]any{"to": []string{"dst@" + shared, "y@ext.test"}, "subject": "Payload",
		"text": clamavtest.EICAR}, nil); code != 200 {
		t.Fatalf("send to agent: %d %v", code, body)
	}
	if code, body := c.do("POST", sendPath, map[string]any{"to": []string{"dst@" + shared}, "subject": "Hello agent",
		"text": "clean"}, nil); code != 200 {
		t.Fatalf("send to agent: %d %v", code, body)
	}
	drain(t, snd)
	var inbound []string
	var avs []string
	p.system(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select m.subject, coalesce(m.auth_results->>'antivirus', '') from messages m join inboxes i on i.id = m.inbox_id
			where i.address = $1 order by m.created_at`, "dst@"+shared)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s, av string
			if err := rows.Scan(&s, &av); err != nil {
				return err
			}
			inbound, avs = append(inbound, s), append(avs, av)
		}
		return rows.Err()
	})
	if fmt.Sprint(inbound) != "[Hello agent]" || fmt.Sprint(avs) != "[clean]" {
		t.Fatalf("agent inbox: %v %v", inbound, avs)
	}
	var status, lastErr string
	var rejected int
	p.system(t, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "select status, coalesce(last_error, '') from messages where subject = 'Payload' and direction = 'out'").
			Scan(&status, &lastErr); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "select count(*) from send_outcomes where outcome = 'rejected'").Scan(&rejected)
	})
	if status != "failed" || !strings.Contains(lastErr, "virus detected") || !strings.Contains(lastErr, clamavtest.Signature) {
		t.Fatalf("infected agent mail: %s %q", status, lastErr)
	}
	if rejected != 0 {
		t.Fatalf("infected content counted as %d hard bounces", rejected)
	}

	// Fail open: accepted with av-unscanned, counted, one alert event per
	// organization per 10 minutes.
	before := core.AVUnscannedTotal()
	c, snd = withAV(down, false)
	code, body = c.do("POST", sendPath, map[string]any{"to": []string{"dst@" + shared}, "subject": "Unscanned", "text": "hi",
		"attachments": att("a.txt", "hello")}, nil)
	if code != 200 {
		t.Fatalf("fail open send: %d %v", code, body)
	}
	code, m := p.a.do("GET", "/v0/inboxes/src@"+shared+"/messages/"+fmt.Sprint(body["message_id"]), nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(m["labels"]), "av-unscanned") {
		t.Fatalf("unscanned outbound: %d %v", code, m)
	}
	drain(t, snd)
	code, list := p.a.do("GET", "/v0/inboxes/dst@"+shared+"/messages?label=av-unscanned", nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(list), "Unscanned") {
		t.Fatalf("unscanned agent mail: %d %v", code, list)
	}
	if got := core.AVUnscannedTotal() - before; got != 2 { // the API attachment, then local delivery
		t.Fatalf("av_unscanned_total grew by %d, want 2", got)
	}
	if ev := p.eventCounts(t); ev["security.av_unavailable"] != 1 {
		t.Fatalf("security.av_unavailable events: %v", ev)
	}

	// Fail closed: the API refuses, agent to agent mail waits in the queue.
	c, snd = withAV(down, true)
	code, body = c.do("POST", sendPath, map[string]any{"to": []string{"x@ext.test"}, "text": "hi", "attachments": att("a.txt", "hello")}, nil)
	if code != 503 || body["code"] != "antivirus_unavailable" {
		t.Fatalf("fail closed send: %d %v", code, body)
	}
	if code, body := c.do("POST", sendPath, map[string]any{"to": []string{"dst@" + shared}, "subject": "Deferred", "text": "hi"}, nil); code != 200 {
		t.Fatalf("send without attachments: %d %v", code, body)
	}
	drain(t, snd)
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select status, coalesce(last_error, '') from messages where subject = 'Deferred'").Scan(&status, &lastErr)
	})
	if status != "queued" || !strings.Contains(lastErr, "virus scanner unavailable") {
		t.Fatalf("fail closed agent mail: %s %q", status, lastErr)
	}
}

// arfReport builds a feedback loop report (RFC 5965) about originalID.
func arfReport(to, feedbackType, originalID string) string {
	return "From: staff@fbl.ext.test\r\nTo: " + to + "\r\nSubject: complaint about message\r\n" +
		"Message-ID: <arf." + strings.Trim(originalID, "<>") + "." + feedbackType + "@fbl.ext.test>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/report; report-type=feedback-report; boundary=f1\r\n\r\n" +
		"--f1\r\nContent-Type: text/plain\r\n\r\nThis is an email abuse report.\r\n" +
		"--f1\r\nContent-Type: message/feedback-report\r\n\r\n" +
		"Feedback-Type: " + feedbackType + "\r\nUser-Agent: FBL/1.0\r\nVersion: 1\r\nOriginal-Rcpt-To: <redacted@ext.test>\r\n\r\n" +
		"--f1\r\nContent-Type: text/rfc822-headers\r\n\r\nFrom: news@" + shared + "\r\nTo: redacted@ext.test\r\nMessage-ID: " + originalID + "\r\n\r\n" +
		"--f1--\r\n"
}

func TestComplaints(t *testing.T) {
	p := newPlatform(t)
	ctx, a := p.ctx, p.a
	if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": "news"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	sendPath := "/v0/inboxes/news@" + shared + "/messages/send"
	for i := range 5 {
		if code, body := a.do("POST", sendPath, map[string]any{"to": []string{fmt.Sprintf("r%d@ext.test", i)}, "subject": "News", "text": "hello"}, nil); code != 200 {
			t.Fatalf("send %d: %d %v", i, code, body)
		}
	}
	drain(t, &sender.Sender{Svc: p.svc, Relay: "smtp+insecure://" + listen(t, smtp.NewServer(&rejectingRelay{})), HeloName: "mx." + shared, Log: p.log})
	var ids []string
	p.system(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select rfc_message_id from messages where direction = 'out' order by to_addrs")
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if len(ids) != 5 {
		t.Fatalf("sent ids: %v", ids)
	}
	fbl := "fbl@" + shared
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 1 << 20, FBLAddress: fbl}, "", "mx."+shared, nil))
	paused := func() (inbox, org bool) {
		p.system(t, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select i.sending_paused_at is not null, o.sending_paused_at is not null
				from inboxes i join organizations o on o.id = i.org_id where i.address = $1`, "news@"+shared).Scan(&inbox, &org)
		})
		return
	}
	complaints := func() int {
		var n int
		p.system(t, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from send_outcomes where outcome = 'complained'").Scan(&n)
		})
		return n
	}

	// A complaint delivered to the inbox, then the same report again.
	for range 2 {
		if err := deliver(t, mxAddr, "staff@fbl.ext.test", "news@"+shared, arfReport("news@"+shared, "abuse", ids[0])); err != nil {
			t.Fatal(err)
		}
	}
	if n := complaints(); n != 1 {
		t.Fatalf("complaints after a repeated report = %d, want 1", n)
	}
	code, m := a.do("GET", "/v0/inboxes/news@"+shared+"/messages/"+ids[0], nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(m["labels"]), "complained") {
		t.Fatalf("complained message: %d %v", code, m)
	}
	// A not-spam report and a report about a message we never sent count nothing.
	if err := deliver(t, mxAddr, "staff@fbl.ext.test", "news@"+shared, arfReport("news@"+shared, "not-spam", ids[1])); err != nil {
		t.Fatal(err)
	}
	if err := deliver(t, mxAddr, "staff@fbl.ext.test", fbl, arfReport(fbl, "abuse", "<unknown.1@"+shared+">")); err != nil {
		t.Fatal(err)
	}
	if n := complaints(); n != 1 {
		t.Fatalf("complaints = %d, want 1", n)
	}
	// Second complaint through the shared feedback loop address.
	if err := deliver(t, mxAddr, "staff@fbl.ext.test", fbl, arfReport(fbl, "abuse", ids[1])); err != nil {
		t.Fatal(err)
	}
	if n := complaints(); n != 2 {
		t.Fatalf("complaints after the shared address = %d, want 2", n)
	}
	if i, o := paused(); i || o {
		t.Fatalf("paused after 2 complaints: inbox %v org %v", i, o)
	}
	// The third complaint in 24 hours pauses a small sender.
	if err := deliver(t, mxAddr, "staff@fbl.ext.test", fbl, arfReport(fbl, "abuse", ids[2])); err != nil {
		t.Fatal(err)
	}
	if i, o := paused(); !i || !o {
		t.Fatalf("after 3 complaints: inbox paused %v, org paused %v", i, o)
	}
	ev := p.eventCounts(t)
	if ev["message.complained"] != 3 || ev["inbox.sending.paused"] != 1 || ev["organization.sending.paused"] != 1 {
		t.Fatalf("events: %v", ev)
	}
	code, inbox := a.do("GET", "/v0/inboxes/news@"+shared, nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(inbox["sending_paused_reason"]), "3 spam complaints") {
		t.Fatalf("paused inbox: %d %v", code, inbox)
	}
	// Reports to the shared address are not stored anywhere.
	var stored int
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from messages where direction = 'in' and to_addrs @> array[$1]", fbl).Scan(&stored)
	})
	if stored != 0 {
		t.Fatalf("%d reports to the shared address were stored", stored)
	}
	if code, body := a.do("POST", sendPath, map[string]any{"to": []string{"r9@ext.test"}, "text": "again"}, nil); code != 403 || body["code"] != "sending_paused" {
		t.Fatalf("send while paused: %d %v", code, body)
	}
}
