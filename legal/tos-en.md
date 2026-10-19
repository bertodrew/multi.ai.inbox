# Terms of Service — multi.ai.inbox (mail hosting service)

**Version:** 1.0-draft · **Date:** 2026-10-19 · **Effective date:** [TO BE SET AT LAUNCH] · **Authentic language:** Italian (`tos-it.md`); this English translation is for convenience — in case of divergence the Italian version prevails.

> ⚠️ **INTERNAL DRAFT — DO NOT PUBLISH AS-IS.** Drafted as part of the multi.ai.inbox legal package (legal review 2026-10-19, finding P0-1). To be reviewed by qualified legal counsel (IT/CH jurisdiction) **before commercial launch and before the first paying customer**. Fields in square brackets `[...]` must be completed before publication.

---

## 1. Parties, imprint and contacts

1.1 **Service provider:** [FULL LEGAL NAME AND ENTITY FORM — TO BE COMPLETED], with registered office at [REGISTERED OFFICE — TO BE COMPLETED], tax/VAT ID [TO BE COMPLETED], company register [TO BE COMPLETED] (the "**Provider**").

1.2 **Customer:** the natural or legal person who signs up for the service (the "**Customer**" or, under the self-service model, the "**Tenant**").

1.3 **Contacts:** support: [SUPPORT EMAIL — TO BE COMPLETED] · legal: [LEGAL EMAIL — TO BE COMPLETED] · privacy: [PRIVACY EMAIL — TO BE COMPLETED].

1.4 **Software and trademark notice.** The service runs on **multi.ai.inbox, a fork of mailcow-dockerized**, licensed under GNU GPL-3.0. "mailcow" is a registered word mark of The Infrastructure Company GmbH; multi.ai.inbox **is not affiliated with, sponsored or endorsed by** mailcow or The Infrastructure Company GmbH. Upstream software components (postfix, dovecot, SOGo, nginx, rspamd, ClamAV and others) remain the property of their respective authors.

---

## 2. Service description

2.1 The Provider provides, in **self-service** mode (per-tenant signup and billing):

- **mailboxes** with SMTP/IMAP access and the **SOGo webmail**;
- a web **administration panel** (standard interface of the fork, based on mailcow-dockerized);
- **DNS management for domains** through a connector to the registrar account of the Customer (currently a Namecheap connector; Infomaniak connector planned), when the Customer links their own API key;
- **API / MCP** for automation (subject to authentication and the limits of this agreement);
- server-side **antispam and antivirus** (rspamd, ClamAV), **TLS** with Let's Encrypt certificates, **SPF, DKIM, DMARC, MTA-STS** management for hosted domains;
- **backups** (§9).

2.2 **Infrastructure.** The service runs on a **single instance (single-tenant, single-instance)** hosted with **Hetzner Online GmbH (Germany, EU)**, without geographic redundancy. This characteristic is reflected in the SLA (§4) and is expressly disclosed.

2.3 **Not included.** [TO BE COMPLETED AT LAUNCH — e.g.: bulk migration of existing mailboxes, dedicated deliverability consulting, per-tenant interface branding (the admin interface is standard and not customizable per tenant today), SLA above 99.5%.] What is not expressly included is not owed.

2.4 **Requirements.** To use the registrar connector the Customer must hold their own account with a supported registrar and a **personal API key** meeting the registrar's requirements (for Namecheap: production API access thresholds — typically ≥ 20 registered domains or ≥ 50 USD balance/spend, **IPv4** whitelist of the Provider server IP, rate limits 50/min · 700/hour · 8,000/day). The Provider documents these requirements in onboarding.

---

## 3. Signup, term, renewal, prices

3.1 Signup is self-service through the platform; the contract is concluded upon activation of the first paid service. [TO BE COMPLETED: identity/minimum-age verification, express acceptance of the Terms.]

3.2 **Term and renewal:** [TO BE COMPLETED AT LAUNCH — e.g.: monthly/annual contract with tacit renewal; termination with 30 days' notice.]

3.3 **Prices and invoicing:** as per the price list published at order time [PRICE LIST — TO BE COMPLETED]. Prices are net of applicable VAT/taxes; invoicing [TO BE COMPLETED: periodicity, payment methods]. Domain/registrar payments remain on the direct Customer–registrar relationship: the Provider does not resell domains and does not collect registrar fees on behalf of third parties.

3.4 **Non-payment:** reminder within [X days], suspension of the service (§6) after [Y days], termination after [Z days].

---

## 4. Service level (SLA)

4.1 The Provider commits to a **monthly uptime of 99.5%** for the mail services (SMTP, IMAP) and webmail, **measured** and backed by service credits under the attached **SLA** (document `SLA`), an integral and substantial part of this agreement.

4.2 **Nature of the commitment.** 99.5% reflects delivery on a **single instance without geographic redundancy** (§2.2): 99.9% or higher is not promised until redundancy is implemented. The SLA defines measurement, exclusions, credits and response times.

---

## 5. Acceptable use policy (AUP)

5.1 **Prohibited.** Using the service for: sending **unsolicited commercial email (UCE/UBE, "spam")** or **mail bombing**; **phishing** or identity theft; distribution of **malware**; content illegal under Italian and European law (including copyright, hate speech, child protection); infringement of third-party rights; **reverse engineering of APIs** or circumvention of protections; deliberate **overload** of systems and APIs, own or third-party; address harvesting for spam. The AUP is aligned with registrar policies (Namecheap Universal ToS §7 and Hosting AUP §9) to avoid chain terminations.

5.2 **Documented opt-in.** For newsletters/bulk communications the Customer must warrant documented **opt-in** consents and compliance with the Italian Privacy Code, GDPR and, where applicable, the CAN-SPAM Act. On request the Provider may require proof of consents.

5.3 **Consequences of abuse.** Spam or abuse by **a single tenant** on the server's shared IP can cause **blacklisting of the IP**, destroying deliverability for **all** hosted domains and triggering termination of the registrar account. The following rules therefore apply:

- **immediate suspension** of the responsible mailboxes (§6), even without notice where spam is actively ongoing;
- **pass-through charge** to the responsible Customer for third-party penalties incurred, including the **penalty of up to USD 500** that the registrar (Namecheap, Hosting AUP §9) assesses in spam/blacklisting cases, plus documented delisting, monitoring and mitigation costs;
- the Provider reports manifestly unlawful uses to the competent authorities.

5.4 **Monitoring.** The Provider monitors outbound traffic (rspamd antispam) and sending rates; per-mailbox/hour sending limits may be imposed to protect the shared reputation [VALUE — TO BE COMPLETED].

---

## 6. Suspension

6.1 The Provider may **suspend** the service, in whole or in part, for: AUP violation (§5); **non-payment**; concrete threat to the **security** of the instance or of third parties (compromise, open relay, botnet, ongoing blacklisting); binding request of authorities or registrar; urgent extraordinary maintenance.

6.2 **Timing and effects.** Suspension for AUP violation may be **immediate and without notice** where the abuse is ongoing. The Provider notifies the suspension [within 24 hours] with its motivation. Re-activation follows removal of the cause and, in payment cases, settlement; the Provider may require reasonable assurances of non-repetition.

6.3 Suspension days **do not** entitle the Customer to a fee reduction where the suspension results from the Customer's violation; they entitle the Customer to SLA credit where attributable to the Provider (subject to SLA exclusions).

---

## 7. Credentials, Customer API keys and registrar connectors

7.1 The Customer's **API keys** (registrar and other connectors) are stored server-side **encrypted (AES-256-GCM)**, used **only for the agreed operations** (reading/managing DNS and domains of the Customer) and **never appear in logs, pages or the repository**. Scope: a read-only key is recommended where the flow allows it; immediate revocation by the Provider upon disconnect and termination, and the Customer must revoke the key directly with the registrar upon termination.

7.2 The Customer is responsible for the correctness and scope of the key provided and **notifies the Provider immediately** of any suspected compromise. The Provider notifies the Customer of any anomalous access detected on stored credentials.

7.3 **Domain ownership.** Domains remain **registered in the Customer's own registrar account**: the Provider acts as an API management tool, **not** as a registrar reseller, assumes no ICANN/RAA obligations and has no title to hold or transfer the domains. Under no circumstances does the Provider move domains into its own accounts without written mandate.

---

## 8. AI agents

8.1 The service may provide **AI agents** for triage, task extraction and draft proposals on mailboxes. This is a governed feature: agents operate with per-agent auditable keys; actions with effects towards third parties require **human review** (human-in-the-loop).

8.2 **No exclusively automated decisions** under Art. 22 GDPR (legal or similarly significant effects: account suspension/deletion, binding responses) are executed by agents without human intervention. Mailbox contents are **not used to train models** without a specific legal basis and the Customer's consent.

8.3 Upon activation of the feature, the Privacy Policy (§11) and, for tenants, the attached DPA apply.

---

## 9. Backups and retention

9.1 The Provider runs **encrypted backups** of mailboxes and configuration on a dedicated volume, with **[30-day — planned] rotation**. Contractual **RPO/RTO targets are stated only after the restore-drill result**: until then the backup is an interim commitment, not a guaranteed result. [RPO/RTO — TO BE SET AFTER THE DRILL.]

9.2 **Retention of service data:** mailboxes (maildir) while active; backups until rotation; system and audit logs [90 days — planned]; invoicing data per tax obligations [10 years, Italian law]. At termination: export and deletion (§12).

---

## 10. Ownership and processing of Customer data

10.1 **Ownership.** Mail, contacts, attachments and brand data belong to the **Customer**. The Provider processes them solely to deliver the service and per the Customer's documented instructions; for tenant mailboxes the Provider is the **processor** under Art. 28 GDPR and the Customer the **controller** (attached DPA), while for the Customer's contact and billing data the Provider is an independent controller.

10.2 **Access to mailbox contents** by Provider personnel happens **only on the Customer's authorized request** (support, incidents, rights requests) and is documented in audit logs. No use of customer data — **not even aggregate analytics** — without written consent.

10.3 Processing of personal data is governed by the **Privacy Policy** (document `Privacy Policy`) and, for tenants, the **DPA** (document `DPA`), integral parts of this agreement.

---

## 11. Software, warranty and deliverability

11.1 **Upstream software without warranty.** The underlying software components (mailcow-dockerized and its components) are provided "**AS IS**" without any warranty under GPLv3 §15. This **does not reduce** the service obligations: the warranty covers the **managed service** delivered by the Provider (this agreement + SLA), not the upstream code as such.

11.2 **Deliverability disclaimer.** The server IP is a datacenter IP: mail delivery also depends on the IP's reputation with recipients, recipient policies, external blacklists and domain **warm-up** practices. The Provider manages SPF/DKIM/DMARC/MTA-STS and monitors reputation, but **does not guarantee inbox placement** with third parties; blacklists caused by the Customer's content or practices are excluded from the SLA and charged to the Customer (§5.3).

---

## 12. Termination and exit

12.1 Upon termination (expiry, notice, withdrawal or resolution), the Customer is entitled to: **full export** of mailboxes (standard Maildir/EML format and/or IMAP migration), export of domain/configuration data, and **migration assistance** within [30 days of termination]; thereafter data is deleted, with residual copies in backups until natural rotation (stated in the Privacy Policy).

12.2 The Provider revokes stored credentials/API keys and assists with the agreed removal/reworking of DNS records. The **domain remains in the Customer's registrar account** (§7.3): no lock-in. No exit penalty beyond amounts due.

12.3 The Provider may withdraw with [30 days'] notice for justified reason (business closure, loss of infrastructure requirements), granting export and migration assistance equal to §12.1.

---

## 13. Limitation of liability

13.1 Except for intent or gross negligence, liability for death/personal injury, and cases where liability cannot be excluded by law, the Provider's overall liability is **limited to the fees paid by the Customer in the last 12 months**; indirect damages, loss of profit, loss of data attributable to the Customer, business opportunities and reputation are **excluded**.

13.2 **GDPR fines:** administrative fines imposed on a party for its own violation remain at that party's expense and are not transferred nor subject to the cap (§13.1); the cap does not limit compensation obligations towards data subjects within the limits of the law. The Provider will take out **professional indemnity insurance** [TO BE ACTIVATED — POLICY DETAILS TO BE COMPLETED].

13.3 **Force majeure** (external blacklisting not attributable to the Provider, failures of the infrastructure provider Hetzner, outages of third-party connector/registrar services, exceptionally large DDoS attacks) **excludes** the Provider's liability for the duration of the event and is excluded from the SLA.

---

## 14. Amendments to the Terms

14.1 Amendments are notified with [30 days'] notice via email/dashboard. If the Customer does not accept, they may terminate with immediate effect within the notice period; silence beyond the term constitutes acceptance. Amendments do not retroactively affect ongoing contracts.

---

## 15. Governing law and venue

15.1 This agreement is governed by **Italian law**. The courts of [PROVIDER'S REGISTERED OFFICE — TO BE COMPLETED] have jurisdiction, save for the mandatory consumer forum where applicable.

15.2 **Versions and precedence.** The Italian version is authentic; the English version is for convenience. In case of conflict among the Terms, SLA, DPA and Privacy Policy the order of precedence is: DPA (data processing) → SLA (measurement and credits) → Terms.

---

## Annexes (integral parts)

- **SLA** (`sla.md`) — 99.5% monthly uptime, measurement, exclusions, credits, response times.
- **DPA Art. 28 GDPR** (`dpa-template.md`) — for tenants, processing of personal data in mailboxes.
- **Privacy Policy** (`privacy-it.md`) — controller, legal bases, recipients, transfers, retention, cookies, rights.
- **AUP** — §5 of this document (no divergent duplicate policies).
