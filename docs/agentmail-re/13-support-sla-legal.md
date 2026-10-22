# 13 · Support, SLA and legal pack

Author role: tech paralegal and customer support operations lead. Date: 2026-10-22. Status: proposal for the founder.

> **Draft, not legal advice, review by counsel.** Every policy text in this document is a starting draft written from public benchmarks and from what the repository actually implements. A qualified lawyer in the chosen member state must review it before anything is published or signed. Placeholders in square brackets (for example `[Company legal name]`) must be filled with real facts; none are invented here.

Inputs read: `README.md`, [01 PRD](01-prd.md) (plans, ICPs), [06 architecture](06-architecture-infrastructure.md), [08 security](08-security-compliance.md), [09 execution plan](09-execution-plan.md) section 8, [10 ADR](10-native-platform.md), `deploy/hetzner/AGENT_RUNBOOK.md`, `deploy/k8s/base/*.yaml`, `internal/core/core.go`, `internal/core/access.go`, plus spot checks in `internal/api`, `internal/blob`, `internal/dispatcher`, `gatekeeper/`. External sources were accessed on 2026-10-22 and are cited inline.

Known facts this pack relies on: solo founder, pre revenue, staging only (`api.2-28-99-252.sslip.io`), EU company with the member state not yet chosen, hosting at Hetzner Online GmbH (Germany), outbound relay Brevo (France), code and container images on GitHub, B2B product (API and MCP server giving AI agents email inboxes), we act as processor for email content, the Gatekeeper flags prompt injection but cannot guarantee safety, proprietary licence, one server today.

## 1. Executive summary

**What to publish at launch** (the moment a stranger can create an account and receive real internet mail):

| # | Document | Why it is needed at launch | Draft in this file |
|---|----------|----------------------------|--------------------|
| 1 | Terms of Service (B2B only) | Contract basis, liability cap, suspension right, AI screening disclaimer | key clauses, section 4.5 |
| 2 | Acceptable Use Policy | Protects shared IP and domain reputation; basis for suspensions | full draft, section 4.3 |
| 3 | Privacy Policy (for our own controller data: account holders, website, support) | GDPR Art. 13 information duty | outline only, section 4.6 |
| 4 | Data Processing Agreement, incorporated by reference in the Terms | GDPR Art. 28(3) requires a contract for every customer that puts personal data in inboxes | outline with clauses, section 4.4 |
| 5 | Subprocessor list (public page) | Art. 28(2) general authorisation needs a list and change notices | table, section 4.4 |
| 6 | Support Policy | Sets expectations a solo founder can meet | full draft, section 4.1 |
| 7 | Status page | Single honest source during incidents; also the only channel that works if our own mail is down | setup, section 2.7 |
| 8 | Security page | Lists measures that exist, nothing more | use the evidence table, section 7 |
| 9 | Legal notice / imprint and DSA points of contact | Required by EU ecommerce rules and DSA Art. 11 and 12 for intermediary services | placeholders, section 4.5 |

**What can wait:**

| Item | Wait until | Reason |
|------|------------|--------|
| Financial SLA (service credits) for Team | second node, PITR backups and external monitoring are live (section 3.8) | a credit promise on one node is a promise we will break |
| Enterprise 99.9% SLA, shared Slack, 24x7 Sev1 | HA architecture and a second on call person | 99.9% is 43 minutes per month; one person cannot hold it |
| SOC 2, ISO 27001 | doc 08 roadmap (€10K MRR) | do not mention them before an audit exists |
| Transparency reports (DSA Art. 15) | likely exempt while a micro or small enterprise, confirm with counsel | see section 6.2 |
| Signed custom DPAs and redlines | first Enterprise deal | click through DPA covers self serve plans |

**Top risks found while reading the repo** (each is reflected in the drafts so we do not over promise):

1. Virus scanning: clamd now runs in `deploy/k8s/base/clamd.yaml` and the SMTP edge rejects infected inbound mail (`internal/mx/clamd.go`). By default mail is accepted unscanned (label `av-unscanned`) while clamd is down, and agent to agent mail and attachments uploaded through the API are not scanned. Claim "inbound mail from the internet is scanned for malware", not more.
2. Inbox deletion now removes the raw `.eml` files and attachments from the blob store through a durable queue (`0006_deletion.sql`, processed by the scheduler within minutes), and the address is quarantined for 90 days (doc 08, T6). Still open: the nightly backup uses `rclone copy` for blobs (`deploy/k8s/base/backup.yaml`), so deleted blobs stay in Object Storage, and database dumps keep deleted rows for 30 days. Deletion promises must state the backup retention.
3. Backups are nightly `pg_dump` plus `rclone sync` of blobs (`deploy/k8s/base/backup.yaml`); Postgres dumps are kept 30 days. RPO is up to 24 hours, and backups silently do nothing until the Object Storage keys are set (runbook section 0). No restore drill exists, so no RTO can be promised yet.
4. Every deploy restarts the single `mai` pod (`strategy: Recreate` in `mai.yaml`), so API and SMTP blink on each merge to `master`. Deploys must count as maintenance or be excluded explicitly.
5. Encryption at rest is not configured in the repo (local path volumes, `sslmode=disable` inside the cluster). TLS exists on the public API via Caddy and Let's Encrypt, and STARTTLS on SMTP when a certificate is configured.
6. If support runs on our own platform, an outage also takes down support. A fallback channel outside our infrastructure is mandatory.

## 2. Support model for a solo founder

### 2.1 Benchmarks (accessed 2026-10-22)

| Peer | Channels by plan | Response targets | Source |
|------|------------------|------------------|--------|
| AgentMail | Discord on all plans; email from Developer ($20); Slack channel and SLA only on Enterprise | not published on pricing page | [agentmail.to/pricing](https://www.agentmail.to/pricing) |
| Resend | ticket support on Free; dedicated Slack channel on Scale ($90); "guaranteed response times" on Enterprise | numbers not published on pricing page | [resend.com/pricing](https://resend.com/pricing) |
| Supabase | tickets; faster targets on Enterprise | Team: Urgent 24 h (24x7), High 1 business day, Normal 1 business day, Low 2 business days. Enterprise: Urgent 1 h (24x7), High 2 business hours, Normal 1 business day, Low 2 business days; business hours 6am to 6pm local | [supabase.com/sla](https://supabase.com/sla) |
| Postmark | not stated on the support page we checked | not found | [postmarkapp.com/support](https://postmarkapp.com/support) |

Takeaway: peers at our price points (Free, €15, €149) offer community plus email with no contractual targets; contractual targets and Slack start at enterprise level. Our model below copies that shape and publishes targets, not guarantees, until Enterprise.

### 2.2 Channels and hours

Support hours: Monday to Friday, 09:00 to 18:00 CET/CEST, excluding public holidays in `[member state]`. All targets below count business hours only, except where marked.

| Plan | Channels | Language |
|------|----------|----------|
| Free | Docs, community (Discord server `[invite link]`), GitHub Discussions or public issues in `[public docs repo]`, status page | English |
| Builder (€15) | Free channels plus email `support@[domain]` | English, `[other languages if any]` |
| Team (€149) | Builder channels plus priority email (`priority@[domain]` or a priority tag on verified Team accounts), Sev1 hotline form on status page | as above |
| Enterprise (later) | Shared Slack Connect channel, named contact, contractual targets | as contracted |
| Abuse and legal (everyone, including non customers) | `abuse@[domain]`, `legal@[domain]`, DSA point of contact | English plus `[official language of member state]` |

Fallback: the status page (GitHub Pages) and a secondary address on an external mail provider `[fallback address]` must be listed, because `support@` on our own platform is unreachable during a full outage.

### 2.3 Severity definitions (email API specific)

| Sev | Definition | Concrete examples |
|-----|------------|-------------------|
| Sev1 Critical | Platform wide outage or data exposure; no workaround | API returns 5xx or times out for all orgs; port 25 refuses or drops connections so inbound mail is not accepted for any domain; suspected cross tenant data leak; leaked secrets; all outbound sends failing |
| Sev2 High | Major function degraded for one or more customers; workaround hard | webhooks not delivered or delayed more than 15 minutes for an org; inbound accepted but not visible via API; sends queued and not leaving; custom domain verification broken; Gatekeeper not producing verdicts |
| Sev3 Normal | Partial or single feature problem with a workaround; deliverability issues | messages land in recipients' spam folders; a specific recipient domain bounces; MCP tool returns an error for one call shape; rate limit questions |
| Sev4 Low | Questions, docs, feature requests | "how do I verify webhook signatures", SDK usage, pricing, feature ideas |

The customer proposes a severity; we may reclassify with a written reason. Abuse reports and security reports are triaged as Sev2 or higher regardless of plan.

### 2.4 First response and update targets

Targets, not guarantees, until an Enterprise contract says otherwise. Business hours unless noted.

| Severity | Free | Builder | Team | Enterprise (only with section 3.8 tier E) |
|----------|------|---------|------|--------------------------------------------|
| Sev1 first response | status page update within 1 h of detection during support hours; best effort otherwise | 4 business hours | 2 business hours, best effort outside hours | 1 h, 24x7 |
| Sev1 updates | status page every 60 min while active | status page every 60 min | status page plus email every 60 min | every 30 min |
| Sev2 first response | best effort | 1 business day | 4 business hours | 2 business hours |
| Sev2 updates | none | daily | every 4 business hours | every 2 hours |
| Sev3 first response | best effort | 2 business days | 1 business day | 1 business day |
| Sev4 first response | best effort | 3 business days | 2 business days | 2 business days |

Platform wide incidents are always communicated on the status page, whatever the plan.

### 2.5 Escalation path and after hours policy

1. Intake: support inbox or Discord. AI triage agent labels severity and drafts a reply (section 2.10).
2. Founder reviews the queue at fixed slots: 09:00, 13:00, 17:00 CET, plus immediately on Sev1 alerts.
3. Engineering escalation: the founder is also the engineer; the runbook (`deploy/hetzner/AGENT_RUNBOOK.md`) gives rollback in under 5 minutes via `git revert` and Flux.
4. Vendor escalation: Hetzner support (server, network, port 25), Brevo support (relay), GitHub status (CI, GHCR).
5. Backup person: `[named backup contact or contractor]` with read only access and the status page credentials, for absences longer than 2 business days. **Decision for the founder.**

After hours: an external probe failure pages the founder's phone (free tier of a monitor or Upptime issue notification to the GitHub mobile app). Outside support hours we act on Sev1 platform outages on a best effort basis; no other after hours work is promised. Planned absences are announced on the status page with the reduced targets.

### 2.6 Zero cost tooling

| Need | Tool | Note |
|------|------|------|
| Shared inbox | our own platform: `support@[domain]` as an inbox, threads read via API or MCP | dogfooding; Gatekeeper screens inbound support mail |
| Fallback inbox | `[external provider address]` | required, see 2.2 |
| Public issues | GitHub Issues or Discussions in a public docs repo | never in the private code repo; template warns not to paste message contents or keys |
| Status page | [Upptime](https://github.com/upptime/upptime.js.org/blob/master/docs/configuration.md) on GitHub Pages | supports HTTP checks and `tcp-ping` to any port; scheduled GitHub Actions cannot run faster than every 5 minutes, and users report runs drifting to about 18 minutes ([discussion 112](https://github.com/orgs/upptime/discussions/112)). Good enough for a status page, too coarse for SLA math (section 3.3) |
| Community | Discord | no customer data in Discord; it is not a subprocessor because we never put customer data there |
| Macros and KB | Markdown files in the docs repo | versioned, reviewable |
| Metrics | a weekly script over the support inbox via our API (labels and timestamps) | gives FRT and TTR without a helpdesk product |

### 2.7 Macros (templates) to prepare

1. Acknowledgement with severity and next update time.
2. Request for diagnostics: org id, inbox address, message id, request id, timestamps in UTC, never the API key.
3. "Please rotate your API key" (key pasted in a ticket or public place).
4. Webhook troubleshooting: signature verification (Standard Webhooks and Svix headers), retries, HTTPS requirement.
5. Deliverability: SPF, DKIM, DMARC records, warm up, recipient side filtering is outside our control.
6. Custom domain verification checklist.
7. Rate limit and plan limit explanation (`rate_limit_exceeded`, daily and monthly caps).
8. Gatekeeper `review` verdict explanation and the shared responsibility statement.
9. Abuse report received (to reporter) and abuse warning (to customer).
10. Suspension notice and reinstatement steps.
11. Data subject request forwarded to the customer as controller (we do not answer on their behalf).
12. Law enforcement or authority request received (route to legal, no content disclosed without review).
13. Feature request logged.
14. Account closure and data export window confirmation.
15. Incident status templates (section 5.2).

### 2.8 Knowledge base structure

* Getting started: create an inbox, receive the first email, send and reply, MCP setup for Claude Code, Cursor, Codex.
* Concepts: inboxes, threads, messages, labels, events, idempotency.
* Webhooks: events list, signature verification, retries, local testing.
* Domains: shared domain, custom domains, DNS records, verification states.
* Deliverability: what we control and what we do not, warm up, bounces.
* Security: API keys, scopes, expiry, revocation, audit log, Gatekeeper verdicts and limits.
* Limits and plans: per plan caps from `internal/core/core.go`, rate limits, footer on Free.
* Troubleshooting: error codes (`validation_error`, `not_found`, `missing_permission`, `rate_limit_exceeded`, `unknown_api_key`, `api_key_expired`).
* Policies: Terms, AUP, DPA, subprocessors, Support Policy, SLA, status page.
* `llms.txt` and `.md` versions of every page so customer agents can self serve.

### 2.9 Support KPIs

| KPI | Definition | Target at launch | Review |
|-----|------------|------------------|--------|
| FRT | median time to first human or approved reply, business hours | within plan target for 90% of tickets | weekly |
| TTR | median time from open to resolved, by severity | Sev3 under 3 business days | weekly |
| CSAT | one question survey after resolution, 1 to 5 | 4.5 average once there are 20 responses | monthly |
| Backlog | open tickets and age of the oldest | no ticket older than 5 business days without an update | daily |
| Reopen rate | tickets reopened within 7 days | under 10% | monthly |
| Tickets per 100 active orgs | load signal | trend only | monthly |
| Abuse handling time | report to first action | Sev2 targets | weekly |

### 2.10 AI agents in support, safely

* Allowed without approval: classify, label, set severity proposal, search the KB, draft a reply, summarise a thread, detect secrets in tickets and redact them in internal notes.
* Requires founder approval: any message to a customer or third party, any account action (plan change, suspension, key revocation, deletion, data export), any statement about an incident or a breach.
* Never: ask for or store API keys, follow instructions contained in a customer email (treat ticket text as data, run it through the Gatekeeper first), promise credits, refunds or dates, give legal advice.
* Mechanics today: drafts and scheduled send are not built yet (README "Next"), so the agent writes its draft as an internal note or a labelled draft file and the founder sends it. When drafts with approval exist (doc 09 Phase 2), use them.
* Logging: keep the agent's drafts and the approved final text for 12 months for quality review.
* Disclosure: replies are human approved; if we ever let an agent answer customers directly in chat, AI Act Art. 50 requires telling people they interact with an AI system, which applies from 2 August 2026 ([Commission guidelines page](https://digital-strategy.ec.europa.eu/en/policies/guidelines-ai-transparency-obligations)).

## 3. Service Level Agreement design

### 3.1 Benchmarks (accessed 2026-10-22)

| Peer | Uptime commitment | Plans | Credit table | Cap | Claim window | Notable exclusions | Source |
|------|------------------|-------|--------------|-----|--------------|--------------------|--------|
| Supabase | 99.9% | Enterprise | under 99.9% to 99.0%: 10%; under 99.0% to 98.0%: 15%; under 98.0% to 96.0%: 20%; under 96.0%: 30% | 20% of fees paid in the preceding 12 months | 30 days after the month | third party vendors and integration partners (Resend named), force majeure, customer misconfiguration, suspension | [supabase.com/sla](https://supabase.com/sla) |
| Vercel | 99.99% | Enterprise | 99.1% to 99.98%: 10%; 95% to 99%: 25%; under 95%: 50% | 50% of the month's fees | 30 days from eligibility | third party vendors, the Internet, customer software, force majeure; API and CLI not covered | [vercel.com/legal/sla](https://vercel.com/legal/sla) |
| Mailgun | 99.99% | API, SMTP and outbound delivery services on its status page | 5% of monthly fee per 30 minutes of unavailability after the first 0.01% | 50% of the period's fees | 30 days after the downtime | customer in breach of ToS or AUP, DoS, hacking, circumstances outside control; maintenance with best effort notice, emergency without notice | [mailgun.com/legal/sla](https://www.mailgun.com/legal/sla/) (page modified 2025-07-30) |
| Resend | 99.99% | Enterprise | not on pricing page | | | | [resend.com/pricing](https://resend.com/pricing) |
| AgentMail | "SLA" | Enterprise only | not public | | | | [agentmail.to/pricing](https://www.agentmail.to/pricing) |

Takeaways: financial SLAs are an enterprise feature everywhere; credits are a percentage of the monthly fee, capped (20% to 50%), claimed within 30 days, sole remedy; third party and customer caused downtime is always excluded.

### 3.2 Recommended tiers (honest for one node)

| Plan | Availability statement | Financial credits | Condition to switch on |
|------|------------------------|-------------------|------------------------|
| Free | best effort, no target | none | |
| Builder | best effort, internal objective 99.0% monthly, not contractual | none | |
| Team | 99.5% monthly uptime commitment | yes, table 3.5 | tier T architecture in 3.8 is live and has run 60 days with measured uptime above 99.5% |
| Enterprise | 99.9% monthly | yes, table 3.5 | tier E architecture in 3.8 plus a second on call person |

Until Team credits are switched on, Team gets the same best effort statement plus the faster support targets in 2.4. Marketing must not print "99.9%" (doc 01 section 8 lists it as a goal; keep it internal).

Budget per 30 day month (43,200 minutes): 99.0% allows 432 minutes (7 h 12 min) of downtime; 99.5% allows 216 minutes (3 h 36 min); 99.9% allows about 43 minutes.

### 3.3 Measurement method

* Probes: an external monitor outside Hetzner, at least every 1 minute, from two locations, against `https://[api host]/healthz` (expects HTTP 204, exists today: `mai.yaml` liveness probe and runbook step 4) and a TCP connect plus SMTP banner on port 25 of each MX host. Upptime alone is too coarse for SLA math (5 minute minimum, irregular runs); use it for the public page and a 1 minute monitor `[tool to choose]` for SLA data.
* A minute is down when both locations fail the same check in that minute. A minute is down for the "Service" if the API check or the SMTP check is down.
* Monthly Uptime Percentage = (total minutes in the calendar month minus Downtime minutes) / total minutes, per region, calculated in UTC.
* Webhook delivery and outbound delivery to recipients are not part of the uptime figure (they depend on customer endpoints and recipient servers); they are tracked as internal SLOs (doc 06: webhook first attempt p95 under 3 s).
* Raw probe data is kept 13 months so customers can audit a claim.

### 3.4 Definitions

* **Downtime**: minutes in which the API endpoint does not return a successful response to the health check, or the SMTP service on port 25 does not accept a connection, as measured in 3.3, excluding Exclusions and Scheduled Maintenance.
* **Monthly Uptime Percentage**: as defined in 3.3.
* **Scheduled Maintenance**: work announced on the status page at least 5 business days ahead (Team) or 10 business days ahead (Enterprise), inside the window `[Tuesday or Wednesday 05:00 to 07:00 CET]`, at most 4 hours per month. Deploys that restart the single pod count as Scheduled Maintenance only when inside the window; otherwise their minutes count as Downtime.
* **Emergency Maintenance**: urgent work to fix a security vulnerability or prevent imminent harm, announced as early as possible. Counts as Downtime for Enterprise; excluded for Team up to 2 hours per month.
* **Service Credit**: a percentage of the monthly fee for the affected plan, applied to a future invoice.

Note: while one node runs Postgres and the `mai` pod, inbound SMTP from other mail servers is normally retried by the sender's server for a period after a temporary failure, so short outages usually delay mail rather than lose it. This is a property of SMTP, not a promise, and must not be marketed as "no mail lost".

### 3.5 Service credit table

| Monthly Uptime Percentage | Team credit | Enterprise credit |
|---------------------------|-------------|-------------------|
| below 99.9% and at least 99.5% | none | 10% |
| below 99.5% and at least 99.0% | 10% | 25% |
| below 99.0% and at least 95.0% | 25% | 50% |
| below 95.0% | 50% | 50% plus termination right if two consecutive months |

Cap: 50% of the monthly fee for the affected month (in line with Vercel and Mailgun). Credits are the sole and exclusive remedy for unavailability, are not cash, and lapse at termination. Free and Builder: no credits.

### 3.6 Claim process

1. Customer emails `support@[domain]` with subject "SLA credit claim", within 30 days after the end of the affected month (Supabase, Vercel and Mailgun all use 30 days).
2. Claim lists dates, times in UTC, affected service and request ids if any.
3. We answer within 10 business days with our probe data. Approved credits are applied to the next invoice.
4. Customer must be current on payments and not in breach of the AUP.

### 3.7 Exclusions

* Customer side: customer code, agents, prompts, webhook endpoints, DNS records, misconfiguration, exceeded plan or rate limits, revoked or expired keys.
* Third party relay and networks: Brevo relay outages, recipient mail servers, DNS providers, registrars, internet backbone, Let's Encrypt issuance outages, GitHub outages that block deploys.
* Force majeure.
* Suspension under the AUP or the Terms, including abuse circuit breakers.
* Beta, preview or labelled experimental features (Gatekeeper classifier, MCP tools marked beta, agent self sign up).
* Recipient side deliverability: spam foldering, blocklists, bounces, greylisting.
* Scheduled Maintenance and, for Team, Emergency Maintenance within the allowance.
* Attacks (DDoS) beyond reasonable mitigation, as Mailgun does; counsel to confirm this is fair for Enterprise.

### 3.8 RPO, RTO and the architecture each tier needs

Today (tier 0, staging): one Hetzner CX33, k3s single node, `postgres` StatefulSet with one replica on a `local-path` volume, `mai` Deployment with `replicas: 1` and `Recreate`, blobs on a 50 Gi local volume, nightly backup at 02:17 cluster time to Object Storage, dumps kept 30 days.

| Statement | Today | Tier T (Team 99.5%) | Tier E (Enterprise 99.9%) |
|-----------|-------|----------------------|---------------------------|
| RPO (max data loss on a disk or node loss) | up to 24 h, and unbounded if the backup secret is missing or the job fails silently | 15 min or less | 5 min or less |
| RTO (time to restore service) | not stated: no restore drill exists | 4 h, tested quarterly | 1 h, tested quarterly |
| What changes | | second Hetzner node; Postgres WAL archiving (PITR) to Object Storage; S3 blob store (TODO item 5) so blobs leave the node; backup success alerts; external 1 minute monitoring; `mai` API with 2 replicas and rolling updates (SMTP edge can stay single with a second MX added); quarterly restore drill | doc 06 stage S2 and doc 10 "Native v1" to "Scale": two edge nodes as MX 10 and MX 20 with separate IPs, Hetzner Load Balancer for HTTPS, Postgres primary plus replica with automated failover, NATS or equivalent once needed, separate outbound IP pools, second on call person, runbooks for IP blocklisting and DKIM rotation |
| Docs | | [06 §4 S1](06-architecture-infrastructure.md#4-scalability-path), [10 §3 Native v1](10-native-platform.md#3-hetzner-deployment) | [06 §4 S2](06-architecture-infrastructure.md#4-scalability-path), [10 §3 Scale](10-native-platform.md#3-hetzner-deployment) |

Publish only the "Today" column at launch: "Backups run nightly and are kept for 30 days. In a severe failure, up to 24 hours of data may be lost."

## 4. Draft policy texts

All texts below: **Draft, not legal advice, review by counsel.**

### 4.1 Support Policy (draft)

> **Support Policy**, `[Company legal name]` ("we"), version `[0.1]`, effective `[date]`.
>
> 1. **Scope.** This policy describes how we support customers of multi.ai.inbox. It forms part of the Terms of Service. Targets in this policy are goals, not guarantees, unless a signed order form states otherwise.
> 2. **Channels.** Free: documentation, community channels and the status page. Builder: email to `support@[domain]`. Team: priority email. Enterprise: as agreed in the order form. Abuse reports: `abuse@[domain]`. Security reports: `security@[domain]`. If our email is unreachable, see `[status page URL]` and write to `[fallback address]`.
> 3. **Hours.** Monday to Friday, 09:00 to 18:00 Central European Time, excluding public holidays in `[member state]`.
> 4. **Severity.** Sev1: the service is unavailable for all customers or data is exposed. Sev2: a major function is degraded. Sev3: a partial issue with a workaround, or a deliverability issue. Sev4: questions and requests. We may change the severity you propose and will tell you why.
> 5. **Targets.** First response targets per plan and severity are listed in the table at `[URL]`. Platform wide incidents are always posted on the status page.
> 6. **What you provide.** Org id, inbox address, message or request ids, UTC timestamps and steps to reproduce. Never send API keys or full message contents unless we ask for a specific message, and then only through `[secure method]`.
> 7. **What is out of scope.** Writing or debugging your agents or prompts, recipient side deliverability, third party services, legal advice.
> 8. **Automation.** We use software, including AI tools, to sort and draft replies. A person reviews every reply before it is sent to you.
> 9. **Changes.** We may update this policy with 30 days notice for paying customers; changes that reduce targets do not apply to a prepaid term.

### 4.2 Service Level Agreement (draft, for Team once tier T is live, and for Enterprise)

> **Service Level Agreement**, `[Company legal name]`, version `[0.1]`.
>
> 1. **Commitment.** For customers on the plans listed in Annex A, we will use commercially reasonable efforts to make the Service available with a Monthly Uptime Percentage of at least the figure in Annex A. "Service" means the REST API and MCP endpoint at `[api host]` and the inbound SMTP service on port 25 of the hosts listed in our MX records, in the `[EU]` region.
> 2. **Definitions.** Downtime, Monthly Uptime Percentage, Scheduled Maintenance, Emergency Maintenance and Service Credit have the meanings in Annex B (section 3.4 of the internal pack).
> 3. **Measurement.** We measure availability with external monitors at least once per minute from two locations. Our measurements prevail, and we share them on request for a claimed period.
> 4. **Credits.** If we miss the commitment in a calendar month, you are entitled to the Service Credit in Annex A, up to 50% of the fee for the affected Service in that month. Service Credits are your sole and exclusive remedy for unavailability.
> 5. **Claims.** Send a claim to `support@[domain]` within 30 days after the end of the month, with dates, UTC times and affected Service. We apply approved credits to your next invoice. Credits have no cash value and expire at termination.
> 6. **Exclusions.** No Downtime is counted for: your systems, agents, code, webhook endpoints, DNS or misconfiguration; exceeding plan or rate limits; suspension under the Terms or Acceptable Use Policy; third party services including email relays, recipient mail servers, DNS and registrars, certificate authorities and the public internet; force majeure; attacks beyond reasonable mitigation; beta or preview features; recipient side deliverability; Scheduled Maintenance; Emergency Maintenance as allowed in Annex B.
> 7. **Data protection objectives.** Backups: `[nightly / continuous]`, retained `[30]` days. Recovery point objective `[value]`, recovery time objective `[value]`. These are objectives, not guarantees, and are not covered by Service Credits.
> 8. **No SLA on Free and Builder.** These plans are provided on a best effort basis.

Annex A: Team 99.5%, Enterprise 99.9%, credit table as in 3.5.

### 4.3 Acceptable Use Policy (draft)

> **Acceptable Use Policy**, `[Company legal name]`, version `[0.1]`.
>
> 1. **Purpose.** multi.ai.inbox gives software agents email inboxes. Shared infrastructure only works if every customer keeps mail wanted, lawful and traceable. You are responsible for everything your users and agents do with your account.
> 2. **No unsolicited bulk or cold outreach.** You may not use the Service to send unsolicited commercial email, cold outreach campaigns, purchased, rented or scraped lists, or any mail to recipients who did not ask for it or do not have an existing relationship with you that the law recognises. Under EU law, unsolicited direct marketing by email generally needs prior consent, with a narrow exception for existing customers and similar products ([ePrivacy Directive Art. 13](https://eur-lex.europa.eu/eli/dir/2002/58/oj)); national rules may be stricter.
> 3. **US recipients.** Commercial email to US recipients must comply with CAN-SPAM: accurate headers and routing, non deceptive subject lines, identification as an advertisement, a valid physical postal address, a working opt out honoured within 10 business days; transactional or relationship messages are exempt from most provisions but may not contain false routing information ([FTC compliance guide](https://www.ftc.gov/business-guidance/resources/can-spam-act-compliance-guide-business)). The FTC notes that both the advertiser and the sender can be liable, so we will enforce this.
> 4. **Prohibited content and conduct.** Phishing, credential harvesting, impersonation of people or organisations, spoofed headers, malware or links to malware, fraud, scams, illegal content under EU or member state law, content that infringes intellectual property, harassment, child sexual abuse material (reported to authorities), content promoting violence or terrorism, and any use that violates sanctions or export laws.
> 5. **Agent conduct.** Agents must not create inboxes to evade limits or bans, sign up to third party services in breach of their terms, or mass register accounts. If your agent interacts with natural persons by email, you must comply with any duty to tell them they are dealing with an AI system (EU AI Act Art. 50, applicable from 2 August 2026, [Commission page](https://digital-strategy.ec.europa.eu/en/policies/guidelines-ai-transparency-obligations)).
> 6. **Limits.** Plan limits apply per organisation: Free 3 inboxes, 100 sends per day, 3,000 per month; Builder 10 inboxes, 1,000 per day, 10,000 per month; Team 150 inboxes, 15,000 per day, 150,000 per month. Requests above the API rate limit receive HTTP 429 `rate_limit_exceeded`. Do not try to bypass limits with multiple accounts.
> 7. **Quality thresholds.** We may pause sending for an organisation whose bounce rate exceeds `[2%]` or complaint rate exceeds `[0.1%]` over `[a rolling 7 day window]`.
> 8. **Security.** No probing, scanning or load testing of our systems without written permission; report vulnerabilities to `security@[domain]`. Keep API keys secret; rotate any exposed key.
> 9. **Abuse handling.** We review abuse reports sent to `abuse@[domain]` and through our DSA notice form. Steps: (a) triage within the Sev2 target; (b) for clear, ongoing harm (phishing, malware, CSAM, active spam run) we suspend sending or the account immediately and notify you with reasons; (c) otherwise we send a warning with a deadline to fix, usually 2 business days; (d) repeated or serious violations lead to suspension, then termination; (e) we keep a record of the decision and the reasons, and you can contest it by replying to the notice. We may preserve data relevant to an investigation and report criminal offences where the law requires.
> 10. **Changes.** We may update this policy; material changes take effect 30 days after notice, or immediately where needed to stop abuse or comply with law.

### 4.4 Data Processing Agreement outline (draft)

Structure follows GDPR Art. 28(3), which requires the contract to set out "the subject-matter and duration of the processing, the nature and purpose of the processing, the type of personal data and categories of data subjects and the obligations and rights of the controller" ([Art. 28 text](https://gdpr-info.eu/art-28-gdpr/); official text on [EUR-Lex](https://eur-lex.europa.eu/eli/reg/2016/679/oj)).

| DPA clause | Art. 28 reference | Draft position |
|------------|-------------------|----------------|
| Parties and roles | 28(1), 28(3) | Customer is controller (or processor for its own clients, then we are sub processor); `[Company legal name]` is processor for Customer Personal Data. For account, billing and support data we are a separate controller (Privacy Policy). |
| Subject matter, duration | 28(3) | Hosting inboxes, receiving, storing, screening, sending email for the term of the Terms plus the deletion period. |
| Nature and purpose | 28(3) | Receive, parse, thread, store, scan (spam via rspamd, prompt injection heuristics via Gatekeeper), deliver webhooks, send and relay mail, back up. |
| Data categories | 28(3) | Email headers, addresses, names, message bodies, attachments, IP addresses of sending servers, authentication results, metadata; may include special categories if senders include them; Customer must not deliberately route special category data without assessing it. |
| Data subjects | 28(3) | Customer's agents' correspondents, Customer's staff, any person who emails a Customer inbox. |
| Documented instructions | 28(3)(a) | The Terms, the API calls and configuration are the instructions; we inform Customer if a law requires other processing, and immediately if an instruction infringes data protection law (28(3) final subparagraph). |
| Confidentiality | 28(3)(b) | Everyone with access is bound by confidentiality; today that is the founder only. |
| Security | 28(3)(c), Art. 32 | Annex 2 below. |
| Sub processors | 28(2), 28(3)(d), 28(4) | General written authorisation; list at `[URL]`; 30 days notice of changes by email and on the page; Customer may object on reasonable data protection grounds and terminate the affected service if no solution; same obligations flowed down. |
| Data subject rights | 28(3)(e) | API lets Customer find, export and delete messages; we forward requests we receive to Customer and do not answer them. |
| Assistance with Art. 32 to 36 | 28(3)(f) | Information for DPIAs, breach notification per 5.4, prior consultation support; reasonable costs for extensive assistance `[decide]`. |
| Deletion or return | 28(3)(g) | On termination: 30 day export window via API, then deletion from live systems within `[30]` days and from backups as they expire (30 day retention). **Blocked by the blob deletion gap (section 1 risk 2); fix before signing.** |
| Audit and information | 28(3)(h) | Written answers to security questionnaires once a year; documentation; on site or remote audit by an independent auditor at Customer cost with 30 days notice, at most once a year unless a breach or authority order. |
| International transfers | 28(3)(a), Chapter V | No transfer of Customer Personal Data outside the EEA by us today (see subprocessor table). SCCs needed if a non EEA sub processor is added, or Module 4 (processor to controller) when the Customer itself sits outside the EEA ([Decision (EU) 2021/914](https://eur-lex.europa.eu/eli/dec_impl/2021/914/oj)). Postmark, for comparison, incorporates its DPA with the new SCCs in its Terms since 27 September 2021 ([postmarkapp.com/eu-privacy](https://postmarkapp.com/eu-privacy)). |
| Records | Art. 30(2) | We keep a processor record of processing. |
| Liability | Art. 82 | Subject to the Terms' cap, counsel to confirm interaction with Art. 82. |
| Precedence | | DPA prevails over the Terms on data protection. |

**Annex 1, subprocessors (draft, confirm each contract before publishing):**

| Subprocessor | Service | Location | Processes Customer Personal Data? | Notes |
|--------------|---------|----------|-----------------------------------|-------|
| Hetzner Online GmbH | server hosting (k3s node: Postgres, blobs, mai), Object Storage for backups | Germany `[confirm data centre: Falkenstein, Nuremberg or Helsinki]`, `[confirm Object Storage location]` | Yes: all stored mail, metadata, backups | sign Hetzner's DPA in the console `[confirm signed]` |
| Brevo `[confirm legal entity name]` | outbound SMTP relay until port 25 is open | France `[confirm processing locations]` | Yes: outbound messages and recipient addresses in transit, plus Brevo's logs | sign Brevo DPA; review Brevo's own subprocessors; removed if direct delivery replaces the relay (runbook section 6) |
| GitHub, Inc. | source code, CI, container registry (GHCR), Flux GitOps source | USA | No: code and images only; no customer data. Rule: never paste customer data into issues, logs or commits | not a subprocessor for Customer data; list as "service provider, no Customer data" for transparency |
| Let's Encrypt (Internet Security Research Group) | TLS certificates via Caddy ACME | USA | No: receives hostnames only; hostnames are published in Certificate Transparency logs | tell customers custom domain hostnames become public in CT logs |
| `[DNSBL providers used by rspamd, if any]` | spam reputation lookups | `[verify]` | Possibly: sending server IP addresses | rspamd runs with default configuration in `rspamd.yaml`; verify which external lookups it performs before publishing |
| Future: billing, error tracking, console auth | | | to assess before adoption | each needs a DPA and, if outside the EEA, SCCs or adequacy |

**Annex 2, security measures (only what exists, with the gaps stated):**

| Measure | Evidence |
|---------|----------|
| Tenant isolation with Postgres row level security on every tenant table, forced | `internal/db/migrations/0001_init.sql`, `0003_permissions_audit.sql`, end to end test in `internal/e2e` |
| API keys stored as hashes, prefix for secret scanning, scoped permissions, expiry, immediate revocation | `internal/core/access.go`, `internal/keys` |
| Audit log of security relevant actions (key create and revoke, inbox, domain, webhook changes), readable via API | `internal/core/access.go` `Audit`, `GET /v0/audit-logs` in `internal/api/api.go` |
| Per organisation rate limit and failed authentication throttling | `internal/api/api.go` |
| Plan limits on inboxes, domains, daily and monthly sends | `internal/core/core.go` |
| Signed webhooks (HMAC SHA256, Standard Webhooks and Svix headers) | `internal/dispatcher/dispatcher.go` |
| TLS for API via Caddy with Let's Encrypt; STARTTLS offered on inbound SMTP when configured; opportunistic TLS outbound | `deploy/k8s/base/caddy.yaml`, `internal/mx`, README known gaps (no MTA-STS or DANE yet) |
| Spam scoring with rspamd | `deploy/k8s/base/rspamd.yaml` |
| Prompt injection heuristics with `clear` or `review` verdict | `gatekeeper/src/lib.rs`; no ML classifier yet |
| Network policies limiting who can reach Postgres and rspamd | `deploy/k8s/base/networkpolicy.yaml` |
| Containers run as non root, read only root filesystem, all capabilities dropped | `deploy/k8s/base/mai.yaml` |
| Nightly off server backups, 30 day retention for database dumps | `deploy/k8s/base/backup.yaml` |
| Secrets kept in the cluster only, never in the repo | `deploy/hetzner/AGENT_RUNBOOK.md` section 0 and 4 |
| Firewall: inbound 22 (admin IP), 25, 80, 443 | runbook section 3 |
| Gaps not to claim: virus scanning beyond inbound internet mail, encryption at rest, deletion from backups, MFA console (no console yet), SOC 2 or ISO certification | |

**Breach notification to customers:** we notify the Customer without undue delay after becoming aware of a Personal Data Breach affecting Customer Personal Data, with a target of 48 hours, even if information is incomplete, and follow up as facts emerge. See 5.4.

### 4.5 Terms of Service, key clauses and recommended positions

| Clause | Recommended position | Counsel check |
|--------|----------------------|---------------|
| Parties | `[Company legal name]`, `[registered address]`, `[registration number]`, `[VAT number]` | |
| B2B only | Service only for businesses and professionals acting for their trade; sign up requires a business declaration and a company name; we may refuse consumers | Consumer law (withdrawal rights, unfair terms rules for consumers) then does not apply; counsel to confirm wording and checks |
| Account and keys | Customer is responsible for all use of its keys and for keeping them secret | |
| Customer responsible for agent actions | Agents act as the Customer; every email sent, every reply, every action triggered by an inbound message is the Customer's act; Customer must keep humans in control of sensitive actions | key clause |
| No warranty on AI screening | Gatekeeper and spam verdicts are aids, may be wrong in both directions, and do not guarantee that content is safe, lawful or free of prompt injection; Customer must not rely on them as the only control | key clause |
| AI Act | Customer is responsible for its own obligations as provider or deployer of AI systems, including Art. 50 disclosure to people its agents email | |
| Acceptable use | AUP incorporated; suspension rights with notice, or without notice for serious harm | |
| Beta features | provided as is, may change or end, excluded from SLA | |
| Fees and limits | plan limits per `internal/core/core.go`; overages blocked, not billed, until billing exists | |
| Customer data | Customer owns its content; licence to us only to provide the Service; DPA incorporated | |
| Confidentiality | mutual | |
| Intellectual property | proprietary software, all rights reserved (LICENSE); no reverse engineering except as allowed by law | |
| Feedback | we may use feedback without obligation | |
| Warranty disclaimer | Service provided "as available"; no warranty of deliverability or uninterrupted service beyond the SLA | |
| Liability cap | Each party's total liability capped at fees paid in the 12 months before the claim; for Free accounts `[€100 or a fixed amount]`; no liability for indirect loss, lost profits or lost data beyond restoring the last backup | Mandatory law limits caps in several member states, for example for intent, gross negligence or personal injury, and standard terms may be subject to fairness control even in B2B; counsel must adapt to the chosen law |
| Uncapped or higher cap | Customer indemnity for spam, unlawful content and third party claims from its agents' emails; our liability for breach of data protection obligations `[decide: same cap, or a higher super cap such as 2x]` | |
| Suspension | immediate for abuse or security risk; otherwise after notice | DSA Art. 17 statement of reasons if we are a hosting service |
| Term and termination | monthly plans, cancel any time; we may terminate for breach | |
| Data on termination | 30 day export window via API, then deletion within `[30]` days and from backups as they expire after 30 days | blocked until blob deletion exists |
| Changes to Terms | 30 days notice by email for paying customers; right to terminate | |
| Force majeure | standard, including upstream provider outages | |
| Governing law and courts | `[member state]` law, courts of `[city]` | founder decision |
| DSA contacts | single point of contact for authorities and for users, languages, and a notice form for illegal content | DSA Art. 11, 12, 16 |
| Sanctions and export | Customer not on sanctions lists and not in embargoed countries | |
| Assignment, entire agreement, order of precedence | Order form, DPA, SLA, Terms, AUP, Support Policy | |

### 4.6 Privacy Policy (outline only)

Covers personal data for which we are controller: account holders and their staff contacts, website visitors, support correspondents, abuse reporters, billing contacts. Sections: controller identity `[Company legal name, address, contact]`, data categories, purposes and legal bases (contract, legitimate interests, legal obligation), retention, recipients (subprocessor list), transfers (none outside the EEA today except GitHub for code collaboration if we ever process personal data there), rights and how to exercise them, right to complain to `[supervisory authority of member state]`, DPO `[appoint or explain why not required]`, cookies (landing page today is static; confirm no trackers). Clarify that email content in customer inboxes is processed for the customer under the DPA, and that recipients should contact the customer.

## 5. Incident and status communication

### 5.1 Lifecycle

1. **Detect**: external probe alert, customer report, or log anomaly.
2. **Triage**: assign severity (2.3); open an incident record (date, UTC times, impact, actions) in `[private incidents repo or file]`.
3. **Communicate**: status page "Investigating" within the targets in 2.4.
4. **Mitigate**: rollback first (runbook: `git revert` on `master`, Flux applies within 5 minutes; or pin the previous SHA), then diagnose.
5. **Identify, monitor, resolve**: status updates at each stage.
6. **Assess personal data impact**: if confidentiality, integrity or availability of personal data was affected, run 5.4 in parallel.
7. **Postmortem**: within 5 business days for Sev1, 10 for Sev2, blameless, published on the status page for Sev1.
8. **Credits**: compute the month's uptime and handle claims (3.6).

### 5.2 Status templates

**Investigating**
> We are investigating `[reports of / alerts about]` `[API errors / inbound mail not being accepted / delayed webhooks]` starting at `[HH:MM UTC]`. Affected: `[components]`. Next update by `[HH:MM UTC]`.

**Identified**
> We have identified the cause: `[plain description]`. We are `[rolling back / restarting / working with our provider]`. Inbound mail sent to you during this time `[is expected to be retried by sending servers / may be delayed]`. Next update by `[HH:MM UTC]`.

**Monitoring**
> A fix was applied at `[HH:MM UTC]`. `[API / SMTP / webhooks]` are operating normally and we are monitoring. Queued `[webhooks / sends]` are being processed. Next update by `[HH:MM UTC]`.

**Resolved**
> This incident is resolved as of `[HH:MM UTC]`. Duration: `[minutes]`. Impact: `[what customers saw]`. A postmortem will follow `[for Sev1: within 5 business days]`.

**Postmortem**
> Summary. Impact (customers, duration, data affected or "no personal data was affected", with how we know). Timeline in UTC. Root cause. What went well. What went wrong. Actions with owners and dates. Changes to this page or our commitments, if any.

### 5.3 Customer notification rules

* Platform wide Sev1 or Sev2: status page always; email to affected Team and Enterprise contacts.
* Org specific issue: direct email to the org contact only; nothing customer identifying on the public page.
* Suspensions: direct email with reasons and how to contest.
* Security incidents without personal data impact: status page if customer action is needed (for example rotate keys).
* Personal data breaches: 5.4; never discussed publicly before affected customers are told.

### 5.4 Personal data breach procedure (GDPR Art. 33 and 34)

Roles: the Customer is controller and decides on notifying the supervisory authority (Art. 33, within 72 hours of the controller becoming aware) and data subjects (Art. 34, high risk). We are processor and must notify the Customer without undue delay after becoming aware (Art. 33(2)). For our own controller data (accounts, support) we notify our own supervisory authority `[authority]` ourselves.

| Step | When | Who | Output |
|------|------|-----|--------|
| 1 Contain | immediately | founder | stop the leak, revoke keys, isolate, preserve logs (`kubectl logs` are short lived: export them) |
| 2 Assess | within 24 h | founder plus `[external counsel or DPO]` | is it a personal data breach, which orgs, which data, how many records |
| 3 Notify customers | without undue delay, target 48 h from awareness | founder | email to each affected Customer's contact: nature, categories and approximate numbers, likely consequences, measures taken, contact point; phased if facts are incomplete |
| 4 Support customers | ongoing | founder | data for their Art. 33 and 34 notices |
| 5 Record | always, even if not notifiable | founder | breach register entry (internal) |
| 6 Our own data | within 72 h if notifiable | founder | notice to `[supervisory authority]` |
| 7 Postmortem | 10 business days | founder | actions, policy updates |

Note: if counsel concludes we are also a provider of a publicly available electronic communications service (section 6.2), separate ePrivacy breach rules and national telecom rules may add duties and shorter timelines.

## 6. Compliance checklist and calendar

### 6.1 Before launch checklist

| # | Item | Owner | Status |
|---|------|-------|--------|
| 1 | Choose member state, incorporate, fill `[Company legal name]`, address, registration and VAT | founder | open |
| 2 | Counsel review of Terms, AUP, DPA, Privacy, SLA, Support Policy | counsel | open |
| 3 | Sign DPAs with Hetzner and Brevo; record their subprocessors | founder | open |
| 4 | Processor record of processing (Art. 30(2)) and controller record | founder | open |
| 5 | Fix blob deletion and implement the 90 day address quarantine | engineering | open |
| 6 | Set Object Storage backup keys, add backup failure alert, run a restore drill and record RTO | engineering | open |
| 7 | Status page (Upptime) and a 1 minute external monitor on `/healthz` and port 25 | engineering | open |
| 8 | Fallback support address outside our infrastructure | founder | open |
| 9 | `abuse@`, `security@`, `legal@`, DSA contact page, notice form | founder | open |
| 10 | Bounce and complaint circuit breakers (doc 08 T1) | engineering | open, not found in code |
| 11 | Business only sign up declaration | engineering | open, console not built |
| 12 | Verify rspamd external lookups (DNSBL) and document them | engineering | open |
| 13 | Security page from the evidence table only | founder | open |
| 14 | Insurance quote for cyber and professional liability | founder | open |

### 6.2 Regulatory notes (to confirm with counsel)

* **GDPR**: processor for mail content, controller for account data. Art. 28 contract, Art. 30(2) records, Art. 32 security, Art. 33(2) breach notice to customers. DPO need depends on core activities and scale; counsel to assess.
* **SCCs**: not needed today for customer data (Hetzner and Brevo are in the EU per our facts; GitHub and Let's Encrypt receive no customer personal data). Needed for any non EEA subprocessor that touches customer data, and Module 4 for customers established outside the EEA ([Decision 2021/914](https://eur-lex.europa.eu/eli/dec_impl/2021/914/oj)).
* **B2B vs consumer**: we sell only to businesses; consumer protection rules (withdrawal rights, consumer unfair terms rules) should not apply if sign up enforces business use. Some B2B fairness rules on standard terms still apply in several member states.
* **Digital Services Act** ([Regulation 2022/2065](https://eur-lex.europa.eu/eli/reg/2022/2065/oj)): an email inbox service that stores content on behalf of customers is likely an intermediary service, possibly a hosting service. All intermediaries: points of contact for authorities (Art. 11) and for users (Art. 12), terms that explain content restrictions and moderation (Art. 14). Hosting services add notice and action (Art. 16) and statements of reasons for restrictions such as suspensions (Art. 17). Transparency reports (Art. 15) do not apply to micro and small enterprises (Art. 15(2)). Recital 14 states that interpersonal communication services such as email are not online platforms, so platform duties should not apply. Counsel to confirm classification.
* **Electronic communications law**: counsel must assess whether the service is a number independent interpersonal communications service under the European Electronic Communications Code. If yes, ePrivacy confidentiality rules on communications content and metadata apply directly to us, national registration or notification may be required, and NIS2 may apply regardless of size (NIS2 Art. 2(2) covers providers of publicly available electronic communications services irrespective of size, [Directive 2022/2555](https://eur-lex.europa.eu/eli/dir/2022/2555/oj)). This is the most material open legal question for an email product.
* **NIS2** otherwise: size thresholds (medium enterprise and above) would exclude us today; cloud and managed service categories to be checked as we grow.
* **AI Act** ([Regulation 2024/1689](https://eur-lex.europa.eu/eli/reg/2024/1689/oj)): Art. 50 transparency duties apply from 2 August 2026; the AI Omnibus (Regulation (EU) 2026/1744) deferred high risk obligations to 2 December 2027 but left most Art. 50 duties unchanged ([Goodwin, August 2026](https://www.goodwinlaw.com/en/insights/publications/2026/08/alerts-technology-dpc-eu-ai-act-transparency-obligations-now-in-force)). Our Gatekeeper is a classifier (heuristics today), not a system that talks to people or generates content; our customers' agents that email people are their responsibility, which the Terms must state. AI literacy duties for providers and deployers also exist; counsel to scope.
* **Anti spam**: ePrivacy Art. 13 consent rule for EU recipients ([Directive 2002/58](https://eur-lex.europa.eu/eli/dir/2002/58/oj)), national implementations; CAN-SPAM for US recipients, up to $53,088 per violating email per the [FTC guide](https://www.ftc.gov/business-guidance/resources/can-spam-act-compliance-guide-business).

### 6.3 Calendar

| Cadence | Task |
|---------|------|
| Daily | support backlog check; abuse queue; backup job status (`kubectl -n mai get jobs`) |
| Weekly | support KPIs; bounce and complaint rates per org; review Sev1 and Sev2 tickets |
| Monthly | uptime calculation and SLA report (once credits exist); credit claims; subprocessor change check (Hetzner, Brevo notices); access review (who holds cluster and GitHub access); dependency and image updates; audit log spot check |
| Quarterly | restore drill from Object Storage with timing (RTO evidence); incident and postmortem review; policy texts versus evidence table (section 7); light security test (doc 08 roadmap); KB refresh; DSA notices and suspensions log review |
| Yearly | counsel review of Terms, AUP, DPA, Privacy, SLA; records of processing update; DPIA review if any; insurance renewal; NIS2 and telecom classification recheck; AI Act obligations recheck; supervisory authority and DSA contact details check; pentest when revenue allows |
| On change | new subprocessor: DPA, risk check, 30 days notice; new feature touching personal data: privacy review; architecture tier change: update SLA and RPO statements |

## 7. Promise to evidence map

Rule: marketing, docs and sales may only state a promise if the evidence column exists and is green. Review quarterly.

| Promise (where it appears) | Evidence or system | Status |
|----------------------------|--------------------|--------|
| "Hosted in the EU" (landing, DPA) | Hetzner node (`deploy/hetzner`, runbook), Brevo relay (France); GitHub and Let's Encrypt hold no customer data | OK, confirm data centre and Object Storage locations |
| "Nightly backups, kept 30 days" (SLA 7, DPA) | `deploy/k8s/base/backup.yaml`: cron `17 2 * * *`, `rclone delete --min-age 30d` on dumps | Partial: needs backup secrets set and a failure alert; blob backup is a mirror (`rclone sync`), so deleted blobs disappear from backup |
| "RPO up to 24 h" | same | OK as a statement of risk |
| "RTO X hours" | none | Do not state until a restore drill is recorded |
| "99.5% / 99.9% uptime" | none: single node (`replicas: 1`, `Recreate`) | Do not state; internal goal only |
| "Status page" | Upptime planned (doc 06 §5) | Not live |
| "Tenant isolation" | RLS forced on all tenant tables (`0001_init.sql`, `0003_permissions_audit.sql`), e2e test | OK |
| "API keys hashed, scoped, expiring, revocable" | `internal/core/access.go`, `internal/keys` | OK |
| "Audit log" | `access.go` `Audit`, `GET /v0/audit-logs` | OK; write failures are logged, not blocking: say "records", not "guarantees" |
| "Signed webhooks" | `internal/dispatcher/dispatcher.go` HMAC SHA256 | OK |
| "Rate limits and plan limits" | `internal/api/api.go`, `internal/core/core.go` | OK; 5 minute burst limit not implemented (TODO known gaps) |
| "Prompt injection screening" | `gatekeeper/src/lib.rs` heuristics, verdicts `clear` and `review` | OK as "heuristic screening", never "protection" or "guarantee"; no ML classifier yet |
| "Spam filtering" | `deploy/k8s/base/rspamd.yaml` | OK |
| "Virus scanning" | none (no clamd in manifests) | Do not state |
| "Encrypted in transit" | Caddy TLS for API; STARTTLS inbound when configured; opportunistic TLS outbound | Partial: say "TLS for the API; TLS for SMTP when the other server supports it" |
| "Encrypted at rest" | none in repo | Do not state |
| "Deleted data is removed within 30 days" | rows deleted with cascades; blobs not deleted (`internal/blob/blob.go`) | Do not state until fixed |
| "Deleted addresses are not reused for 90 days" | doc 08 T6 only | Do not state until implemented |
| "Network isolation" | `deploy/k8s/base/networkpolicy.yaml` | OK |
| "Hardened containers" | `mai.yaml` securityContext | OK |
| "Rollback in minutes" | runbook Rollback section, Flux | OK internally; not a customer promise |
| "Support response within X" | process in section 2 | OK as targets, not guarantees |
| "Breach notice to customers within 48 h" | process in 5.4 | OK as target; depends on detection, which needs monitoring and log retention |
| "Bounce and complaint auto pause" (doc 01 §8) | not found in code | Do not state until implemented |
| "Free plan footer" | `internal/core/messages.go`, `Footer: true` in `core.go` | OK |
| "MFA, SSO, console roles" | console not built | Do not state |
| "SOC 2, ISO 27001, GDPR certified" | none; GDPR has no general certification we hold | Never state |
| "No mail lost during outages" | SMTP sender retry behaviour only | Do not state |

## 8. Decisions for the founder

1. Member state of incorporation and governing law (drives liability cap rules, supervisory authority, telecom classification).
2. Whether to publish a financial SLA at all before tier T exists (recommendation: no).
3. Team SLA level and credit table (recommendation: 99.5%, table 3.5, cap 50%).
4. Support hours, holidays calendar and response targets in 2.4.
5. Backup person or contractor for absences and incidents.
6. Fallback support channel provider outside our infrastructure.
7. 1 minute monitoring tool for SLA measurement next to Upptime.
8. Maintenance window day and time.
9. Liability cap for Free accounts and whether data protection breaches get a higher cap.
10. Deletion timeline after termination (30 day export plus 30 day deletion) and priority of the blob deletion fix.
11. Bounce and complaint thresholds for automatic pause.
12. Whether Discord or GitHub Discussions is the community channel.
13. Budget for counsel review and for cyber insurance.
14. Whether to appoint a DPO or document why one is not required.
