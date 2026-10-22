# TODO

Updated: 2026-10-22. Source of truth for priorities: `docs/agentmail-re/09-execution-plan.md` and ADR `docs/agentmail-re/10-native-platform.md`.

## Done
* Reverse engineering pack of agentmail.to (docs 01 to 10).
* Native MVP in Go + Rust replacing mailcow: inboxes, threads, messages, send, reply, labels, signed webhooks, MCP, gatekeeper heuristics, DKIM, agent to agent delivery, RLS, e2e test.

## Next (in order)
1. Remove the mailcow tree from the repo (needs founder approval, blocked by the safety classifier in the last session).
2. Decide D6 (licence), D8 (libraries), D9 (Postgres location), D10 (founder's human mailboxes).
3. Production cutover on Hetzner: firewall 25 + 443, PTR, `.env`, first `domain-add`, MX switch, relay (SES EU) until port 25 opens.
4. Custom domains API: `POST /v0/domains`, verification job (DNS lookups), `domain.verified` event.
5. Registrar one click DNS (Namecheap, Infomaniak, Cloudflare): single record updates only.
6. S3 blob store (Hetzner Object Storage) behind `blob.Store`.
7. Drafts + scheduled send, WebSocket events, polling webhooks.
8. Console (Next.js + Supabase Auth) per doc 03; agent self sign up.
9. Pods, scoped keys, allow and block lists, metrics.
10. Gatekeeper classifier (ONNX) on top of heuristics; feedback endpoint.

## Known gaps in the MVP
* Free plan footer and daily limit exist; month and 5 minute limits do not.
* SPF and DMARC are not evaluated in Go yet; rspamd results are recorded when `MAI_RSPAMD_URL` is set.
* Enrich lease: a crash while a message is `pending` leaves it pending; add a timeout sweep.
* Outbound direct delivery uses opportunistic TLS; MTA-STS and DANE checks to add.
