package core

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// vapidPair generates a throwaway P-256 key pair for a test. Never a real one:
// the deployment's keys live in the secret store and are not in this repo.
func vapidPair(t *testing.T) (private, public string) {
	t.Helper()
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	return private, public
}

func getenv(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

// Without a key pair push is off, and the status says what is missing without
// ever carrying key material.
func TestLoadPushDisabled(t *testing.T) {
	push, st := LoadPush(getenv(nil))
	if push != nil || st.Enabled {
		t.Fatalf("no key must mean no push, got %v %v", push, st)
	}
	if len(st.Warnings) == 0 || !strings.Contains(strings.Join(st.Warnings, " "), EnvVAPIDKeys) {
		t.Fatalf("warnings must name the variable to set: %v", st.Warnings)
	}
}

func TestLoadPushFromPair(t *testing.T) {
	private, public := vapidPair(t)
	push, st := LoadPush(getenv(map[string]string{
		EnvVAPIDPrivateKey: private,
		EnvVAPIDPublicKey:  public,
		EnvVAPIDSubject:    "mailto:push@agents.test",
	}))
	if push == nil || !st.Enabled {
		t.Fatalf("a valid pair must enable push: %v", st.Warnings)
	}
	if push.PublicKey() != public {
		t.Fatal("the public key must be the one a browser subscribes with")
	}
	if push.privateKey != private || push.subject != "mailto:push@agents.test" {
		t.Fatal("the private key and the subject must be kept")
	}
	// The status is safe to print, log and return: no key material in it.
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), private) {
		t.Fatal("the status must never carry the private key")
	}
}

func TestLoadPushFromSingleVariable(t *testing.T) {
	private, public := vapidPair(t)
	doc, _ := json.Marshal(map[string]string{"public_key": public, "private_key": private, "subject": "push@agents.test"})
	push, st := LoadPush(getenv(map[string]string{EnvVAPIDKeys: string(doc)}))
	if push == nil || !st.Enabled {
		t.Fatalf("MAI_VAPID_KEYS must be enough: %v", st.Warnings)
	}
	if push.PublicKey() != public || push.subject != "push@agents.test" {
		t.Fatal("the single variable must carry both halves and the subject")
	}
}

// A public key that does not belong to the private key would make every
// notification fail at the push service, so the pair is checked at start up.
func TestLoadPushRejectsMismatchedPair(t *testing.T) {
	private, _ := vapidPair(t)
	_, otherPublic := vapidPair(t)
	push, st := LoadPush(getenv(map[string]string{
		EnvVAPIDPrivateKey: private,
		EnvVAPIDPublicKey:  otherPublic,
		EnvVAPIDSubject:    "mailto:push@agents.test",
	}))
	if push != nil || st.Enabled {
		t.Fatal("a mismatched pair must leave push off")
	}
	joined := strings.Join(st.Warnings, " ")
	if !strings.Contains(joined, "does not belong") {
		t.Fatalf("the warning must say why: %v", st.Warnings)
	}
	if strings.Contains(joined, private) || strings.Contains(joined, otherPublic) {
		t.Fatal("a warning must never repeat key material")
	}
}

func TestLoadPushRejectsJunk(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"truncated private": {EnvVAPIDPrivateKey: "nope", EnvVAPIDPublicKey: "nope", EnvVAPIDSubject: "a@b"},
		"missing subject":   {EnvVAPIDPrivateKey: "AA", EnvVAPIDPublicKey: "AA"},
		"broken json":       {EnvVAPIDKeys: "{not json"},
	} {
		if push, st := LoadPush(getenv(values)); push != nil || len(st.Warnings) == 0 {
			t.Errorf("%s: got push %v warnings %v", name, push, st.Warnings)
		}
	}
	// A subject is required: the push service needs a contact for this sender.
	private, public := vapidPair(t)
	if push, _ := LoadPush(getenv(map[string]string{EnvVAPIDPrivateKey: private, EnvVAPIDPublicKey: public})); push != nil {
		t.Error("push needs a VAPID subject")
	}
}

// The notification is read on a lock screen anyone can see. It names the inbox
// and links into the console, and carries nothing else about the mail.
func TestPushPayloadCarriesNoMailContent(t *testing.T) {
	raw, err := buildPushPayload("support@example.com", "0f8fad5b-d9cb-469f-a165-70867728950e", 42)
	if err != nil {
		t.Fatal(err)
	}
	var p pushPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.Title != "New mail in support@example.com" {
		t.Fatalf("title = %q", p.Title)
	}
	if !strings.Contains(p.Body, "Ticket #42") || !strings.Contains(p.Body, "console") {
		t.Fatalf("body = %q", p.Body)
	}
	if p.Tag != PushTag {
		t.Fatalf("tag = %q, want %q", p.Tag, PushTag)
	}
	if !strings.HasPrefix(p.URL, "/console/inbox/?") {
		t.Fatalf("url = %q", p.URL)
	}
	link, err := url.Parse(p.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := link.Query().Get("address"); got != "support@example.com" {
		t.Fatalf("the link must land on the inbox, got %q", got)
	}
	if got := link.Query().Get("thread"); got != "0f8fad5b-d9cb-469f-a165-70867728950e" {
		t.Fatalf("the link must land on the thread, got %q", got)
	}
	// Nothing that belongs to the message itself.
	for _, forbidden := range []string{"subject", "sender", "vendor@ext.test", "preview", "snippet", "text", "html"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("payload must not carry %q: %s", forbidden, raw)
		}
	}
	if len(raw) > pushPayloadLimit {
		t.Fatalf("payload is %d bytes", len(raw))
	}
}

// An address with a plus tag must survive the round trip through the query.
func TestPushPayloadEscapesTheAddress(t *testing.T) {
	raw, err := buildPushPayload("sales+eu@agents.test", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var p pushPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(p.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := link.Query().Get("address"); got != "sales+eu@agents.test" {
		t.Fatalf("address = %q", got)
	}
	if strings.Contains(p.Body, "Ticket") {
		t.Fatalf("a thread without a ticket must not invent one: %q", p.Body)
	}
}

func TestValidateSubscription(t *testing.T) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p256dh := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}

	good := PushSubscribeRequest{Endpoint: "https://web.push.apple.com/abc", UserAgent: "Mozilla/5.0 (iPhone)"}
	good.Keys.P256dh = p256dh
	good.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)
	endpoint, gotP256dh, gotAuth, err := validateSubscription(good)
	if err != nil {
		t.Fatalf("a subscription from a browser must pass: %v", err)
	}
	if endpoint != good.Endpoint || gotP256dh != p256dh || gotAuth != good.Keys.Auth {
		t.Fatal("the endpoint and keys must be stored as the browser gave them")
	}

	for name, mutate := range map[string]func(*PushSubscribeRequest){
		"plain http":      func(r *PushSubscribeRequest) { r.Endpoint = "http://web.push.apple.com/abc" },
		"no scheme":       func(r *PushSubscribeRequest) { r.Endpoint = "web.push.apple.com/abc" },
		"empty":           func(r *PushSubscribeRequest) { r.Endpoint = "" },
		"short p256dh":    func(r *PushSubscribeRequest) { r.Keys.P256dh = "AAAA" },
		"off curve":       func(r *PushSubscribeRequest) { r.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65)) },
		"short auth":      func(r *PushSubscribeRequest) { r.Keys.Auth = "AAAA" },
		"not base64url":   func(r *PushSubscribeRequest) { r.Keys.Auth = "!!!!" },
		"negative expiry": func(r *PushSubscribeRequest) { e := int64(-1); r.ExpirationTime = &e },
	} {
		req := good
		mutate(&req)
		if _, _, _, err := validateSubscription(req); err == nil {
			t.Errorf("%s: must be refused", name)
		} else if strings.Contains(err.Error(), good.Keys.Auth) || strings.Contains(err.Error(), p256dh) {
			t.Errorf("%s: the error must not repeat a key", name)
		}
	}
}

func TestPushOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		want   pushOutcome
	}{
		{"created", http.StatusCreated, nil, pushSent},
		{"no content", http.StatusNoContent, nil, pushSent},
		{"gone", http.StatusGone, nil, pushGone},
		{"unknown device", http.StatusNotFound, nil, pushGone},
		{"too many", http.StatusTooManyRequests, nil, pushRetry},
		{"server error", http.StatusInternalServerError, nil, pushRetry},
		{"bad request", http.StatusBadRequest, nil, pushPermanent},
		{"forbidden", http.StatusForbidden, nil, pushPermanent},
		{"payload too large", http.StatusRequestEntityTooLarge, nil, pushPermanent},
		{"unreachable", 0, errUnreachable{}, pushRetry},
	}
	for _, c := range cases {
		if got := pushOutcomeFor(c.status, c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// A transport error quotes the URL it could not reach. That URL is the
// endpoint, so it never reaches the queue table or a log line.
func TestPushErrorReasonHidesTheEndpoint(t *testing.T) {
	endpoint := "https://web.push.apple.com/QG-capability"
	err := errors.New(`Post "` + endpoint + `": dial tcp 1.2.3.4:443: connect: connection refused`)
	got := pushErrorReason(err, endpoint)
	if strings.Contains(got, "web.push.apple.com") || strings.Contains(got, "QG-capability") {
		t.Fatalf("the endpoint survived: %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Fatalf("the reason must still say what happened: %q", got)
	}
	if long := pushErrorReason(errors.New(strings.Repeat("x", 1000)), endpoint); len(long) > 300 {
		t.Fatalf("the reason must be bounded, got %d bytes", len(long))
	}
}

type errUnreachable struct{}

func (errUnreachable) Error() string { return "dial tcp: connection refused" }

func TestPushBackoffGrows(t *testing.T) {
	last := pushBackoff(1)
	for attempt := 2; attempt <= PushMaxAttempts; attempt++ {
		next := pushBackoff(attempt)
		if next < last {
			t.Fatalf("attempt %d waits %v, less than the previous %v", attempt, next, last)
		}
		last = next
	}
}

func TestPushDeviceLabel(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)": "iPhone",
		"Mozilla/5.0 (Linux; Android 15; Pixel 9)":               "Android",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)":        "Mac",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64)":              "Windows",
		"": "Unknown device",
	}
	for agent, want := range cases {
		if got := pushDevice(agent); got != want {
			t.Errorf("%q: got %q want %q", agent, got, want)
		}
	}
}

// notification_write is the permission that wakes a device; a member holds it,
// a scoped key only when it is listed.
func TestNotificationWritePermission(t *testing.T) {
	if !slices.Contains(AllPermissions, PermNotificationWrite) {
		t.Fatal("notification_write must be in AllPermissions")
	}
	if !slices.Contains(GrantablePermissions, PermNotificationWrite) {
		t.Fatal("notification_write must be grantable")
	}
	if !slices.Contains(PermissionsForRole(RoleMember), PermNotificationWrite) {
		t.Fatal("a member must be able to register a device")
	}
	if !slices.Contains(PermissionsForRole(RoleAdmin), PermNotificationWrite) {
		t.Fatal("an admin must be able to register a device")
	}
	if !(&Principal{}).Can(PermNotificationWrite) {
		t.Fatal("a full access key must hold notification_write")
	}
	scoped := &Principal{Permissions: []string{PermMessageRead}}
	if scoped.Can(PermNotificationWrite) {
		t.Fatal("a scoped key must not hold a permission it does not list")
	}
	// It is not an explicit-only permission: it reveals nothing about the mail.
	if slices.Contains(ExplicitPermissions, PermNotificationWrite) {
		t.Fatal("notification_write must not be explicit-only")
	}
}

// The operator command generates the pair with crypto/ecdh; the loader must
// accept it, and so must the library that signs the request.
func TestGeneratedPairIsAccepted(t *testing.T) {
	public, private, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	push, st := LoadPush(getenv(map[string]string{
		EnvVAPIDKeys:    `{"public_key":"` + public + `","private_key":"` + private + `"}`,
		EnvVAPIDSubject: "mailto:ops@agents.test",
	}))
	if push == nil {
		t.Fatalf("a generated pair must be accepted: %v", st.Warnings)
	}
	// The same encoding the third party generator produces, so a key made by
	// either tool works.
	libPrivate, libPublic := vapidPair(t)
	if _, err := decodeVAPIDKey(libPrivate); err != nil || len(libPrivate) == 0 {
		t.Fatalf("the library's private key must decode: %v", err)
	}
	if _, err := decodeVAPIDKey(libPublic); err != nil {
		t.Fatalf("the library's public key must decode: %v", err)
	}
}

// Guard the private key against an accidental re-export: the only place a
// browser sees it is the VAPID signature the push service verifies.
func TestPrivateKeyIsNotExported(t *testing.T) {
	private, public := vapidPair(t)
	push, _ := LoadPush(getenv(map[string]string{
		EnvVAPIDPrivateKey: private,
		EnvVAPIDPublicKey:  public,
		EnvVAPIDSubject:    "mailto:push@agents.test",
	}))
	if push == nil {
		t.Fatal("push must be on")
	}
	raw, err := json.Marshal(push)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), private) {
		t.Fatal("marshalling the config must not expose the private key")
	}
}
