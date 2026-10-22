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

One binary, `mai`, runs any subset of roles: `api`, `mx`, `sender`, `dispatcher`, `enrich`. Events use a Postgres outbox (no extra queue until it is needed).

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
| `deploy/` | Docker Compose for a Hetzner server (Postgres, mai, rspamd, Caddy) |

## Run locally

```bash
# Postgres 16 with a non superuser role (RLS does not apply to superusers)
export DATABASE_URL=postgres://mai_app:mai_app@localhost:5432/mai
export MAI_SHARED_DOMAIN=agents.localhost MAI_HOSTNAME=mx.localhost MAI_ALLOW_HTTP_WEBHOOKS=true
(cd gatekeeper && cargo build --release) && export MAI_GATEKEEPER_BIN=$PWD/gatekeeper/target/release/gatekeeper

go run ./cmd/mai migrate
go run ./cmd/mai admin domain-add -name agents.localhost -mx mx.localhost
go run ./cmd/mai admin org-create -name "Acme"             # prints organization_id
go run ./cmd/mai admin key-create -org <organization_id>   # prints api_key once
go run ./cmd/mai serve                                      # API :8080, SMTP :2525

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

The end to end test creates a fresh database owned by a non superuser role and covers SMTP in, RLS isolation between organizations, gatekeeper verdicts, signed webhooks, idempotent replies through a relay with DKIM, agent to agent delivery and MCP.

## Deploy on Hetzner

See `deploy/`: copy `.env.example` to `.env`, set the server PTR to `MAI_HOSTNAME`, open only ports 25 and 443 in the Hetzner firewall, then `docker compose -f deploy/docker-compose.yml up -d`. Until Hetzner unblocks outbound port 25, set `MAI_SMTP_RELAY` to an EU relay.

## Status

MVP of ADR 10, phase M1 to M2 scope: inboxes, threads, messages, send, reply, labels, webhooks, MCP, gatekeeper heuristics. Not yet: custom domain verification API, drafts, WebSocket, pods, scoped keys, S3 blob store, IMAP, web console, billing. See `.claude/context/TODO.md`.

Licence: to be decided (ADR 10, decision D6). The mailcow fork this repository started from (GPL 3.0) was removed; its history stays in git, and none of its code is used here.
