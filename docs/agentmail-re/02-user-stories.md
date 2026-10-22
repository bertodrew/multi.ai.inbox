# 02 · Personas, epics and user stories

Format: `As a <persona>, I want <capability>, so that <outcome>.` Each story has acceptance criteria (AC) and a phase: **P0** MVP, **P1** growth, **P2** scale, **P3** enterprise. See [09 execution plan](09-execution-plan.md) for dates.

## Personas

| Id | Persona | Context | Primary channel |
|----|---------|---------|-----------------|
| DEV | Agent developer | Builds an AI agent in Python or TypeScript, wants an inbox in one call | SDK, REST |
| AGT | The AI agent itself | Runs inside Claude Code, Cursor, OpenClaw or a custom loop; acts through MCP or CLI | MCP, CLI |
| PLT | Platform engineer | Embeds email for many end customers, needs isolation per tenant | REST, pods |
| OPS | Operator / human in the loop | Reviews drafts, flagged mail, deliverability | Console |
| ADM | Org admin / buyer | Billing, seats, security review, domains | Console |
| SUP | multi.ai.inbox staff (founder) | Abuse, suspensions, support | Internal admin |

## Epic E1 · Onboarding and identity

**E1.1 (P0)** As DEV, I want to sign up with Google or email and land on a page with my first API key, so that I can call the API in minutes.
AC: social login and email + password with verification; an org is created automatically; one API key shown once, copy button; quickstart snippet pre filled in Python, TypeScript, cURL; event `org.created` logged.

**E1.2 (P0)** As DEV, I want to create and revoke API keys with a name, so that I can rotate secrets.
AC: key shown once, stored hashed (argon2id or SHA 256 with pepper); prefix visible (`mai_live_xxxx`); last used timestamp; revocation takes effect within 60 s.

**E1.3 (P1)** As AGT, I want to sign up for an account by myself with only my human's email, so that I can get an inbox without a human opening a browser.
AC: `POST /v0/agent/sign-up` returns an API key and an inbox; until a human verifies the attached email, the account can only send to that human's address; daily cap 10 sends; clear error code `agent_unverified` when blocked.

**E1.4 (P1)** As DEV, I want to claim an inbox my agent created, so that it moves to my org with full limits.
AC: human receives a claim link; after claim, limits match the org plan; audit log entry.

**E1.5 (P2)** As ADM, I want to invite teammates with roles (owner, admin, member), so that the team can share the org.
AC: seats count against the plan; role checks enforced by RLS.

## Epic E2 · Inboxes

**E2.1 (P0)** As DEV, I want `POST /v0/inboxes` with optional `username`, `domain`, `display_name`, so that each agent gets its own address.
AC: without username a readable random one is generated; returns `inbox_id` equal to the full address; 409 on taken address; idempotent with `client_id`.

**E2.2 (P0)** As DEV, I want to list, get, update and delete inboxes, so that I can manage their lifecycle.
AC: cursor pagination (`limit`, `page_token`); delete is soft for 30 days then hard purge; deleted addresses cannot be reused for 90 days (anti takeover).

**E2.3 (P2)** As PLT, I want inboxes inside a pod, so that tenant A never sees tenant B.
AC: pod scoped key cannot read outside its pod (403); cross pod thread queries impossible by construction.

## Epic E3 · Sending

**E3.1 (P0)** As DEV, I want to send an email with to, cc, bcc, subject, text, html and attachments, so that my agent can start conversations.
AC: returns `message_id` and `thread_id`; enforces plan limits (month, day, 5 min) with 429 and `Retry-After`; free tier appends footer; `Idempotency-Key` header dedupes for 24 h.

**E3.2 (P0)** As AGT, I want reply, reply all and forward on a message id, so that threading headers are always right.
AC: `In-Reply-To` and `References` set; subject prefixed once; reply lands in the same thread at the recipient (verified on Gmail and Outlook).

**E3.3 (P1)** As OPS, I want agents to create drafts that I approve in the console, so that sensitive mail is reviewed.
AC: draft CRUD; `send_at` for scheduled send; console shows "pending approval" queue; approve sends, reject deletes with reason.

**E3.4 (P2)** As ADM, I want allow and block lists for send and receive, so that an agent never mails outside approved domains.
AC: entries for address or domain; separate directions (send, receive, reply); blocked send returns 403 `recipient_blocked`.

## Epic E4 · Receiving and threads

**E4.1 (P0)** As DEV, I want inbound mail parsed into Message objects grouped by Thread, so that the agent reads conversations, not MIME.
AC: fields from, to, cc, subject, text, html, attachments, headers, labels; thread grouping by `References`, `In-Reply-To`, then normalized subject within 30 days; raw `.eml` retrievable.

**E4.2 (P1)** As AGT, I want `extracted_text` without quoted history and signature, so that I spend fewer tokens.
AC: quoted blocks of Gmail, Outlook, Apple Mail removed; tested on a fixture set of 200 real emails with 95% accuracy.

**E4.3 (P1)** As AGT, I want plain text extracted from PDF, DOCX, XLSX attachments, so that I can read invoices directly.
AC: extraction async, result on the attachment object; size cap 25 MB.

**E4.4 (P1)** As DEV, I want labels on messages and threads, so that my agent tracks workflow state.
AC: system labels (`unread`, `sent`, `received`, `spam`, `draft`); custom labels; filter list endpoints by label.

**E4.5 (P1)** As DEV, I want org wide search over threads and messages, so that I can find context across inboxes.
AC: Postgres full text search (P1), semantic search (P2 with pgvector).

## Epic E5 · Events

**E5.1 (P0)** As DEV, I want webhooks for message received, sent, delivered, bounced, complained, rejected, so that my agent runs on events.
AC: signed with HMAC SHA 256 (Svix compatible headers `webhook-id`, `webhook-timestamp`, `webhook-signature`); at least once with exponential retries over 24 h; per endpoint event type filter and inbox or pod filter; console shows delivery log and replay.

**E5.2 (P1)** As DEV, I want a WebSocket to stream events, so that I do not need a public URL during development.
AC: `wss://` with API key auth; subscribe to inbox ids or pods; heartbeats every 30 s; resume from last event id within 5 min.

**E5.3 (P1)** As AGT, I want polling webhooks, so that I can pull events with no endpoint and no persistent socket.
AC: cursor based `GET` of events; retention 7 days.

## Epic E6 · Domains and deliverability

**E6.1 (P1)** As ADM, I want to add my domain and get the exact DNS records, so that agents send from my brand.
AC: records MX, SPF include, DKIM CNAME or TXT, DMARC, return path; zone file download; verification job every 5 min for 72 h; `domain.verified` event.

**E6.2 (P1)** As ADM, I want "Connect Namecheap" or "Connect Infomaniak" to write the records for me, so that setup takes one click.
AC: single record updates only, never full zone replace (lesson from this repo: `setHosts` wipes records); preview diff before apply; existing MX conflict warning.

**E6.3 (P2)** As ADM, I want automatic warm up and a suppression list, so that a new domain does not land in spam.
AC: per domain daily ramp; hard bounces and complaints auto suppressed; metrics per domain.

## Epic E7 · Agent tooling

**E7.1 (P1)** As AGT, I want a hosted MCP server with tools to create inboxes, send, list threads and read messages, so that Claude or Cursor can use email natively.
AC: streamable HTTP transport; OAuth or API key; tools mirror REST; tested in Claude Code and Cursor.

**E7.2 (P1)** As DEV, I want official SDKs for TypeScript and Python and a CLI, so that integration is trivial.
AC: generated from OpenAPI 3.1; published to npm and PyPI; CLI supports `inbox create`, `send`, `tail` (websocket).

**E7.3 (P1)** As AGT, I want `llms.txt` and `.md` versions of every docs page, so that I can read docs in context.
AC: `llms.txt` index, per page markdown, a copy paste "give yourself access" prompt.

## Epic E8 · Security and safety

**E8.1 (P0)** As SUP, I want spam and virus scanning on inbound, so that agents never ingest malware.
AC: infected rejected at SMTP; spam stored with `spam` label and excluded from default list.

**E8.2 (P2)** As OPS, I want inbound mail screened for prompt injection with a review queue, so that a malicious email cannot steer my agent.
AC: verdict `safe`, `review`, `blocked` with score; `message.security.review` event; release or confirm actions; feedback endpoint improves the model.

**E8.3 (P2)** As ADM, I want scoped API keys (inbox list, read, send, manage), so that each agent has least privilege.
AC: permission matrix enforced in API and RLS; 403 with `missing_permission` detail.

**E8.4 (P0)** As SUP, I want automatic suspension on abuse signals, so that our IPs stay clean.
AC: complaint rate over 0.3% or bounce over 8% over a rolling 1,000 sends pauses sending; owner notified; appeal flow.

## Epic E9 · Billing and usage

**E9.1 (P1)** As ADM, I want to upgrade, downgrade and buy add ons, so that I pay for what I use.
AC: Stripe Checkout and Customer Portal; prorated increases, decreases at renewal; limits update within 1 min of webhook.

**E9.2 (P1)** As ADM, I want a usage page (emails sent, received, storage, inboxes), so that I know when I hit limits.
AC: daily aggregates; alert emails at 80% and 100%.

## Epic E10 · Calendar and identity (P3)

**E10.1** As AGT, I want a calendar per inbox that fills from emailed invitations, so that I can coordinate schedules.
**E10.2** As AGT, I want to accept or decline iMIP invitations by API.
**E10.3** As AGT, I want to sign in to third party apps with an agent identity, so that I can create accounts without a human.
