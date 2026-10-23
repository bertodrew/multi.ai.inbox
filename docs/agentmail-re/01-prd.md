# 01 · Product Requirements Document

Product: **multi.ai.inbox**, email inboxes for AI agents, EU first.
Reference product: AgentMail (agentmail.to). Tags: **[observed]** public evidence, **[inferred]** reasoning, **[decision]** our choice.

## 1. Problem

AI agents need to talk to the outside world, and the outside world still runs on email: signups, 2FA codes, invoices, vendor quotes, support threads, calendar invites. Consumer mailboxes (Gmail, Outlook) were built for humans:

* creating an account per agent is manual, rate limited and against ToS at scale [inferred];
* the only automation path is OAuth on a human's mailbox or a computer-use agent clicking UI [observed, product page "No more having your agents click through UI via CUA"];
* transactional APIs (SES, Postmark, Resend) send well but do not model a two way inbox with threads, replies and state [inferred].

AgentMail's tagline captures the gap: "We're not AI for your email, we're email for your AI" [observed].

## 2. Market evidence (from the reference product)

| Signal | Value | Tag |
|--------|-------|-----|
| API traffic | 1B+ requests in 30 days, peak 100K req/min | observed |
| Emails processed | 1B+ | observed |
| Inboxes created | 10M+ | observed |
| Logos | Clay, Replit, Salient, eve, Factory, Alchemy, Polsia, Prime Intellect, Opendoor, Browser Use | observed |
| Backing | Y Combinator company page linked in footer | observed |
| Compliance | SOC 2 Type II (2025), Vanta trust center | observed |
| Case study KPI | AgentCard by Alchemy: about 70% more agentic transactions after integrating | observed |

Conclusion [inferred]: demand is real and dominated by AI native startups that provision inboxes programmatically (one per agent, per user or per customer), not by humans reading mail.

## 3. Ideal customer profiles

1. **AI agent startups** building assistants that sign up to services, receive OTPs, negotiate with vendors, run support. Buy on Developer and Startup tiers.
2. **Platforms embedding email for their own customers** (white label, one pod per tenant): e.g. vertical SaaS, Clay style "email as a column".
3. **Indie hackers and agent tinkerers** using Claude Code, Cursor, OpenClaw, MCP. Free tier, viral loop through agent self sign up.
4. **EU regulated buyers** (legal, fintech, health) who need EU data residency. AgentMail gates EU region behind Enterprise [observed]; this is our wedge [decision].

## 4. Positioning

* Reference: "Email inboxes for AI agents", developer first, API as the product, dark technical brand [observed].
* **multi.ai.inbox** [decision]: "EU hosted inboxes for AI agents. Bring your domains, keep your data in Europe."
  * EU region on every plan, not only Enterprise.
  * Native registrar connectors (Namecheap, Infomaniak) so custom domains are one click, not a DNS tutorial.
  * Open core on a mailcow base: self host option for teams that cannot use SaaS.
  * API shape compatible with the market leader, so migration is a base URL change.

## 5. Goals and non goals

### Goals (first 6 months)
* G1: A developer creates an inbox and receives the first email in under 2 minutes from signup.
* G2: Inbound email reaches the customer's webhook in p95 under 3 seconds after SMTP DATA ends.
* G3: 100 active organizations, 1,000 inboxes, 10 paying customers.
* G4: Zero cost infrastructure until first revenue (Hetzner box already paid, free tiers elsewhere).

### Non goals
* Cold outreach or bulk marketing. AgentMail explicitly refuses cold outreach [observed]; we do the same to protect IP reputation.
* A human email client competing with Gmail. The console shows mail for debugging and human in the loop only.
* Building our own LLM agent. We are the mailbox, not the brain.

## 6. Scope

### 6.1 Core (must have, MVP)
| Capability | Description | Reference |
|-----------|-------------|-----------|
| Organizations and API keys | Org owns everything; keys authenticate the API | observed |
| Inboxes | Create via one API call, random or chosen username, default shared domain, display name | observed |
| Send | Send, reply, reply all, forward; text and HTML; cc, bcc; attachments | observed |
| Receive | Inbound parsed to Message objects, grouped into Threads | observed |
| Threads | Conversation grouping across replies and forwards; org wide queries | observed |
| Webhooks | `message.received`, `message.sent`, `message.delivered`, `message.bounced`, `message.complained`, `message.rejected`, `domain.verified`; signed | observed |
| Console | Sign up, API keys, inboxes, message viewer, webhooks, usage | inferred |
| Free tier | 3 inboxes, 3,000 emails/month, 100/day, 3 GB storage | observed |

### 6.2 Growth (should have, V1)
| Capability | Description | Reference |
|-----------|-------------|-----------|
| Custom domains | Add domain, get zone file, verify SPF, DKIM, DMARC, MX; subdomain inboxes | observed |
| Registrar one click DNS | Namecheap and Infomaniak write the records for the user | decision (we already have the connectors) |
| Drafts and scheduled send | Human in the loop review, send later | observed |
| Labels | Agent workflow state on messages and threads | observed |
| Attachments | Upload, download, text extraction from PDF, Word, Excel | observed |
| Reply extraction | `extracted_text` and `extracted_html` without quoted history | observed |
| WebSockets | Real time events without public endpoint | observed |
| Polling webhooks | Pull events when no public endpoint exists | observed |
| Idempotency | `client_id` on create, `Idempotency-Key` header on send | observed |
| MCP server | Hosted MCP for Claude, ChatGPT, Cursor | observed |
| SDKs | TypeScript and Python generated from OpenAPI; CLI | observed |
| Agent self sign up | An agent creates its own account; limited until a human claims it | observed |
| **Tickets (human side)** | An inbox marked as support intake turns threads into tickets: state, priority, assignment to a human or an agent key, SLA, internal notes. Same thread, no mirror. See [15](15-ticketing.md) | decision |

### 6.3 Scale (could have, V2)
| Capability | Description | Reference |
|-----------|-------------|-----------|
| Pods | Multi tenant workspaces, isolated inboxes, domains, keys, webhooks | observed |
| Scoped permissions | API keys restricted to resources and operations | observed |
| Allow and block lists | Per inbox and per org, separate send, receive, reply policies | observed |
| Metrics | Query events, usage and rates | observed |
| Agent Armor equivalent | Prompt injection screening on inbound mail with review queue | observed |
| Spam and virus | Reject infected, quarantine spam | observed (we get it from rspamd and clamd) |
| IMAP and SMTP access | Use inboxes from any mail client | observed |
| Calendar | Per inbox calendar, iMIP invitations, RRULE, event webhooks | observed |
| Message expiry | Optional 24 hour access expiry | observed |
| AgentID | Agent identity and sign in to third party apps | observed |
| Payments for agents | x402 and Stripe MPP integrations | observed |

### 6.4 Enterprise (later)
White label, dedicated IPs, BYO cloud, OIDC and SAML SSO, SLA, Slack support channel, DPA [observed in reference]. For us EU cloud is not an enterprise item [decision].

## 7. Pricing

Reference [observed]:

| Plan | Price | Inboxes | Emails/mo | Emails/day | Emails/5 min | Storage | Domains | Seats |
|------|-------|---------|-----------|-----------|--------------|---------|---------|-------|
| Free | $0 | 3 | 3,000 | 100 | none | 3 GB | 1 (beta) | 2 |
| Developer | $20/mo | 10 | 10,000 | 1,000 | 100 | 10 GB | 10 | 2 |
| Startup | $200/mo | 150 | not listed | 15,000 | 1,500 | not listed | 150 | 10 |
| Enterprise | custom | unlimited | custom | custom | custom | custom | custom | custom |

Add ons: inbox $2/mo, domain $2/mo, 1,000 sends $2/mo, seat $5/mo; yearly is 20% off. Free tier adds a "Sent via AgentMail" footer.

multi.ai.inbox proposal [decision], priced in EUR, slightly under the reference and with EU included:

| Plan | Price | Inboxes | Emails/mo | Domains | Seats | Notes |
|------|-------|---------|-----------|---------|-------|-------|
| Free | €0 | 3 | 3,000 | 1 | 1 | Footer, shared domain, EU |
| Builder | €15/mo | 10 | 10,000 | 5 | 2 | No footer, registrar one click DNS |
| Team | €149/mo | 150 | 150,000 | 50 | 10 | Pods, scoped keys, metrics, Armor |
| Enterprise | custom | custom | custom | custom | custom | Dedicated IP, self host support, SSO, DPA |

Same add on shape (inbox, domain, 1,000 sends, seat) so upgrade paths are familiar.

## 8. Success metrics

* North star: **weekly active inboxes** (inboxes with at least one inbound or outbound message in 7 days).
* Activation: percentage of new orgs that receive an inbound email within 24 h of signup (target 40%).
* Time to first email (target p50 under 2 min).
* Deliverability: bounce rate under 2%, complaint rate under 0.1% per org (auto pause above).
* Revenue: free to paid conversion 3%, net revenue retention over 110% via add ons.
* Reliability: API availability 99.9%, webhook delivery success 99.5% within retry window.

## 9. Constraints and assumptions

* Team: one technical founder. Shipping speed over perfection.
* Budget: zero to minimal before revenue. Existing Hetzner CX33 (EU) running mailcow.
* Hetzner blocks outbound port 25 until a support ticket is approved [observed in this repo's README]; until then, outbound goes through a relay.
* Stack rules from the founder: TypeScript (Next.js, Supabase with RLS everywhere, Zod on every input), Rust for AI workloads, Go for connectivity.

## 10. Open questions

1. Shared sending domain name (we need a short, neutral domain for free inboxes, like `agentmail.to`).
2. Outbound relay choice before port 25 opens: Amazon SES EU, Scaleway TEM or Brevo (cost versus EU residency).
3. Do we expose IMAP in V1 (cheap with Dovecot) or keep it V2 to avoid support load?
4. Open core licence boundary: mailcow is GPL 3.0; the API and console are new code, decide AGPL versus proprietary.
