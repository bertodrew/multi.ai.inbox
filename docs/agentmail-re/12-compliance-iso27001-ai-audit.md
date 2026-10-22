# 12 · Compliance: ISO/IEC 27001 readiness and AI governance audit

Status: draft for founder review · Date: 2026-10-22 · Author role: security and compliance architect (ISO/IEC 27001:2022 lead implementer, AI governance auditor)

Scope of this review: the repository at commit `6f56cc5` (native Go and Rust platform, k3s on one Hetzner CX33, Flux GitOps, self hosted Postgres, Brevo relay). multi.ai.inbox holds **no certification of any kind**. Every statement below says "aligned with" or "ready for", never "compliant" or "certified". External facts carry an inline source and date; numbers without a source are marked *estimate*.

Legend used in tables: **I** implemented, **P** partial, **M** missing, **NA** not applicable (justified), **INH** inherited from a supplier (still applicable, evidenced through supplier assurance under A.5.19 to A.5.23).

---

## 1. Executive summary

**What applies now (legal):**

| Regime | Applies? | Why | Source |
|--------|----------|-----|--------|
| GDPR Art. 28 (processor) | Yes, as soon as the first customer mail is processed | We process customer email content on documented instructions; we need a DPA, sub-processor authorisation, assistance with Art. 32 to 36 | [Art. 28 GDPR](https://gdpr-info.eu/art-28-gdpr/) (accessed 2026-10-22) |
| GDPR Art. 32 (security of processing) | Yes | Risk based technical and organisational measures | [Art. 32 GDPR](https://gdpr-info.eu/art-32-gdpr/) (accessed 2026-10-22) |
| GDPR Art. 30(2) records, Art. 33(2) breach notice to controller | Yes | Processor records and "without undue delay" notice | [Art. 33 GDPR](https://gdpr-info.eu/art-33-gdpr/) (accessed 2026-10-22) |
| EU AI Act Art. 4 (AI literacy) | Yes, for internal use of AI agents and for the future classifier | Applies since 2 Feb 2025; softened by the Digital Omnibus to "take measures to support the development of AI literacy" | [AI Act Service Desk timeline](https://ai-act-service-desk.ec.europa.eu/en/ai-act/timeline/timeline-implementation-eu-ai-act), [White & Case on Omnibus](https://www.whitecase.com/insight-alert/eu-ai-omnibus-enters-force-amending-ai-act) (2026) |
| EU AI Act Art. 50 transparency | Not today (no system interacts directly with natural persons or generates synthetic content); re-check if we add LLM drafting or summaries | Art. 50 applies from 2 Aug 2026 | [Art. 50](https://artificialintelligenceact.eu/article/50/), [Morgan Lewis, 2026-08-12](https://www.morganlewis.com/blogs/sourcingatmorganlewis/2026/08/eu-ai-acts-transparency-rules-what-went-into-effect-on-2-august) |
| EU AI Act high risk regime | No (email security screening is not an Annex III use case) | Annex III obligations moved to 2 Dec 2027 by Regulation (EU) 2026/1744 | [Jones Walker, 2026-07-16](https://www.joneswalker.com/en/insights/blogs/ai-law-blog/yes-august-2-still-matters-the-eu-approved-a-high-risk-ai-delay-but-most-trans.html), [Swfte timeline citing EUR-Lex](https://www.swfte.com/eu/ai-act-timeline) |
| NIS2 (Germany: NIS2UmsuCG, in force since 6 Dec 2025) | Probably not by size cap (micro enterprise). **Open legal question**: providers of publicly available electronic communications services are in scope regardless of size, and an email service may qualify | [GlobalCert NIS2 applicability](https://www.glocertinternational.com/resources/guides/nis2-applicability-essential-vs-important-entities/), [NIS Solutions, Germany](https://nis-solutions.eu/countries/germany/) (accessed 2026-10-22) |

**What applies later (market, voluntary):** ISO/IEC 27001:2022 certification (93 Annex A controls in four themes; the 2013 edition transition ended 31 Oct 2025, so any new certificate is against the 2022 edition: [ISTO](https://www.isto.ch/insights/iso27001-2022-changes), [Bright Defense, 2026](https://www.brightdefense.com/news/iso-27001-what-changed-in-2026/)); ISO/IEC 42001:2023 (AI management system, 38 Annex A controls under nine objectives A.2 to A.10: [Konfirmity](https://www.konfirmity.com/blog/iso-42001-controls), accessed 2026-10-22); SOC 2 if US buyers ask.

**Recommended path:**

1. **Now to day 90: ISO 27001 readiness, not certification.** Build the ISMS as code in this repository (policies, risk register, Statement of Applicability, evidence produced by CI). Fix the eight technical gaps that a customer security questionnaire would expose first (section 5, top of the ranked list: inbound SMTP TLS, backup encryption and immutability, secrets as code, audit log immutability, CI vulnerability scanning, Kubernetes restricted profile, operator agent token scoping, retention and DSR tooling).
2. **Day 90 to 180: ISO 42001 aligned AI governance** folded into the same ISMS (one risk register, one policy set, one management review). Do this before the ONNX classifier ships, because 42001 A.6 and A.7 are much cheaper to satisfy while the model is designed than after.
3. **Certification only when a deal requires it.** Trigger: an enterprise prospect makes ISO 27001 a contract condition, or two or more security questionnaires per month. Until then, publish a "security and compliance" page with the honest wording "operated in line with ISO/IEC 27001:2022, not certified".

**Cost and time for a solo founder:**

| Item | Cost | Time | Basis |
|------|------|------|-------|
| Readiness (this plan) | €0 to €500 (standards purchase, optional) | 15 to 25 founder days over 90 days | *estimate* |
| GRC automation platform (optional) | €3,000 to €10,000 per year | saves 5 to 10 days | *estimate* |
| ISO 27001 certification audit, EU, micro scope | €6,000 to €12,000 first year; surveillance years 2 and 3 about one third of that | Stage 1 and Stage 2 after at least one full internal audit and management review cycle, realistically 6 to 9 months from today | [High Table cost FAQ](https://hightable.io/iso-27001-certification-costs-faq/), [Axipro 2026](https://axipro.co/iso-27001-certification-cost/) (accessed 2026-10-22) |
| All in first year for a small cloud native startup | $10,000 to $35,000 | | [Axipro 2026](https://axipro.co/iso-27001-certification-cost/) |
| ISO 42001 certification on top of 27001 (integrated audit) | €4,000 to €10,000 additional | +2 to 3 months | *estimate*, no source found for micro companies |
| External pentest light before first paying customers | €3,000 to €8,000 | 1 week | *estimate* |

---

## 2. ISMS scope, risk method and initial risk register

### 2.1 Scope statement (draft for clause 4.3)

> The information security management system of multi.ai.inbox covers the design, development, operation and support of the multi.ai.inbox email platform for AI agents: the REST API `/v0`, the MCP endpoint `/mcp`, inbound SMTP (`mx`), outbound mail (`sender`), webhooks (`dispatcher`), screening (`enrich` and the Rust `gatekeeper`), the scheduler, their data stores, the deployment pipeline and the people, devices and suppliers that support them. It is operated by the founder from Germany/EU, on infrastructure hosted by Hetzner Online GmbH in Germany.

| Element | In scope |
|---------|----------|
| **Information assets** | Customer email content and attachments (raw `.eml` in the `mai-blobs` volume, parsed rows in Postgres), metadata (addresses, auth results, gatekeeper verdicts), API key hashes, DKIM private keys, webhook secrets, audit log, backups in Hetzner Object Storage, source code and CI configuration on GitHub, container images on GHCR, operational secrets (Brevo SMTP key, S3 keys, GHCR token, database passwords) |
| **Systems** | Hetzner CX33 `2.28.99.252` (Ubuntu 24.04, k3s `v1.35.8+k3s1`, Flux `2.9.6`), namespace `mai` (`deploy/k8s/base/*.yaml`), Hetzner Cloud firewall `mai-staging`, Object Storage bucket, GitHub repository `bertodrew/multi.ai.inbox`, GitHub Actions, GHCR |
| **Locations** | Hetzner data center park (Falkenstein or Nuremberg; confirm which, see open questions); founder's home office (laptop, phone with MFA) |
| **Interfaces** | Internet SMTP on 25; HTTPS 443 and HTTP 80 (ACME, landing) via Caddy; SSH 22 from `ADMIN_IP` only; outbound SMTP 587 to `smtp-relay.brevo.com`; outbound HTTPS to customer webhooks; Flux pull from GitHub; image pull from GHCR; backup upload to Object Storage; Let's Encrypt ACME; sslip.io DNS for staging names; AI agents (customers' and the founder's) via API keys and MCP |
| **People and roles** | Founder: ISMS owner, risk owner, developer, operator, incident manager. AI agents acting for the founder (Claude Code for development, an operations agent such as Hermes per `deploy/hetzner/AGENT_RUNBOOK.md`) are treated as privileged non human users |
| **Exclusions** | Hetzner's own data center operations (physical controls inherited, see A.7); customers' AI agents and what they do with mail (the customer is controller and operator of their agents); the future Next.js console on Vercel and Supabase Auth (out until built; add by scope change under clause 6.3); the founder's personal mailboxes (none, D10) |

### 2.2 Risk assessment method (clauses 6.1.2 and 8.2)

* **Asset based and scenario based**: each risk names an asset, a threat, a vulnerability observed in this repository, and the consequence.
* **Likelihood (L)** 1 rare (less than once in 5 years), 2 unlikely (once in 2 to 5 years), 3 possible (yearly), 4 likely (several times a year), 5 almost certain (monthly).
* **Impact (I)** 1 negligible, 2 minor (internal only), 3 moderate (one customer affected, short outage under 4 h, no personal data breach), 4 major (several customers, outage over 4 h, notifiable breach possible), 5 severe (cross tenant data exposure, loss of the mail corpus, business ending).
* **Score = L × I.** 1 to 6 accept and monitor; 8 to 12 treat within 90 days; 15 to 25 treat within 30 days or stop the activity.
* **Treatment options**: modify (controls), avoid, share (supplier, insurance), retain (accept with sign off). Residual risk is re assessed after treatment and signed off in the management review.
* **Review**: quarterly, after any incident, and on each scope change (new supplier, new node, model release).
* Guidance consulted for AI risks: ISO/IEC 23894:2023 (AI risk management) and ISO/IEC 42005:2025 (AI system impact assessment, published 2025 and guidance only, not certifiable: [ISO](https://www.iso.org/standard/42005), [CASRAI](https://casrai.org/guides/iso-42005-ai-system-impact-assessment), accessed 2026-10-22).

### 2.3 Initial risk register (owner: founder for all, as sole person; "Ops agent" marks a task delegated to an AI operator under STOP gates)

| ID | Risk (asset · threat · vulnerability) | Evidence | L | I | Score | Treatment | Residual target |
|----|----------------------------------------|----------|---|---|-------|-----------|-----------------|
| R01 | Whole platform · single node failure or Hetzner host loss · one CX33 runs Postgres, blobs, mx, API; `local-path` volumes | `deploy/k8s/base/postgres.yaml`, `mai.yaml` (`replicas: 1`, `Recreate`) | 3 | 4 | 12 | Documented RTO/RPO, scripted rebuild, tested restore, second node plan (5.12) | 8 |
| R02 | Postgres data · corruption or loss · logical dump only nightly, never restored in a test | `backup.yaml` (`pg_dump` at 02:17) | 2 | 5 | 10 | Monthly automated restore test, WAL archiving for RPO 1 h | 5 |
| R03 | Backups · disclosure · dumps and blobs uploaded unencrypted; S3 key in cluster | `backup.yaml` (plain `rclone copy`) | 2 | 5 | 10 | rclone `crypt` remote with key held offline, separate bucket credentials | 4 |
| R04 | Backups · destruction by attacker or bug · `rclone sync` mirrors deletions, `rclone delete --min-age 30d` uses a key that can delete | `backup.yaml` | 2 | 5 | 10 | Object Lock retention on the backup bucket ([Hetzner docs](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/protect-object-lock-retention/), accessed 2026-10-22), versioning, put only key | 4 |
| R05 | Operational secrets · theft from node or kubeconfig · all secrets live only as imperative Kubernetes Secrets; recovery depends on re running bootstrap | `deploy/hetzner/bootstrap.sh` | 2 | 4 | 8 | SOPS + age encrypted secrets in Git (Flux decrypts), offline age key, rotation runbook | 4 |
| R06 | DKIM private keys and webhook secrets · disclosure via DB read, dump or backup · stored in clear text columns | `0001_init.sql` (`domains.dkim_private_key`, `webhooks.secret`), `internal/core/webhooks.go` | 2 | 4 | 8 | Envelope encryption (AES-256-GCM, key outside DB), rotation | 4 |
| R07 | Outbound mail · Brevo account or key compromise, or Brevo suspension · single relay, key in `MAI_SMTP_RELAY` | `bootstrap.sh`, `internal/sender/sender.go` | 2 | 4 | 8 | Key rotation every 90 days, Brevo account MFA, Brevo DPA signed, fallback path (direct MX after port 25 check) | 4 |
| R08 | Customer agents · indirect prompt injection in inbound mail · `message.received` is emitted at ingest, before the gatekeeper verdict exists; heuristics are regex only; `review` messages are not hidden from default listings (doc 08 says they should be) | `internal/core/ingest.go` (event at line 108), `internal/core/workers.go`, `gatekeeper/src/lib.rs` | 4 | 4 | 16 | Option to hold `message.received` until screened, verdict in every payload, default list filter, documented agent guardrails, classifier (section 4) | 8 |
| R09 | Customer mailboxes · excessive agency of third party agents through MCP · keys default to all permissions; `send_message` callable by any agent with a full key | `internal/core/access.go` (`Permissions nil means all`), `internal/mcpserver` | 3 | 4 | 12 | Least privilege key templates, send budgets per key, optional human approval drafts, per key allow lists | 6 |
| R10 | Internal network · webhook SSRF · guard blocks private, loopback, link local, unspecified, but not CGNAT `100.64.0.0/10`, `198.18.0.0/15`, multicast or broadcast; `MAI_ALLOW_HTTP_WEBHOOKS` can weaken it | `internal/dispatcher/dispatcher.go` `SafeClient` | 2 | 3 | 6 | Extend deny list, add egress NetworkPolicy blocking cluster CIDRs and `169.254.169.254` | 3 |
| R11 | All tenants · cross tenant leak via RLS bypass · the app role `mai_app` owns the database and schema (can `ALTER TABLE ... DISABLE ROW LEVEL SECURITY`), and any session may `set app.system = 'on'` | `deploy/k8s/base/postgres-init.sh`, `0001_init.sql` (`app_system()`) | 2 | 5 | 10 | Split owner role (migrations) and runtime role (DML only), separate system role for workers, cross tenant tests on every endpoint (already in e2e), SQL lint | 4 |
| R12 | Infrastructure · operator AI agent misuse · Hetzner token is project wide read/write; STOP gates are instructions, not technical controls; the agent could be prompt injected by content it reads | `deploy/hetzner/AGENT_RUNBOOK.md` section 0 | 3 | 5 | 15 | Separate Hetzner project per environment, short lived token created per session and deleted after, snapshot before change, command allow list, session transcript archived as evidence | 6 |
| R13 | Code and images · supply chain compromise · Actions pinned by tag not SHA, no image signing, no SBOM, no dependency or image scanning, `release.yml` pushes to `master` with `contents: write`; bootstrap PAT has Administration read/write | `.github/workflows/*.yml`, `bootstrap.sh` | 3 | 4 | 12 | Pin Actions by SHA, Dependabot or Renovate, govulncheck, cargo audit, Trivy, syft SBOM, cosign signing, revoke bootstrap PAT after use | 6 |
| R14 | All accounts · founder account takeover (GitHub, Hetzner, Brevo, registrar) · MFA state not documented | none in repo | 2 | 5 | 10 | Hardware key MFA on all admin accounts (doc 08 section 4 intent), recovery codes offline, quarterly access review | 4 |
| R15 | Mail in transit · interception · inbound `mx` offers no STARTTLS (no `TLSConfig` on the go-smtp server); outbound direct delivery uses opportunistic TLS with `InsecureSkipVerify` | `internal/mx/mx.go` `NewServer`, `internal/sender/sender.go` lines 192 to 199 | 4 | 3 | 12 | Inbound STARTTLS with a valid certificate, MTA-STS and TLS-RPT for the shared domain, DANE/MTA-STS aware outbound later | 4 |
| R16 | Detection · incidents go unnoticed · no monitoring, alerting or central logs; container logs rotate locally | none in repo | 4 | 3 | 12 | External uptime and backup heartbeat checks, alerts to phone, log shipping with 90 day retention | 6 |
| R17 | Accountability · audit trail tampering or gaps · `audit_log` is mutable by the app role, cascades on org delete, write failures are non fatal; REST `messages/send` and `messages/{id}/reply` are not audited (MCP equivalents are), `mai admin` operator actions are not audited | `0003_permissions_audit.sql`, `internal/api/api.go`, `internal/mcpserver` | 3 | 3 | 9 | Append only grants, hash chain, daily export to Object Lock bucket, full coverage | 3 |
| R18 | Operations · key person dependency · solo founder, no deputy, no break glass | n/a | 3 | 4 | 12 | Sealed break glass envelope (age key, recovery codes), written runbooks, status page, trusted deputy under NDA | 8 |
| R19 | Personal data · retention and data subject requests not implemented · no retention job, no export or delete per org or per address; deleted data persists in backups for 30 days | `internal/scheduler`, `backup.yaml` | 4 | 3 | 12 | Retention settings per org, scheduler purge, `mai admin org-export / org-delete / subject-erase`, documented backup expiry | 4 |
| R20 | Reputation · spam and abuse from sign ups · IP or domain blocklisting | doc 08 T1 | 3 | 4 | 12 | Existing plan limits plus bounce and complaint circuit breakers, AUP enforcement | 6 |
| R21 | Platform · vulnerable third party images · `postgres:16-alpine`, `rspamd/rspamd:4.2.3`, `rclone/rclone:1.75.2` pinned by tag not digest, not scanned | `deploy/k8s/base/*.yaml` | 3 | 3 | 9 | Pin by digest, Renovate updates, Trivy scan of manifests' images | 4 |
| R22 | Code and data · AI coding agent introduces insecure code or leaks secrets or customer data into prompts | `.clauderules/stack-rules.md` (stack only) | 3 | 3 | 9 | AI use policy (Appendix B), no production data in agent sessions, required CI checks and secret scanning before merge | 4 |
| R23 | Compliance · regulatory misclassification (NIS2 as electronic communications service, German TKG notification, AI Act role) | section 1 | 2 | 4 | 8 | One hour of legal advice before launch, record decision | 4 |
| R24 | Future classifier · data poisoning via feedback labels or crafted training mail | doc 08 section 3 step 4 | 2 | 3 | 6 | Labels only from authenticated customers, reviewer approval, dataset versioning and provenance (A.7) | 3 |
| R25 | Availability · third party naming and certificates · staging relies on sslip.io and Let's Encrypt HTTP-01 | `deploy/k8s/staging/kustomization.yaml`, `caddy.yaml` | 2 | 3 | 6 | Buy the brand domain (D1), monitor certificate expiry | 3 |

---

## 3. ISO/IEC 27001:2022 Annex A gap analysis

Clauses 4 to 10 (context, leadership, planning, support, operation, performance evaluation, improvement) are mandatory and today all **missing** except risk related thinking in doc 08: there is no scope document, no Statement of Applicability, no internal audit, no management review. They are covered by the roadmap in section 6. Annex A theme counts: organizational 37, people 8, physical 14, technological 34 ([ISTO](https://www.isto.ch/insights/iso27001-2022-changes), accessed 2026-10-22).

Summary of the current state (count of controls):

| Theme | I | P | M | NA / INH |
|-------|---|---|---|----------|
| Organizational (37) | 1 | 15 | 20 | 1 NA |
| People (8) | 0 | 1 | 5 | 2 NA |
| Physical (14) | 0 | 2 | 2 | 10 INH |
| Technological (34) | 5 | 22 | 6 | 1 NA |
| **Total (93)** | **6** | **40** | **33** | **14** |

### 3.1 Organizational controls (A.5)

| Control | Status | Evidence in repo | Action |
|---------|--------|------------------|--------|
| 5.1 Policies for information security | M | doc 08 states intent only | Approve policy set (section 7, Appendix A) in `docs/compliance/01-policies/` |
| 5.2 Roles and responsibilities | M | none | Role register: founder holds all roles; AI agents listed as privileged non human users |
| 5.3 Segregation of duties | P | CI must pass before release (`native-ci.yml`) | Compensating controls for a solo founder: required checks, automated review, immutable audit, monthly self review signed in binder |
| 5.4 Management responsibilities | M | none | Quarterly management review record |
| 5.5 Contact with authorities | M | none | List: BfDI/Land DPA of seat, BSI (CERT-Bund), Hetzner abuse; incident runbook references them |
| 5.6 Contact with special interest groups | M | none | Subscribe to Go, Rust, k3s, Flux, Postgres security announcements; M3AAWG for mail |
| 5.7 Threat intelligence | M | none | GitHub advisories + Dependabot alerts, OWASP LLM Top 10 review twice a year, blocklist monitoring for sending IPs |
| 5.8 Information security in project management | P | ADRs and threat model in docs 08 and 10 | Add "security and privacy impact" section to the ADR template and PR template |
| 5.9 Inventory of information and assets | P | README path table, `kustomization.yaml` resources | `docs/compliance/03-assets/assets.yaml` generated partly from manifests |
| 5.10 Acceptable use | M | none | Acceptable use policy (internal) and customer AUP (doc 08 launch item) |
| 5.11 Return of assets | NA | no staff or contractors | Revisit at first hire or contractor |
| 5.12 Classification of information | M | none | Four levels: Public, Internal, Confidential, Customer Content (highest) |
| 5.13 Labelling of information | M | none | Label in asset inventory and doc headers; mail content is always Customer Content |
| 5.14 Information transfer | P | HTTPS via Caddy, TLS to Brevo (`sender.go` `tls()` with TLS 1.2 minimum), signed webhooks (`dispatcher.go`) | Inbound STARTTLS on `mx` missing; MTA-STS; transfer rules in cryptography policy |
| 5.15 Access control | P | Scoped API keys (`internal/core/access.go`), RLS (`0001_init.sql`), SSH limited to `ADMIN_IP` (runbook section 3) | Written access control policy; default new keys to least privilege |
| 5.16 Identity management | P | API keys with name, creator, expiry (`0003_permissions_audit.sql`) | Register of human and agent identities across GitHub, Hetzner, Brevo, GHCR |
| 5.17 Authentication information | P | API keys hashed with SHA-256, shown once (`internal/keys`); `mai_live_` prefix for secret scanning (doc 08 T4) | Register prefix with GitHub secret scanning partner program; password manager for founder; MFA evidence |
| 5.18 Access rights | P | `RevokeKey`, escalation checks in `CreateScopedKey` | Quarterly access review script and record |
| 5.19 Supplier relationships | M | none | Supplier register (section 5.15) and risk rating |
| 5.20 Security in supplier agreements | M | none | File DPAs: Hetzner, Brevo ([Brevo DPA in Terms, Appendix 3](https://www.brevo.com/legal/termsofuse/), accessed 2026-10-22), GitHub |
| 5.21 ICT supply chain | P | Approved dependency list (`.clauderules/stack-rules.md`), `--locked` cargo builds, `go.sum` | SBOM, signing, digest pinning, Actions pinned by SHA |
| 5.22 Supplier monitoring and change | M | none | Annual review of supplier certificates and sub-processor lists |
| 5.23 Cloud services | P | Hetzner chosen for EU residency (doc 06, D9) | Cloud service policy: exit plan, data location, shared responsibility matrix |
| 5.24 Incident management planning | M | Rollback only (runbook "Rollback") | Incident response plan with severity, roles (founder + deputy), contacts, templates |
| 5.25 Assessment of events | M | none | Triage criteria, event log in `docs/compliance/08-incidents/` |
| 5.26 Response to incidents | M | none | Playbooks: key leak, tenant leak, relay compromise, injection campaign, node loss |
| 5.27 Learning from incidents | M | none | Blameless postmortem template |
| 5.28 Collection of evidence | M | `request_id` and audit rows exist | Forensic steps: snapshot first (`hcloud server create-image`), export logs and audit with hashes |
| 5.29 Security during disruption | M | none | Define degraded modes (MX defers with 4xx, API 503) and keep them in the BCP |
| 5.30 ICT readiness for business continuity | P | Snapshot before rebuild, Flux rebuild path, nightly backups | RTO/RPO targets, restore test, second node plan |
| 5.31 Legal, regulatory, contractual requirements | P | doc 08 section 4 | Legal register: GDPR, AI Act, NIS2/TKG check, German TTDSG/TDDDG for the site |
| 5.32 Intellectual property rights | P | Proprietary licence (D6), GPL/AGPL ban in `stack-rules.md` | Licence scan in CI (cargo deny or go-licenses) |
| 5.33 Protection of records | P | Git history, audit table | Retention schedule; WORM export of audit and evidence |
| 5.34 Privacy and protection of PII | P | EU only hosting, RLS, processor role stated (doc 08) | RoPA (Art. 30(2)), DPA template, DSR tooling, retention, privacy notice |
| 5.35 Independent review | M | none | Internal audit by an external peer once a year (or paid one day review) |
| 5.36 Compliance with policies | M | none | Quarterly self check against SoA, CI policy checks |
| 5.37 Documented operating procedures | I | `deploy/hetzner/AGENT_RUNBOOK.md` (day two table), README | Keep; add backup restore, key rotation and incident procedures |

Summary: I 1, P 15, M 20, NA 1 (5.11).

### 3.2 People controls (A.6)

| Control | Status | Evidence | Action |
|---------|--------|----------|--------|
| 6.1 Screening | NA | sole founder, no staff | Apply before any hire or contractor with production access |
| 6.2 Terms and conditions of employment | NA | no employees | Security clauses in future contracts |
| 6.3 Awareness, education and training | M | none | Yearly self training log (OWASP LLM Top 10, GDPR, AI Act Art. 4 literacy), record in binder |
| 6.4 Disciplinary process | M | none | One paragraph in the information security policy for future staff and contractors |
| 6.5 Responsibilities after termination | M | none | Offboarding checklist (revoke GitHub, Hetzner, Brevo, API keys) |
| 6.6 Confidentiality agreements | M | none | NDA template for deputy, contractors, pentester |
| 6.7 Remote working | P | SSH restricted by IP, k3s API not exposed (firewall allows 22/25/80/443 only) | Remote working rules: full disk encryption, screen lock, no public Wi-Fi without VPN |
| 6.8 Event reporting | M | none | `security@` address, `/.well-known/security.txt` on the landing, internal reporting channel for agents (STOP and report) |

Summary: I 0, P 1, M 5, NA 2.

### 3.3 Physical controls (A.7)

Hetzner Online states it is certified to DIN ISO/IEC 27001:2022 with a scope covering infrastructure, operation and customer support of its data center parks in Nuremberg, Falkenstein and Helsinki ([Hetzner certificates](https://www.hetzner.com/unternehmen/zertifizierung/), accessed 2026-10-22). Download the certificate and keep it in the binder; data center controls are **INH** (inherited, applicable, evidenced through 5.19 to 5.23). Home office controls apply to the founder's devices.

| Control | Status | Evidence | Action |
|---------|--------|----------|--------|
| 7.1 Physical security perimeters | INH | Hetzner certificate | File certificate and verify scope yearly |
| 7.2 Physical entry | INH | Hetzner | as above |
| 7.3 Securing offices, rooms and facilities | INH / P | Hetzner; home office lockable | Note home office arrangements in remote working rules |
| 7.4 Physical security monitoring | INH | Hetzner | as above |
| 7.5 Physical and environmental threats | INH | Hetzner | as above |
| 7.6 Working in secure areas | INH | Hetzner | as above |
| 7.7 Clear desk and clear screen | M | none | Screen lock under 5 min, no printed secrets |
| 7.8 Equipment siting and protection | INH | Hetzner | as above |
| 7.9 Security of assets off premises | P | none documented | Laptop full disk encryption and remote wipe, documented |
| 7.10 Storage media | M | none | No removable media for customer data; break glass USB encrypted |
| 7.11 Supporting utilities | INH | Hetzner | as above |
| 7.12 Cabling security | INH | Hetzner | as above |
| 7.13 Equipment maintenance | INH | Hetzner; OS patching via `unattended-upgrades` (`bootstrap.sh`) | Covered under 8.8 for software |
| 7.14 Secure disposal or re use | INH / P | Hetzner handles disks; volumes are cloud storage | Document `hcloud` deletion of servers, volumes and snapshots, and laptop wipe procedure |

Summary: I 0, P 2 (7.9, 7.14 own part), M 2 (7.7, 7.10), INH 10.

### 3.4 Technological controls (A.8)

| Control | Status | Evidence | Action |
|---------|--------|----------|--------|
| 8.1 User endpoint devices | M | none | Founder laptop baseline: FDE, OS updates, EDR or built in protection, separate browser profile for admin consoles |
| 8.2 Privileged access rights | P | kubeconfig mode 600 (`bootstrap.sh`), root SSH by key | Named non root SSH user with sudo, root login disabled, short lived Hetzner tokens for agents, PAT revocation after bootstrap |
| 8.3 Information access restriction | I | RLS `force row level security` on every tenant table, `mai_app nosuperuser nobypassrls` (`postgres-init.sh`), permission checks in `api.go` and MCP `gate.need` | Split owner and runtime roles (R11) |
| 8.4 Access to source code | P | Private GitHub repo | Branch protection on `master`, CODEOWNERS, no direct pushes except the release bot (or replace it with Flux image automation) |
| 8.5 Secure authentication | P | Bearer API keys with expiry, auth failure limiter (`api.go` `AuthFailures`) | Hardware key MFA on GitHub, Hetzner, Brevo; console MFA when built |
| 8.6 Capacity management | P | Resource requests and limits on `mai`, `postgres`, `rspamd`, `caddy`; plan limits and rate limits | Disk usage alert for `mai-blobs` (50Gi) and Postgres (20Gi) |
| 8.7 Protection against malware | P | rspamd scoring in `mx`; distroless image | clamd planned in doc 08 T3 but not deployed: add clamd (GPL daemon over network, not linked: confirm licence position) or attachment type controls |
| 8.8 Management of technical vulnerabilities | P | `unattended-upgrades`; pinned k3s and Flux versions | govulncheck, cargo audit, Trivy, Dependabot or Renovate; patch SLA (critical 7 days) |
| 8.9 Configuration management | I | Everything declarative in Git, applied by Flux; kubeconform in CI (`native-ci.yml` job `manifests`) | Add policy checks (kube-linter or conftest) |
| 8.10 Information deletion | M | `DELETE /v0/inboxes/{inbox}` exists; no retention, no blob purge proof | Retention job, org delete, backup expiry statement |
| 8.11 Data masking | M | Request logs include IP and org, not content (`gateway.go`) | Masking rules: never log bodies, addresses truncated in logs; synthetic data in tests |
| 8.12 Data leakage prevention | P | Egress unrestricted; webhook SSRF guard | Egress NetworkPolicy, GitHub secret scanning and push protection |
| 8.13 Information backup | P | Nightly `pg_dump` + blob sync (`backup.yaml`) | Encrypt, immutable, restore test, dedicated read only backup role (job currently uses the `postgres` superuser password) |
| 8.14 Redundancy | M | single node, single replica everywhere | Second node plan and MX 20 (section 5.12) |
| 8.15 Logging | P | Structured request logs with `request_id`, IP, org (`gateway.go`); `audit_log` table | Central log shipping, retention, audit completeness and immutability |
| 8.16 Monitoring activities | M | liveness and readiness probes only | Uptime, backup heartbeat, error rate, auth failure spikes, outbound bounce rate alerts |
| 8.17 Clock synchronization | P | Ubuntu default `systemd-timesyncd` (not verified in repo) | Assert in bootstrap: `timedatectl show -p NTPSynchronized` |
| 8.18 Privileged utility programs | P | `mai admin` subcommands via `kubectl exec` (runbook day two) | Audit `mai admin` actions to `audit_log` with actor `operator` |
| 8.19 Installation of software on operational systems | I | Only via GitOps images and `bootstrap.sh` pinned versions | Keep; forbid manual `kubectl apply` outside Flux (documented) |
| 8.20 Networks security | P | Hetzner firewall 22 (admin IP), 25, 80, 443; k3s API not public | Document firewall in Git (hcloud CLI script or Terraform later) |
| 8.21 Security of network services | P | Caddy TLS with Let's Encrypt; Brevo over STARTTLS with verification | HSTS header, TLS config review, MTA-STS |
| 8.22 Segregation of networks | P | NetworkPolicies for Postgres and rspamd ingress (`networkpolicy.yaml`) | Default deny ingress and egress per namespace, explicit allows |
| 8.23 Web filtering | NA | No staff browsing on corporate networks; outbound HTTP is product traffic governed by 8.12 and the SSRF guard | Revisit with first hire |
| 8.24 Use of cryptography | P | TLS on API, DKIM signing (`sender.go`), HMAC SHA-256 webhook signatures, k3s `--secrets-encryption` | Inbound STARTTLS, envelope encryption of DKIM keys and webhook secrets, backup encryption, Postgres `sslmode` (in cluster, accept with justification or enable TLS) |
| 8.25 Secure development life cycle | P | CI with fmt, vet, clippy `-D warnings`, race tests, e2e including cross tenant RLS test | Written SDLC, threat model update per feature, security review step in PR template |
| 8.26 Application security requirements | P | Threat model doc 08; security headers in `gateway.go` | Adopt OWASP ASVS 5.0 level 2 as the requirement baseline (ASVS 5.0.0 dated May 2025: [OWASP ASVS](https://github.com/owasp/asvs), accessed 2026-10-22) |
| 8.27 Secure architecture and engineering principles | I | RLS defence in depth, non root distroless image with read only root fs and dropped capabilities (`mai.yaml`), SSRF safe client, idempotency | Record principles in `docs/compliance/01-policies/secure-development.md` |
| 8.28 Secure coding | P | gofmt, go vet, clippy, parameterised SQL via pgx | gosec or staticcheck, cargo audit, secret scanning |
| 8.29 Security testing in development and acceptance | P | e2e tests for RLS isolation, signed webhooks, gatekeeper verdicts | DAST (OWASP ZAP baseline against staging), prompt injection regression corpus |
| 8.30 Outsourced development | P | AI coding agents act as outsourced developers; rules in `.clauderules/stack-rules.md` | AI use policy (Appendix B), all agent output via PR and CI |
| 8.31 Separation of dev, test, prod | P | Local Docker Compose, CI Postgres, staging cluster; no production yet | Separate Hetzner project and cluster for production; production data never in dev or CI |
| 8.32 Change management | I | GitOps: PR, CI, merge, image pinned by SHA, Flux apply, `git revert` rollback (runbook) | Keep; add change log in release notes |
| 8.33 Test information | P | Synthetic seed (`mai admin seed`), tests create fresh databases | Rule: no real customer mail in tests or agent sessions |
| 8.34 Protection during audit testing | M | none | Rules of engagement template for pentests and audits |

Summary: I 5 (8.3, 8.9, 8.19, 8.27, 8.32), P 22, M 6 (8.1, 8.10, 8.11, 8.14, 8.16, 8.34), NA 1 (8.23).

---

## 4. AI audit

### 4.1 AI system inventory

| ID | System | What it does | Technique | Status | Our role | Data in | Affected persons |
|----|--------|--------------|-----------|--------|----------|---------|------------------|
| AI-1 | `gatekeeper-heuristics-v1` (`gatekeeper/src/lib.rs`) | Scores inbound mail for prompt injection (`instruction_override`, `role_hijack`, `data_exfiltration`, `fake_tool_markup`, hidden HTML, invisible Unicode, DMARC fail), threshold 0.5 gives `review`; extracts the new reply text | Hand written regex rules with fixed weights | Production MVP | Developer and operator | Subject, text, HTML, DMARC result | Senders and recipients of mail (indirectly), customers' agents |
| AI-2 | Future ONNX classifier (doc 08 section 3, DeBERTa v3 small fine tuned, `ort` in Rust) | Same task, learned | Supervised ML | Planned P2 | Provider | Public injection datasets + labelled customer mail | as above |
| AI-3 | MCP server `/mcp` (`internal/mcpserver`) | Exposes 12 tools (create inbox, list, read, send, reply, labels, attachments, domains) to third party AI agents | Not AI; an interface that gives AI agents agency | Production MVP | Tool provider | API key, tool arguments | Recipients of agent sent mail |
| AI-4 | Operations agent ("Hermes" or similar) | Executes `AGENT_RUNBOOK.md`: Hetzner rebuild, firewall, bootstrap, secrets handling | General purpose LLM agent (third party) | In use for deploy | Deployer (internal use) | Hetzner token, GitHub PATs, Brevo key, server shell | Customers (availability, confidentiality) |
| AI-5 | AI coding agents (Claude Code and similar) | Write code, docs, manifests in this repository | Third party LLM | In use | Deployer (internal use) | Source code, docs; must never see customer data | Customers through code quality |

### 4.2 EU AI Act classification

| System | Is it an "AI system" (Art. 3(1))? | Risk class | Obligations today |
|--------|-----------------------------------|------------|-------------------|
| AI-1 | **Probably not.** Recital 12 and the Commission guidelines of 6 Feb 2025 exclude systems "based on the rules defined solely by natural persons to automatically execute operations", including systems based on classical heuristics ([Covington summary](https://www.insideprivacy.com/artificial-intelligence/european-commission-guidelines-on-the-definition-of-an-ai-system/), [Commission guidelines PDF](https://ai-act-service-desk.ec.europa.eu/sites/default/files/2025-08/commission_guidelines_on_the_definition_of_an_artificial_intelligence_system_established_by_regulation_eu_20241689_ai_actenglish_nf2skcqfrtjdfggjavcodopcwz4_112455.PDF), accessed 2026-10-22). Gatekeeper v1 rules and weights are written by hand. | Outside scope | None under the Act; treat with 42001 discipline anyway |
| AI-2 | **Yes** (learned model that infers outputs) | Minimal risk: not a prohibited practice (Art. 5), not an Annex III use case, not a safety component of an Annex I product; no direct interaction with natural persons, no synthetic content, so Art. 50(1) and 50(2) do not apply ([Art. 50 text](https://artificialintelligenceact.eu/article/50/)) | Art. 4 AI literacy; voluntary codes; good practice documentation. We are the **provider** because we develop it and put it into service under our name |
| AI-3 | No, it is software that exposes tools. Customers' agents are their AI systems; customers are their providers or deployers | n/a | Contractual: AUP, documentation of tool risks, least privilege keys |
| AI-4, AI-5 | Yes (third party general purpose AI systems) | Minimal risk use (internal operations, coding) | We are a **deployer**: Art. 4 AI literacy (applies since 2 Feb 2025, wording softened by Regulation (EU) 2026/1744: [AI literacy Q&A](https://digital-strategy.ec.europa.eu/en/faqs/ai-literacy-questions-answers), accessed 2026-10-22) |
| GPAI | We do not train or place general purpose AI models on the market; fine tuning a small classifier is not a GPAI model | n/a | none |

Triggers that change the classification: generating text for humans (auto drafts, summaries shown to people) brings Art. 50(2) marking duties for providers; offering a chatbot to end users brings Art. 50(1); using screening to make decisions about people in employment, credit or essential services contexts could touch Annex III (now applicable from 2 Dec 2027 per [Jones Walker, 2026-07-16](https://www.joneswalker.com/en/insights/blogs/ai-law-blog/yes-august-2-still-matters-the-eu-approved-a-high-risk-ai-delay-but-most-trans.html)). Re run this table at each such change.

### 4.3 ISO/IEC 42001:2023 Annex A mapping (gap level)

38 controls under objectives A.2 to A.10 ([Konfirmity](https://www.konfirmity.com/blog/iso-42001-controls), [Witness AI](https://witness.ai/blog/iso-42001-controls/), accessed 2026-10-22).

| Objective | Controls | Status | Evidence | Gap and action |
|-----------|----------|--------|----------|----------------|
| A.2 Policies related to AI | A.2.2 AI policy, A.2.3 alignment with other policies, A.2.4 review | M | none | Approve AI policy (Appendix B); review yearly with the ISMS |
| A.3 Internal organization | A.3.2 roles, A.3.3 reporting of concerns | M | none | Founder is AI system owner; concerns via `security@` and a public "report a wrong verdict" path |
| A.4 Resources for AI systems | A.4.2 resource documentation, A.4.3 data, A.4.4 tooling, A.4.5 compute, A.4.6 human resources | P | `stack-rules.md` lists `regex`, `serde`; doc 08 names `ort` and DeBERTa | Resource sheet per AI system (model, libraries, compute, datasets, people) |
| A.5 Assessing impacts | A.5.2 process, A.5.3 documentation, A.5.4 individuals and groups, A.5.5 societal | M | doc 08 T2 only | AI system impact assessment per ISO/IEC 42005 for AI-1 and AI-2 (false negatives harm customers; false positives delay legitimate mail; senders cannot contest) |
| A.6 AI system life cycle | A.6.1.2 objectives, A.6.1.3 processes, A.6.2.2 requirements, A.6.2.3 design docs, A.6.2.4 verification and validation, A.6.2.5 deployment, A.6.2.6 operation and monitoring, A.6.2.7 technical documentation, A.6.2.8 event logs | P | Unit tests in gatekeeper, `MODEL` constant `gatekeeper-heuristics-v1` stored with every verdict, CI clippy and tests | Model card, evaluation set and thresholds as release gate, verdict metrics, versioned rules changelog |
| A.7 Data for AI systems | A.7.2 data for development, A.7.3 acquisition, A.7.4 quality, A.7.5 provenance, A.7.6 preparation | M (needed before AI-2) | none | Datasheet per dataset; customer mail used for training only with contractual permission (DPA clause) and pseudonymisation |
| A.8 Information for interested parties | A.8.2 user documentation, A.8.3 external reporting, A.8.4 communication of incidents, A.8.5 information for interested parties | P | MCP tool description tells agents to check the verdict; `reasons` returned with each verdict | Public "how screening works" page with limits, error rates, and how to report |
| A.9 Use of AI systems | A.9.2 processes for responsible use, A.9.3 objectives, A.9.4 intended use | P | `AGENT_RUNBOOK.md` STOP steps, never rules | AI use policy (Appendix B) for AI-4 and AI-5; intended use statement for AI-1 and AI-2 ("advisory signal, not a guarantee") |
| A.10 Third party and customer relationships | A.10.2 allocating responsibilities, A.10.3 suppliers, A.10.4 customers | M | none | Shared responsibility statement in terms: we screen, the customer's agent decides and must gate sensitive actions; supplier terms of the LLM vendors used for AI-4 and AI-5 reviewed (training on our data off) |

### 4.4 AI specific risks and controls

Mapped to OWASP Top 10 for LLM Applications 2025 ([LLM01 Prompt Injection](https://genai.owasp.org/llmrisk/llm01-prompt-injection/), [LLM06 Excessive Agency](https://genai.owasp.org/llmrisk/llm062025-excessive-agency/), accessed 2026-10-22).

| Risk | OWASP | Where | Control (existing → needed) |
|------|-------|-------|-----------------------------|
| Indirect prompt injection reaches customer agents before screening | LLM01 | `ingest.go` emits `message.received` before `enrich` runs | Org setting `screen_before_notify` (default on for new orgs): emit `message.received` only after the verdict, with a timeout fallback that sets `gatekeeper.status = "timeout"`; verdict object always in webhook and MCP payloads |
| Review verdict ignored | LLM01 | listing endpoints return `review` messages | Default filter excludes `review` unless `include_review=true`; `POST /messages/{id}/security/release` with audit row |
| Heuristic evasion (paraphrase, other languages, encodings) | LLM01 | `lib.rs` English centric patterns | Multilingual regression corpus, AI-2 classifier, base64 and homoglyph normalisation |
| Excessive agency via MCP tools | LLM06 | full permission keys, `send_message` | Key templates (`read_only`, `triage`, `sender`), per key send budget, recipient allow list, optional draft and approve flow |
| Data poisoning of AI-2 | LLM04 | feedback endpoint (planned) | Labels only from authenticated orgs, two step acceptance, dataset hash pinned in the model card, anomaly checks on label distribution |
| Sensitive information disclosure through agents | LLM02 | agents read full bodies | `extracted_text` minimisation, attachment size caps (MCP 5 MB), docs on not forwarding secrets |
| Explainability of verdicts | n/a | `reasons` array already returned | Keep reasons human readable, document each rule, return model version and threshold |
| Human oversight | n/a | none | Customer facing review queue in console (planned), release and feedback actions audited |
| Operator agent compromise | LLM06, LLM01 | `AGENT_RUNBOOK.md` | Section 4.7 policy, technical gates |

### 4.5 Model and dataset documentation

* **Model card** (`docs/compliance/12-ai/model-cards/gatekeeper-<version>.md`): intended use and out of scope uses, rules or architecture, training data summary (link to datasheet), evaluation set version and metrics, threshold and its rationale, known failure modes (languages, encodings), version history, owner, date, approval.
* **Datasheet** (`.../datasheets/<dataset>.md`): source and licence of each public dataset, collection period, labelling instructions and labellers, personal data present and its legal basis, pseudonymisation steps, class balance, hash of the frozen snapshot, retention and deletion.
* **Rules changelog** for AI-1: every rule or weight change goes through PR with before and after metrics on the evaluation corpus.

### 4.6 Evaluation, monitoring and incident handling

| Metric | Definition | Target (initial, *estimate*) | Where |
|--------|------------|------------------------------|-------|
| Recall on injection corpus | flagged injections / all injections in the frozen eval set | ≥ 0.85 heuristics, ≥ 0.95 classifier | CI job `gatekeeper-eval` |
| Precision of `review` | true injections / all `review` verdicts (from customer feedback) | ≥ 0.6 | monthly report |
| False positive budget | `review` rate on legitimate mail | ≤ 1 % of inbound per org per week | Postgres query, alert at 2 % |
| Screening latency | p95 time from ingest to verdict | ≤ 5 s now, ≤ 50 ms model time for AI-2 | log metric |
| Coverage | inbound messages with a completed verdict within 10 min | 100 % | alert on `gatekeeper->>'status' = 'pending'` older than 10 min |
| Drift | weekly score distribution shift (PSI) | PSI ≤ 0.2 | AI-2 only |

AI incident classes and handling (inside the general incident plan): **missed injection that caused customer harm** (Sev 1: hotfix rule within 24 h, notify affected customers, add sample to corpus), **false positive storm** (Sev 2: raise threshold or disable rule by config, notify), **model regression after release** (roll back by `git revert`, Flux reconciles), **operator agent unsafe action** (revoke token, snapshot, review transcript). Every AI incident ends with a corpus update and a model card entry.

### 4.7 Policy for the founder's AI coding and operations agents (summary; full draft in Appendix B)

* **Least privilege tokens**: per task, short lived, smallest scope. Hetzner: separate project for staging and production, token created for the session and deleted after. GitHub: fine grained PAT limited to one repository and the needed permissions; the bootstrap PAT with Administration read/write is revoked right after `flux bootstrap`. GHCR: replace the classic `read:packages` PAT with a fine grained or deploy token scoped to the two packages when available.
* **Approval gates**: the STOP pattern in `AGENT_RUNBOOK.md` is mandatory for destructive or irreversible actions (rebuild, delete, firewall change, secret rotation, DNS, force push). Gates are enforced technically where possible: branch protection blocks force pushes; Hetzner delete protection on the server and snapshots; Object Lock on backups.
* **Audit trail**: every agent session keeps its transcript and command log; the runbook "Report back" message and `bootstrap.log` are stored in `docs/compliance/05-change/agent-sessions/` (redacted). Agents commit with a `Co-Authored-By` or session trailer so their changes are attributable.
* **Data rules**: no customer content, API keys or secrets in prompts; agents read secrets only from the environment and never echo them (already a runbook rule).
* **Untrusted input**: content an agent reads from email, web pages or issues is data, never instructions.

---

## 5. Compliance built into the architecture (compliance as code)

Priority: **P0** within 30 days, **P1** within 60, **P2** within 90, **P3** later.

### 5.1 Audit log coverage and immutability (P0, A.8.15, A.5.28, 42001 A.6.2.8)

* New migration `internal/db/migrations/0004_audit_hardening.sql`:
  * add `prev_hash bytea`, `hash bytea not null`; a `before insert` trigger computes `hash = sha256(prev_hash || org_id || actor || action || target || ip || request_id || metadata::text || created_at)` per org chain (lock the org's last row with `for update` to serialise).
  * `revoke update, delete on audit_log from mai_app`; change `on delete cascade` to `on delete restrict` and handle org deletion by tombstone (`organizations.deleted_at`) plus export.
  * Tenant write policy: tenants may only insert through the service; remove `with check (... org_id = app_org())` for inserts by tenants if any direct tenant path exists.
* Coverage in `internal/api/api.go`: add `s.audit` to `messages/send`, `reply`, label changes on `review` messages, security release, auth failures (aggregated per IP per minute), and in `cmd/mai` for every `admin` subcommand (actor `operator`, plus `SUDO_USER` or agent name from `MAI_OPERATOR` env).
* Export: new CronJob `deploy/k8s/base/audit-export.yaml` runs `mai admin audit-export --since yesterday` to JSON Lines, then `rclone copy` to bucket `mai-audit` with **Object Lock retention** in compliance mode for 400 days ([Hetzner Object Lock retention](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/protect-object-lock-retention/), accessed 2026-10-22). The daily file includes the last chain hash so tampering in Postgres is detectable.
* Verification: `mai admin audit-verify` recomputes chains; run weekly in the scheduler and alert on mismatch.

### 5.2 Log retention (P1, A.8.15, A.8.16)

| Log | Retention | Store |
|-----|-----------|-------|
| `audit_log` | 400 days in Postgres, 400 days WORM export | Postgres + `mai-audit` bucket |
| HTTP request logs (no bodies) | 90 days | log shipping target (open question: self hosted Loki on the node vs EU SaaS) |
| SMTP session logs | 90 days | same |
| `events`, `webhook_deliveries` | 30 days after delivery | Postgres, purged by scheduler |
| Gatekeeper verdicts | life of the message | `messages.gatekeeper` |
| CI evidence artifacts | 3 years | GitHub release assets or evidence bucket |

Add a log policy line to `gateway.go`: never log request bodies or full addresses; current fields are acceptable.

### 5.3 Secrets management (P0, A.5.17, A.8.24)

* Adopt **SOPS + age** with Flux native decryption: `deploy/k8s/staging/secrets/*.enc.yaml`, `.sops.yaml` at repo root, Flux Kustomization `decryption: { provider: sops, secretRef: { name: sops-age } }` in `deploy/flux/clusters/staging/apps.yaml`. `bootstrap.sh` then only creates the `sops-age` secret; everything else is declarative and reproducible. Store the age private key offline (password manager + paper in the break glass envelope). Alternative: Sealed Secrets (needs approval in `stack-rules.md`, Apache-2.0). Recommendation: SOPS, no in cluster controller to run.
* Keep k3s `--secrets-encryption` (already in `bootstrap.sh`); add `k3s secrets-encrypt rotate-keys` to the yearly key rotation runbook.
* Application secrets at rest: envelope encrypt `domains.dkim_private_key` and `webhooks.secret` with AES-256-GCM (Go standard library) under `MAI_DATA_KEK` from the secret; column `key_version` for rotation; migration `0005_encrypt_secrets.sql` plus a one off re encryption command.

### 5.4 Encryption at rest and in transit (P0 to P2, A.8.24)

| Layer | Today | Target |
|-------|-------|--------|
| Kubernetes Secrets | k3s secrets encryption on | keep, rotate yearly |
| Node disk | Hetzner Cloud default (provider side; verify) | LUKS on a separate Hetzner Volume for `/var/lib/rancher/k3s/storage` (P2), unlock key via Clevis/Tang is overkill for one node: accept with justification or manual unlock on reboot (trade off with availability, founder decision) |
| Backups | plain | rclone `crypt` remote (rclone is approved): `RCLONE_CONFIG_SECURE_TYPE=crypt` wrapping the S3 remote in `backup.yaml` (P0) |
| Inbound SMTP | **no STARTTLS** | `mx.NewServer` sets `s.TLSConfig` from a mounted certificate for `MAI_HOSTNAME`; certificate from cert-manager (Apache-2.0, needs approval) or a Caddy issued certificate exported by a small sync job (P0) |
| Outbound SMTP | relay verified TLS; direct MX opportunistic | MTA-STS and DANE aware policy before direct delivery (P2) |
| API | Caddy TLS | add `Strict-Transport-Security` in `gateway.go` or Caddyfile (P1) |
| Postgres in cluster | `sslmode=disable` | accept for single node pod network with NetworkPolicy (document), enable TLS when Postgres moves to another node (P3) |

### 5.5 Key rotation (P1, A.8.24)

| Key | Rotation | How |
|-----|----------|-----|
| Brevo SMTP key | 90 days | runbook day two command; SOPS file update |
| S3 backup keys | 180 days | Hetzner console, SOPS |
| Database passwords | 180 days | `alter role` + SOPS + rollout |
| DKIM keys per domain | yearly, selector `mai1` → `mai2` | publish new TXT, dual sign window |
| Webhook secrets | on request, API to rotate with overlap | add `POST /v0/webhooks/{id}/rotate-secret` |
| `MAI_DATA_KEK` | yearly | `key_version` re encryption job |
| age key, k3s encryption key | yearly or on suspicion | runbook |
| Agent tokens (Hetzner, GitHub) | per session | Appendix B |

### 5.6 Access reviews (P1, A.5.18)

Script `scripts/compliance/access-review.sh` (quarterly) prints: GitHub collaborators and deploy keys (`gh api`), Actions secrets list, Hetzner API tokens (manual check), API keys with `permissions is null` or no expiry (`mai admin key-list -all`), `audit_log` actor summary. Output and a signed note go to `docs/compliance/04-access-reviews/YYYY-Qn.md`.

### 5.7 Vulnerability management in CI (P0, A.8.8, A.5.21)

Add to `.github/workflows/native-ci.yml`:

* `govulncheck ./...` (Go) and `cargo audit` plus `cargo deny check licenses` (Rust) as required jobs.
* `trivy fs` on the repo and `trivy config deploy/` (misconfigurations); `trivy image` on the built image in `release.yml`, failing on CRITICAL with a fix available.
* `gitleaks` or GitHub secret scanning with push protection (repository setting).
* `.github/dependabot.yml` for `gomod`, `cargo`, `github-actions`, `docker` (or Renovate). Pin all `uses:` to commit SHAs.
* Patch SLA: critical 7 days, high 30 days, recorded in the vulnerability register.

### 5.8 SBOM and signed images (P1, A.5.21, A.8.32)

* `release.yml`: `anchore/sbom-action` (syft) produces SPDX JSON for both images, attached as build artifacts and as an OCI attestation.
* `cosign sign --yes` keyless with GitHub OIDC (add `id-token: write`), plus `cosign attest --type spdxjson`. Note: keyless signing records entries in the public Rekor log (image digest and repo identity); acceptable for a private repo but a founder decision.
* Verification at deploy: Kyverno `verifyImages` policy (needs stack approval) or, lighter, a CI step that verifies the signature of the digest before the tag is pinned. Pin images by digest in `deploy/k8s/staging/kustomization.yaml` (`digest:` instead of `newTag:`), and stop pushing `:latest`.

### 5.9 Branch protection and change control (P0, A.8.4, A.8.32)

* `master`: require PR, required status checks (`gatekeeper`, `go`, `manifests`, new security jobs), linear history, no force push, no deletion, signed commits recommended.
* Solo founder cannot approve own PRs: require an automated review check (AI review or CODEOWNERS bot) as a compensating control and document it under 5.3.
* Replace the bot push in `release.yml` (`contents: write` to `master`) with **Flux image automation** (`ImageRepository`, `ImagePolicy`, `ImageUpdateAutomation`), which removes write access from CI, or keep the bot with a narrowly scoped GitHub App allowed to bypass only for `deploy/k8s/staging/kustomization.yaml`.
* Add `.github/CODEOWNERS` and `.github/pull_request_template.md` with a security and privacy checklist.

### 5.10 Kubernetes hardening (P0 to P1, A.8.9, A.8.22)

* `deploy/k8s/base/namespace.yaml`: `pod-security.kubernetes.io/enforce: restricted` (today `baseline`). Requires:
  * `postgres.yaml`: `securityContext` with `runAsUser: 70` (alpine postgres uid), `fsGroup: 70`, `runAsNonRoot`, `seccompProfile: RuntimeDefault`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`.
  * `rspamd.yaml`: same, `readOnlyRootFilesystem: true` with `emptyDir` for `/var/lib/rspamd` and `/run/rspamd`; check the image's user.
  * `caddy.yaml`: listen on 8080/8443 inside the pod and map Service ports 80/443 to them, so no `NET_BIND_SERVICE` is needed; read only root fs with `/data` and `/config` volumes.
  * `backup.yaml`: non root for both containers.
* `networkpolicy.yaml`: add `default-deny` (Ingress and Egress, all pods), then allow: DNS to kube-system; `mai` egress to postgres 5432, rspamd 11333, and `0.0.0.0/0` on 25, 587, 443 **except** `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10`, `169.254.0.0/16`; caddy ingress 80/443 and egress to `mai-http` 8080 and ACME 443; backup egress to postgres and 443.
* Pin third party images by digest; resource limits already present on most pods; add CPU limits only if noisy neighbour issues appear.
* Add `automountServiceAccountToken: false` on all pods (none need the Kubernetes API).

### 5.11 Backup and restore tests (P0, A.8.13)

* `backup.yaml`: use a dedicated `mai_backup` role (`login bypassrls`, `pg_read_all_data`) instead of the `postgres` superuser; wrap the remote with rclone `crypt`; replace `rclone sync` for blobs with `rclone copy` plus bucket versioning, or keep sync on a versioned and Object Locked bucket; remove `rclone delete` and let bucket lifecycle and lock retention expire objects (30 days postgres, 30 days noncurrent blob versions).
* New CronJob `deploy/k8s/base/restore-test.yaml` (monthly): start an ephemeral `postgres:16-alpine` pod, download and decrypt the latest dump, `pg_restore`, run `mai migrate` (must be a no op) and row count checks, write a JSON result and push a heartbeat. The result file is evidence for 8.13 and 5.30.

### 5.12 DR targets and second node plan (P1 to P3, A.5.30, A.8.14)

| Stage | RPO | RTO | How |
|-------|-----|-----|-----|
| Now (staging) | 24 h | 8 h (*estimate*: rebuild, bootstrap, restore) | nightly dump, documented rebuild |
| First customers (P1) | 1 h | 4 h | WAL archiving to Object Storage (WAL-G, Apache-2.0, needs approval), scripted restore |
| Growth (P3) | 5 min | 1 h | second CX node in another Hetzner location as k3s agent with Postgres streaming replica and second MX (`MX 20`); blob store moved to Object Storage (README "S3 blob store" item) so pods are stateless |

Inbound mail survives a short outage because sending MTAs retry for days; document this in customer facing SLAs.

### 5.13 Monitoring and alerting (P0 to P1, A.8.16)

* External: an EU uptime monitor on `/healthz`, SMTP banner on 25, certificate expiry (P0).
* Heartbeats: backup, restore test, audit export and audit verify jobs ping a heartbeat URL; missing ping alerts the founder's phone (P0).
* Internal metrics (P1): Prometheus format `/metrics` in `mai` (request rate, error rate, auth failures, queue depth, gatekeeper pending, webhook failures, bounce rate) scraped by a small Prometheus or VictoriaMetrics on the node (needs approval) or exported to an EU SaaS.
* Security alerts: auth failure spikes, new full permission key created, `review` rate above budget, outbound volume anomaly per org.

### 5.14 Data subject requests, retention and deletion (P1, GDPR Art. 28(3)(e), A.8.10)

* Migration `0006_retention.sql`: `organizations.retention_days int null`, `organizations.deleted_at`.
* Scheduler (`internal/scheduler`): daily purge of messages and blobs older than `retention_days`; purge `events` and `webhook_deliveries` older than 30 days and `idempotency_keys` older than 24 h; hard delete orgs 30 days after `deleted_at`.
* `cmd/mai admin`: `org-export -org <id>` (JSON Lines of all rows + tar of raw `.eml` and attachments, checksum manifest); `org-delete -org <id>` (soft, then hard after 30 days); `subject-search -org <id> -address <a>` and `subject-erase` to help the controller answer access and erasure requests. All audited.
* API for customers: `GET /v0/exports` (async job) and `DELETE /v0/messages/{id}` (hard delete with blob removal).
* Statement in the DPA: deleted data disappears from backups within 30 days by rotation.

### 5.15 DPA, records of processing and vendor register (P0, Art. 28, Art. 30(2), A.5.19 to A.5.23)

Records of processing (processor, Art. 30(2)) template, `docs/compliance/11-privacy/ropa.md`:

| Field | Entry |
|-------|-------|
| Processor | multi.ai.inbox (legal entity, address, contact) |
| Controllers | each customer organisation (link to customer list) |
| Categories of processing | receiving, storing, parsing, screening, sending email for AI agents; webhook delivery |
| Categories of data | email headers and bodies, attachments, addresses, IPs of senders, authentication results; may include special categories sent by third parties |
| Sub-processors | Hetzner Online GmbH (DE, hosting, Object Storage); Brevo (EU, outbound relay); GitHub (US, code only, no customer data) |
| Transfers outside EEA | none for customer data; GitHub holds no customer data by policy |
| Security measures | link to Appendix A and the SoA |

DPA outline (`docs/compliance/11-privacy/dpa-template.md`): subject and duration; nature and purpose; data and subjects; documented instructions; confidentiality; Art. 32 measures (annex); sub-processor general authorisation with 30 day notice and objection; assistance with DSRs and Art. 32 to 36; breach notice without undue delay and target 48 h; deletion or return at end; audits (documents first, on site at customer cost); AI screening clause (no training on customer mail without opt in).

Vendor register (`docs/compliance/03-assets/vendors.md`):

| Vendor | Service | Data | Assurance found | Action |
|--------|---------|------|-----------------|--------|
| Hetzner Online GmbH | CX33, firewall, Object Storage, DNS for PTR | all customer data | ISO/IEC 27001:2022 certificate for its data center parks ([Hetzner](https://www.hetzner.com/unternehmen/zertifizierung/), accessed 2026-10-22) | Sign DPA in Hetzner console, file certificate, enable delete protection |
| Brevo | SMTP relay | outbound mail content and addresses | DPA in Terms, Appendix 3, with sub-processor list ([Brevo Terms](https://www.brevo.com/legal/termsofuse/), accessed 2026-10-22); certifications not verified here | Accept DPA, record sub-processors, MFA on account |
| GitHub (Microsoft) | code, CI, GHCR | source code, no customer data | not verified in this review | Check GitHub trust portal, enable secret scanning and push protection |
| Let's Encrypt (ISRG) | certificates | hostnames only | n/a | none |
| sslip.io | staging DNS names | IP in hostname | n/a | remove when brand domain exists |
| AI agent vendors (Claude Code, Hermes model provider) | development and operations | code, infra tokens in session | review data use terms | record settings (no training) |

### 5.16 Evidence collection automation (P1, clause 9.1, 7.5)

* CI uploads per run: test report, govulncheck, cargo audit, Trivy reports, SBOM, kubeconform output, cosign verification. A weekly workflow `compliance-evidence.yml` collects the latest of each, adds `git rev-parse HEAD`, branch protection JSON (`gh api repos/.../branches/master/protection`), Dependabot alert counts, and publishes them to the `mai-evidence` bucket (Object Lock 3 years) or as a GitHub release asset `evidence-YYYY-WW`.
* Cluster side jobs (backup, restore test, audit export, audit verify, access review reminder) write JSON results to the same bucket.
* The binder (section 6) links to these rather than copying them.

---

## 6. 30/60/90 day roadmap and evidence binder

Effort figures are *estimates* for one founder working part time on compliance.

### Days 1 to 30 (about 8 days): stop the bleeding, set the frame

* Approve Appendix A and B; write scope (2.1), risk method (2.2), and this register into the binder.
* MFA with hardware keys on GitHub, Hetzner, Brevo, registrar; recovery codes offline; revoke the bootstrap PAT; Hetzner delete protection on server and snapshots.
* P0 code and infra: inbound STARTTLS; backup encryption, Object Lock, `mai_backup` role; SOPS + age; audit hardening migration and coverage; CI security jobs and Dependabot; branch protection; default deny NetworkPolicies and `restricted` Pod Security; `screen_before_notify` for gatekeeper.
* Sign DPAs with Hetzner and Brevo; start vendor register and RoPA.
* External uptime and backup heartbeat alerts.

### Days 31 to 60 (about 7 days): make it operate

* Restore test CronJob, first successful restore recorded; DR targets published.
* Key rotation runbook and first rotation of Brevo and S3 keys.
* Retention and DSR tooling (`org-export`, `org-delete`, retention purge).
* SBOM and cosign in `release.yml`, images pinned by digest.
* Incident response plan, playbooks and one tabletop exercise (prompt injection campaign) recorded.
* Gatekeeper evaluation corpus and CI job; model card v1 for `gatekeeper-heuristics-v1`; AI impact assessment.
* First quarterly access review.

### Days 61 to 90 (about 6 days): prove it

* Statement of Applicability v1 (all 93 controls with justification), policy set complete (section 7).
* Internal audit of the ISMS against clauses 4 to 10 and the SoA (peer reviewer or a paid one day review), nonconformities logged with corrective actions.
* Management review (inputs: risks, audit results, incidents, metrics, objectives) and signed minutes.
* Public security page, `security.txt`, sub-processor page, DPA ready for customers.
* Decision point: book a certification body for Stage 1, or stay in readiness mode.

### Evidence binder structure (`docs/compliance/`)

| Folder | Contents |
|--------|----------|
| `00-isms/` | scope, context and interested parties, ISMS objectives, Statement of Applicability, roles, legal register |
| `01-policies/` | the policy set (section 7), each with version, owner, approval date |
| `02-risk/` | risk method, risk register, treatment plan, risk acceptance sign offs |
| `03-assets/` | asset inventory, data classification, vendor register, supplier certificates and DPAs (PDFs stored outside Git if confidential, linked here) |
| `04-access-reviews/` | quarterly access review outputs and sign offs |
| `05-change/` | change management notes, release notes, redacted agent session logs |
| `06-vulnerability/` | vulnerability register, links to CI scan artifacts, patch SLA tracking, pentest reports |
| `07-backup-dr/` | backup configuration, restore test results, DR plan, RTO/RPO |
| `08-incidents/` | incident plan, playbooks, event log, postmortems, tabletop records |
| `09-training/` | AI literacy and security training log |
| `10-audits-reviews/` | internal audit reports, management review minutes, corrective actions |
| `11-privacy/` | RoPA, DPA template, sub-processor list, DSR log, retention schedule, privacy notice |
| `12-ai/` | AI system inventory, impact assessments, model cards, datasheets, evaluation reports, AI incident log |
| `evidence/` | index of automated evidence (links to buckets and release assets), never secrets |

---

## 7. Policy set

| Policy | Purpose |
|--------|---------|
| Information security policy | Top level commitment, scope, objectives, roles and the rule that all other policies derive from it. Full draft in Appendix A. |
| Access control policy | Who and what (humans, AI agents, API keys) may access which systems; least privilege, MFA, key lifetimes, quarterly reviews, joiner and leaver steps, break glass access. |
| Cryptography policy | Approved algorithms and protocols (TLS 1.2 or higher, AES-256-GCM, SHA-256, Ed25519 or RSA 2048 for DKIM), where encryption is mandatory (transit, backups, secrets), key ownership, storage and rotation periods (table 5.5). |
| Supplier security policy | How suppliers are selected, assessed (certificates, DPA, location), registered, monitored yearly and exited; sub-processor change notice to customers. |
| Incident response policy | Definitions, severity levels, reporting channels, the founder and deputy roles, timelines (customer notice without undue delay, target 48 h; controller supports Art. 33 72 h deadline), evidence preservation, postmortems. |
| Business continuity and DR policy | Critical services, degraded modes, RTO and RPO, backup and restore testing, second node plan, break glass envelope, communication during outages. |
| Secure development policy | SDLC from design (threat model in ADR) to release (required checks, scans, SBOM, signing), OWASP ASVS 5.0 level 2 as baseline, dependency approval via `stack-rules.md`, AI generated code rules. |
| Acceptable use policy | Internal: how the founder and any helper use devices, accounts and AI tools. External (customer AUP): no cold outreach, no spam, no malware, limits for agent self sign up. |
| AI policy | Principles for AI we build (gatekeeper) and AI we use (coding and operations agents): intended use, impact assessment, evaluation gates, human oversight, transparency, data rules. Full draft in Appendix B. |
| Data retention and deletion policy | Retention per data category (customer content per org setting, logs 90 days, audit 400 days, backups 30 days), deletion methods, DSR handling, legal holds. |
| Logging and monitoring policy | What is logged (security events, admin actions, auth failures), what is never logged (mail bodies, secrets), retention, integrity (hash chain, WORM), review cadence and alert thresholds. |

---

## Ranked list of recommended changes

| # | Change | Files | Priority |
|---|--------|-------|----------|
| 1 | Gatekeeper before notify: emit `message.received` after the verdict (org setting, timeout fallback), exclude `review` from default lists, release endpoint | `internal/core/ingest.go`, `internal/core/workers.go`, `internal/core/messages.go`, `internal/api/api.go`, `internal/mcpserver`, `openapi/openapi.yaml` | P0 |
| 2 | Inbound STARTTLS on port 25 with a valid certificate | `internal/mx/mx.go`, `cmd/mai/main.go`, `deploy/k8s/base/mai.yaml` (cert volume), cert issuance (cert-manager or Caddy) | P0 |
| 3 | Backups: rclone crypt, `mai_backup` role instead of superuser, Object Lock + versioning, no `sync` deletes, monthly restore test | `deploy/k8s/base/backup.yaml`, `deploy/k8s/base/postgres-init.sh`, new `deploy/k8s/base/restore-test.yaml` | P0 |
| 4 | Operator agent least privilege: per session Hetzner token, separate projects, delete protection, revoke bootstrap PAT, archived transcripts | `deploy/hetzner/AGENT_RUNBOOK.md`, `deploy/hetzner/bootstrap.sh` | P0 |
| 5 | Secrets as code with SOPS + age, Flux decryption | `.sops.yaml`, `deploy/flux/clusters/staging/apps.yaml`, `deploy/k8s/staging/secrets/`, `deploy/hetzner/bootstrap.sh` | P0 |
| 6 | Database role split: owner role for migrations, DML only runtime role, separate system role | `deploy/k8s/base/postgres-init.sh`, `internal/db`, `cmd/mai/main.go` (`migrate` uses owner DSN) | P0 |
| 7 | Audit log: hash chain, no update/delete grants, no cascade, full coverage (REST send/reply, admin CLI, auth failures), daily WORM export and verify | new `internal/db/migrations/0004_audit_hardening.sql`, `internal/core/access.go`, `internal/api/api.go`, `cmd/mai`, new `deploy/k8s/base/audit-export.yaml` | P0 |
| 8 | CI security: govulncheck, cargo audit and deny, Trivy, secret scanning, Dependabot, Actions pinned by SHA | `.github/workflows/native-ci.yml`, `.github/workflows/release.yml`, new `.github/dependabot.yml` | P0 |
| 9 | Kubernetes hardening: `restricted` Pod Security, default deny ingress and egress, metadata IP blocked, no SA token automount | `deploy/k8s/base/namespace.yaml`, `networkpolicy.yaml`, `postgres.yaml`, `rspamd.yaml`, `caddy.yaml`, `backup.yaml` | P0 |
| 10 | Branch protection, CODEOWNERS, PR template; replace bot push with Flux image automation | GitHub settings, new `.github/CODEOWNERS`, `.github/pull_request_template.md`, `.github/workflows/release.yml`, `deploy/flux/clusters/staging/` | P0 |
| 11 | Encrypt DKIM keys and webhook secrets (envelope, key versioning), webhook secret rotation API | new `internal/db/migrations/0005_encrypt_secrets.sql`, `internal/core/domains.go`, `internal/core/webhooks.go`, `internal/core/workers.go` | P1 |
| 12 | Retention and DSR tooling | new `0006_retention.sql`, `internal/scheduler`, `cmd/mai` admin commands, `internal/api/api.go` | P1 |
| 13 | Monitoring: uptime, heartbeats, `/metrics`, alerts | `internal/api`, `deploy/k8s/base` | P1 |
| 14 | SBOM and cosign signatures, digest pinning, stop `:latest` | `.github/workflows/release.yml`, `deploy/k8s/staging/kustomization.yaml`, `deploy/k8s/base/*.yaml` | P1 |
| 15 | SSRF guard: add CGNAT, benchmarking, multicast and broadcast ranges | `internal/dispatcher/dispatcher.go` | P1 |
| 16 | HSTS header | `internal/api/gateway.go` or `deploy/k8s/base/caddy.yaml` | P1 |
| 17 | Least privilege API key templates and per key send budget | `internal/core/access.go`, `internal/core/messages.go`, `0003` follow up migration | P1 |
| 18 | Gatekeeper evaluation corpus and CI gate, model card | `gatekeeper/tests/corpus/`, `.github/workflows/native-ci.yml`, `docs/compliance/12-ai/` | P1 |
| 19 | WAL archiving (RPO 1 h) and second node plan | `deploy/k8s/base/postgres.yaml`, new manifests | P2 |
| 20 | `security.txt`, security page, sub-processor page | `landing/` | P2 |

---

## Open questions for the founder

1. **NIS2 and TKG**: do we accept the risk that a public email service could be an "electronic communications service" (in scope of NIS2 regardless of size, and possibly subject to BNetzA notification under the German TKG)? Recommendation: one hour with a German telecoms lawyer before the first paying customer.
2. **Legal entity and seat**: which entity signs DPAs and which German Land DPA is competent?
3. **Hetzner location**: is the CX33 in Falkenstein or Nuremberg, and is the Object Storage bucket in the same location (single location risk for backups)?
4. **Certification trigger**: which signal (deal size, number of questionnaires) books the Stage 1 audit? SOC 2 needed for US prospects?
5. **Stack approvals** needed for: cert-manager, SOPS/age (CLI only), WAL-G, Kyverno or other admission policy, a metrics stack, Trivy, syft, cosign, gitleaks. Add to `.clauderules/stack-rules.md`?
6. **Monitoring vendor**: self host on the single node (dies with it) or an EU SaaS for uptime and logs?
7. **Disk encryption**: accept manual unlock after a reboot (LUKS) for better confidentiality, or rely on provider side protection for availability?
8. **Sigstore keyless**: acceptable that image digests and repository identity appear in the public Rekor log?
9. **Training on customer mail** for the future classifier: opt in per org, opt out, or never (only public datasets and synthetic data)?
10. **Gatekeeper default**: should new orgs get `screen_before_notify` on by default (safer, adds seconds of latency)?
11. **Deputy and break glass**: is there a trusted person who can hold the sealed envelope and act during founder unavailability?
12. **Operator agent**: which product and model run "Hermes", where do its transcripts live, and does its vendor train on session data?
13. **Retention default**: keep mail until deleted (doc 08) or a default such as 365 days with an org override?

---

## Appendix A: Information security policy (draft v0.1)

**Owner:** Founder · **Approved:** pending · **Review:** yearly and on major change

1. **Purpose.** multi.ai.inbox processes email on behalf of customers whose AI agents act on it. This policy sets management direction for protecting the confidentiality, integrity and availability of that information and of our systems, in line with ISO/IEC 27001:2022 and GDPR Art. 32.
2. **Scope.** All information, systems, people (including contractors) and AI agents acting for the company, as defined in the ISMS scope statement.
3. **Commitments.** The founder commits to: meet legal, regulatory and contractual requirements; manage risk with a documented method; provide the resources needed; continually improve the ISMS; never claim certifications the company does not hold.
4. **Objectives (first year).** No cross tenant data exposure; restore tested monthly with RPO 24 h or better; critical vulnerabilities fixed within 7 days; 100 % of inbound mail screened before notification for orgs that opt in; incidents reported to affected customers without undue delay, target 48 h.
5. **Principles.** Least privilege for humans, services and AI agents. Defence in depth (row level security plus explicit filters). Everything as code and reviewed through CI. Encrypt in transit and at rest where the data is customer content or a secret. Log security relevant actions immutably. Customer content is the highest classification and never used for testing, demos or AI prompts.
6. **Roles.** The founder is ISMS owner, risk owner and incident manager. AI agents are privileged non human users bound by the AI use policy. A deputy, when appointed, holds break glass material under NDA.
7. **Supporting policies.** Access control, cryptography, supplier, incident response, business continuity, secure development, acceptable use, AI, data retention, logging and monitoring.
8. **Compliance.** Breaches of this policy by staff or contractors lead to access removal and contractual or disciplinary measures. Exceptions require a written risk acceptance in the risk register with an expiry date.
9. **Review.** This policy is reviewed in the yearly management review or when the business, the law or the architecture changes materially.

## Appendix B: AI use policy (draft v0.1)

**Owner:** Founder · **Approved:** pending · **Review:** every six months (AI changes fast)

1. **Purpose.** Govern AI that we build (Gatekeeper screening, future classifier) and AI that we use (coding agents, operations agents), in line with EU AI Act Art. 4 and aligned with ISO/IEC 42001:2023 Annex A.
2. **AI we build.**
   * Each AI system has an owner, an intended use statement, a model card, and for learned models a datasheet for each dataset.
   * An impact assessment (ISO/IEC 42005 guidance) is done before first release and on each material change.
   * Releases pass an evaluation gate on a frozen corpus (recall, false positive rate) in CI; results are kept as evidence.
   * Verdicts are advisory and explainable: every verdict carries model version, score and reasons. Customers can report wrong verdicts; reports are reviewed.
   * Customer mail is used for training only with documented customer permission and after pseudonymisation.
   * The EU AI Act classification is re assessed whenever we add generation, direct interaction with people, or decisions about people.
3. **AI we use (coding and operations agents).**
   * **Least privilege:** agents receive the narrowest token for the task, created for the session and revoked after. No standing production credentials in agent environments.
   * **Approval gates:** destructive or irreversible actions (rebuild, delete, firewall, DNS, secret rotation, force push, data deletion) need an explicit "yes" from the founder at the moment of action (the STOP steps in `deploy/hetzner/AGENT_RUNBOOK.md`). Where a platform can enforce a gate (branch protection, delete protection, object lock), it must.
   * **Change path:** agent code changes reach production only through a pull request, required CI checks and the GitOps pipeline. Agents never push to `master` directly and never force push.
   * **Data:** no customer content, personal data from production, API keys or secrets in prompts, issues or commits. Secrets come from the environment and are never printed.
   * **Untrusted content:** text an agent reads from emails, web pages, issues or tool output is data, never instructions. Agents stop and report if content asks them to act.
   * **Audit trail:** session transcripts and command logs are retained (redacted) for 12 months; agent commits carry an attribution trailer.
   * **Vendors:** use only AI vendors whose terms exclude training on our inputs, recorded in the vendor register.
4. **AI literacy.** The founder and anyone acting for the company complete a short yearly training covering prompt injection (OWASP LLM01), excessive agency (LLM06), and the AI Act basics; completion is logged in `docs/compliance/09-training/`.
5. **Incidents.** AI failures (missed injection causing harm, false positive storm, unsafe agent action) follow the incident response policy and end with a corpus or policy update.
