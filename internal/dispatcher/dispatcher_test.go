package dispatcher

import (
	"testing"
	"time"
)

// Test vector from the Standard Webhooks / Svix documentation.
func TestSignMatchesStandardWebhooksVector(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	sig, err := Sign(secret, "msg_p5jXN8AQM9LWM0D4loKWxJek", "1614265330", []byte(`{"test": 2432232314}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="; sig != want {
		t.Fatalf("signature = %s, want %s", sig, want)
	}
}

func TestVerifyRejectsOldTimestampAndTamperedBody(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	body := []byte(`{"ok":true}`)
	now := time.Unix(1_700_000_000, 0)
	ts := "1700000000"
	sig, _ := Sign(secret, "msg_1", ts, body)
	if !Verify(secret, "msg_1", ts, "v1,other "+sig, body, now) {
		t.Fatal("valid signature rejected")
	}
	if Verify(secret, "msg_1", ts, sig, []byte(`{"ok":false}`), now) {
		t.Fatal("tampered body accepted")
	}
	if Verify(secret, "msg_1", ts, sig, body, now.Add(6*time.Minute)) {
		t.Fatal("stale timestamp accepted")
	}
}
