package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/metrics"
)

// Gateway holds the edge concerns of the API: who the client is, which
// browser origins may call it, and what every response must carry.
type Gateway struct {
	// CORSOrigins are browser origins allowed to call the API (the console).
	CORSOrigins []string
	// TrustedProxies are networks whose X-Forwarded-For is believed (Caddy).
	TrustedProxies []*net.IPNet
}

// ParseCIDRs turns "10.0.0.0/8,127.0.0.1/32" into networks, skipping blanks.
func ParseCIDRs(list string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, c := range strings.Split(list, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

type reqInfo struct {
	id    string
	ip    string
	orgID string
}

type infoKey struct{}

func info(ctx context.Context) *reqInfo {
	if i, ok := ctx.Value(infoKey{}).(*reqInfo); ok {
		return i
	}
	return &reqInfo{}
}

func requestID(ctx context.Context) string { return info(ctx).id }
func clientIP(ctx context.Context) string  { return info(ctx).ip }

// RequestID and ClientIP expose the gateway's view of a request to other surfaces (MCP).
func RequestID(ctx context.Context) string { return requestID(ctx) }
func ClientIP(ctx context.Context) string  { return clientIP(ctx) }
func setLogOrg(ctx context.Context, org string) {
	info(ctx).orgID = org
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush keeps streaming (MCP server sent events) working through the wrapper.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) gateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ri := &reqInfo{id: r.Header.Get("X-Request-Id"), ip: s.Gateway.realIP(r)}
		if ri.id == "" || len(ri.id) > 64 {
			ri.id = newRequestID()
		}
		h := w.Header()
		h.Set("X-Request-Id", ri.id)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/v0/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			h.Set("Cache-Control", "no-store")
		}
		if origin := r.Header.Get("Origin"); origin != "" && slices.Contains(s.Gateway.CORSOrigins, origin) {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After, Aimulti-Api-Version, Sunset, Link")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("panic", "request_id", ri.id, "panic", rec)
				if sw.status == 0 {
					s.fail(sw, &core.Error{Status: http.StatusInternalServerError, Name: "Internal Server Error",
						Code: "internal_error", Message: "internal error, request " + ri.id})
				}
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			metrics.HTTPRequests.With(metrics.RouteClass(r.URL.Path), strconv.Itoa(status)).Inc()
			if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
				s.Log.Info("request", "request_id", ri.id, "method", r.Method, "path", r.URL.Path, "status", sw.status,
					"bytes", sw.bytes, "ms", time.Since(start).Milliseconds(), "ip", ri.ip, "org", ri.orgID)
			}
		}()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), infoKey{}, ri)))
	})
}

// realIP trusts X-Forwarded-For only from a trusted proxy, and takes the
// rightmost address that is not itself a trusted proxy.
func (g Gateway) realIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !g.trusted(host) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if ip != "" && !g.trusted(ip) {
			return ip
		}
	}
	return host
}

func (g Gateway) trusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range g.TrustedProxies {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}
