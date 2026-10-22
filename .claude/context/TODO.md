# TODO

Updated: 2026-10-22. Source of truth for priorities: `docs/agentmail-re/09-execution-plan.md` and ADR `docs/agentmail-re/10-native-platform.md`.

## Done
* Reverse engineering pack of agentmail.to and planning docs 01 to 14 (PRD, API, architecture, ADR, GTM and pricing, ISO 27001 and AI audit, support and SLA, disaster recovery).
* mailcow removed; native Go + Rust platform: inboxes, threads, messages, send, reply, labels, attachments, custom domains with DNS verification, plan and rate limits, signed webhooks, scoped API keys, audit log, API and MCP gateways, gatekeeper, SMTP STARTTLS, DKIM, agent to agent mail, RLS, e2e tests.
* k3s + Flux GitOps manifests, Hetzner bootstrap and operations agent runbook, public website in web/, landing kept at /legacy/.
* PR 1 merged on 2026-10-22.
* Outbound retry window 5 days per RFC 5321 (DR8), `MAI_SEND_MAX_AGE` (default `120h`).
* Database role split (R11): `mai_owner` (migrations, init container), `mai_app` (tenant DML), `mai_system` (workers, the only role whose `app.system` is honoured); migration `0004_roles.sql`, e2e proves the runtime roles cannot disable RLS.
* Screen before notify (doc 12 R08, migration `0005_screening.sql`): org setting on by default, `message.received` only after a clean verdict or a release, review messages hidden unless `include_review` with `message_review`, release endpoint and MCP tool with `message_release` and audit, fail closed on gatekeeper down or 15 minute timeout, `mai admin org-screening`.

## Next (in order)
1. Release workflow green on master (fix: Docker build context was missing openapi/).
2. Ops agent: runbook steps 1 to 7 on the CX33 (snapshot, rebuild, firewall, PTR, bootstrap, smoke test). Before that, protect the Primary IP and enable Hetzner backups.
3. Security P0 from doc 12: encrypted append only backups with a restore drill (deploy/dr), least privilege token for the ops agent.
4. Founder decisions: open questions in docs 11, 12, 13, 14 (pricing Growth plan, tools to approve, backup key custody, retention, legal counsel); should full access keys hold `message_release` (the reference excludes it), and a tenant API to change `screen_before_notify` (new `organization_write` permission).
5. Branding, then buy the domain: `mai admin domain-add`, update `deploy/k8s/staging` config, DNS records, PTR.
6. Blob deletion on inbox delete and 90 day address quarantine (blocks deletion promises in DPA and Terms).
7. Console (Next.js) and Stripe billing; drafts, WebSocket events; S3 blob store; pods; metrics.

## Known gaps in the MVP
* Plan limits cover inboxes, domains, day and month sends; the 5 minute burst limit is not implemented.
* SPF and DMARC are not evaluated in Go yet; rspamd results are recorded when `MAI_RSPAMD_URL` is set.
* Outbound direct delivery uses opportunistic TLS; MTA-STS and DANE checks to add.
* No antivirus scanning deployed (clamd is not in the k8s manifests).
* Bounce and complaint auto pause not implemented.
