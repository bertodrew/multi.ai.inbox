# 16 · Ticketing: review by CEO, CPO, CTO, CISO

Subject: [15-ticketing.md](15-ticketing.md). Date: 2026-10-23.
Rule of this review: every hat states what it rejects and what it accepts **with conditions**.
A condition is not a comment: it is a change to the plan, listed in §5.

---

## 1. CEO

**Question asked**: does this sell, or does it dilute?

**What I reject**: building a Zendesk competitor. Our ICP is AI agent startups that
provision inboxes programmatically. They do not buy a helpdesk, and we are one person plus
agents. A ticket module that grows macros, portals and CSAT is how the product loses focus
and the roadmap loses two quarters.

**What I accept**: a ticket layer that exists because the *human* needs to answer the mail
their agent handles. That is retention, not a new market. It keeps the customer on our
inbox instead of exporting threads into Zammad.

**Conditions**:
1. **No new SKU.** Tickets are included in the `team` plan. No pricing page change.
2. **No marketing claim** until one real customer runs a queue for 30 days.
3. **Phase D (public portal) is deferred** until a customer asks in writing. It is the door
   to becoming a helpdesk.
4. **Kill criterion**: if in 90 days after phase B no customer creates a ticket, the module
   is frozen at phase B and gets no more investment. Written here so we cannot rationalise
   later.

**Verdict**: APPROVE phases A and B. Reject C until phase B ships, reject D for now.

## 2. CPO

**Question asked**: does this fragment the experience we just shipped?

**What I reject**: a second inbox. If the queue copies the message body or re-implements the
reader, we maintain two viewers and they will diverge. Also rejected: a new top-level nav
item competing with Inbox.

**What I accept**: a ticket is a thread with state. One reader, one composer, one set of
components.

**Conditions**:
1. No message body in `tickets`. The ticket references `thread_id`.
2. The queue reuses the thread reader and the composer, not a copy.
3. "Create ticket" lives **inside the thread reader**, as an action, not in the sidebar.
4. The non-goals in §10 of the plan are contractual: a PR adding macros, CSAT or merge/split
   requires a new document, not a review comment.
5. Empty state of the queue must teach the model: "tickets come from threads in an intake
   inbox", with the action to mark an inbox as intake.

**Verdict**: APPROVE A and B with the conditions above.

## 3. CTO

**Question asked**: does it fit the architecture, and is the estimate honest?

**What I reject**: (a) a scheduler that mutates state on its own, (b) per-message PATCH loops
(the webmail agent already had to do N requests per label change), (c) an SLA clock computed
in the browser.

**What I accept**: the model in the plan, with four corrections.

**Conditions**:
1. **Numbering**: `number` must be allocated under a lock on the organization row
   (`select ... for update` then `max(number)+1`), otherwise two concurrent ticket creations
   collide. Unique index `(org_id, number)` stays as the safety net.
2. **SLA in code**, computed once at creation and recomputed on priority change; the
   scheduler only *marks* breaches, never moves state.
3. **Phase C only after Prometheus exists** (the observability work): an SLA without metrics
   is a promise we cannot verify.
4. **One commit rule**: migration, openapi, console `PERMISSIONS`, MCP tools and tests land
   together. The spec-check failing the build is a feature; do not work around it.
5. RLS on all three tables, plus a test that `mai_app` sees only its organization's tickets.

**Estimate**: A 5 days, B 5 days, C 3 days. Anything beyond 15 days means the scope grew.

**Verdict**: APPROVE, with the corrections.

## 4. CISO

**Question asked**: what can leak, and what can an agent do that a human cannot?

**Threats considered**:
1. An internal note reaching the customer. This is the worst outcome: a note often contains
   "this customer is a pain" or a price floor.
2. An agent with `ticket_write` plus `message_send` replying to a customer without review.
3. Ticket content in logs or in a support bundle.
4. The public portal (phase D) as an unauthenticated write path.
5. GDPR: ticket threads are personal data, and an email can contain Art. 9 data.

**Conditions**:
1. **Notes in their own table** (`ticket_notes`) with no code path to the sender, and a test
   that fails if `sender` ever reads the table. Rejected: an `internal` flag on `messages`.
2. **Export must include tickets, notes and the timeline** in `GET /v0/export`, in the same
   release as phase A. Without it the GDPR answer becomes wrong the day the feature ships.
3. **Every agent action writes `ticket_events` and `audit_log`** with the key id: an agent
   assigning itself a customer's ticket must be attributable.
4. **`ticket_write` alone does not grant reply.** Replying needs `message_send`, so a key can
   triage without the ability to email. Documented in the keys screen.
5. **No ticket body in logs or metrics**; the audit detail carries state transitions only.
6. **Retention**: closed tickets are kept 24 months, then purged by the existing scheduler
   pass; `docs/legal/retention-and-deletion.md` updated in the same release.
7. Phase D (portal): rate limit per IP, honeypot, no data in the response beyond the ticket
   number, and a captcha only if abuse appears.

**Verdict**: APPROVE with the conditions. Condition 2 is blocking for phase A.

---

## 5. What the review changed in the plan

| # | Change | Forced by |
|---|---|---|
| 1 | `ticket_notes` as its own table, with a test that the sender never reads it | CISO |
| 2 | `GET /v0/export` extended to tickets and notes in phase A | CISO |
| 3 | `number` allocated under a lock on the organization row | CTO |
| 4 | Scheduler marks breaches only, never changes state | CTO |
| 5 | Phase C after Prometheus | CTO |
| 6 | Migration, openapi, console permissions, MCP and tests in one commit | CTO |
| 7 | "Create ticket" inside the thread reader, no new nav item | CPO |
| 8 | Non-goals contractual | CPO |
| 9 | No new SKU, no marketing before a real queue runs 30 days | CEO |
| 10 | Phase D deferred; 90-day kill criterion for the module | CEO |
| 11 | Replying requires `message_send`, not just `ticket_write` | CISO |
| 12 | Retention 24 months for closed tickets, doc updated | CISO |

## 6. Consolidated verdict

**All four hats approve phases A and B.** C is approved in principle, gated on the
observability work. D is rejected for now.

Build order: **A** (schema, core, REST, MCP, permissions, export, tests) → **B** (queue and
ticket panel in the console) → **C** after Prometheus.

Nothing in phases A and B may be announced as a product capability until phase B is verified
in staging with a real intake inbox and a human answering a ticket.
