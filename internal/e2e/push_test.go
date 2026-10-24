// Web push against a real Postgres and a fake push service: registration,
// tenant isolation, the lock screen payload (decrypted, as the browser would),
// the 404 and 410 removal path, the failure threshold, the rate limit, and the
// promise that a broken push service never fails the delivery of mail.
package e2e

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"crypto/hkdf"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// fakeDevice is one browser at the push service: the answer it gives, the
// subscription it was registered with, and the last request it received.
type fakeDevice struct {
	code   int
	priv   *ecdh.PrivateKey
	auth   []byte
	p256dh string
	mu     sync.Mutex
	hits   int
	body   []byte
	header http.Header
}

// fakePush is a push service: /iphone and friends each answer with the status
// their device was configured with.
type fakePush struct {
	mu      sync.Mutex
	devices map[string]*fakeDevice
}

func (f *fakePush) device(path string) *fakeDevice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.devices[path]
}

func (f *fakePush) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	dev := f.device(r.URL.Path)
	if dev == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	dev.mu.Lock()
	dev.hits++
	dev.body = body
	dev.header = r.Header.Clone()
	code := dev.code
	dev.mu.Unlock()
	w.WriteHeader(code)
}

// newDevice generates the subscription keys a browser would, as a throwaway
// pair: the test never uses a real VAPID key or a real push service.
func newDevice(t *testing.T, f *fakePush, path string, code int) *fakeDevice {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	dev := &fakeDevice{code: code, priv: key, auth: auth,
		p256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())}
	f.mu.Lock()
	if f.devices == nil {
		f.devices = map[string]*fakeDevice{}
	}
	f.devices[path] = dev
	f.mu.Unlock()
	return dev
}

// decryptPush opens an aes128gcm body exactly as the service worker would
// (RFC 8291 and RFC 8188), so the assertions below are about the bytes the
// browser receives, not about an internal struct.
func decryptPush(t *testing.T, body []byte, dev *fakeDevice) []byte {
	t.Helper()
	if len(body) < 21+65 {
		t.Fatalf("push body is %d bytes, too short for a header", len(body))
	}
	salt := body[:16]
	idLen := int(body[20])
	if len(body) < 21+idLen {
		t.Fatalf("push body is %d bytes, key id is %d", len(body), idLen)
	}
	asPub, err := ecdh.P256().NewPublicKey(body[21 : 21+idLen])
	if err != nil {
		t.Fatalf("the application server key is not a P-256 point: %v", err)
	}
	secret, err := dev.priv.ECDH(asPub)
	if err != nil {
		t.Fatal(err)
	}
	uaPub := dev.priv.PublicKey().Bytes()
	info := append([]byte("WebPush: info\x00"), uaPub...)
	info = append(info, body[21:21+idLen]...)
	ikm, err := hkdf.Key(sha256.New, secret, dev.auth, string(info), 32)
	if err != nil {
		t.Fatal(err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := aead.Open(nil, nonce, body[21+idLen:], nil)
	if err != nil {
		t.Fatalf("the payload does not open with the subscription keys: %v", err)
	}
	// Data, then 0x02 as the delimiter, then zero padding.
	i := len(plain) - 1
	for i >= 0 && plain[i] == 0 {
		i--
	}
	if i < 0 || plain[i] != 2 {
		t.Fatal("the payload has no padding delimiter")
	}
	return plain[:i]
}

func pushEnv(private, public string) func(string) string {
	return func(k string) string {
		switch k {
		case core.EnvVAPIDPrivateKey:
			return private
		case core.EnvVAPIDPublicKey:
			return public
		case core.EnvVAPIDSubject:
			return "mailto:push@agents.test"
		}
		return ""
	}
}

func TestWebPush(t *testing.T) {
	ctx := context.Background()
	d := openPlatform(t, ctx, freshDatabase(t))
	log := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// A throwaway VAPID pair, generated in code for this test only.
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	push, status := core.LoadPush(pushEnv(private, public))
	if push == nil {
		t.Fatalf("push must be on: %v", status.Warnings)
	}

	svc := &core.Service{DB: d, Blob: blob.FS{Root: t.TempDir()}, SharedDomain: shared,
		Hostname: "mx." + shared, AllowHTTPWebhooks: true, Push: push}
	svc.Log = log
	if _, err := svc.AddDomain(ctx, "", shared); err != nil {
		t.Fatal(err)
	}
	orgA, err := svc.CreateOrg(ctx, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := svc.CreateOrg(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	keyA, err := svc.CreateKey(ctx, orgA.ID, "console")
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := svc.CreateKey(ctx, orgB.ID, "console")
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := svc.CreateOperatorKey(ctx, orgA.ID, "narrow", []string{core.PermMessageRead})
	if err != nil {
		t.Fatal(err)
	}
	// message.received fires at ingest, which is what a push hangs off. Held
	// mail is covered by TestScreenBeforeNotify: no event, no notification.
	if _, err := svc.SetScreening(ctx, orgA.ID, false); err != nil {
		t.Fatal(err)
	}
	inbox, _, err := svc.CreateInbox(ctx, orgA.ID, core.CreateInboxRequest{Username: "support"})
	if err != nil {
		t.Fatal(err)
	}

	httpSrv := httptest.NewServer((&api.Server{Svc: svc, Log: log}).Handler())
	defer httpSrv.Close()
	a := client{t: t, base: httpSrv.URL, key: keyA.Key}
	b := client{t: t, base: httpSrv.URL, key: keyB.Key}

	// TLS, because a real push endpoint is always https and the API refuses
	// anything else. The sender trusts this certificate through the client
	// httptest hands out, which is also why it is not the SSRF guarded client.
	pushSrv := &fakePush{}
	pushHTTP := httptest.NewTLSServer(pushSrv)
	defer pushHTTP.Close()

	// One sender per subtest keeps the thresholds independent; the client is a
	// plain one so the test can reach 127.0.0.1, which the SSRF guard refuses.
	sender := func(limit int) *core.PushSender {
		return &core.PushSender{Svc: svc, Log: log, Client: pushHTTP.Client(), FailureLimit: limit,
			MinInterval: time.Nanosecond}
	}

	subscribe := func(c client, endpoint, agent string) (int, map[string]any) {
		return c.do("POST", "/v0/push/subscriptions", map[string]any{
			"endpoint":        endpoint,
			"keys":            map[string]any{"p256dh": "", "auth": ""},
			"expiration_time": nil,
			"user_agent":      agent,
		}, nil)
	}
	subscribeDevice := func(t *testing.T, c client, dev *fakeDevice, path, agent string) map[string]any {
		t.Helper()
		code, body := c.do("POST", "/v0/push/subscriptions", map[string]any{
			"endpoint":        pushHTTP.URL + path,
			"keys":            map[string]any{"p256dh": dev.p256dh, "auth": base64.RawURLEncoding.EncodeToString(dev.auth)},
			"expiration_time": nil,
			"user_agent":      agent,
		}, nil)
		if code != http.StatusCreated {
			t.Fatalf("subscribe %s: %d %v", path, code, body)
		}
		return body
	}
	countSubs := func(t *testing.T, orgID string) int {
		t.Helper()
		var n int
		if err := d.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from push_subscriptions").Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	subExists := func(t *testing.T, id string) bool {
		t.Helper()
		var ok bool
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select exists(select 1 from push_subscriptions where id = $1)", id).Scan(&ok)
		}); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	ingest := func(t *testing.T, id, subject string) {
		t.Helper()
		raw := []byte("From: Vendor <vendor@ext.test>\r\nTo: " + inbox.InboxID + "\r\nSubject: " + subject +
			"\r\nMessage-ID: <" + id + ">\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
			"Please quote 40 units at the agreed price.\r\n")
		rc := []*core.LocalRecipient{{InboxID: inbox.ID, OrgID: orgA.ID, Address: inbox.InboxID}}
		if err := svc.Ingest(ctx, raw, rc, core.InboundMeta{}); err != nil {
			t.Fatalf("ingest %s: %v", id, err)
		}
	}

	t.Run("disabled without a key pair", func(t *testing.T) {
		off := *svc
		off.Push = nil
		srv := httptest.NewServer((&api.Server{Svc: &off, Log: log}).Handler())
		defer srv.Close()
		c := client{t: t, base: srv.URL, key: keyA.Key}
		code, body := subscribe(c, "https://web.push.apple.com/x", "")
		if code != http.StatusNotImplemented || body["code"] != "push_disabled" {
			t.Fatalf("push without a VAPID key must be 501 push_disabled, got %d %v", code, body)
		}
		if code, _ := c.do("GET", "/v0/push/subscriptions", nil, nil); code != http.StatusNotImplemented {
			t.Fatalf("listing without a key pair: %d", code)
		}
	})

	t.Run("permission", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/narrow", http.StatusCreated)
		c := client{t: t, base: httpSrv.URL, key: narrow.Key}
		code, body := c.do("POST", "/v0/push/subscriptions", map[string]any{
			"endpoint":   pushHTTP.URL + "/narrow",
			"keys":       map[string]any{"p256dh": dev.p256dh, "auth": base64.RawURLEncoding.EncodeToString(dev.auth)},
			"user_agent": "narrow",
		}, nil)
		if code != http.StatusForbidden || body["code"] != "missing_permission" {
			t.Fatalf("a key without notification_write must be refused, got %d %v", code, body)
		}
	})

	t.Run("subscribe upserts and stays inside the tenant", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/iphone", http.StatusCreated)
		first := subscribeDevice(t, a, dev, "/iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)")
		if first["push_subscription_id"] == nil || first["device"] != "iPhone" {
			t.Fatalf("subscribe: %v", first)
		}
		id := first["push_subscription_id"].(string)
		// A returning device re-subscribes on every fresh permission grant.
		again := subscribeDevice(t, a, dev, "/iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X)")
		if again["push_subscription_id"] != id {
			t.Fatalf("the same endpoint must upsert, got %v then %v", id, again["push_subscription_id"])
		}
		if n := countSubs(t, orgA.ID); n != 1 {
			t.Fatalf("org A has %d subscriptions, want 1", n)
		}

		code, list := a.do("GET", "/v0/push/subscriptions", nil, nil)
		if code != http.StatusOK || list["count"].(float64) != 1 {
			t.Fatalf("list: %d %v", code, list)
		}
		raw, _ := json.Marshal(list)
		for _, secret := range []string{"endpoint", "p256dh", "auth", pushHTTP.URL} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("the list must not leak %q: %s", secret, raw)
			}
		}

		// Another tenant sees nothing and cannot delete it.
		if code, other := b.do("GET", "/v0/push/subscriptions", nil, nil); code != http.StatusOK || other["count"].(float64) != 0 {
			t.Fatalf("org B must not see org A devices: %d %v", code, other)
		}
		if code, _ := b.do("DELETE", "/v0/push/subscriptions/"+id, nil, nil); code != http.StatusNotFound {
			t.Fatalf("org B deleting org A device: %d", code)
		}
		if !subExists(t, id) {
			t.Fatal("org A's subscription must survive org B's delete")
		}
		// Row level security, not just the handler.
		var visible int
		_ = d.WithOrg(ctx, orgB.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from push_subscriptions").Scan(&visible)
		})
		if visible != 0 {
			t.Fatalf("RLS leak: org B sees %d push subscriptions", visible)
		}
	})

	t.Run("a notification names the inbox and nothing else", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/lock", http.StatusCreated)
		subscribeDevice(t, a, dev, "/lock", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
		ingest(t, "push-1@ext.test", "Quote for 40 units")

		// Every device of the organization is notified, so the batch may carry
		// the devices the earlier subtests registered too.
		ps := sender(core.PushFailureLimit)
		if n, err := ps.RunOnce(ctx, 10); err != nil || n < 1 {
			t.Fatalf("send: %d %v", n, err)
		}
		dev.mu.Lock()
		hits, body, header := dev.hits, dev.body, dev.header
		dev.mu.Unlock()
		if hits != 1 {
			t.Fatalf("the device got %d requests, want 1", hits)
		}
		if enc := header.Get("Content-Encoding"); enc != "aes128gcm" {
			t.Fatalf("Content-Encoding = %q", enc)
		}
		if ttl := header.Get("TTL"); ttl != "43200" {
			t.Fatalf("TTL = %q", ttl)
		}
		if auth := header.Get("Authorization"); !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k=") {
			t.Fatalf("the request must be signed with VAPID, got %q", auth)
		}

		plain := decryptPush(t, body, dev)
		var payload struct {
			Title string         `json:"title"`
			Body  string         `json:"body"`
			Tag   string         `json:"tag"`
			URL   string         `json:"url"`
			Data  map[string]any `json:"data"`
		}
		if err := json.Unmarshal(plain, &payload); err != nil {
			t.Fatalf("payload is not JSON: %s", plain)
		}
		if !strings.Contains(payload.Title, inbox.InboxID) {
			t.Fatalf("the title must name the inbox, got %q", payload.Title)
		}
		if payload.Tag != "message.received" || !strings.HasPrefix(payload.URL, "/console/inbox/?") {
			t.Fatalf("payload = %s", plain)
		}
		// What must never reach a lock screen.
		for _, forbidden := range []string{"Quote for 40 units", "vendor@ext.test", "agreed price", "Subject"} {
			if strings.Contains(string(plain), forbidden) {
				t.Fatalf("payload carries mail content (%q): %s", forbidden, plain)
			}
		}
		if payload.Data["inbox"] != inbox.InboxID {
			t.Fatalf("data.inbox = %v", payload.Data["inbox"])
		}

		// The subscription is stamped as used and the job is done.
		var used bool
		var pending int
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, "select last_used_at is not null from push_subscriptions where endpoint = $1",
				pushHTTP.URL+"/lock").Scan(&used); err != nil {
				return err
			}
			return tx.QueryRow(ctx, "select count(*) from push_jobs where sent_at is null").Scan(&pending)
		}); err != nil {
			t.Fatal(err)
		}
		if !used || pending != 0 {
			t.Fatalf("last_used_at set = %v, pending jobs = %d", used, pending)
		}

		// Both calls are audited, and the audit names the device, not the endpoint.
		var audited int
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from audit_log where action = 'push.subscribe'").Scan(&audited)
		}); err != nil {
			t.Fatal(err)
		}
		if audited == 0 {
			t.Fatal("a subscription must be audited")
		}
	})

	t.Run("404 and 410 remove the device", func(t *testing.T) {
		gone := newDevice(t, pushSrv, "/gone", http.StatusGone)
		unknown := newDevice(t, pushSrv, "/unknown", http.StatusNotFound)
		subGone := subscribeDevice(t, a, gone, "/gone", "Mozilla/5.0 (Linux; Android 15)")
		subUnknown := subscribeDevice(t, a, unknown, "/unknown", "Mozilla/5.0 (Linux; Android 15)")
		ingest(t, "push-gone@ext.test", "Anything")
		if _, err := sender(core.PushFailureLimit).RunOnce(ctx, 10); err != nil {
			t.Fatal(err)
		}
		for name, id := range map[string]string{"410": subGone["push_subscription_id"].(string),
			"404": subUnknown["push_subscription_id"].(string)} {
			if subExists(t, id) {
				t.Fatalf("a device the push service reports %s must be removed at once", name)
			}
		}
	})

	t.Run("repeated failures retire the device", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/forbidden", http.StatusForbidden)
		sub := subscribeDevice(t, a, dev, "/forbidden", "Mozilla/5.0 (Windows NT 10.0)")
		id := sub["push_subscription_id"].(string)
		ps := sender(2) // two strikes, instead of waiting for the default five

		ingest(t, "push-403-a@ext.test", "First")
		if _, err := ps.RunOnce(ctx, 10); err != nil {
			t.Fatal(err)
		}
		if !subExists(t, id) {
			t.Fatal("one failure must not retire the device yet")
		}
		ingest(t, "push-403-b@ext.test", "Second")
		if _, err := ps.RunOnce(ctx, 10); err != nil {
			t.Fatal(err)
		}
		if subExists(t, id) {
			t.Fatal("the device must be retired at the failure limit")
		}
	})

	t.Run("an unreachable push service never fails the delivery", func(t *testing.T) {
		// A device whose push service is down: the URL answers nothing at all.
		dev := newDevice(t, pushSrv, "/unreachable", http.StatusCreated)
		dead := httptest.NewTLSServer(http.NotFoundHandler())
		deadURL := dead.URL
		dead.Close()
		code, body := a.do("POST", "/v0/push/subscriptions", map[string]any{
			"endpoint":        deadURL + "/device",
			"keys":            map[string]any{"p256dh": dev.p256dh, "auth": base64.RawURLEncoding.EncodeToString(dev.auth)},
			"expiration_time": nil,
			"user_agent":      "Mozilla/5.0 (Windows NT 10.0)",
		}, nil)
		if code != http.StatusCreated {
			t.Fatalf("subscribe a dead device: %d %v", code, body)
		}
		id := body["push_subscription_id"].(string)

		// The mail is stored and the event fires; nothing waits for the push
		// service, because ingest only writes rows.
		ingest(t, "push-dead@ext.test", "Delivery must survive")
		var stored int
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from messages where rfc_message_id = '<push-dead@ext.test>'").Scan(&stored)
		}); err != nil {
			t.Fatal(err)
		}
		if stored != 1 {
			t.Fatalf("the message must be stored (%d rows)", stored)
		}

		ps := sender(3)
		if _, err := ps.RunOnce(ctx, 10); err != nil {
			t.Fatalf("a failed push must not be an error of the worker: %v", err)
		}
		// The job stays for a retry and the device is not retired by one outage.
		var attempts int
		var sent bool
		var lastError string
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select attempts, sent_at is not null, last_error from push_jobs
				where subscription_id = $1 order by id desc limit 1`, id).Scan(&attempts, &sent, &lastError)
		}); err != nil {
			t.Fatal(err)
		}
		if sent || attempts != 1 {
			t.Fatalf("the job must wait for a retry (attempts %d, sent %v)", attempts, sent)
		}
		// The URL an http.Client quotes in its error is the endpoint, which is
		// a capability: it may not be written down.
		if lastError == "" {
			t.Fatal("the queue row must say why the send failed")
		}
		if strings.Contains(lastError, deadURL) || strings.Contains(lastError, "/device") {
			t.Fatalf("the queue row must not keep the endpoint: %q", lastError)
		}
		if !subExists(t, id) {
			t.Fatal("a temporary outage must not remove the device")
		}
	})

	t.Run("the rate limit keeps a burst to one banner", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/slow", http.StatusCreated)
		sub := subscribeDevice(t, a, dev, "/slow", "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X)")
		id := sub["push_subscription_id"].(string)
		// As if a notification went out a moment ago.
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "update push_subscriptions set last_used_at = now() where id = $1", id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		ingest(t, "push-slow@ext.test", "Burst")
		ps := &core.PushSender{Svc: svc, Log: log, Client: pushHTTP.Client(), MinInterval: time.Hour}
		if _, err := ps.RunOnce(ctx, 10); err != nil {
			t.Fatal(err)
		}
		dev.mu.Lock()
		hits := dev.hits
		dev.mu.Unlock()
		if hits != 0 {
			t.Fatalf("the device got %d notifications inside the rate window", hits)
		}
		var pending int
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from push_jobs where subscription_id = $1 and sent_at is null", id).Scan(&pending)
		}); err != nil {
			t.Fatal(err)
		}
		if pending != 1 {
			t.Fatalf("the deferred notification must stay queued, got %d", pending)
		}
	})

	t.Run("a burst of mail collapses into one notification", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/burst", http.StatusCreated)
		sub := subscribeDevice(t, a, dev, "/burst", "Mozilla/5.0 (iPhone)")
		id := sub["push_subscription_id"].(string)
		// Three messages to the same inbox before the queue is drained: one
		// pending job, carrying the newest thread.
		ingest(t, "push-burst-1@ext.test", "First")
		ingest(t, "push-burst-2@ext.test", "Second")
		ingest(t, "push-burst-3@ext.test", "Third")
		var jobs int
		var thread string
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `select count(*) from push_jobs where subscription_id = $1 and sent_at is null`, id).Scan(&jobs); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `select payload->>'url' from push_jobs where subscription_id = $1 and sent_at is null`, id).Scan(&thread)
		}); err != nil {
			t.Fatal(err)
		}
		if jobs != 1 {
			t.Fatalf("the burst queued %d notifications for one device", jobs)
		}
		var newest string
		if err := d.WithOrg(ctx, orgA.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select t.id::text from threads t join messages m on m.thread_id = t.id
				where m.rfc_message_id = '<push-burst-3@ext.test>'`).Scan(&newest)
		}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(thread, newest) {
			t.Fatalf("the pending notification must carry the newest thread (%s), got %s", newest, thread)
		}
		// Draining sends exactly one, not three.
		if _, err := sender(core.PushFailureLimit).RunOnce(ctx, 50); err != nil {
			t.Fatal(err)
		}
		dev.mu.Lock()
		hits := dev.hits
		dev.mu.Unlock()
		if hits != 1 {
			t.Fatalf("the device got %d notifications for a burst of 3", hits)
		}
	})

	t.Run("unsubscribe by id and by endpoint", func(t *testing.T) {
		one := newDevice(t, pushSrv, "/del-one", http.StatusCreated)
		sub := subscribeDevice(t, a, one, "/del-one", "Mozilla/5.0 (X11; Linux x86_64)")
		id := sub["push_subscription_id"].(string)
		if code, _ := a.do("DELETE", "/v0/push/subscriptions/"+id, nil, nil); code != http.StatusNoContent {
			t.Fatalf("delete by id: %d", code)
		}
		if code, _ := a.do("DELETE", "/v0/push/subscriptions/"+id, nil, nil); code != http.StatusNotFound {
			t.Fatalf("deleting a gone device must be 404, got %d", code)
		}

		two := newDevice(t, pushSrv, "/del-two", http.StatusCreated)
		subscribeDevice(t, a, two, "/del-two", "Mozilla/5.0 (X11; Linux x86_64)")
		if code, _ := a.do("DELETE", "/v0/push/subscriptions", map[string]any{"endpoint": pushHTTP.URL + "/del-two"}, nil); code != http.StatusNoContent {
			t.Fatalf("delete by endpoint: %d", code)
		}
		// Idempotent: the browser has already dropped it.
		if code, _ := a.do("DELETE", "/v0/push/subscriptions", map[string]any{"endpoint": pushHTTP.URL + "/never"}, nil); code != http.StatusNoContent {
			t.Fatalf("an unknown endpoint must still be 204, got %d", code)
		}
		if code, _ := a.do("DELETE", "/v0/push/subscriptions", map[string]any{}, nil); code != http.StatusBadRequest {
			t.Fatalf("no id and no endpoint must be 400, got %d", code)
		}
	})

	t.Run("a malformed subscription is refused", func(t *testing.T) {
		dev := newDevice(t, pushSrv, "/bad", http.StatusCreated)
		for name, body := range map[string]map[string]any{
			"plain http": {"endpoint": "http://push.example.test/x",
				"keys": map[string]any{"p256dh": dev.p256dh, "auth": base64.RawURLEncoding.EncodeToString(dev.auth)}},
			"short key": {"endpoint": pushHTTP.URL + "/bad",
				"keys": map[string]any{"p256dh": "AAAA", "auth": base64.RawURLEncoding.EncodeToString(dev.auth)}},
			"extra field": {"endpoint": pushHTTP.URL + "/bad", "unknown": true,
				"keys": map[string]any{"p256dh": dev.p256dh, "auth": base64.RawURLEncoding.EncodeToString(dev.auth)}},
		} {
			if code, out := a.do("POST", "/v0/push/subscriptions", body, nil); code != http.StatusBadRequest {
				t.Errorf("%s: got %d %v", name, code, out)
			}
		}
	})

	// The queue is a queue, not a log: a finished job does not stay forever.
	t.Run("finished jobs are purged", func(t *testing.T) {
		if err := d.WithSystem(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "update push_jobs set sent_at = now() - interval '30 days' where sent_at is not null")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PurgePushJobs(ctx); err != nil {
			t.Fatal(err)
		}
		var old int
		if err := d.WithSystem(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select count(*) from push_jobs where sent_at < now() - interval '7 days'").Scan(&old)
		}); err != nil {
			t.Fatal(err)
		}
		if old != 0 {
			t.Fatalf("%d finished jobs survived the retention sweep", old)
		}
	})
}
