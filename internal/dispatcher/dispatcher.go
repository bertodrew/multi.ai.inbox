// Package dispatcher delivers events to customer webhooks, signed with the
// Standard Webhooks scheme. It sends both webhook-* and svix-* headers so
// Svix verification libraries work unchanged.
package dispatcher

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

type Dispatcher struct {
	Svc    *core.Service
	Log    *slog.Logger
	Client *http.Client
}

// SafeClient refuses connections to private, loopback and link local
// addresses so webhook URLs cannot reach internal services (SSRF).
func SafeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("webhook target %s is not a public address", host)
		}
		return nil
	}}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	for {
		n, err := d.RunOnce(ctx)
		if err != nil {
			d.Log.Error("dispatcher batch failed", "err", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
}

func (d *Dispatcher) RunOnce(ctx context.Context) (int, error) {
	fanned, err := d.Svc.FanOutEvents(ctx, 100)
	if err != nil {
		return 0, err
	}
	deliveries, err := d.Svc.ClaimDeliveries(ctx, 20)
	if err != nil {
		return fanned, err
	}
	for _, dl := range deliveries {
		code, sendErr := d.send(ctx, dl)
		if sendErr != nil {
			d.Log.Warn("webhook attempt failed", "delivery", dl.ID, "attempt", dl.Attempt, "err", sendErr)
		}
		if err := d.Svc.FinishDelivery(ctx, dl, code, sendErr); err != nil {
			return fanned + len(deliveries), err
		}
	}
	return fanned + len(deliveries), nil
}

// Envelope is the JSON body sent to webhooks.
type Envelope struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	EventID   string          `json:"event_id"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

func (d *Dispatcher) send(ctx context.Context, dl core.Delivery) (int, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep <message-id@host> readable for agents
	if err := enc.Encode(Envelope{Type: "event", EventType: dl.EventType, EventID: dl.EventID, CreatedAt: dl.CreatedAt, Data: dl.Payload}); err != nil {
		return 0, err
	}
	body := bytes.TrimRight(buf.Bytes(), "\n")
	msgID := "msg_" + dl.EventID
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig, err := Sign(dl.Secret, msgID, ts, body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dl.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "multi.ai.inbox-webhooks/1")
	for _, prefix := range []string{"webhook", "svix"} {
		req.Header.Set(prefix+"-id", msgID)
		req.Header.Set(prefix+"-timestamp", ts)
		req.Header.Set(prefix+"-signature", sig)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("endpoint returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// Sign returns "v1,<base64 HMAC-SHA256(id.timestamp.body)>" for a whsec_ secret.
func Sign(secret, msgID, timestamp string, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return "", fmt.Errorf("invalid webhook secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify checks a signature header (space separated list) within a 5 minute tolerance.
func Verify(secret, msgID, timestamp, header string, body []byte, now time.Time) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return false
	}
	want, err := Sign(secret, msgID, timestamp, body)
	if err != nil {
		return false
	}
	for _, got := range strings.Fields(header) {
		if hmac.Equal([]byte(got), []byte(want)) {
			return true
		}
	}
	return false
}
