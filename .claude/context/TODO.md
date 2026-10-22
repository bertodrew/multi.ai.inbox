# TODO

Updated: 2026-10-22. Source of truth for priorities: `docs/agentmail-re/09-execution-plan.md` and ADR `docs/agentmail-re/10-native-platform.md`.

## Done
* Reverse engineering pack of agentmail.to (docs 01 to 10).
* mailcow removed from the repository (2026-10-22, founder approval).
* Production MVP: custom domains with DNS verification, attachments download, plan limits, rate limit, readiness, scheduler, staging seed, k3s + Flux GitOps, CI image release.
* Native MVP in Go + Rust replacing mailcow: inboxes, threads, messages, send, reply, labels, signed webhooks, MCP, gatekeeper heuristics, DKIM, agent to agent delivery, RLS, e2e test.
* Outbound retry window 5 days per RFC 5321 (DR8), `MAI_SEND_MAX_AGE` (default `120h`).

## Next (in order)
1. Founder: rebuild the CX33 with Ubuntu 24.04 and run `deploy/hetzner/bootstrap.sh` (needs GitHub PATs, Brevo SMTP key, Object Storage keys); set firewall and PTR.
2. Merge PR 1 to master: CI publishes the image and Flux deploys it.
3. Branding, then buy the domain: `mai admin domain-add`, update `deploy/k8s/staging` config, DNS records, PTR.
4. Registrar one click DNS (Namecheap, Infomaniak, Cloudflare): single record updates only.
5. S3 blob store (Hetzner Object Storage) behind `blob.Store` (today: PVC + nightly rclone copy).
6. Drafts + scheduled send, WebSocket events, polling webhooks.
7. Console (Next.js + Supabase Auth or own auth) per doc 03; agent self sign up; Stripe billing.
8. Pods, scoped keys, allow and block lists, metrics.
9. Gatekeeper classifier (ONNX) on top of heuristics; feedback endpoint.

## Known gaps in the MVP
* Plan limits cover inboxes, domains, day and month sends; the 5 minute burst limit is not implemented.
* SPF and DMARC are not evaluated in Go yet; rspamd results are recorded when `MAI_RSPAMD_URL` is set.
* Outbound direct delivery uses opportunistic TLS; MTA-STS and DANE checks to add.
