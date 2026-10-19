# Privacy Policy — multi.ai.inbox (Art. 13/14 GDPR notice)

**Version:** 1.0-draft · **Date:** 2026-10-19 · **Authentic language:** Italian (`privacy-it.md`); this English translation is for convenience — in case of divergence the Italian version prevails.

> ⚠️ **INTERNAL DRAFT — DO NOT PUBLISH AS-IS.** Drafted as part of the multi.ai.inbox legal package (privacy review 2026-10-19, finding P0-1). To be reviewed by qualified legal counsel (IT/CH jurisdiction) **before publication on the site**. Retention periods stated are **planned values**: they must be technically enforced (§6) **before** publication, or this notice would be inaccurate. Fields `[...]` to be completed.

---

## 1. Who we are, roles and contacts

1.1 **Controller:** [FULL LEGAL NAME AND ENTITY FORM — TO BE COMPLETED], with registered office at [ADDRESS — TO BE COMPLETED] (hereinafter "**multi.ai.inbox**" or the "**Provider**"). Privacy contact: [PRIVACY EMAIL — TO BE COMPLETED].

1.2 **DPO:** not appointed; the obligation to appoint must be verified at the current scale with qualified counsel. **Privacy lead:** [NAME — TO BE COMPLETED].

1.3 **Roles for service and contents.** For contact, billing and service-usage data, multi.ai.inbox is the **controller**. For tenant mailboxes (resold hosting) the **controller is the customer** (who decides the purposes of their own correspondence) and multi.ai.inbox is the **processor** under Art. 28 GDPR, with a dedicated DPA.

1.4 **Transfers and territorial scope.** We process data of people in the EU and, where applicable, Switzerland: the GDPR applies and, for data of people in CH, the Swiss **nLPD** (2023) — processed in coherence with both.

---

## 2. What data we process and why

| # | Purpose | Personal data | Data subjects | Legal basis |
|---|---|---|---|---|
| T1 | Delivering the mail service of hosted mailboxes (receiving, sending, storing, webmail) | Mail contents, headers (senders, recipients, subject), IPs in delivery paths, quotas | Customers/tenants and their collaborators; **external senders and recipients** | Contract (for the customer); **legitimate interest + Art. 14 notice** for external correspondents (indirect collection: they write into the mailboxes) |
| T2 | Access to the site and panels (login, connection forms) | Access credentials, registrar account username, API key, client IP, timestamps | Operators, customers | Contract performance (access); legitimate interest (security) |
| T3 | Security and audit logs of actions (panel, API/MCP) | Event, timestamp, tool used, domain, outcome, client IP | Operators, customers | Legitimate interest (security, Accountability Art. 5(2)) |
| T4 | Domain management via registrar connectors | Registrar account data (username, API key, whitelisted IP), registrant/WHOIS data where processed | Customers | Contract performance |
| T5 | Contact and invoicing | Customer contact persons, billing/usage data | Customers | Contract; legal obligations (tax) |
| T6 | **AI agents on mailboxes (planned — not active today)** | Mail contents and metadata, incidental special categories | Senders/recipients (including third parties) | Legitimate interest + **written LIA** (to be completed before activation); Art. 14 notice |
| T7 | Hosting resold to tenants | Tenant mailbox contents | Customers' collaborators | For the contents the **controller is the customer**; we are the processor (DPA) |

**Special categories** (Art. 9 GDPR) are not actively processed; they may appear incidentally in mail contents: we minimize (extraction only of what the task requires), no logging of contents, **no model training on mailbox contents** without a specific basis.

---

## 3. Cookies and similar technologies

We use **only strictly necessary technical cookies** — no profiling cookies, no third-party trackers, no banner (ePrivacy exception for strictly necessary cookies).

| Cookie | Purpose | Duration | Type |
|---|---|---|---|
| `MULTIAI_LANDING` | Landing session (login/connection) | Session | Technical, HttpOnly, Secure, SameSite=Lax |
| Panel authentication session (mailcow) | Admin authentication | Session | Technical |
| Webmail session (SOGo) | Webmail authentication | Session | Technical |

---

## 4. Recipients and processors

| Recipient | Country | Role | Shared data | Contract basis |
|---|---|---|---|---|
| **Hetzner Online GmbH** | Germany (EU) | Processor (server hosting, database, backups) | All data processed on the instance | DPA Art. 28 — **to be activated in the Hetzner account** (it does not conclude automatically, T&C §6.2); processing limited to EU/EEA |
| **Namecheap Inc.** | **USA** (outside EEA) | Processor for domain/WHOIS management; independent controller for its own accounts | Account username, API key, server IP | DPA with **incorporated SCCs** + documented **TIA**; EU representative (EDPO, Brussels, Art. 27); **not certified under the EU-US Data Privacy Framework** |
| **Infomaniak** | **Switzerland** (planned) | Processor (when the connector is active) | Domain/account data | EU→CH **adequacy decision** (Decision 2000/518/EC, confirmed 2024) — no SCCs required; DPA generated from the Manager |
| **Let's Encrypt** (ISRG) | USA | Independent controller (TLS certificate issuance) | Domain name | Technical necessity of the service |
| **LLM provider** (planned, only if AI agents read mailboxes) | To be defined | Processor | Mail contents only where the flow requires it | DPA + SCC/DPF per vendor, **or** EU-based LLM — to be defined **before** activation |
| Judicial/administrative authorities | EU | Independent controllers | What the law requires | Legal obligation |

Technical note: the server's egress IP is detected via an external public service (currently `api.ipify.org`, USA; relating to the server, not a person); replacement planned with Hetzner metadata.

---

## 5. Transfers outside the EEA

- **USA (Namecheap):** transfer of personal data (username where the account is personal, API key as pseudonymous data, IP) with **SCCs** incorporated in the Namecheap DPA, accompanied by a documented **TIA**. Namecheap does not appear DPF-certified.
- **USA (Let's Encrypt):** domain name for TLS certificate issuance.
- **CH (Infomaniak, planned):** country with an **adequacy decision** — no additional safeguards required.
- No other extra-EEA transfer takes place; any new recipients (including LLM providers) will be added to this section **before** activation, with the relevant mechanism.

---

## 6. Retention

| Data | Retention |
|---|---|
| Mailbox contents (maildir) | While the mailbox is active; deleted upon mailbox removal or termination |
| Mailbox and database backups | **30-day** rotation (planned) |
| System and audit logs (panel, MCP) | **90 days** (planned) |
| Landing connection sessions | Session — **8–12h TTL** (to be set) |
| Customer billing and contact data | **10 years** (tax obligations, Italian law); contacts up to 12 months after termination |

⚠️ Values marked "planned/to be set" are **targets**: their technical implementation (lTrim/expire on the Redis API log, session TTL, logrotate, backup rotation) is a **precondition** for publishing this notice. The right to erasure in backups is exhausted by natural rotation (accepted practice).

---

## 7. Security of processing

TLS on all surfaces; API credentials stored server-side encrypted with **AES-256-GCM** and never in logs/pages/repositories; container isolation (read-only rootfs, cap-drop), rate limiting, secret-free audit logs; encrypted backups on a dedicated volume; personnel access to mailbox contents **only upon the customer's authorized request** and logged. In case of a **data breach** with risk to data subjects, notification to the supervisory authority without undue delay (Art. 33) and communication to data subjects when high risk (Art. 34); for tenant mailboxes, immediate notification to the customer-controller per the DPA.

---

## 8. AI agents

Today **no AI agent reads mailboxes** (the available tools operate at domain level). If the feature is activated: minimal per-task extraction, **no model training on contents**, actions towards third parties with **human review** (human-in-the-loop), **no exclusively automated decisions** with legal or similarly significant effects (Art. 22 GDPR — e.g. account suspension/deletion, binding responses), auditable per-agent keys, LIA and Art. 14 notice to correspondents before activation.

---

## 9. Data subject rights

9.1 You have the rights of Artt. **15–22 GDPR**: access, rectification, erasure, restriction, portability, objection; for processing based on legitimate interest you may object at any time (Art. 21). Exercise of rights: [PRIVACY EMAIL — TO BE COMPLETED]; response within 1 month.

9.2 **External senders/recipients** (people writing into hosted mailboxes): contact [PRIVACY EMAIL] — for customer-tenant mailboxes we are the processor and will assist the customer-controller in handling your request (Art. 28(3)(e)).

9.3 **Complaint:** you may lodge a complaint with the **Garante per la protezione dei dati personali** (www.garanteprivacy.it) or, in Switzerland, the **FDPIC**; and in any case with the controller.

---

## 10. Changes to this notice

Amendments are published on this page with an update date; substantial changes are notified to customers by email. Current version: 1.0-draft, 2026-10-19.

---

*Drafted on the basis of the privacy review 2026-10-19 (data verified in the code: landing, Go backend, docker-compose) and verified vendor documents (Hetzner DPA, T&C §6.2, Namecheap privacy/DPA, Infomaniak DPA, EU→CH adequacy decision). To be reviewed with qualified legal counsel IT/CH before publication.*
