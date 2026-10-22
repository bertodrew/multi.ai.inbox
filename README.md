# multi.ai.inbox

Email inboxes for AI agents, hosted in the EU. Agent native, written in Go (connectivity) and Rust (AI screening), with no PHP and no mailcow.

An inbox is a database row, mail is an event. Agents create inboxes with one API call, receive mail as parsed threads, get signed webhooks, reply with correct threading, and use the same capabilities as MCP tools from Claude Code, Cursor or Codex.

Design and research: [`docs/agentmail-re/`](docs/agentmail-re/README.md) (PRD, API spec, architecture, [ADR 10](docs/agentmail-re/10-native-platform.md)).

## What runs

```mermaid
flowchart LR
    Internet((Internet SMTP)) -->|":25"| MX["mx (Go)<br/>RCPT check · DKIM verify · rspamd"]
    MX --> CORE["core (Go)<br/>parse · thread · store"]
    Agents["agents · SDK · MCP"] -->|"HTTPS /v0 · /mcp"| API["api + MCP (Go)"] --> CORE
    CORE --> PG[("Postgres<br/>RLS on every table")]
    CORE --> BLOB[("raw .eml + attachments")]
    PG --> SND["sender (Go)<br/>DKIM sign · relay or direct MX · local agent to agent"]
    PG --> DSP["dispatcher (Go)<br/>signed webhooks · retries"]
    PG --> ENR["enrich → gatekeeper (Rust)<br/>prompt injection screening · reply extraction"]
    SND --> Out((Internet))
    DSP --> Hooks["customer webhooks"]
```

One binary, `mai`, runs any subset of roles: `api`, `mx`, `sender`, `dispatcher`, `enrich`, `scheduler`. Events use a Postgres outbox (no extra queue until it is needed).

| Path | What |
|------|------|
| `cmd/mai` | binary: `serve`, `migrate`, `admin org-create / key-create / domain-add` |
| `internal/core` | all business rules, shared by REST, MCP, SMTP and workers |
| `internal/api`, `internal/mcpserver` | REST `/v0` and MCP at `/mcp`, same API key |
| `internal/mx`, `internal/sender` | SMTP in, SMTP out |
| `internal/dispatcher`, `internal/enrich` | webhooks (Standard Webhooks + Svix headers), gatekeeper pipe |
| `internal/db/migrations` | schema with row level security |
| `gatekeeper/` | Rust crate and JSON lines worker |
| `openapi/openapi.yaml` | API contract |
| `deploy/` | k3s manifests, Flux GitOps, Hetzner bootstrap, local Docker Compose |
| `web/` | public site (Next.js static export): home, docs from the OpenAPI spec, pricing, security, FAQ, privacy, terms, careers, `llms.txt`, Markdown twins of docs |
| `landing/` | previous landing imported from the old server, served under `/legacy/` |

## Run locally

```bash
# Postgres 16 with a non superuser role (RLS does not apply to superusers)
export DATABASE_URL=postgres://mai_app:mai_app@localhost:5432/mai
export MAI_SHARED_DOMAIN=agents.localhost MAI_HOSTNAME=mx.localhost MAI_ALLOW_HTTP_WEBHOOKS=true
(cd gatekeeper && cargo build --release) && export MAI_GATEKEEPER_BIN=$PWD/gatekeeper/target/release/gatekeeper

go run ./cmd/mai admin seed     # shared domain, "Staging" org, user.tester@ and admin.tester@, API keys (shown once)
go run ./cmd/mai serve          # API :8080, SMTP :2525, all workers

curl -s -X POST localhost:8080/v0/inboxes -H "Authorization: Bearer $KEY" -d '{"username":"sales"}'
```

MCP for Claude Code:

```bash
claude mcp add --transport http multi-ai-inbox http://localhost:8080/mcp --header "X-Api-Key: $KEY"
```

## Test

```bash
(cd gatekeeper && cargo clippy --all-targets -- -D warnings && cargo test)
MAI_TEST_DATABASE_URL=postgres://postgres@localhost:5432/postgres \
MAI_TEST_GATEKEEPER_BIN=$PWD/gatekeeper/target/release/gatekeeper \
go test -race ./...
```

The end to end test creates a fresh database owned by a non superuser role and covers SMTP in, RLS isolation between organizations, gatekeeper verdicts, signed webhooks, idempotent replies through a relay with DKIM, agent to agent delivery and MCP. `TestScreenBeforeNotify` covers the hold: no `message.received` before the verdict, review messages hidden on REST and MCP, release with its permission and audit entry, webhook order, and the fail closed paths (gatekeeper unavailable, screening timeout).

## API surface

REST under `/v0` (contract in `openapi/openapi.yaml`), same capabilities as MCP tools at `/mcp`:

| Area | Endpoints |
|------|-----------|
| Identity | `GET /v0/auth/me` |
| Inboxes | `POST/GET /v0/inboxes`, `GET/PATCH/DELETE /v0/inboxes/{inbox}` |
| Threads | `GET /v0/inboxes/{inbox}/threads`, `GET /v0/threads`, `GET /v0/threads/{id}` |
| Messages | list, get, raw, labels (`PATCH`), `messages/send`, `{message}/reply`, `{message}/attachments/{id}`, `POST {message}/release` (needs `message_release`, audited) |
| Domains | `POST/GET /v0/domains`, `GET/DELETE /v0/domains/{domain}`, `POST /v0/domains/{domain}/verify` |
| Webhooks | `POST/GET /v0/webhooks`, `DELETE /v0/webhooks/{id}` |
| Access | `POST/GET /v0/api-keys`, `DELETE /v0/api-keys/{id}` (scoped permissions, no escalation), `GET /v0/audit-logs` |
| Ops | `GET /healthz`, `GET /readyz`, `GET /openapi.yaml` |

Security at the edge: request IDs, security headers, CORS allowlist, per organization rate limit, per IP limit on failed keys, X-Forwarded-For only from trusted proxies, SMTP STARTTLS (certificate from Caddy, self signed fallback). The MCP gateway applies the same permissions per tool and writes agent actions to the audit log. `mai mcp-config -url <api>` prints the setup for Claude Code, Cursor and Codex.

Events: `message.received`, `message.received.spam`, `message.sent`, `message.failed`, `message.security.completed`, `message.security.review`, `domain.verified`.

Screen before notify (organization setting `screen_before_notify`, on by default, shown in `GET /v0/auth/me`, changed with `mai admin org-screening -org <id> -on|-off`): inbound mail is held until the gatekeeper verdict exists. Clean mail fires `message.security.completed`, then `message.received`. Mail with verdict `review`, or that could not be screened (gatekeeper unavailable, or no verdict within 15 minutes), is held for review and fires only `message.security.review`, with bodies and previews left out of the payload. Held messages are hidden from every message and thread read on REST and MCP; `include_review=true` shows those held for review to keys with `message_review`. `POST .../messages/{message}/release` (MCP `release_message`, permission `message_release`) releases one, writes `message.release` to the audit log and fires `message.received`. Keep both permissions away from the agent that reads the inbox.

| Event | Monitor mode (setting off) | Screen before notify (default) |
|-------|----------------------------|--------------------------------|
| `message.received` | at ingest | after a `clear` verdict, or after a release |
| `message.security.completed` | after screening | after a `clear` verdict, before `message.received` |
| `message.security.review` | after screening, full payload | when held for review, no bodies |

Plans (`mai admin org-plan`): free 3 inboxes, 100/day, 3,000/month, footer; builder 10 inboxes, 1,000/day, 10,000/month; team 150 inboxes, 15,000/day, 150,000/month.

## Deploy (Hetzner, k3s, GitOps)

Production runs on the Hetzner CX33 with k3s and Flux; this repository is the source of truth.

1. Rebuild the server with Ubuntu 24.04, then run `deploy/hetzner/bootstrap.sh` as root (an operations agent can do all of this: see `deploy/hetzner/AGENT_RUNBOOK.md`). It installs k3s and Flux, asks for the secrets (GitHub tokens, Brevo SMTP key, Object Storage keys) and keeps them only in the cluster, bootstraps Flux on `deploy/flux/clusters/staging`, seeds the staging testers and checks outbound port 25.
2. Hetzner firewall: inbound 22 (your IP), 25, 80, 443. Set the PTR of the server IP to `mx.2-28-99-252.sslip.io`.
3. From then on, merging to `master` deploys: `release.yml` builds the image to GHCR and pins its tag in `deploy/k8s/staging`; Flux applies it.

| Path | What |
|------|------|
| `deploy/k8s/base` | Postgres, mai, rspamd, Caddy, nightly backup to Object Storage, network policies |
| `deploy/k8s/staging` | staging config (sslip.io hostnames, `agents.localhost`, Brevo SPF) and image tag |
| `deploy/flux/clusters/staging` | Flux Kustomization |
| `deploy/local` | Docker Compose for local development |

When the brand domain is bought: `mai admin domain-add -name agents.<brand>`, set `MAI_SHARED_DOMAIN`, `MAI_HOSTNAME`, `MAI_API_HOST` in `deploy/k8s/staging/kustomization.yaml`, publish the DNS records it prints, update the PTR.

## Status

Production MVP: inboxes, threads, messages, send, reply, labels, attachments, custom domains with DNS verification, plan limits, rate limits, signed webhooks, MCP, gatekeeper, staging seed, GitOps deploy. Next: drafts, WebSocket, pods, scoped keys, S3 blob store, IMAP, web console, billing. See `.claude/context/TODO.md`.

Proprietary software, all rights reserved (see `LICENSE`). Approved dependencies: `.clauderules/stack-rules.md`.
