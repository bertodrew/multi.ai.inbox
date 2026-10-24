package core

// Web push: the server half of the notification card in the console.
//
// The design is web/docs/push-design.md. A browser that grants notification
// permission gets an endpoint and two keys from its push service; the console
// posts them here and this file stores them, one row per organization and
// endpoint. On message.received the ingest transaction queues one job per
// subscription (no HTTP inside that transaction, so a push can never fail a
// delivery) and a worker sends it through the push service.
//
// What a notification may say is a privacy rule, not a formatting choice: the
// payload is rendered on a lock screen anyone can see, so it carries the inbox
// address and a link into the console, and never the sender, the subject or a
// snippet of the body. The keys and the endpoint never leave the database.
//
// VAPID keys come from the environment (MAI_VAPID_KEYS, or
// MAI_VAPID_PRIVATE_KEY and MAI_VAPID_PUBLIC_KEY). They are read once at start
// up, never printed and never returned by any endpoint: without them push is
// off and every route answers 501 push_disabled, exactly like billing.

import (
	"context"
	"crypto/ecdh"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Environment variables of the VAPID key pair. MAI_VAPID_KEYS carries both
// halves in one JSON document, which is convenient for a secret store that
// holds one value per service.
const (
	EnvVAPIDPrivateKey = "MAI_VAPID_PRIVATE_KEY"
	EnvVAPIDPublicKey  = "MAI_VAPID_PUBLIC_KEY"
	EnvVAPIDKeys       = "MAI_VAPID_KEYS"
	EnvVAPIDSubject    = "MAI_VAPID_SUBJECT"
)

// Limits of the notification path.
const (
	// PushMaxSubscriptions bounds the devices one organization may register, so
	// a key cannot turn the platform into a fan out machine.
	PushMaxSubscriptions = 50
	// PushMaxAttempts is how often a temporary push failure is retried before
	// the job is dropped.
	PushMaxAttempts = 5
	// PushFailureLimit is how many consecutive permanent failures retire a
	// subscription. A device that was uninstalled answers 404 or 410 and is
	// removed at once; this covers the services that keep saying 403.
	PushFailureLimit = 5
	// PushMinInterval is the shortest gap between two notifications to the same
	// device. It is the rate limit: a burst of mail still shows one banner.
	PushMinInterval = 30 * time.Second
	// PushTTL is how long the push service keeps a notification for a device
	// that is offline, in seconds (12 hours).
	PushTTL = 12 * 60 * 60
	// PushJobRetention is how long a finished job is kept for diagnosis.
	PushJobRetention = 7 * 24 * time.Hour
	// pushPayloadLimit stays under the 4 KB Apple accepts, with room for the
	// encryption header.
	pushPayloadLimit = 3500
	maxPushEndpoint  = 2048
	maxPushAgent     = 512
)

// Push is the VAPID configuration. Both keys are unexported on purpose: no
// handler, no encoder and no log line can reach them by accident.
type Push struct {
	publicKey  string
	privateKey string
	// subject is the "sub" claim of the VAPID token: an https URL or an email
	// address for the push service to contact if this deployment misbehaves.
	subject string
}

// PublicKey is the half a browser needs to subscribe. Safe to publish.
func (p *Push) PublicKey() string { return p.publicKey }

// PushStatus is the resolved push configuration, safe to print, log and return
// from a CLI. It never carries key material.
type PushStatus struct {
	Enabled   bool     `json:"enabled"`
	PublicKey string   `json:"public_key,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Warnings  []string `json:"warnings"`
}

// LoadPush resolves the VAPID key pair from the environment. Push is on only
// when a usable pair is present: the private key must be a P-256 scalar and the
// public key the matching uncompressed point. Anything else returns nil, so the
// routes stay 501 instead of sending notifications nobody can verify.
//
// getenv is injected (os.Getenv in main) so this is testable offline and so a
// test can generate a throwaway pair without touching the deployment's keys.
func LoadPush(getenv func(string) string) (*Push, PushStatus) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	st := PushStatus{Warnings: []string{}}
	public := strings.TrimSpace(getenv(EnvVAPIDPublicKey))
	private := strings.TrimSpace(getenv(EnvVAPIDPrivateKey))
	subject := strings.TrimSpace(getenv(EnvVAPIDSubject))

	if raw := strings.TrimSpace(getenv(EnvVAPIDKeys)); raw != "" {
		var doc struct {
			PublicKey  string `json:"public_key"`
			PrivateKey string `json:"private_key"`
			Subject    string `json:"subject"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			st.Warnings = append(st.Warnings, EnvVAPIDKeys+" is not a JSON object with public_key and private_key")
			return nil, st
		}
		public, private = strings.TrimSpace(doc.PublicKey), strings.TrimSpace(doc.PrivateKey)
		if subject == "" {
			subject = strings.TrimSpace(doc.Subject)
		}
	}

	switch {
	case public == "" && private == "":
		st.Warnings = append(st.Warnings, EnvVAPIDKeys+" is not set (and neither is "+EnvVAPIDPrivateKey+"), so push is off")
		return nil, st
	case private == "":
		st.Warnings = append(st.Warnings, EnvVAPIDPrivateKey+" is not set, so push is off")
		return nil, st
	case public == "":
		st.Warnings = append(st.Warnings, EnvVAPIDPublicKey+" is not set, so no browser can subscribe")
		return nil, st
	}

	scalar, err := decodeVAPIDKey(private)
	if err != nil || len(scalar) != 32 {
		st.Warnings = append(st.Warnings, EnvVAPIDPrivateKey+" is not a base64url P-256 private key")
		return nil, st
	}
	point, err := decodeVAPIDKey(public)
	if err != nil || len(point) != 65 || point[0] != 4 {
		st.Warnings = append(st.Warnings, EnvVAPIDPublicKey+" is not an uncompressed P-256 public key in base64url")
		return nil, st
	}
	curve := elliptic.P256()
	x, y := curve.ScalarBaseMult(scalar)
	derived := elliptic.Marshal(curve, x, y)
	if !bytesEqual(derived, point) {
		// A mismatch means every notification would be rejected by the service.
		st.Warnings = append(st.Warnings, EnvVAPIDPublicKey+" does not belong to "+EnvVAPIDPrivateKey)
		return nil, st
	}
	if subject == "" {
		st.Warnings = append(st.Warnings, EnvVAPIDSubject+" is not set, so push is off until it names a contact")
		return nil, st
	}
	if !strings.HasPrefix(subject, "https://") && !strings.Contains(subject, "@") {
		st.Warnings = append(st.Warnings, EnvVAPIDSubject+" must be an https URL or an email address")
		return nil, st
	}

	st.Enabled = true
	st.PublicKey = public
	st.Subject = subject
	return &Push{publicKey: public, privateKey: private, subject: subject}, st
}

// decodeVAPIDKey accepts padded and unpadded base64url, which is what the two
// encoders in the wild produce.
func decodeVAPIDKey(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if s == "" {
		return nil, errors.New("empty key")
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// GenerateVAPIDKeys returns a fresh pair in the encoding MAI_VAPID_KEYS uses:
// the public half is an uncompressed P-256 point, the private half the 32 byte
// scalar. For the operator once (mai admin vapid-keygen) and for tests. A
// rotation invalidates every subscription already registered.
func GenerateVAPIDKeys() (publicKey, privateKey string, err error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	public := key.PublicKey().Bytes()
	return base64.RawURLEncoding.EncodeToString(public), base64.RawURLEncoding.EncodeToString(key.Bytes()), nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// push returns the VAPID configuration or the error every route answers with
// when the deployment has no key pair.
func (s *Service) push() (*Push, error) {
	if s.Push == nil {
		return nil, errf(http.StatusNotImplemented, "push_disabled", "web push is not configured on this deployment")
	}
	return s.Push, nil
}

// PushSubscription is what a client may see of a subscription: never the
// endpoint, never the keys.
type PushSubscription struct {
	ID         string     `json:"push_subscription_id"`
	Device     string     `json:"device"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// PushSubscribeRequest is the body of POST /v0/push/subscriptions, exactly the
// shape the console sends (PushSubscription.toJSON plus the user agent).
type PushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	ExpirationTime *int64 `json:"expiration_time"`
	UserAgent      string `json:"user_agent"`
}

// SubscribePush stores the subscription of a device, or updates the row of an
// endpoint this organization already knows: a returning device re-subscribes on
// every fresh permission grant and must not accumulate rows.
func (s *Service) SubscribePush(ctx context.Context, p *Principal, req PushSubscribeRequest) (*PushSubscription, error) {
	if _, err := s.push(); err != nil {
		return nil, err
	}
	if err := p.Require(PermNotificationWrite); err != nil {
		return nil, err
	}
	endpoint, p256dh, auth, err := validateSubscription(req)
	if err != nil {
		return nil, err
	}
	agent := strings.TrimSpace(req.UserAgent)
	if len(agent) > maxPushAgent {
		agent = agent[:maxPushAgent]
	}
	// One owner: a session is a person, an API key is an agent.
	var user, key any
	if p.UserID != "" {
		user = p.UserID
	} else {
		key = p.KeyID
	}
	out := &PushSubscription{UserAgent: agent, Device: pushDevice(agent)}
	err = s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var known bool
		if err := tx.QueryRow(ctx, "select exists(select 1 from push_subscriptions where endpoint = $1)", endpoint).Scan(&known); err != nil {
			return err
		}
		if !known {
			var count int
			if err := tx.QueryRow(ctx, "select count(*) from push_subscriptions").Scan(&count); err != nil {
				return err
			}
			if count >= PushMaxSubscriptions {
				return errf(http.StatusConflict, "too_many_subscriptions",
					"this organization already has %d devices subscribed; turn one off first", PushMaxSubscriptions)
			}
		}
		return tx.QueryRow(ctx, `insert into push_subscriptions (org_id, user_id, api_key_id, endpoint, p256dh, auth, user_agent)
			values ($1, $2, $3, $4, $5, $6, $7)
			on conflict (org_id, endpoint) do update set
				user_id = excluded.user_id, api_key_id = excluded.api_key_id,
				p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent,
				failure_count = 0
			returning id, created_at`, p.Org.ID, user, key, endpoint, p256dh, auth, agent).Scan(&out.ID, &out.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// validateSubscription checks the shape of what the browser sent and returns
// the endpoint with the two keys in canonical unpadded base64url. The keys are
// capability material: they are validated, stored and never written anywhere
// else, and no error here repeats them.
func validateSubscription(req PushSubscribeRequest) (endpoint, p256dh, auth string, err error) {
	endpoint = strings.TrimSpace(req.Endpoint)
	if endpoint == "" {
		return "", "", "", Validation("endpoint is required")
	}
	if len(endpoint) > maxPushEndpoint {
		return "", "", "", Validation("endpoint is longer than %d characters", maxPushEndpoint)
	}
	u, perr := url.Parse(endpoint)
	if perr != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", "", Validation("endpoint must be an absolute https URL")
	}
	if req.ExpirationTime != nil && *req.ExpirationTime < 0 {
		return "", "", "", Validation("expiration_time must be milliseconds since the epoch, or null")
	}
	point, derr := decodeVAPIDKey(req.Keys.P256dh)
	if derr != nil || len(point) != 65 || point[0] != 4 {
		return "", "", "", Validation("keys.p256dh must be an uncompressed P-256 public key in base64url")
	}
	if x, y := elliptic.Unmarshal(elliptic.P256(), point); x == nil || y == nil {
		return "", "", "", Validation("keys.p256dh is not a point on P-256")
	}
	secret, aerr := decodeVAPIDKey(req.Keys.Auth)
	if aerr != nil || len(secret) != 16 {
		return "", "", "", Validation("keys.auth must be 16 bytes in base64url")
	}
	return endpoint, base64.RawURLEncoding.EncodeToString(point), base64.RawURLEncoding.EncodeToString(secret), nil
}

// pushDevice names the device for the console from the user agent, without
// claiming to identify it.
func pushDevice(agent string) string {
	switch {
	case agent == "":
		return "Unknown device"
	case strings.Contains(agent, "iPhone"):
		return "iPhone"
	case strings.Contains(agent, "iPad"):
		return "iPad"
	case strings.Contains(agent, "Android"):
		return "Android"
	case strings.Contains(agent, "Macintosh"), strings.Contains(agent, "Mac OS X"):
		return "Mac"
	case strings.Contains(agent, "Windows"):
		return "Windows"
	case strings.Contains(agent, "Linux"):
		return "Linux"
	default:
		return "Browser"
	}
}

// UnsubscribePush removes a subscription by id, or by endpoint for the client
// that only knows the endpoint it is dropping. Removing an endpoint this
// organization does not have is not an error: the caller is unsubscribing, and
// the browser has already dropped the subscription either way.
func (s *Service) UnsubscribePush(ctx context.Context, p *Principal, id, endpoint string) error {
	if _, err := s.push(); err != nil {
		return err
	}
	if err := p.Require(PermNotificationWrite); err != nil {
		return err
	}
	id, endpoint = strings.TrimSpace(id), strings.TrimSpace(endpoint)
	if id == "" && endpoint == "" {
		return Validation("pass the subscription id in the path, or endpoint in the body or the query")
	}
	return s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		var err error
		var tag pgconn.CommandTag
		switch {
		case id != "":
			tag, err = tx.Exec(ctx, "delete from push_subscriptions where id::text = $1", id)
		default:
			tag, err = tx.Exec(ctx, "delete from push_subscriptions where endpoint = $1", endpoint)
		}
		if err != nil {
			return err
		}
		if id != "" && tag.RowsAffected() == 0 {
			return NotFound("push subscription")
		}
		return nil
	})
}

// ListPushSubscriptions returns the devices the calling key or person
// registered, never their endpoints or keys.
func (s *Service) ListPushSubscriptions(ctx context.Context, p *Principal) ([]*PushSubscription, error) {
	if _, err := s.push(); err != nil {
		return nil, err
	}
	if err := p.Require(PermNotificationWrite); err != nil {
		return nil, err
	}
	owner, value := "api_key_id", p.KeyID
	if p.UserID != "" {
		owner, value = "user_id", p.UserID
	}
	list := []*PushSubscription{}
	err := s.DB.WithOrg(ctx, p.Org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id, user_agent, created_at, last_used_at from push_subscriptions
			where `+owner+` = $1 order by created_at desc`, value)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sub := &PushSubscription{}
			if err := rows.Scan(&sub.ID, &sub.UserAgent, &sub.CreatedAt, &sub.LastUsedAt); err != nil {
				return err
			}
			sub.Device = pushDevice(sub.UserAgent)
			list = append(list, sub)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// pushPayload is what the service worker renders on the lock screen: the inbox
// address, a console link, a tag that collapses repeats, and the thread so the
// click lands on the conversation. No sender, no subject, no body, no snippet.
type pushPayload struct {
	Title string          `json:"title"`
	Body  string          `json:"body"`
	Tag   string          `json:"tag"`
	URL   string          `json:"url"`
	Data  pushPayloadData `json:"data"`
}

type pushPayloadData struct {
	Kind         string `json:"kind"`
	Inbox        string `json:"inbox"`
	ThreadID     string `json:"thread_id,omitempty"`
	TicketNumber int64  `json:"ticket_number,omitempty"`
}

// PushTag collapses repeat notifications for the same event type, as the
// design asks; the service worker uses it as the notification tag.
const PushTag = "message.received"

// buildPushPayload renders the notification body. inbox is the organization's
// own address, threadID is a platform uuid and ticket is the support number
// when the thread has one: none of them is mail content.
func buildPushPayload(inbox, threadID string, ticket int64) ([]byte, error) {
	body := "Open the console to read it."
	if ticket > 0 {
		body = fmt.Sprintf("Ticket #%d. Open the console to read it.", ticket)
	}
	q := url.Values{}
	q.Set("address", inbox)
	if threadID != "" {
		q.Set("thread", threadID)
	}
	p := pushPayload{
		Title: "New mail in " + inbox,
		Body:  body,
		Tag:   PushTag,
		URL:   "/console/inbox/?" + q.Encode(),
		Data:  pushPayloadData{Kind: PushTag, Inbox: inbox, ThreadID: threadID, TicketNumber: ticket},
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(raw) > pushPayloadLimit {
		return nil, fmt.Errorf("push payload is %d bytes, over the %d byte limit", len(raw), pushPayloadLimit)
	}
	return raw, nil
}

// enqueuePush queues one notification per subscription of the organization, in
// the transaction that is already storing the message: no HTTP call happens
// here, so the delivery of mail never waits for a push service and never fails
// because of one. The work is wrapped in a savepoint on purpose: if the queue
// cannot be written the message is still stored, which is the right trade.
//
// A job that is still pending for the same subscription and inbox is updated
// instead of duplicated, so a burst of mail collapses into one notification.
func (s *Service) enqueuePush(ctx context.Context, tx pgx.Tx, orgID, inbox, threadID string) {
	if s.Push == nil {
		return
	}
	payload, err := buildPushPayload(inbox, threadID, s.threadTicketNumber(ctx, tx, threadID))
	if err != nil {
		s.log().Warn("push payload skipped", "err", err)
		return
	}
	sp, err := tx.Begin(ctx) // savepoint: a push problem must not roll back the message
	if err != nil {
		s.log().Warn("push queue unavailable", "err", err)
		return
	}
	_, err = sp.Exec(ctx, `insert into push_jobs (org_id, subscription_id, inbox_address, payload)
		select $1, s.id, $2, $3 from push_subscriptions s where s.failure_count < $4
		on conflict (subscription_id, inbox_address) where sent_at is null
		do update set payload = excluded.payload, next_attempt_at = now(), created_at = now()`,
		orgID, inbox, payload, PushFailureLimit)
	if err != nil {
		_ = sp.Rollback(ctx)
		s.log().Warn("push not queued", "err", err)
		return
	}
	if err := sp.Commit(ctx); err != nil {
		s.log().Warn("push not queued", "err", err)
	}
}

// threadTicketNumber returns the support number of a thread, or 0 when it has
// no ticket.
func (s *Service) threadTicketNumber(ctx context.Context, tx pgx.Tx, threadID string) int64 {
	if threadID == "" {
		return 0
	}
	var number int64
	if err := tx.QueryRow(ctx, "select coalesce((select number from tickets where thread_id = $1), 0)", threadID).Scan(&number); err != nil {
		return 0
	}
	return number
}

// PushJob is one claimed delivery.
type PushJob struct {
	ID           int64
	Subscription string
	Inbox        string
	Payload      []byte
	Attempts     int
	// Subscription state, for the rate limit and the failure count.
	Endpoint string
	P256dh   string
	Auth     string
	LastUsed *time.Time
}

// pushOutcome is what the sender must do with a claimed job.
type pushOutcome int

const (
	pushSent      pushOutcome = iota // 2xx
	pushGone                         // 404 or 410: the device is gone
	pushPermanent                    // 4xx that will never succeed
	pushRetry                        // 429, 5xx, or no answer
	pushDeferred                     // too soon after the previous notification
)

// pushOutcomeFor classifies a push service answer.
func pushOutcomeFor(status int, err error) pushOutcome {
	switch {
	case err != nil:
		return pushRetry
	case status >= 200 && status < 300:
		return pushSent
	case status == http.StatusNotFound || status == http.StatusGone:
		return pushGone
	case status == http.StatusTooManyRequests || status >= 500:
		return pushRetry
	default:
		return pushPermanent
	}
}

// pushBackoff is the delay before the next attempt.
func pushBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 5 * time.Minute
	case attempts == 3:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

// PushSender delivers queued notifications in batches. It is a separate role
// from the API so a slow push service cannot occupy a request worker, and it
// runs on its own connection to the queue.
type PushSender struct {
	Svc    *Service
	Log    *slog.Logger
	Client *http.Client

	// MaxAttempts, FailureLimit and MinInterval default to the constants above;
	// tests lower them instead of waiting for minutes.
	MaxAttempts  int
	FailureLimit int
	MinInterval  time.Duration
}

func (p *PushSender) logger() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return p.Svc.log()
}

func (p *PushSender) maxAttempts() int {
	if p.MaxAttempts > 0 {
		return p.MaxAttempts
	}
	return PushMaxAttempts
}

func (p *PushSender) failureLimit() int {
	if p.FailureLimit > 0 {
		return p.FailureLimit
	}
	return PushFailureLimit
}

func (p *PushSender) minInterval() time.Duration {
	if p.MinInterval > 0 {
		return p.MinInterval
	}
	return PushMinInterval
}

// Run drains the queue until the context ends.
func (p *PushSender) Run(ctx context.Context) {
	for {
		n, err := p.RunOnce(ctx, 20)
		if err != nil {
			p.logger().Error("push batch failed", "err", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// RunOnce claims a batch, sends it and records each result. It returns how many
// jobs it handled.
func (p *PushSender) RunOnce(ctx context.Context, limit int) (int, error) {
	if p.Svc.Push == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	jobs, err := p.Svc.claimPushJobs(ctx, limit)
	if err != nil {
		return 0, err
	}
	handled := 0
	for _, job := range jobs {
		handled++
		if job.Endpoint == "" {
			// The subscription was removed while the job waited: nothing to send.
			if err := p.Svc.donePushJob(ctx, job.ID, "subscription removed"); err != nil {
				return handled, err
			}
			continue
		}
		if job.LastUsed != nil {
			if wait := p.minInterval() - time.Since(*job.LastUsed); wait > 0 {
				// Rate limited: keep the job, send it once the window opens.
				if err := p.Svc.deferPushJob(ctx, job.ID, time.Now().Add(wait)); err != nil {
					return handled, err
				}
				continue
			}
		}
		status, sendErr := p.send(ctx, job)
		outcome := pushOutcomeFor(status, sendErr)
		if outcome == pushRetry && job.Attempts >= p.maxAttempts() {
			outcome = pushPermanent
		}
		reason := fmt.Sprintf("push service answered %d", status)
		if sendErr != nil {
			reason = pushErrorReason(sendErr, job.Endpoint)
		}
		switch outcome {
		case pushSent:
			if err := p.Svc.finishPushJob(ctx, job.ID, job.Subscription); err != nil {
				return handled, err
			}
		case pushGone:
			p.logger().Info("push subscription removed", "reason", "the push service reported it gone", "status", status)
			if err := p.Svc.removePushSubscription(ctx, job.Subscription); err != nil {
				return handled, err
			}
		case pushPermanent:
			p.logger().Warn("push delivery failed", "status", status)
			if err := p.Svc.failPushJob(ctx, job.ID, job.Subscription, p.failureLimit(), reason); err != nil {
				return handled, err
			}
		case pushRetry:
			if err := p.Svc.retryPushJob(ctx, job.ID, time.Now().Add(pushBackoff(job.Attempts)), reason); err != nil {
				return handled, err
			}
		}
	}
	return handled, nil
}

// pushErrorReason turns a transport error into a line fit for the queue table.
// An http.Client error quotes the URL it could not reach, and that URL is the
// endpoint, a capability nobody may read back: it is replaced here, and the
// result is bounded so a long message cannot bloat the row.
func pushErrorReason(err error, endpoint string) string {
	msg := err.Error()
	if endpoint != "" {
		msg = strings.ReplaceAll(msg, endpoint, "(endpoint)")
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// send posts one notification through the push service. The endpoint is never
// logged and never returned: only the status code leaves this function.
func (p *PushSender) send(ctx context.Context, job PushJob) (int, error) {
	sub := &webpush.Subscription{Endpoint: job.Endpoint, Keys: webpush.Keys{P256dh: job.P256dh, Auth: job.Auth}}
	resp, err := webpush.SendNotificationWithContext(ctx, job.Payload, sub, &webpush.Options{
		HTTPClient:      p.Client,
		Subscriber:      p.Svc.Push.subject,
		TTL:             PushTTL,
		Urgency:         webpush.UrgencyNormal,
		VAPIDPublicKey:  p.Svc.Push.publicKey,
		VAPIDPrivateKey: p.Svc.Push.privateKey,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// claimPushJobs leases a batch of due jobs and loads the subscription each one
// belongs to. Attempts are counted at claim time so a worker that dies mid
// flight does not retry forever.
func (s *Service) claimPushJobs(ctx context.Context, limit int) ([]PushJob, error) {
	var jobs []PushJob
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `with ready as (
				select id from push_jobs where sent_at is null and next_attempt_at <= now()
				order by id limit $1 for update skip locked
			)
			update push_jobs j set attempts = j.attempts + 1, next_attempt_at = now() + interval '5 minutes'
			from ready where j.id = ready.id
			returning j.id, j.subscription_id, j.inbox_address, j.payload, j.attempts`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			job := PushJob{}
			if err := rows.Scan(&job.ID, &job.Subscription, &job.Inbox, &job.Payload, &job.Attempts); err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range jobs {
			err := tx.QueryRow(ctx, `select coalesce(endpoint, ''), coalesce(p256dh, ''), coalesce(auth, ''), last_used_at
				from push_subscriptions where id = $1`, jobs[i].Subscription).
				Scan(&jobs[i].Endpoint, &jobs[i].P256dh, &jobs[i].Auth, &jobs[i].LastUsed)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // removed while queued; the job is finished without a send
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	return jobs, err
}

// finishPushJob records a delivered notification and clears the failure count.
func (s *Service) finishPushJob(ctx context.Context, jobID int64, subscription string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "update push_jobs set sent_at = now(), last_error = '' where id = $1", jobID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "update push_subscriptions set last_used_at = now(), failure_count = 0 where id = $1", subscription)
		return err
	})
}

func (s *Service) donePushJob(ctx context.Context, jobID int64, reason string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update push_jobs set sent_at = now(), last_error = $2 where id = $1", jobID, reason)
		return err
	})
}

func (s *Service) deferPushJob(ctx context.Context, jobID int64, at time.Time) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		// A deferral is not an attempt: the job was never sent.
		_, err := tx.Exec(ctx, `update push_jobs set next_attempt_at = $2, attempts = greatest(attempts - 1, 0) where id = $1`, jobID, at)
		return err
	})
}

// failPushJob drops the job and counts the failure against the subscription,
// retiring it once it reaches the limit.
func (s *Service) failPushJob(ctx context.Context, jobID int64, subscription string, limit int, reason string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "update push_jobs set sent_at = now(), last_error = $2 where id = $1", jobID, reason); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "update push_subscriptions set failure_count = failure_count + 1 where id = $1", subscription); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "delete from push_subscriptions where id = $1 and failure_count >= $2", subscription, limit)
		return err
	})
}

// removePushSubscription drops a device the push service no longer knows. The
// queued jobs go with it (on delete cascade).
func (s *Service) removePushSubscription(ctx context.Context, subscription string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "delete from push_subscriptions where id = $1", subscription)
		return err
	})
}

func (s *Service) retryPushJob(ctx context.Context, jobID int64, at time.Time, reason string) error {
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update push_jobs set next_attempt_at = $2, last_error = $3 where id = $1", jobID, at, reason)
		return err
	})
}

// PurgePushJobs drops finished jobs past the retention window, so the queue
// table does not grow forever.
func (s *Service) PurgePushJobs(ctx context.Context) (int64, error) {
	var n int64
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "delete from push_jobs where sent_at is not null and sent_at < now() - $1::interval",
			PushJobRetention.String())
		n = tag.RowsAffected()
		return err
	})
	return n, err
}
