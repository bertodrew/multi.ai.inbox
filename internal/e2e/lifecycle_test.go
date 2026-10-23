package e2e

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-smtp"
	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/db"
	"github.com/bertodrew/multi.ai.inbox/internal/metrics"
	"github.com/bertodrew/multi.ai.inbox/internal/mx"
	"github.com/bertodrew/multi.ai.inbox/internal/sender"
)

type platform struct {
	ctx      context.Context
	d        *db.DB
	svc      *core.Service
	log      *slog.Logger
	blobRoot string
	a, b     client
	orgA     *core.Org
}

// newPlatform starts the API on a fresh database with two organizations:
// A on a paid plan, B on the free plan.
func newPlatform(t *testing.T) *platform {
	t.Helper()
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	p := &platform{ctx: ctx, d: d, log: slog.New(slog.NewTextHandler(testWriter{t}, nil)), blobRoot: t.TempDir()}
	p.svc = &core.Service{DB: d, Blob: blob.FS{Root: p.blobRoot}, SharedDomain: shared, Hostname: "mx." + shared}
	if _, err := p.svc.AddDomain(ctx, "", shared); err != nil {
		t.Fatal(err)
	}
	p.orgA, _ = p.svc.CreateOrg(ctx, "Acme")
	orgB, _ := p.svc.CreateOrg(ctx, "Other")
	// These tests cover deletion, antivirus and bounces, not the screening hold
	// (TestScreenBeforeNotify): deliver mail as visible at ingest.
	for _, id := range []string{p.orgA.ID, orgB.ID} {
		if _, err := p.svc.SetScreening(ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.svc.SetPlan(ctx, p.orgA.ID, "builder"); err != nil {
		t.Fatal(err)
	}
	keyA, _ := p.svc.CreateKey(ctx, p.orgA.ID, "agent")
	keyB, _ := p.svc.CreateKey(ctx, orgB.ID, "agent")
	srv := httptest.NewServer((&api.Server{Svc: p.svc, Log: p.log}).Handler())
	t.Cleanup(srv.Close)
	p.a = client{t: t, base: srv.URL, key: keyA.Key}
	p.b = client{t: t, base: srv.URL, key: keyB.Key}
	return p
}

func (p *platform) system(t *testing.T, fn func(pgx.Tx) error) {
	t.Helper()
	if err := p.d.WithSystem(p.ctx, fn); err != nil {
		t.Fatal(err)
	}
}

// deliver sends one message to the platform MX.
func deliver(t *testing.T, addr, from, to, body string) error {
	t.Helper()
	c, err := smtp.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail(from, nil); err != nil {
		return err
	}
	if err := c.Rcpt(to, nil); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, body); err != nil {
		return err
	}
	return w.Close()
}

func blobFiles(t *testing.T, root string) map[string]bool {
	t.Helper()
	files := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(root, path)
			files[rel] = true
		}
		return nil
	})
	return files
}

func TestInboxDeletionAndQuarantine(t *testing.T) {
	p := newPlatform(t)
	ctx, a, b := p.ctx, p.a, p.b
	for _, u := range []string{"gone", "keep"} {
		if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": u}, nil); code != 201 {
			t.Fatalf("create %s: %d %v", u, code, body)
		}
	}
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 10 << 20}, "", "mx."+shared, nil))
	accepted, unknown := metrics.SMTPAccepted.Value(), metrics.SMTPRejected.With("unknown_recipient").Value()
	for _, to := range []string{"gone@" + shared, "keep@" + shared} {
		if err := deliver(t, mxAddr, "vendor@ext.test", to, "From: vendor@ext.test\r\nTo: "+to+"\r\nSubject: Invoice\r\n"+
			"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b1\r\n\r\n--b1\r\nContent-Type: text/plain\r\n\r\nSee attached.\r\n"+
			"--b1\r\nContent-Type: text/csv\r\nContent-Disposition: attachment; filename=i.csv\r\n\r\na,b\r\n--b1--\r\n"); err != nil {
			t.Fatal(err)
		}
	}
	if code, body := a.do("POST", "/v0/inboxes/gone@"+shared+"/messages/send", map[string]any{
		"to": []string{"x@ext.test"}, "subject": "Out", "text": "outbound copy"}, nil); code != 200 {
		t.Fatalf("send: %d %v", code, body)
	}

	// The blobs of each inbox, as the database knows them.
	keysOf := func(address string) []string {
		var keys []string
		p.system(t, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `select m.raw_key from messages m join inboxes i on i.id = m.inbox_id where i.address = $1
				union all select a.storage_key from attachments a join messages m on m.id = a.message_id
				join inboxes i on i.id = m.inbox_id where i.address = $1`, address)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var k string
				if err := rows.Scan(&k); err != nil {
					return err
				}
				keys = append(keys, k)
			}
			return rows.Err()
		})
		return keys
	}
	gone, keep := keysOf("gone@"+shared), keysOf("keep@"+shared)
	if len(gone) != 3 || len(keep) != 2 { // inbound raw + attachment (+ outbound raw)
		t.Fatalf("blob keys: gone %v keep %v", gone, keep)
	}
	files := blobFiles(t, p.blobRoot)
	for _, k := range append(append([]string{}, gone...), keep...) {
		if !files[k] {
			t.Fatalf("blob %s missing on disk before deletion; have %v", k, files)
		}
	}

	if code, body := a.do("DELETE", "/v0/inboxes/gone@"+shared, nil, nil); code != 204 {
		t.Fatalf("delete inbox: %d %v", code, body)
	}
	var queued int
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from blob_deletions").Scan(&queued)
	})
	if queued != 3 {
		t.Fatalf("queued blob deletions = %d, want 3", queued)
	}
	// The /metrics backlog gauges read the same queues (no dispatcher runs
	// here, so the message events are still in the outbox).
	st, err := p.svc.QueueStats(ctx)
	if err != nil || st.BlobDeletions != 3 || st.OutboxPending == 0 || st.OutboxLagSeconds < 0 || st.ScreeningPending != 0 {
		t.Fatalf("queue stats: %+v %v", st, err)
	}
	if n, err := p.svc.ProcessBlobDeletions(ctx, 100); err != nil || n != 3 {
		t.Fatalf("process deletions: %d %v", n, err)
	}
	files = blobFiles(t, p.blobRoot)
	for _, k := range gone {
		if files[k] {
			t.Errorf("blob %s still on disk after deletion", k)
		}
	}
	for _, k := range keep {
		if !files[k] {
			t.Errorf("blob %s of another inbox was removed", k)
		}
	}
	// Idempotent: a key whose blob is already gone (a retry after a crash) completes cleanly.
	p.system(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into blob_deletions (org_id, storage_key) values ($1, $2)", p.orgA.ID, gone[0])
		return err
	})
	if n, err := p.svc.ProcessBlobDeletions(ctx, 100); err != nil || n != 1 {
		t.Fatalf("repeat deletion: %d %v", n, err)
	}
	if n, err := p.svc.ProcessBlobDeletions(ctx, 100); err != nil || n != 0 {
		t.Fatalf("empty queue: %d %v", n, err)
	}

	// Mail to the deleted address is refused at RCPT.
	if err := deliver(t, mxAddr, "vendor@ext.test", "gone@"+shared, "Subject: x\r\n\r\nx\r\n"); err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("mail to deleted inbox: %v", err)
	}
	if metrics.SMTPAccepted.Value() < accepted+2 || metrics.SMTPRejected.With("unknown_recipient").Value() < unknown+1 {
		t.Fatalf("smtp counters: accepted %d (was %d), unknown recipient %d (was %d)", metrics.SMTPAccepted.Value(), accepted,
			metrics.SMTPRejected.With("unknown_recipient").Value(), unknown)
	}
	// Quarantine: no organization can take the address for 90 days.
	for name, c := range map[string]client{"other org": b, "same org": a} {
		code, body := c.do("POST", "/v0/inboxes", map[string]any{"username": "gone"}, nil)
		if code != 409 || body["code"] != "address_quarantined" || !strings.Contains(fmt.Sprint(body["message"]), "gone@"+shared) {
			t.Fatalf("%s recreating a quarantined address: %d %v", name, code, body)
		}
	}
	var until int
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select extract(day from quarantined_until - deleted_at)::int from address_quarantine where address = $1`,
			"gone@"+shared).Scan(&until)
	})
	if until != 90 {
		t.Fatalf("quarantine lasts %d days, want 90", until)
	}
	// After the window the address is free again, and the purge removes the row.
	p.system(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update address_quarantine set quarantined_until = now() - interval '1 second' where address = $1", "gone@"+shared)
		return err
	})
	if n, err := p.svc.PurgeExpiredQuarantine(ctx); err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if code, body := b.do("POST", "/v0/inboxes", map[string]any{"username": "gone"}, nil); code != 201 {
		t.Fatalf("recreate after the window: %d %v", code, body)
	}
}

// With clamd unreachable and the default fail open policy, mail is accepted
// and labelled av-unscanned.
func TestAntivirusFailOpen(t *testing.T) {
	p := newPlatform(t)
	if code, body := p.a.do("POST", "/v0/inboxes", map[string]any{"username": "scan"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	down := l.Addr().String()
	l.Close()
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 1 << 20, Clamd: &clamav.Clamd{Addr: down}}, "", "mx."+shared, nil))
	if err := deliver(t, mxAddr, "v@ext.test", "scan@"+shared, "From: v@ext.test\r\nSubject: hi\r\nMessage-ID: <av1@ext.test>\r\n\r\nhello\r\n"); err != nil {
		t.Fatalf("fail open must accept: %v", err)
	}
	code, m := p.a.do("GET", "/v0/inboxes/scan@"+shared+"/messages/<av1@ext.test>", nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(m["labels"]), "av-unscanned") ||
		m["authentication_results"].(map[string]any)["antivirus"] != "unscanned" {
		t.Fatalf("unscanned message: %d %v", code, m)
	}
}

// rejectingRelay accepts every recipient except those starting with "ghost",
// which get 550 5.1.1 like an unknown mailbox.
type rejectingRelay struct {
	mu   sync.Mutex
	sent []string
}

func (r *rejectingRelay) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &rejectSession{r: r}, nil
}

type rejectSession struct {
	r  *rejectingRelay
	to string
}

func (s *rejectSession) Reset()                               {}
func (s *rejectSession) Logout() error                        { return nil }
func (s *rejectSession) Mail(string, *smtp.MailOptions) error { return nil }
func (s *rejectSession) Data(r io.Reader) error {
	_, _ = io.Copy(io.Discard, r)
	s.r.add(s.to)
	return nil
}
func (r *rejectingRelay) add(to string) { r.mu.Lock(); r.sent = append(r.sent, to); r.mu.Unlock() }
func (s *rejectSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if strings.HasPrefix(to, "ghost") {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "No such user"}
	}
	s.to = to
	return nil
}

func TestBounceBreaker(t *testing.T) {
	p := newPlatform(t)
	ctx, a := p.ctx, p.a
	if code, body := a.do("POST", "/v0/inboxes", map[string]any{"username": "bulk"}, nil); code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	sendPath := "/v0/inboxes/bulk@" + shared + "/messages/send"
	// 20 sends: one rejected at RCPT (5%, not above the limit), one accepted
	// and bounced later by a DSN, which trips the breaker.
	for i := range 20 {
		to := fmt.Sprintf("r%d@ext.test", i)
		switch i {
		case 0:
			to = "ghost@ext.test"
		case 1:
			to = "late@ext.test"
		}
		if code, body := a.do("POST", sendPath, map[string]any{"to": []string{to}, "subject": "Hi", "text": "hello"}, nil); code != 200 {
			t.Fatalf("send %d: %d %v", i, code, body)
		}
	}
	relay := &rejectingRelay{}
	snd := &sender.Sender{Svc: p.svc, Relay: "smtp+insecure://" + listen(t, smtp.NewServer(relay)), HeloName: "mx." + shared, Log: p.log}
	for {
		n, err := snd.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	relay.mu.Lock()
	accepted := len(relay.sent)
	relay.mu.Unlock()
	if accepted != 19 {
		t.Fatalf("relay accepted %d messages, want 19", accepted)
	}
	paused := func() (inbox, org bool) {
		p.system(t, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select i.sending_paused_at is not null, o.sending_paused_at is not null
				from inboxes i join organizations o on o.id = i.org_id where i.address = $1`, "bulk@"+shared).Scan(&inbox, &org)
		})
		return
	}
	if i, o := paused(); i || o {
		t.Fatalf("paused at exactly 5%%: inbox %v org %v", i, o)
	}

	var lateID string
	p.system(t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select rfc_message_id from messages where 'late@ext.test' = any(to_addrs)").Scan(&lateID)
	})
	dsn := "From: MAILER-DAEMON@mx.ext.test\r\nTo: bulk@" + shared + "\r\nSubject: Undelivered Mail Returned to Sender\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/report; report-type=delivery-status; boundary=r1\r\n\r\n" +
		"--r1\r\nContent-Type: text/plain\r\n\r\nYour message could not be delivered.\r\n" +
		"--r1\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; mx.ext.test\r\n\r\n" +
		"Final-Recipient: rfc822; late@ext.test\r\nAction: failed\r\nStatus: 5.1.1\r\nDiagnostic-Code: smtp; 550 5.1.1 User unknown\r\n\r\n" +
		"--r1\r\nContent-Type: text/rfc822-headers\r\n\r\nFrom: bulk@" + shared + "\r\nTo: late@ext.test\r\nMessage-ID: " + lateID + "\r\n\r\n--r1--\r\n"
	mxAddr := listen(t, mx.NewServer(&mx.Backend{Svc: p.svc, Log: p.log, MaxBytes: 1 << 20}, "", "mx."+shared, nil))

	// A report from an ordinary sender (not the null reverse path) is not a bounce.
	if err := deliver(t, mxAddr, "prankster@ext.test", "bulk@"+shared, dsn); err != nil {
		t.Fatal(err)
	}
	if i, o := paused(); i || o {
		t.Fatal("a forged DSN must not count")
	}
	if err := deliver(t, mxAddr, "", "bulk@"+shared, dsn); err != nil {
		t.Fatal(err)
	}
	if i, o := paused(); !i || !o {
		t.Fatalf("after the bounce (10%%): inbox paused %v, org paused %v", i, o)
	}
	code, m := a.do("GET", "/v0/inboxes/bulk@"+shared+"/messages/"+lateID, nil, nil)
	if code != 200 || !strings.Contains(fmt.Sprint(m["labels"]), "bounced") {
		t.Fatalf("bounced message: %d %v", code, m)
	}
	events := map[string]int{}
	p.system(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select type, count(*) from events group by type")
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
	if events["message.bounced"] != 1 || events["inbox.sending.paused"] != 1 || events["organization.sending.paused"] != 1 {
		t.Fatalf("events: %v", events)
	}
	code, inbox := a.do("GET", "/v0/inboxes/bulk@"+shared, nil, nil)
	if code != 200 || inbox["sending_paused_at"] == nil || !strings.Contains(fmt.Sprint(inbox["sending_paused_reason"]), "10.0%") {
		t.Fatalf("paused inbox: %d %v", code, inbox)
	}

	// Sends are refused with a clear code until an operator resumes.
	code, body := a.do("POST", sendPath, map[string]any{"to": []string{"r1@ext.test"}, "text": "again"}, nil)
	if code != 403 || body["code"] != "sending_paused" || !strings.Contains(fmt.Sprint(body["message"]), "organization") {
		t.Fatalf("send while org paused: %d %v", code, body)
	}
	if err := p.svc.ResumeOrgSending(ctx, p.orgA.ID); err != nil {
		t.Fatal(err)
	}
	code, body = a.do("POST", sendPath, map[string]any{"to": []string{"r1@ext.test"}, "text": "again"}, nil)
	if code != 403 || body["code"] != "sending_paused" || !strings.Contains(fmt.Sprint(body["message"]), "bulk@"+shared) {
		t.Fatalf("send while inbox paused: %d %v", code, body)
	}
	if _, err := p.svc.ResumeInboxSending(ctx, "BULK@"+shared); err != nil {
		t.Fatal(err)
	}
	if _, err := p.svc.ResumeInboxSending(ctx, "nobody@"+shared); err == nil || core.AsError(err).Code != "not_found" {
		t.Fatalf("resume unknown inbox: %v", err)
	}
	if code, body := a.do("POST", sendPath, map[string]any{"to": []string{"ghost2@ext.test"}, "text": "after resume"}, nil); code != 200 {
		t.Fatalf("send after resume: %d %v", code, body)
	}
	// The window restarts at the resume: one more hard bounce does not trip it again.
	if _, err := snd.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if i, o := paused(); i || o {
		t.Fatalf("re-paused right after resume: inbox %v org %v", i, o)
	}
}
