package metrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestHandlerFormat(t *testing.T) {
	r := NewRegistry()
	req := r.NewCounterVec("t_requests_total", "Requests by route.", "route", "code")
	sess := r.NewCounter("t_sessions_total", "Sessions.")
	r.NewCounterVec("t_unused_total", "Never incremented.", "reason")
	req.With("inboxes", "200").Add(3)
	req.With("messages", "404").Inc()
	req.With(`we"ird\`, "500").Inc()
	sess.Inc()
	r.Collect("t_queue", func(context.Context) ([]Sample, error) {
		return []Sample{
			{Name: "t_lag_seconds", Help: "Lag.\nSecond line.", Value: 1.5},
			{Name: "t_held", Help: "Held.", Labels: map[string]string{"state": "review"}, Value: 2},
			{Name: "t_held", Help: "Held.", Labels: map[string]string{"state": "pending"}, Value: 0},
		}, nil
	})
	r.Collect("t_broken", func(context.Context) ([]Sample, error) { return nil, errors.New("db down") })

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("content type %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# HELP t_requests_total Requests by route.\n# TYPE t_requests_total counter\n" +
			`t_requests_total{code="200",route="inboxes"} 3` + "\n" +
			`t_requests_total{code="404",route="messages"} 1` + "\n" +
			`t_requests_total{code="500",route="we\"ird\\"} 1` + "\n",
		"# TYPE t_sessions_total counter\nt_sessions_total 1\n",
		"# HELP t_unused_total Never incremented.\n# TYPE t_unused_total counter\n",
		"# HELP t_lag_seconds Lag.\\nSecond line.\n# TYPE t_lag_seconds gauge\nt_lag_seconds 1.5\n",
		"# TYPE t_held gauge\n" + `t_held{state="pending"} 0` + "\n" + `t_held{state="review"} 2` + "\n",
		"t_queue_scrape_success 1\n",
		"t_broken_scrape_success 0\n",
		"# TYPE go_goroutines gauge\ngo_goroutines ",
		"# TYPE go_memstats_heap_alloc_bytes gauge\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing\n%s\nin\n%s", want, body)
		}
	}

	// Every line is a comment or a valid sample, and families are sorted.
	sample := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*",?)*\})? -?[0-9.e+]+$`)
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			names = append(names, strings.Fields(line)[2])
			continue
		}
		if strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if !sample.MatchString(line) {
			t.Errorf("invalid sample line %q", line)
		}
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("families not sorted or duplicated: %s before %s", names[i-1], names[i])
		}
	}

	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status %d", rec.Code)
	}
}

func TestCountersConcurrent(t *testing.T) {
	r := NewRegistry()
	v := r.NewCounterVec("t_total", "T.", "result")
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				v.With([]string{"ok", "failed"}[j%2]).Inc()
			}
			if i == 0 { // scrape while others write
				resp, err := http.Get(srv.URL)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}(i)
	}
	wg.Wait()
	if got := v.With("ok").Value() + v.With("failed").Value(); got != 8000 {
		t.Fatalf("count %d, want 8000", got)
	}
}

func TestRouteClass(t *testing.T) {
	for path, want := range map[string]string{
		"/healthz":                          "healthz",
		"/readyz":                           "readyz",
		"/openapi.yaml":                     "openapi",
		"/mcp":                              "mcp",
		"/v0/inboxes":                       "inboxes",
		"/v0/inboxes/a@b.eu":                "inboxes",
		"/v0/inboxes/a@b.eu/messages":       "messages",
		"/v0/inboxes/a@b.eu/messages/m/raw": "messages",
		"/v0/inboxes/a@b.eu/threads/t":      "threads",
		"/v0/threads":                       "threads",
		"/v0/api-keys/k":                    "api-keys",
		"/v0/auth/me":                       "auth",
		"/v0/unknown-thing":                 "other",
		"/wp-login.php":                     "other",
	} {
		if got := RouteClass(path); got != want {
			t.Errorf("RouteClass(%q) = %q, want %q", path, got, want)
		}
	}
}
