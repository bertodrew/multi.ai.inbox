# Stack rules

Approved by the founder on 2026-10-22 (decision D8). Anything not listed needs a discussion first.

## Languages
* Go for connectivity (SMTP, API, MCP, workers). Rust for AI and parsing workloads. TypeScript for the web console.

## Go modules
| Module | Licence | Use |
|--------|---------|-----|
| github.com/jackc/pgx/v5 | MIT | Postgres driver and pool |
| github.com/emersion/go-smtp | MIT | SMTP server (mx) and client (sender) |
| github.com/emersion/go-sasl | MIT | SMTP AUTH for the relay |
| github.com/emersion/go-message | MIT | MIME parsing and building |
| github.com/emersion/go-msgauth | MIT | DKIM sign and verify |
| github.com/modelcontextprotocol/go-sdk | MIT | MCP server |

## Web packages (web/)
Next.js, React, Tailwind CSS and their tooling as pinned in web/package.json, plus:

| Package | Licence | Use |
|---------|---------|-----|
| zod 4.6.5 (exact pin, published 2026-09-13) | MIT | validation of console inputs and API responses |

## Rust crates
| Crate | Licence | Use |
|-------|---------|-----|
| regex | MIT/Apache-2.0 | gatekeeper rules |
| serde, serde_json | MIT/Apache-2.0 | JSON lines protocol |

## Runtime and infrastructure
| Component | Licence | Use |
|-----------|---------|-----|
| Postgres 16 | PostgreSQL | primary database, self hosted (D9) |
| rspamd | Apache-2.0 | spam scoring over HTTP |
| Caddy | Apache-2.0 | TLS termination, Let's Encrypt |
| k3s | Apache-2.0 | Kubernetes on the Hetzner server |
| Flux | Apache-2.0 | GitOps from this repository |
| rclone | MIT | off site backups to Hetzner Object Storage |
| Brevo | service | EU SMTP relay until outbound port 25 is confirmed |
| ClamAV (clamd) | GPL-2.0 | antivirus as a separate network service (INSTREAM over TCP); never linked into `mai`, approved by the founder on 2026-10-22 |
| Doppler | service | founder custody of the backup crypt keys (plus an offline copy; a cofounder later), not used by the cluster at runtime |

## Not allowed
* AWS services for mail (founder wants EU IaaS).
* GPL or AGPL code inside the product (the code is proprietary, D6). Separate GPL services reached over the network, such as clamd, are allowed case by case.
