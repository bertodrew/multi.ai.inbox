package metrics

import "strings"

// Default is the process registry served at /metrics.
var Default = NewRegistry()

// Platform counters. Label values are fixed sets documented on each family.
var (
	// route: see RouteClass; code: HTTP status code.
	HTTPRequests = Default.NewCounterVec("mai_http_requests_total", "API requests by route class and status code.", "route", "code")

	SMTPSessions = Default.NewCounter("mai_smtp_sessions_total", "Inbound SMTP connections.")
	SMTPAccepted = Default.NewCounter("mai_smtp_messages_accepted_total", "Inbound messages accepted at DATA and ingested.")
	// reason: unknown_recipient, recipient_lookup_error, too_large, virus,
	// antivirus_unavailable, spam, ingest_error.
	SMTPRejected = Default.NewCounterVec("mai_smtp_rejected_total", "Inbound SMTP commands refused, permanently or temporarily, by reason.", "reason")

	// result: sent, failed (final), deferred (retry scheduled).
	Outbound = Default.NewCounterVec("mai_outbound_attempts_total", "Outbound delivery attempts by result.", "result")
	// result: ok, failed (one attempt; failed attempts are retried on schedule).
	WebhookDeliveries = Default.NewCounterVec("mai_webhook_deliveries_total", "Webhook delivery attempts by result.", "result")
)

var routeResources = map[string]bool{
	"api-keys": true, "audit-logs": true, "auth": true, "inboxes": true,
	"threads": true, "domains": true, "webhooks": true,
}

// RouteClass maps a request path to a bounded label: healthz, readyz,
// openapi, mcp, the /v0 resource (inboxes, threads, domains, webhooks,
// api-keys, audit-logs, auth), messages or threads for the paths below
// /v0/inboxes/{inbox}/messages and /threads, or other.
func RouteClass(path string) string {
	switch path {
	case "/healthz":
		return "healthz"
	case "/readyz":
		return "readyz"
	case "/openapi.yaml":
		return "openapi"
	case "/mcp", "/mcp/":
		return "mcp"
	}
	rest, ok := strings.CutPrefix(path, "/v0/")
	if !ok {
		return "other"
	}
	parts := strings.Split(rest, "/")
	if !routeResources[parts[0]] {
		return "other"
	}
	if parts[0] == "inboxes" && len(parts) > 2 && (parts[2] == "messages" || parts[2] == "threads") {
		return parts[2]
	}
	return parts[0]
}
