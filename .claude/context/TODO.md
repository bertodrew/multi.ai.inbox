# TODO

Updated: 2026-10-23. Source of truth for priorities: `docs/agentmail-re/09-execution-plan.md` and ADR `docs/agentmail-re/10-native-platform.md`.

## Done
* Reverse engineering pack of agentmail.to and planning docs 01 to 14 (PRD, API, architecture, ADR, GTM and pricing, ISO 27001 and AI audit, support and SLA, disaster recovery).
* mailcow removed; native Go + Rust platform: inboxes, threads, messages, send, reply, labels, attachments, custom domains with DNS verification, plan and rate limits, signed webhooks, scoped API keys, audit log, API and MCP gateways, gatekeeper, SMTP STARTTLS, DKIM, agent to agent mail, RLS, e2e tests.
* k3s + Flux GitOps manifests, Hetzner bootstrap and operations agent runbook, public website in web/, landing kept at /legacy/.
* PR 1 merged on 2026-10-22.
* Outbound retry window 5 days per RFC 5321 (DR8), `MAI_SEND_MAX_AGE` (default `120h`).
* Database role split (R11): `mai_owner` (migrations, init container), `mai_app` (tenant DML), `mai_system` (workers, the only role whose `app.system` is honoured); migration `0004_roles.sql`, e2e proves the runtime roles cannot disable RLS.
* Screen before notify (doc 12 R08, migration `0005_screening.sql`): org setting on by default, `message.received` only after a clean verdict or a release, review messages hidden unless `include_review` with `message_review`, release endpoint and MCP tool with `message_release` and audit, fail closed on gatekeeper down or 15 minute timeout, `mai admin org-screening`.
* Inbox deletion removes raw `.eml` and attachment blobs through a durable queue (`0006_deletion.sql`, triggers + scheduler) and quarantines the address for 90 days (`409 address_quarantined`); MCP `delete_inbox`.
* Antivirus: clamd Deployment in `deploy/k8s/base/clamd.yaml`, INSTREAM scan at SMTP DATA (`554 5.7.1` on FOUND), fail open with `av-unscanned` by default, `MAI_CLAMD_FAIL_CLOSED=true` for `451 4.7.1`.
* Encrypted add only backups (rclone crypt, `mai_backup` read only role, Object Lock ready), weekly restore drill in `deploy/k8s/base/restore-drill.yaml`, least privilege ops runbook (Flux deploy key, Primary IP and server protection, Hetzner backups).
* Drafts with scheduled send (`0008_drafts.sql`), MCP draft tools; Server Sent Events stream `GET /v0/events/stream` (redacted like webhooks, resume with Last-Event-ID).
* S3 blob store (Hetzner Object Storage, hand written SigV4) behind `MAI_BLOB_BACKEND=s3`; Prometheus metrics on `MAI_METRICS_ADDR`.
* SPF and DMARC at the SMTP edge (`MAI_DMARC_ENFORCE`), MTA-STS for direct delivery, 5 minute send burst limit (10% of the daily cap).
* Stripe billing over net/http (`0009_billing.sql`): checkout, portal, signed webhooks, 7 day grace period; Growth plan in code.
* Organization delete (`mai admin org-delete`), antivirus for API attachments, drafts and agent to agent mail, complaint breaker from ARF reports (`0011_complaints.sql`).
* Customer console at `/console` (Zod validated, tested in Chromium desktop and mobile).
* Bounce breaker (`0007_sending_health.sql`): per inbox and per org, hard bounce rate above 5% over at least 20 sends in 24h pauses sending (`403 sending_paused`, events `inbox.sending.paused` / `organization.sending.paused`); DSNs with a null sender count as bounces (`message.bounced`); `mai admin inbox-resume` / `org-resume`.

## Next (in order)
1. Release workflow green on master (fix: Docker build context was missing openapi/).
2. Ops agent: runbook steps 1 to 7 on the CX33 (snapshot, rebuild, firewall, PTR, bootstrap, smoke test). Before that, protect the Primary IP and enable Hetzner backups.
3. Founder decisions still open: legal counsel; a tenant API to change `screen_before_notify` (new `organization_write` permission).
4. Branding, then buy the domain: `mai admin domain-add`, update `deploy/k8s/staging` config, DNS records, PTR.
5. Stripe setup by the founder (prices, webhook, portal, Stripe Tax) and keys in the staging secrets.
6. Pods; IMAP; DANE; Prometheus in the cluster; an S3 migration tool for existing blobs.

## Founder decisions (2026-10-22)
* Growth plan at €59 per month (doc 11).
* Backup crypt keys: Doppler plus one offline copy; a cofounder gets a custody copy later.
* Mail retention: kept until the customer deletes it; backups expire on their own schedule.
* ClamAV (GPL, separate network service) approved.
* `message_release` is explicit only: full access keys do not hold it; `mai admin key-create -permissions` issues the first release key.

## Known gaps in the MVP
* Outbound direct delivery enforces MTA-STS; DANE not checked yet.
* Relay bounces count only when rejected at send time (Brevo keeps its own return path). `MAI_FBL_ADDRESS` is not set in the manifests yet; reserve that username.
* Blob backups use `rclone copy`, so blobs of deleted inboxes stay in Object Storage; decide retention or switch to sync with versioning.
