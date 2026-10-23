# Data Processing Agreement (template) — AImulti.inbox

> **Internal note (IT).** Questo è un template tecnico, non un contratto valido: le
> clausole, la legge applicabile, i termini di preavviso e gli allegati vanno validati da
> un avvocato (`[lawyer]`) prima dell'uso con un cliente. I segnaposto `[ ]` sono dati
> reali che non possiamo inventare. Pagina pubblica corrispondente:
> `landing/legal/dpa.html`; lista sub-processor: `docs/legal/subprocessors.md`.

**Data Processing Agreement**

between

**[Customer legal name]**, registered office at [address], VAT/registration no. [number]
(the "**Controller**"),

and

**[Provider legal name]**, registered office at [address], VAT/registration no. [number]
(the "**Processor**"),

together the "**Parties**".

## 1. Subject matter, duration, nature and purpose

1.1 The Processor provides the email infrastructure service described in the Order or in
the Terms of Service (the "**Service**"). This Agreement applies to every processing of
personal data carried out by the Processor on behalf of the Controller in the context of
the Service.

1.2 Duration: for the term of the underlying service agreement and until return or
deletion of the personal data under §10.

1.3 Nature and purpose: hosting of mailboxes, reception, storage and transmission of
email and attachments, screening of inbound mail, attachment scanning, delivery of
webhook events, security logging and support. Annex I lists the categories of data and
data subjects.

## 2. Roles

2.1 For the personal data of the Controller's end users and correspondents processed in
the mailboxes, the Controller is the controller and the Processor acts only on documented
instructions.

2.2 For contact, billing and account data of the Controller, the Processor acts as an
independent controller as described in its Privacy Policy `[lawyer: confirm the split]`.

## 3. Processor obligations

3.1 **Instructions.** The Processor processes personal data only on documented
instructions from the Controller, including with regard to transfers to a third country,
unless required to do otherwise by Union or Member State law; in that case it informs the
Controller unless the law prohibits it. The Terms of Service, this Agreement and the
settings the Controller applies in the Service (for example `screen_before_notify`) are
the documented instructions.

3.2 **Confidentiality.** Persons authorised to process the data are bound by
confidentiality and receive access only when needed.

3.3 **Security.** The Processor implements the technical and organisational measures in
Annex II and assists the Controller in ensuring compliance with Art. 32 GDPR.

3.4 **Sub-processors.** Annex III lists the authorised sub-processors. The Processor
informs the Controller of any intended change at least **30 days** in advance
`[lawyer: term]`, giving the Controller the opportunity to object. The Processor imposes
the same data protection obligations on each sub-processor by contract, and remains
liable for their performance.

3.5 **Data subject requests.** Taking into account the nature of the processing, the
Processor assists the Controller by appropriate technical and organisational measures.
The Service provides self-service export (`GET /v0/export`) and deletion
(`DELETE /v0/organization`), described in the API documentation. Where a request is
addressed directly to the Processor and concerns Controller data, the Processor forwards
it without undue delay and does not answer it on the Controller's behalf
`[lawyer: wording of the referral]`.

3.6 **Assistance.** The Processor assists the Controller with data protection impact
assessments and prior consultations, taking into account the information available to it.

3.7 **Personal data breach.** The Processor notifies the Controller without undue delay
after becoming aware of a personal data breach, with the information listed in Art.
33(3) GDPR to the extent available, and supports the Controller in its own notification
`[lawyer: hours]`. See `docs/legal/incident-response-breach.md`.

3.8 **Audit.** The Processor makes available all information necessary to demonstrate
compliance and allows for and contributes to audits, including inspections, by the
Controller or an auditor mandated by it, with **30 days** notice, at most once a year,
during business hours, subject to confidentiality and to not accessing other tenants'
data `[lawyer: scope]`.

## 4. Controller obligations

4.1 The Controller is responsible for the lawfulness of the processing, for the content
of the mailboxes, for the information given to its data subjects, and for the instructions
it gives.

4.2 The Controller ensures that its use of the Service (including AI agents acting through
API keys) complies with applicable law and does not instruct the Processor to process data
unlawfully.

## 5. International transfers

5.1 The Processor hosts the Service in the European Union (Hetzner Online GmbH, Germany).
Transfers outside the EEA occur only towards the sub-processors listed in Annex III under
§5.2.

5.2 Where a sub-processor is located outside the EEA without an adequacy decision, the
transfer relies on the Standard Contractual Clauses (Commission Implementing Decision (EU)
2021/914), module two (controller to processor), incorporated by reference, together with
the transfer impact assessment described in Annex IV `[lawyer: complete the TIA]`.

## 6. Deletion and return

6.1 On termination, at the Controller's choice, the Processor deletes or returns the
personal data and deletes existing copies, unless Union or Member State law requires
storage. The Service offers export before deletion; deletion is available self-service and
through the operator.

6.2 Deleted mailbox addresses remain quarantined for 90 days so that they cannot be
re-registered by another customer. This quarantine stores the address only, not the
mailbox content.

6.3 Backup copies are encrypted client-side and kept under a 30-day immutable lock with a
45-day lifecycle for database dumps; deletion from backups is therefore exhausted by
rotation, and immutable blob copies require a manual procedure with the vault key
`[lawyer: Art. 17 position]`.

## 7. Liability and indemnity

7.1 Each Party is liable for its own violations of applicable data protection law. The
limitation of liability in the Terms of Service applies to this Agreement to the extent
permitted by law; administrative fines for a Party's own violation remain at that Party's
expense `[lawyer]`.

## 8. Term, termination, precedence

8.1 This Agreement is effective for the term of the service agreement. In case of
conflict, this Agreement prevails over the Terms of Service and the SLA with respect to
data protection.

8.2 Governing law and forum: `[lawyer: IT / CH / other]`.

---

## Annex I — Categories of data and data subjects

| Data subjects | Categories of personal data |
|---------------|-----------------------------|
| Controller's users and agents (API clients) | Email address, display name, client identifier, API key metadata, IP address, timestamps |
| Correspondents (senders and recipients, including indirect collection) | Email address, name, message headers and content, attachments, delivery-path IPs |
| Controller's administrative contacts | Name, email, billing details (no card data) |

Special categories of data (Art. 9) may appear incidentally in email content. The
Processor does not log content, extracts only what a task requires, and does not train
models on mailbox content.

## Annex II — Technical and organisational measures

| Area | Measure |
|------|---------|
| Encryption in transit | TLS on API, SMTP (STARTTLS), IMAP and web surfaces; MTA-STS |
| Encryption at rest | Backups encrypted client-side (rclone crypt) before upload; credentials and provider secrets encrypted with AES-256-GCM |
| Tenant isolation | PostgreSQL Row Level Security per organization; scoped API keys; explicit-only permissions for sensitive actions (`message_release`, `data_delete`) |
| Access control | API keys with per-permission scopes and expiry; operator actions audited; personnel access to mailbox content only on authorised request, logged |
| Logging and monitoring | Security audit log per organization (`actor`, `action`, `target`, `ip`, `request_id`), 90-day retention, no content or secrets; metrics and health endpoints; backup dead-man's switch |
| Malware and abuse | ClamAV scanning fail-closed, RSPAMD screening, gatekeeper verdict before notification, rate limiting per organization and per IP |
| Resilience | Backups every 6 hours, encrypted, immutable 30 days, restore drills documented |
| Deletion | Cascading deletion of tenant data, asynchronous blob deletion queue with retry, 90-day address quarantine |
| Development | Secrets never in the repository or in logs; migrations under version control; end-to-end tests in CI |

## Annex III — Authorised sub-processors

| Sub-processor | Country | Service | Transfer basis |
|---------------|---------|---------|----------------|
| Hetzner Online GmbH | Germany (EU) | Hosting, database, object storage, backups | Within the EEA |
| Namecheap Inc. | USA | Domain registration and WHOIS | SCC + TIA `[lawyer]` |
| Infomaniak Network SA | Switzerland | Domain connector (not active yet) | Adequacy decision |
| Internet Security Research Group (Let's Encrypt) | USA | TLS certificate issuance (domain name only) | Technical necessity |
| LLM provider `[to be defined]` | `[ ]` | AI agents reading mailboxes (not active) | `[lawyer]` before activation |

The public, always-current list is published at `/legal/subprocessors.html`.

## Annex IV — Transfer impact assessment (outline)

`[lawyer]` — for each third-country recipient: nature of the data, purposes, recipient's
role, applicable legal framework and access by public authorities, supplementary measures
(encryption, pseudonymisation, contractual notification duties, challenge of requests),
and the conclusion. Currently required for Namecheap (USA).
