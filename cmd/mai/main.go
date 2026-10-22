// Command mai runs the multi.ai.inbox platform. One binary, several roles:
//
//	mai serve [-roles api,mx,sender,dispatcher,enrich]
//	mai migrate
//	mai admin org-create -name "Acme"
//	mai admin key-create -org <uuid> [-name agent]
//	mai admin domain-add -name agents.example.eu [-org <uuid>] -mx mx.example.eu
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/api"
	"github.com/bertodrew/multi.ai.inbox/internal/blob"
	"github.com/bertodrew/multi.ai.inbox/internal/core"
	"github.com/bertodrew/multi.ai.inbox/internal/db"
	"github.com/bertodrew/multi.ai.inbox/internal/dispatcher"
	"github.com/bertodrew/multi.ai.inbox/internal/enrich"
	"github.com/bertodrew/multi.ai.inbox/internal/mcpserver"
	"github.com/bertodrew/multi.ai.inbox/internal/mx"
	"github.com/bertodrew/multi.ai.inbox/internal/sender"
)

var version = "dev"

type config struct {
	DatabaseURL   string
	HTTPAddr      string
	SMTPAddr      string
	Hostname      string
	SharedDomain  string
	BlobDir       string
	Relay         string
	RspamdURL     string
	GatekeeperBin string
	MaxMessage    int64
	AllowHTTPHook bool
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadConfig() config {
	maxMsg, _ := strconv.ParseInt(env("MAI_MAX_MESSAGE_BYTES", "26214400"), 10, 64)
	return config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HTTPAddr:      env("MAI_HTTP_ADDR", ":8080"),
		SMTPAddr:      env("MAI_SMTP_ADDR", ":2525"),
		Hostname:      env("MAI_HOSTNAME", "localhost"),
		SharedDomain:  os.Getenv("MAI_SHARED_DOMAIN"),
		BlobDir:       env("MAI_BLOB_DIR", "./var/blobs"),
		Relay:         os.Getenv("MAI_SMTP_RELAY"),
		RspamdURL:     os.Getenv("MAI_RSPAMD_URL"),
		GatekeeperBin: os.Getenv("MAI_GATEKEEPER_BIN"),
		MaxMessage:    maxMsg,
		AllowHTTPHook: os.Getenv("MAI_ALLOW_HTTP_WEBHOOKS") == "true",
	}
}

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
		err = withDB(ctx, cfg, func(d *db.DB) error { return d.Migrate(ctx) })
	case "admin":
		err = admin(ctx, cfg, os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
	}
	if err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mai serve|migrate|admin|version")
	os.Exit(2)
}

func withDB(ctx context.Context, cfg config, fn func(*db.DB) error) error {
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	return fn(d)
}

func service(d *db.DB, cfg config) *core.Service {
	return &core.Service{DB: d, Blob: blob.FS{Root: cfg.BlobDir}, SharedDomain: cfg.SharedDomain,
		Hostname: cfg.Hostname, AllowHTTPWebhooks: cfg.AllowHTTPHook}
}

func serve(ctx context.Context, cfg config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	roles := fs.String("roles", "api,mx,sender,dispatcher,enrich", "comma separated roles to run")
	_ = fs.Parse(args)
	enabled := map[string]bool{}
	for _, r := range strings.Split(*roles, ",") {
		enabled[strings.TrimSpace(r)] = true
	}
	return withDB(ctx, cfg, func(d *db.DB) error {
		if err := d.Migrate(ctx); err != nil {
			return err
		}
		svc := service(d, cfg)
		var wg sync.WaitGroup
		errs := make(chan error, 2)

		if enabled["api"] {
			srv := &api.Server{Svc: svc, Log: log}
			srv.MCP = mcpserver.Handler(svc, api.OrgFrom, version)
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
			smtpSrv := mx.NewServer(&mx.Backend{Svc: svc, Log: log, MaxBytes: cfg.MaxMessage, RspamdURL: cfg.RspamdURL}, cfg.SMTPAddr, cfg.Hostname)
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
			s := &sender.Sender{Svc: svc, Relay: cfg.Relay, HeloName: cfg.Hostname, Log: log}
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
				log.Warn("enrich role disabled: MAI_GATEKEEPER_BIN is not set")
			} else {
				e := &enrich.Enricher{Svc: svc, Bin: cfg.GatekeeperBin, Log: log}
				wg.Add(1)
				go func() { defer wg.Done(); e.Run(ctx) }()
			}
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

func admin(ctx context.Context, cfg config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mai admin org-create|key-create|domain-add")
	}
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	name := fs.String("name", "", "name")
	org := fs.String("org", "", "organization id")
	mxHost := fs.String("mx", cfg.Hostname, "public MX host name")
	_ = fs.Parse(args[1:])
	return withDB(ctx, cfg, func(d *db.DB) error {
		svc := service(d, cfg)
		var out any
		var err error
		switch args[0] {
		case "org-create":
			out, err = svc.CreateOrg(ctx, *name)
		case "key-create":
			if *org == "" {
				return errors.New("-org is required")
			}
			out, err = svc.CreateKey(ctx, *org, *name)
		case "domain-add":
			out, err = svc.AddDomain(ctx, *org, *name, *mxHost)
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
