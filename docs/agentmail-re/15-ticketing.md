# 15 · Ticketing: the human side of an agent inbox

Status: **proposed**, to be reviewed by CTO, CPO, CISO, CEO (see `16-ticketing-review.md`).
Decision: build it in-house. No third-party desk (licence: we resell; AGPL forces source
disclosure on a network service, GPL is copyleft, and the MIT-only options are immature).

## 1. Why in-house

The product already owns 70% of a service desk:

| Service desk piece | Already in multi.ai.inbox |
|---|---|
| Threads, messages, attachments | `threads`, `messages`, blob store |
| Reply / compose | console composer, `messages/send` |
| Labels | message labels |
| Agent access | MCP server + REST |
| People and roles | `users`, `memberships` (owner/admin/member) |
| Inbound and outbound mail | mx, sender, relay |

What is missing is the **ticket layer**: state, assignment, priority, SLA, internal notes,
queue. Building it keeps one stack, one auth model, one backup, one audit log, and lets us
sell it without a licence obligation.

**The differentiator**: a ticket *is* a thread. The human and the agent work on the same
conversation, with the same messages, instead of syncing two systems (desk and mailbox).

## 2. Product position

* Tickets are **opt-in per inbox** (`inboxes.ticket_intake`). An inbox that is not a
  support queue behaves exactly as today.
* Ticket state lives on the thread, not on a copy: no mirror entity, no drift.
* The agent is a first-class assignee (`assignee_key_id`), beside human assignees.
* Not a Zendesk competitor: no multi-brand portals, no voice, no CSAT surveys in v1.

## 3. Data model

```sql
-- 0013_tickets.sql (additive; no change to existing tables except one column)

alter table inboxes add column ticket_intake boolean not null default false;

create table tickets (
  id              uuid primary key default gen_random_uuid(),
  org_id          uuid not null references organizations(id) on delete cascade,
  number          bigint not null,              -- per org, human readable: T-1042
  inbox_id        text not null,
  thread_id       uuid not null,
  subject         text not null,
  requester       text not null,                -- email of the requester
  state           text not null default 'new'
                  check (state in ('new','open','pending','resolved','closed')),
  priority        text not null default 'normal'
                  check (priority in ('low','normal','high','urgent')),
  assignee_user   uuid references users(id) on delete set null,
  assignee_key    uuid references api_keys(id) on delete set null,
  sla_policy      text,
  due_first_reply timestamptz,
  due_resolution  timestamptz,
  first_reply_at  timestamptz,
  resolved_at     timestamptz,
  closed_at       timestamptz,
  tags            text[] not null default '{}',
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  unique (org_id, number),
  unique (thread_id)
);

create table ticket_notes (           -- internal only, never sent by email
  id         uuid primary key default gen_random_uuid(),
  org_id     uuid not null references organizations(id) on delete cascade,
  ticket_id  uuid not null references tickets(id) on delete cascade,
  author_user uuid references users(id) on delete set null,
  author_key  uuid references api_keys(id) on delete set null,
  body       text not null,
  created_at timestamptz not null default now()
);

create table ticket_events (          -- timeline and audit trail
  id         bigserial primary key,
  org_id     uuid not null references organizations(id) on delete cascade,
  ticket_id  uuid not null references tickets(id) on delete cascade,
  kind       text not null,          -- created, state, priority, assigned, note, reply, sla_breach
  actor_user uuid references users(id) on delete set null,
  actor_key  uuid references api_keys(id) on delete set null,
  detail     jsonb not null default '{}',
  created_at timestamptz not null default now()
);
```

RLS: all three tables get the tenant policy (`org_id = current_setting('mai.org_id')`),
identical to the other tenant tables. `ticket_notes.body` is never exposed to the
mail path: there is no code path from notes to the sender.

### Why a separate `ticket_notes` table

An internal note must be impossible to email by accident. Reusing `messages` with an
`internal` flag would make a single forgotten `where internal = false` a data breach.

## 4. State machine

```
             ┌──────────── reopen ────────────┐
             ▼                                │
  new ──► open ◄──► pending ──► resolved ──► closed
   │        │                      │
   └────────┴──────────────────────┘
```

* `new`: created, nobody touched it.
* `open`: a human or an agent is working on it.
* `pending`: waiting on the customer (SLA clock paused).
* `resolved`: answered, waiting for confirmation; reopens on a new inbound message.
* `closed`: terminal for this number. Fase A keeps one ticket per thread, so a new message
  in a closed thread reopens the same ticket with `PATCH` and the queue shows the count
  it already had; a second number on the same thread (the design sketched below) needs
  the numbering to stop being unique per thread, which is Fase B work.

Transitions are explicit and audited. Every transition writes a `ticket_events` row.

## 5. SLA

Policy per plan, stored in code (not in the database) so it is reviewable next to the
pricing page:

| Plan | First reply | Resolution |
|---|---|---|
| free | none | none |
| builder | 24 h | 5 business days |
| growth | 8 h | 3 business days |
| team | 4 h | 2 business days |

Priority multipliers: `urgent` ×0.25, `high` ×0.5, `normal` ×1, `low` ×2.
Business hours: Monday to Friday, 09:00 to 18:00 CET, computed in `core` with an explicit
holiday list per year (a single function, unit tested).

A scheduler pass every five minutes:

* marks `sla_breach` events for overdue tickets (once per due date),
* moves nothing on its own: state changes stay human or agent decisions.

## 6. API

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/v0/tickets` | `ticket_read` | filters: `state`, `assignee`, `priority`, `q`, `inbox`, cursor |
| POST | `/v0/tickets` | `ticket_write` | from an existing thread, or subject+requester+body to create one |
| GET | `/v0/tickets/{id}` | `ticket_read` | ticket + timeline + thread id |
| PATCH | `/v0/tickets/{id}` | `ticket_write` | state, priority, tags, assignee |
| POST | `/v0/tickets/{id}/notes` | `ticket_write` | internal note, never emailed |
| POST | `/v0/tickets/{id}/reply` | `message_send` | sends through the existing sender, marks first reply |
| GET | `/v0/tickets/stats` | `ticket_read` | counts by state, SLA breaches, median first reply |

New permissions: `ticket_read`, `ticket_write`. Both must be added to the console
`PERMISSIONS` list and to `openapi.yaml` (the build fails otherwise, by design).

## 7. MCP tools

Same operations, agent first:

| Tool | What an agent does with it |
|---|---|
| `create_ticket` | open a ticket from a thread it is handling |
| `list_tickets` | find work: unassigned, breached, waiting on the customer |
| `get_ticket` | read state, timeline, thread |
| `update_ticket` | state, priority, tags |
| `assign_ticket` | take it, or hand it to a human |
| `add_ticket_note` | leave reasoning for the human, without emailing the customer |
| `reply_ticket` | answer the customer through the thread |

`message_release` and `message_review` stay unchanged: a ticket never bypasses the
gatekeeper.

## 8. Console UI

Extends the existing `console/inbox` three panes, adds one route:

* `/console/tickets`: queue. Columns: number, subject, requester, state, priority,
  assignee, SLA countdown. Filters and saved views (`assigned to me`, `unassigned`,
  `breaching`).
* `/console/tickets/[id]`: the existing thread reader on the right, ticket panel on the
  left (state, assignee, priority, tags, SLA), notes tab.
* `/console/inbox/[thread]`: a "Create ticket" action on any thread.

No second design language: same components, same rows, same keyboard behaviour.

## 9. Phases

| Phase | Scope | Effort | Exit criterion |
|---|---|---|---|
| **A** | migration 0013, `core/tickets.go`, REST, MCP tools, permissions, tests | 1 week | `go test ./...` green, MCP `create_ticket` and `list_tickets` work against staging |
| **B** | console queue + ticket panel + "create ticket" from a thread | 1 week | a human can run a queue end to end in staging |
| **C** | SLA scheduler + stats + breach alerts on the existing check | 4 days | a forced overdue ticket produces one `sla_breach` event and one alert |
| **D** | public customer portal (form to intake inbox) | 4 days | a form submission creates a ticket, rate limited, honeypot |

Ship A and B before any marketing claim. C is required before promising SLA.

## 10. Non-goals for v1

* Multi-brand / per-tenant portals (the domain plumbing does not exist yet).
* CSAT surveys, canned responses library, macros, time tracking.
* Voice, WhatsApp, social channels: email only, the product's own channel.
* Ticket merge and split.

## 11. Risks

| Risk | Mitigation |
|---|---|
| An internal note leaks to a customer | separate table, no code path to the sender, test asserting the sender never reads notes |
| SLA promises on a single node | publish SLA only after phase C and after the second server |
| Agent assignment becomes an unaudited actor on personal data | every agent action writes `ticket_events` + `audit_log` with the key id |
| Scope creep toward a full desk | non-goals above are contractual: a PR adding macros needs a new document |
| The queue becomes a second inbox to maintain | one thread, one reader, no copy of the message body in `tickets` |

## 12. What Fase A shipped

Built in this repository, no third-party desk, MIT-only dependencies (the Go and
Next stacks we already ship).

| Piece | Where |
|---|---|
| Schema: `inboxes.ticket_intake`, `tickets`, `ticket_notes`, `ticket_events`, RLS on all three | `internal/db/migrations/0013_tickets.sql` |
| State machine, SLA in business hours, notes, reply, stats, intake | `internal/core/tickets.go` |
| Intake on ingest, same transaction as the thread | `internal/core/ingest.go` |
| HTTP: `GET/POST /v0/tickets`, `GET /v0/tickets/{ticket}`, `PATCH`, `/notes`, `/reply`, `/stats` | `internal/api/tickets.go` |
| MCP: `create_ticket`, `list_tickets`, `get_ticket`, `update_ticket`, `assign_ticket`, `add_ticket_note`, `reply_ticket` | `internal/mcpserver/tickets.go` |
| Permissions `ticket_read`, `ticket_write` (member reads, does not write) | `internal/core/access.go`, `users.go` |
| Console queue with filters, state moves, notes, reply, timeline | `web/src/components/console/TicketsView.tsx`, `/console/tickets` |
| Tickets in the GDPR export (Art. 15 and 20) | `internal/core/export.go` |
| Tests: transitions, business hours, SLA, breach, permission separation | `internal/core/tickets_test.go` |

Decisions taken while building, worth knowing:

* the number is allocated with `pg_advisory_xact_lock` on the organization, not a
  row lock, so the writer needs no `UPDATE` on `organizations`;
* `assignee_key` exists because an agent can own a ticket; the console shows the
  assignment and leaves handing over to the API and MCP, where the actor is
  known;
* the SLA table lives in code next to the pricing page, in business hours
  (Mon to Fri, 09:00 to 18:00 CET), free plan has no promise.

Still open, in order: Fase B (agent closes a ticket from the reply, saved views,
SLA per plan editable by the operator), Fase C (second server, then publish the
SLA), Fase D (public portal, deferred until a customer asks in writing).
