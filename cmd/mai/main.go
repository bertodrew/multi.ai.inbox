// Command mai runs the multi.ai.inbox platform. One binary, several roles:
//
//	mai serve [-roles api,mx,sender,dispatcher,enrich,scheduler]
//	mai migrate
//	mai admin seed                              staging org, user and admin testers
//	mai admin org-create -name "Acme"
//	mai admin org-plan -org <uuid> -plan team   free, builder, growth, team (Stripe sets it when billing is on)
//	mai admin org-screening -org <uuid> [-on|-off]  screen before notify (no flag prints it)
//	mai admin key-create -org <uuid> [-name agent]
//	mai admin domain-add -name agents.example.eu [-org <uuid>]
//	mai admin inbox-resume -inbox a@b.eu        resume sending paused by the bounce breaker
//	mai admin org-resume -org <uuid>            resume sending for a paused organization
//	mai admin org-delete -org <uuid> -confirm <org name>  revoke keys, print a final summary, delete with cascade
//	mai admin check-port25                      can this host deliver directly?
//	mai admin billing-status                    resolved Stripe settings, keys redacted (offline)
//	mai admin push-status                       resolved VAPID settings, keys redacted (offline)
//	mai admin vapid-keygen                      print a fresh VAPID pair once, for MAI_VAPID_KEYS (offline)
//	mai mcp-config -url https://api.example.eu  MCP setup for Claude Code, Cursor, Codex
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	authpkg "github.com/bertodrew/multi.ai.inbox/internal/auth"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/clamav"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/db"
	"github.com/bertodrew/multi.ai.inbox/internal/dispatcher"
	"github.com/bertodrew/multi.ai.inbox/internal/enrich"
	"github.com/bertodrew/multi.ai.inbox/internal/mcpserver"
	"github.com/bertodrew/multi.ai.inbox/internal/metrics"
	"github.com/bertodrew/multi.ai.inbox/internal/mx"
	"github.com/bertodrew/multi.ai.inbox/internal/scheduler"
	"github.com/bertodrew/multi.ai.inbox/internal/sender"
	"github.com/bertodrew/multi.ai.inbox/internal/tlsx"
)

var version = "dev"

type config struct {
	DatabaseURL    string
	SystemDBURL    string
	MigrateDBURL   string
	HTTPAddr       string
	SMTPAddr       string
	Hostname       string
	SharedDomain   string
	BlobDir        string
	Blob           blob.Store
	MetricsAddr    string
	Relay          string
	RspamdURL      string
	ClamdAddr      string
	ClamdFailClose bool
	FBLAddress     string
	DMARCEnforce   bool
	GatekeeperBin  string
	MaxMessage     int64
	AllowHTTPHook  bool
	SPFInclude     string
	RatePerSecond  float64
	Gateway        api.Gateway
	TLSCert        string
	TLSKey         string
	SendMaxAge     time.Duration
	Billing        *core.Billing
	BillingStatus  core.BillingStatus
	// Web push (VAPID). A nil Push disables the push routes with 501; the
	// status says what is missing and never carries a key.
	Push       *core.Push
	PushStatus core.PushStatus
	// People signing in. AuthSecret enables the endpoints at all; the OAuth
	// client ids are optional, a provider without them answers 501.
	AuthSecret     string
	AuthMailFrom   string
	WebOrigin      string
	PublicAPIURL   string
	GoogleClientID string
	GoogleSecret   string
	GitHubClientID string
	GitHubSecret   string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadConfig() config {
	maxMsg, _ := strconv.ParseInt(env("MAI_MAX_MESSAGE_BYTES", "26214400"), 10, 64)
	rate, _ := strconv.ParseFloat(env("MAI_RATE_LIMIT_PER_SECOND", "20"), 64)
	// Default: trust X-Forwarded-For only from loopback and cluster networks (Caddy).
	proxies, err := api.ParseCIDRs(env("MAI_TRUSTED_PROXIES", "127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid MAI_TRUSTED_PROXIES:", err)
		os.Exit(2)
	}
	// How long temporary send failures are retried before message.failed (RFC 5321: 4 to 5 days).
	sendMaxAge, err := time.ParseDuration(env("MAI_SEND_MAX_AGE", "120h"))
	if err != nil || sendMaxAge <= 0 {
		fmt.Fprintln(os.Stderr, "invalid MAI_SEND_MAX_AGE: want a positive duration such as 120h")
		os.Exit(2)
	}
	// Sign in mail comes from the relay, so the sender must be a domain the
	// relay is allowed to send for.
	authMailFrom := os.Getenv("MAI_AUTH_MAIL_FROM")
	if authMailFrom == "" {
		domain := env("MAI_SHARED_DOMAIN", env("MAI_HOSTNAME", "localhost"))
		authMailFrom = "no-reply@" + domain
	}
	blobDir := env("MAI_BLOB_DIR", "./var/blobs")
	store, err := blobStore(blobDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Billing is env-gated: core.LoadBilling returns nil (501 billing_disabled
	// everywhere) unless both Stripe secrets are set. Secrets are never printed.
	billing, billingStatus := core.LoadBilling(os.Getenv)
	// Push is env-gated the same way: no VAPID key pair, no push endpoint.
	push, pushStatus := core.LoadPush(os.Getenv)
	return config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		SystemDBURL:    os.Getenv("MAI_SYSTEM_DATABASE_URL"),
		MigrateDBURL:   os.Getenv("MAI_MIGRATE_DATABASE_URL"),
		HTTPAddr:       env("MAI_HTTP_ADDR", ":8080"),
		SMTPAddr:       env("MAI_SMTP_ADDR", ":2525"),
		Hostname:       env("MAI_HOSTNAME", "localhost"),
		SharedDomain:   os.Getenv("MAI_SHARED_DOMAIN"),
		BlobDir:        blobDir,
		Blob:           store,
		MetricsAddr:    env("MAI_METRICS_ADDR", ":9090"),
		Relay:          os.Getenv("MAI_SMTP_RELAY"),
		RspamdURL:      os.Getenv("MAI_RSPAMD_URL"),
		ClamdAddr:      os.Getenv("MAI_CLAMD_ADDR"),
		ClamdFailClose: os.Getenv("MAI_CLAMD_FAIL_CLOSED") == "true",
		FBLAddress:     strings.ToLower(strings.TrimSpace(os.Getenv("MAI_FBL_ADDRESS"))),
		DMARCEnforce:   os.Getenv("MAI_DMARC_ENFORCE") == "true",
		GatekeeperBin:  os.Getenv("MAI_GATEKEEPER_BIN"),
		MaxMessage:     maxMsg,
		AllowHTTPHook:  os.Getenv("MAI_ALLOW_HTTP_WEBHOOKS") == "true",
		SPFInclude:     os.Getenv("MAI_SPF_INCLUDE"),
		RatePerSecond:  rate,
		Gateway:        api.Gateway{CORSOrigins: splitList(os.Getenv("MAI_CORS_ORIGINS")), TrustedProxies: proxies},
		TLSCert:        os.Getenv("MAI_SMTP_TLS_CERT"),
		TLSKey:         os.Getenv("MAI_SMTP_TLS_KEY"),
		SendMaxAge:     sendMaxAge,
		Billing:        billing,
		BillingStatus:  billingStatus,
		Push:           push,
		PushStatus:     pushStatus,
		AuthSecret:     os.Getenv("MAI_AUTH_SECRET"),
		AuthMailFrom:   authMailFrom,
		WebOrigin:      env("MAI_WEB_ORIGIN", "https://aimulti.inbox"),
		PublicAPIURL:   os.Getenv("MAI_PUBLIC_API_URL"),
		GoogleClientID: os.Getenv("MAI_GOOGLE_CLIENT_ID"),
		GoogleSecret:   os.Getenv("MAI_GOOGLE_CLIENT_SECRET"),
		GitHubClientID: os.Getenv("MAI_GITHUB_CLIENT_ID"),
		GitHubSecret:   os.Getenv("MAI_GITHUB_CLIENT_SECRET"),
	}
}

// blobStore picks the blob backend: fs (default, MAI_BLOB_DIR) or s3
// (Hetzner Object Storage, path style, Signature Version 4).
func blobStore(dir string) (blob.Store, error) {
	switch backend := env("MAI_BLOB_BACKEND", "fs"); backend {
	case "fs":
		return blob.FS{Root: dir}, nil
	case "s3":
		s, err := blob.NewS3(os.Getenv("MAI_S3_ENDPOINT"), os.Getenv("MAI_S3_REGION"), os.Getenv("MAI_S3_BUCKET"),
			os.Getenv("MAI_S3_ACCESS_KEY"), os.Getenv("MAI_S3_SECRET_KEY"))
		if err != nil {
			return nil, fmt.Errorf("MAI_BLOB_BACKEND=s3: %w (set MAI_S3_ENDPOINT, MAI_S3_REGION, MAI_S3_BUCKET, MAI_S3_ACCESS_KEY, MAI_S3_SECRET_KEY)", err)
		}
		return s, nil
	default:
		return nil, fmt.Errorf("invalid MAI_BLOB_BACKEND %q: want fs or s3", backend)
	}
}

// Billing settings are resolved by core.LoadBilling (see loadConfig), which
// never returns the keys and reports what is missing.
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := loadConfig()
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(ctx, cfg, log, os.Args[2:])
	case "migrate":
		err = migrate(ctx, cfg)
	case "admin":
		err = admin(ctx, cfg, os.Args[2:])
	case "version":
		fmt.Println(version)
	case "mcp-config":
		err = mcpConfig(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mai serve|migrate|admin|mcp-config|version")
	os.Exit(2)
}

// migrate applies pending migrations as the schema owner: the role in
// MAI_MIGRATE_DATABASE_URL (mai_owner in production), or DATABASE_URL in
// development when that role owns the schema.
func migrate(ctx context.Context, cfg config) error {
	url := cfg.MigrateDBURL
	if url == "" {
		url = cfg.DatabaseURL
	}
	if url == "" {
		return errors.New("MAI_MIGRATE_DATABASE_URL or DATABASE_URL is required")
	}
	d, err := db.Open(ctx, url)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Migrate(ctx)
}

// withDB opens the runtime pools: DATABASE_URL for tenant transactions
// (mai_app) and MAI_SYSTEM_DATABASE_URL for system ones (mai_system; falls
// back to DATABASE_URL for development with a single privileged role).
// Pending migrations are applied first; with only the runtime roles
// configured, Migrate merely checks that the schema is current.
func withDB(ctx context.Context, cfg config, fn func(*db.DB) error) error {
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if cfg.MigrateDBURL != "" {
		if err := migrate(ctx, cfg); err != nil {
			return err
		}
	}
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Migrate(ctx); err != nil {
		return err
	}
	if cfg.SystemDBURL != "" {
		if err := d.OpenSystem(ctx, cfg.SystemDBURL); err != nil {
			return err
		}
	}
	if err := d.CheckSystem(ctx); err != nil {
		return err
	}
	return fn(d)
}

func service(d *db.DB, cfg config) *core.Service {
	svc := &core.Service{DB: d, Blob: cfg.Blob, SharedDomain: cfg.SharedDomain,
		Hostname: cfg.Hostname, AllowHTTPWebhooks: cfg.AllowHTTPHook, SPFInclude: cfg.SPFInclude,
		SendMaxAge: cfg.SendMaxAge, AntivirusFailClosed: cfg.ClamdFailClose, Billing: cfg.Billing, Push: cfg.Push}
	if cfg.AuthSecret != "" {
		// Without a relay a magic link cannot be delivered, so the link
		// endpoint reports 501 and OAuth keeps working.
		authCfg := &core.AuthConfig{Secret: []byte(cfg.AuthSecret), WebOrigin: cfg.WebOrigin, MailFrom: cfg.AuthMailFrom,
			Google: authpkg.Google(cfg.GoogleClientID, cfg.GoogleSecret), GitHub: authpkg.GitHub(cfg.GitHubClientID, cfg.GitHubSecret)}
		if cfg.Relay != "" {
			authCfg.Mailer = &authpkg.RelayMailer{Relay: cfg.Relay, From: cfg.AuthMailFrom, Helo: cfg.Hostname}
		}
		svc.Auth = authCfg
	}
	if cfg.ClamdAddr != "" {
		// API uploaded attachments and agent to agent mail; the SMTP edge has its own client.
		svc.Antivirus = &clamav.Clamd{Addr: cfg.ClamdAddr}
	}
	return svc
}

func serve(ctx context.Context, cfg config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	roles := fs.String("roles", "api,mx,sender,dispatcher,enrich,scheduler,push", "comma separated roles to run")
	_ = fs.Parse(args)
	enabled := map[string]bool{}
	for _, r := range strings.Split(*roles, ",") {
		enabled[strings.TrimSpace(r)] = true
	}
	return withDB(ctx, cfg, func(d *db.DB) error {
		svc := service(d, cfg)
		svc.Log = log
		var wg sync.WaitGroup
		errs := make(chan error, 3)

		if cfg.MetricsAddr != "off" {
			metricsSrv := metricsServer(svc, cfg.MetricsAddr, log)
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Info("metrics listening", "addr", cfg.MetricsAddr)
				if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errs <- err
				}
			}()
			go func() {
				<-ctx.Done()
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = metricsSrv.Shutdown(shutdown)
			}()
		}

		if enabled["api"] {
			srv := &api.Server{Svc: svc, Log: log}
			if cfg.RatePerSecond > 0 {
				srv.Limiter = api.NewLimiter(cfg.RatePerSecond, cfg.RatePerSecond*2)
			}
			srv.MCP = mcpserver.Handler(svc, mcpCaller, log, version)
			srv.Auth = api.AuthSurface{WebOrigin: cfg.WebOrigin, PublicURL: cfg.PublicAPIURL}
			srv.MagicLinks = api.NewLimiter(5.0/60, 5)     // 5 sign in links per IP, refilling 5 per minute
			srv.AuthFailures = api.NewLimiter(10.0/60, 10) // 10 failed keys per IP, refilling 10 per minute
			srv.Gateway = cfg.Gateway
			if cfg.Billing == nil {
				log.Info("billing disabled", "webhook", cfg.BillingStatus.WebhookPath,
					"hint", "set MAI_STRIPE_SECRET_KEY and MAI_STRIPE_WEBHOOK_SECRET; mai admin billing-status shows what is missing")
			} else {
				log.Info("billing enabled", "prices", len(cfg.Billing.Prices), "warnings", cfg.BillingStatus.Warnings)
			}
			if cfg.Push == nil {
				log.Info("web push disabled", "hint", "set MAI_VAPID_KEYS or MAI_VAPID_PRIVATE_KEY, MAI_VAPID_PUBLIC_KEY and MAI_VAPID_SUBJECT; mai admin push-status shows what is missing")
			} else {
				log.Info("web push enabled", "warnings", cfg.PushStatus.Warnings)
			}
			srv.Done = ctx.Done() // open event streams end so shutdown does not wait for them
			httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Info("api listening", "addr", cfg.HTTPAddr)
				if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errs <- err
				}
			}()
			go func() {
				<-ctx.Done()
				shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				_ = httpSrv.Shutdown(shutdown)
			}()
		}
		if enabled["mx"] {
			certs := &tlsx.Loader{CertGlob: cfg.TLSCert, KeyGlob: cfg.TLSKey, Hostname: cfg.Hostname}
			backend := &mx.Backend{Svc: svc, Log: log, MaxBytes: cfg.MaxMessage, RspamdURL: cfg.RspamdURL, ClamdFailClosed: cfg.ClamdFailClose,
				FBLAddress: cfg.FBLAddress, MailAuth: &mx.MailAuth{Enforce: cfg.DMARCEnforce}}
			if cfg.ClamdAddr != "" {
				backend.Clamd = &clamav.Clamd{Addr: cfg.ClamdAddr}
			} else {
				log.Warn("antivirus disabled: MAI_CLAMD_ADDR is not set")
			}
			smtpSrv := mx.NewServer(backend, cfg.SMTPAddr, cfg.Hostname, certs.Config())
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Info("smtp listening", "addr", cfg.SMTPAddr)
				if err := smtpSrv.ListenAndServe(); err != nil && ctx.Err() == nil {
					errs <- err
				}
			}()
			go func() {
				<-ctx.Done()
				shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = smtpSrv.Shutdown(shutdown)
			}()
		}
		if enabled["sender"] {
			s := &sender.Sender{Svc: svc, Relay: cfg.Relay, HeloName: cfg.Hostname, Log: log,
				MTASTS: &sender.MTASTS{HTTP: dispatcher.SafeClient(), Log: log}}
			wg.Add(1)
			go func() { defer wg.Done(); s.Run(ctx) }()
		}
		if enabled["dispatcher"] {
			dp := &dispatcher.Dispatcher{Svc: svc, Log: log, Client: dispatcher.SafeClient()}
			wg.Add(1)
			go func() { defer wg.Done(); dp.Run(ctx) }()
		}
		if enabled["enrich"] {
			if cfg.GatekeeperBin == "" {
				log.Warn("enrich role disabled: MAI_GATEKEEPER_BIN is not set; organizations with screen_before_notify hold inbound mail until the scheduler times screening out to review")
			} else {
				e := &enrich.Enricher{Svc: svc, Bin: cfg.GatekeeperBin, Log: log}
				wg.Add(1)
				go func() { defer wg.Done(); e.Run(ctx) }()
			}
		}
		if enabled["scheduler"] {
			sc := &scheduler.Scheduler{Svc: svc, Log: log}
			wg.Add(1)
			go func() { defer wg.Done(); sc.Run(ctx) }()
		}
		if enabled["push"] {
			// Delivery in its own role: a slow push service occupies this loop,
			// never a request handler or the SMTP edge.
			ps := &core.PushSender{Svc: svc, Log: log, Client: dispatcher.SafeClient()}
			wg.Add(1)
			go func() { defer wg.Done(); ps.Run(ctx) }()
		}
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			wg.Wait()
			return nil
		}
	})
}

// metricsServer serves GET /metrics (Prometheus text format) on its own
// address, which Caddy does not proxy; the NetworkPolicy admits scrapes from
// the namespace only. Backlog gauges are read from the database per scrape.
func metricsServer(svc *core.Service, addr string, log *slog.Logger) *http.Server {
	metrics.Default.Log = log
	metrics.Default.Collect("mai_queue", func(ctx context.Context) ([]metrics.Sample, error) {
		st, err := svc.QueueStats(ctx)
		if err != nil {
			return nil, err
		}
		return []metrics.Sample{
			{Name: "mai_outbox_pending_events", Help: "Events not yet fanned out to webhooks.", Value: float64(st.OutboxPending)},
			{Name: "mai_outbox_lag_seconds", Help: "Age of the oldest event not yet fanned out (0 when none).", Value: st.OutboxLagSeconds},
			{Name: "mai_screening_messages", Help: "Inbound messages held by screening, by state.", Labels: map[string]string{"state": "pending"}, Value: float64(st.ScreeningPending)},
			{Name: "mai_screening_messages", Help: "Inbound messages held by screening, by state.", Labels: map[string]string{"state": "review"}, Value: float64(st.ScreeningReview)},
			{Name: "mai_blob_deletion_queue", Help: "Blobs queued for deletion.", Value: float64(st.BlobDeletions)},
		}, nil
	})
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Default.Handler())
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
}

func admin(ctx context.Context, cfg config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mai admin seed|org-create|org-plan|org-screening|key-create|domain-add|inbox-resume|org-resume|org-delete|check-port25|billing-status")
	}
	// billing-status reads the environment only: no database, no Stripe call,
	// and the keys are never printed.
	if args[0] == "billing-status" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg.BillingStatus)
	}
	if args[0] == "push-status" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg.PushStatus)
	}
	// vapid-keygen needs neither a database nor a network: it prints a fresh
	// key pair once, as the JSON document MAI_VAPID_KEYS expects.
	if args[0] == "vapid-keygen" {
		return vapidKeygen()
	}
	if args[0] == "check-port25" {
		return checkPort25(ctx)
	}
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	name := fs.String("name", "", "name")
	org := fs.String("org", "", "organization id")
	plan := fs.String("plan", "", "plan: free, builder, growth, team")
	on := fs.Bool("on", false, "org-screening: hold inbound mail until the gatekeeper verdict exists")
	off := fs.Bool("off", false, "org-screening: notify message.received at ingest (not recommended)")
	inbox := fs.String("inbox", "", "inbox address")
	confirm := fs.String("confirm", "", "org-delete: the organization's exact name")
	perms := fs.String("permissions", "", "key-create: comma separated permissions (empty: full access, which never includes message_release)")
	_ = fs.Parse(args[1:])
	return withDB(ctx, cfg, func(d *db.DB) error {
		svc := service(d, cfg)
		var out any
		var err error
		switch args[0] {
		case "seed":
			out, err = seed(ctx, svc, cfg)
		case "org-create":
			out, err = svc.CreateOrg(ctx, *name)
		case "org-plan":
			err = svc.SetPlan(ctx, *org, *plan)
			out = map[string]string{"organization_id": *org, "plan": *plan}
		case "org-screening":
			out, err = orgScreening(ctx, svc, *org, *on, *off)
		case "key-create":
			if *org == "" {
				return errors.New("-org is required")
			}
			if *perms != "" {
				out, err = svc.CreateOperatorKey(ctx, *org, *name, splitList(*perms))
			} else {
				out, err = svc.CreateKey(ctx, *org, *name)
			}
		case "domain-add":
			out, err = svc.AddDomain(ctx, *org, *name)
		case "inbox-resume":
			if *inbox == "" {
				return errors.New("-inbox is required")
			}
			out, err = svc.ResumeInboxSending(ctx, *inbox)
		case "org-resume":
			if *org == "" {
				return errors.New("-org is required")
			}
			err = svc.ResumeOrgSending(ctx, *org)
			out = map[string]string{"organization_id": *org, "sending": "resumed"}
		case "org-delete":
			if *org == "" || *confirm == "" {
				return errors.New("-org and -confirm (the organization's exact name) are required")
			}
			// The summary goes to stdout before anything is deleted; keep it,
			// it holds the organization's audit log.
			out, err = svc.DeleteOrg(ctx, *org, *confirm, func(sum *core.OrgDeletionSummary) error {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sum)
			})
		default:
			return fmt.Errorf("unknown admin command %q", args[0])
		}
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	})
}

// orgScreening prints or changes an organization's screen_before_notify
// setting; a change is written to the organization's audit log.
func orgScreening(ctx context.Context, svc *core.Service, org string, on, off bool) (*core.Org, error) {
	switch {
	case org == "":
		return nil, errors.New("-org is required")
	case on && off:
		return nil, errors.New("pass -on or -off, not both")
	case !on && !off:
		o, err := svc.GetOrg(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.NotFound("organization")
		}
		return o, err
	}
	o, err := svc.SetScreening(ctx, org, on)
	if err != nil {
		return nil, err
	}
	meta, _ := json.Marshal(map[string]any{"screen_before_notify": on})
	if err := svc.Audit(ctx, o.ID, core.AuditEntry{Actor: "operator", Action: "org.screening", Target: o.ID, Metadata: meta}); err != nil {
		return nil, fmt.Errorf("setting changed but the audit entry failed: %w", err)
	}
	return o, nil
}

type seedResult struct {
	Organization *core.Org          `json:"organization"`
	Inboxes      []*core.Inbox      `json:"inboxes"`
	Keys         []*core.CreatedKey `json:"api_keys,omitempty"`
	Note         string             `json:"note"`
}

// seed prepares a staging environment: the shared domain, a "Staging" org on
// the team plan, user and admin tester inboxes, and one API key per tester.
// It is safe to run again: existing objects are reused and keys are only
// created the first time (they are shown once).
func seed(ctx context.Context, svc *core.Service, cfg config) (*seedResult, error) {
	if cfg.SharedDomain == "" {
		return nil, errors.New("MAI_SHARED_DOMAIN is required")
	}
	if _, err := svc.AddDomain(ctx, "", cfg.SharedDomain); err != nil && core.AsError(err).Code != "already_exists" {
		return nil, err
	}
	org, created, err := svc.EnsureOrg(ctx, "Staging")
	if err != nil {
		return nil, err
	}
	if err := svc.SetPlan(ctx, org.ID, "team"); err != nil {
		return nil, err
	}
	org.Plan = "team"
	res := &seedResult{Organization: org, Note: "API keys are printed only when the organization is created; use key-create to add more."}
	for _, u := range []string{"user.tester", "admin.tester"} {
		inbox, _, err := svc.CreateInbox(ctx, org.ID, core.CreateInboxRequest{Username: u, ClientID: "seed-" + u, DisplayName: u})
		if err != nil {
			return nil, err
		}
		res.Inboxes = append(res.Inboxes, inbox)
		if created {
			k, err := svc.CreateKey(ctx, org.ID, u)
			if err != nil {
				return nil, err
			}
			res.Keys = append(res.Keys, k)
		}
	}
	return res, nil
}

// vapidKeygen prints a fresh VAPID key pair once, as the JSON document
// MAI_VAPID_KEYS expects. Run it where the secret store is reachable, put the
// output there, and give the public half to the web build. It never touches the
// database and never logs the key: stdout is the only copy.
func vapidKeygen() error {
	public, private, err := core.GenerateVAPIDKeys()
	if err != nil {
		return err
	}
	doc, err := json.MarshalIndent(map[string]string{"public_key": public, "private_key": private}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "This pair is shown once. Store it as MAI_VAPID_KEYS (secret) and build the console with "+
		"NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY set to the public half. Rotating it unsubscribes every device.")
	_, err = os.Stdout.Write(append(doc, '\n'))
	return err
}

// checkPort25 tells whether outbound SMTP is open on this host (Hetzner
// blocks it on new accounts until a support request is approved).
func checkPort25(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", "gmail-smtp-in.l.google.com:25")
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		fmt.Println("port 25 outbound: UNKNOWN, DNS lookup failed (" + err.Error() + "). Fix the resolver and retry.")
		return nil
	}
	if err != nil {
		fmt.Println("port 25 outbound: BLOCKED (" + err.Error() + "). Set MAI_SMTP_RELAY to the Brevo relay or ask Hetzner to unblock it.")
		return nil
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	banner := make([]byte, 128)
	n, _ := conn.Read(banner)
	fmt.Printf("port 25 outbound: OPEN (%s)\n", strings.TrimSpace(string(banner[:n])))
	return nil
}

func mcpCaller(ctx context.Context) mcpserver.Caller {
	return mcpserver.Caller{Principal: api.PrincipalFrom(ctx), IP: api.ClientIP(ctx), RequestID: api.RequestID(ctx)}
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mcpConfig prints ready to paste MCP registrations for common agent clients.
// The key is read from MAI_API_KEY at use time, never written into files here.
func mcpConfig(args []string) error {
	fs := flag.NewFlagSet("mcp-config", flag.ExitOnError)
	base := fs.String("url", "http://localhost:8080", "API base URL")
	_ = fs.Parse(args)
	endpoint := strings.TrimSuffix(*base, "/") + "/mcp"
	fmt.Printf(`# Claude Code
claude mcp add --transport http multi-ai-inbox %[1]s --header "X-Api-Key: $MAI_API_KEY"

# Cursor (.cursor/mcp.json)
{"mcpServers": {"multi-ai-inbox": {"url": "%[1]s", "headers": {"X-Api-Key": "${env:MAI_API_KEY}"}}}}

# Codex (~/.codex/config.toml), sends the key as a bearer token
[mcp_servers.multi-ai-inbox]
url = "%[1]s"
bearer_token_env_var = "MAI_API_KEY"

# Tip: give agents a scoped key, e.g. POST /v0/api-keys {"permissions":["inbox_read","message_read","message_send"]}
`, endpoint)
	return nil
}
