# TODO

Updated: 2026-10-22. Source of truth for priorities: `docs/agentmail-re/09-execution-plan.md` and ADR `docs/agentmail-re/10-native-platform.md`.

## Done
* Reverse engineering pack of agentmail.to and planning docs 01 to 14 (PRD, API, architecture, ADR, GTM and pricing, ISO 27001 and AI audit, support and SLA, disaster recovery).
* mailcow removed; native Go + Rust platform: inboxes, threads, messages, send, reply, labels, attachments, custom domains with DNS verification, plan and rate limits, signed webhooks, scoped API keys, audit log, API and MCP gateways, gatekeeper, SMTP STARTTLS, DKIM, agent to agent mail, RLS, e2e tests.
* k3s + Flux GitOps manifests, Hetzner bootstrap and operations agent runbook, public website in web/, landing kept at /legacy/.
* PR 1 merged on 2026-10-22.
* Outbound retry window 5 days per RFC 5321 (DR8), `MAI_SEND_MAX_AGE` (default `120h`).
* Inbox deletion removes raw `.eml` and attachment blobs through a durable queue (`0006_deletion.sql`, triggers + scheduler) and quarantines the address for 90 days (`409 address_quarantined`); MCP `delete_inbox`.
* Antivirus: clamd Deployment in `deploy/k8s/base/clamd.yaml`, INSTREAM scan at SMTP DATA (`554 5.7.1` on FOUND), fail open with `av-unscanned` by default, `MAI_CLAMD_FAIL_CLOSED=true` for `451 4.7.1`.
* Bounce breaker (`0007_sending_health.sql`): per inbox and per org, hard bounce rate above 5% over at least 20 sends in 24h pauses sending (`403 sending_paused`, events `inbox.sending.paused` / `organization.sending.paused`); DSNs with a null sender count as bounces (`message.bounced`); `mai admin inbox-resume` / `org-resume`.

## Next (in order)
1. Release workflow green on master (fix: Docker build context was missing openapi/).
2. Ops agent: runbook steps 1 to 7 on the CX33 (snapshot, rebuild, firewall, PTR, bootstrap, smoke test). Before that, protect the Primary IP and enable Hetzner backups.
3. Security P0 from doc 12: split database roles (owner, runtime, system), screen before notify option with review messages hidden by default, encrypted append only backups with a restore drill (deploy/dr), least privilege token for the ops agent.
4. Founder decisions: open questions in docs 11, 12, 13, 14 (pricing Growth plan, tools to approve, backup key custody, retention, legal counsel).
5. Branding, then buy the domain: `mai admin domain-add`, update `deploy/k8s/staging` config, DNS records, PTR.
6. Console (Next.js) and Stripe billing; drafts, WebSocket events; S3 blob store; pods; metrics.

## Known gaps in the MVP
* Plan limits cover inboxes, domains, day and month sends; the 5 minute burst limit is not implemented.
* SPF and DMARC are not evaluated in Go yet; rspamd results are recorded when `MAI_RSPAMD_URL` is set.
* Outbound direct delivery uses opportunistic TLS; MTA-STS and DANE checks to add.
* Complaints (feedback loops, ARF) are not tracked; the breaker counts hard bounces only. DSNs reach us only for direct delivery (Brevo keeps its own return path), so relay bounces count only when rejected at send time.
* Antivirus covers inbound internet mail only: agent to agent mail and attachments uploaded through the API are not scanned. Alerting on `av-unscanned` to add.
* Blob backups use `rclone copy`, so blobs of deleted inboxes stay in Object Storage; decide retention or switch to sync with versioning.
* No organization delete command yet; the deletion triggers already cover an org delete (cascade).
