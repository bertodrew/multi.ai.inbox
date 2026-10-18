# mailcow: dockerized - 🐮 + 🐋 = 💕

[![Translation status](https://translate.mailcow.email/widgets/mailcow-dockerized/-/translation/svg-badge.svg)](https://translate.mailcow.email/engage/mailcow-dockerized/)
[![Twitter URL](https://img.shields.io/twitter/url/https/twitter.com/mailcow_email.svg?style=social&label=Follow%20%40mailcow_email)](https://twitter.com/mailcow_email)
![Mastodon Follow](https://img.shields.io/mastodon/follow/109388212176073348?domain=https%3A%2F%2Fmailcow.social&label=Follow%20%40doncow%40mailcow.social&link=https%3A%2F%2Fmailcow.social%2F%40doncow)


## Want to support mailcow?

Please [consider a support contract with Servercow](https://www.servercow.de/mailcow?lang=en#support) to support further development. _We_ support _you_ while _you_ support _us_. :)

You can also [get a SAL](https://www.servercow.de/mailcow?lang=en#sal) which is a one-time payment with no liabilities or returning fees.

Or just spread the word: moo.

## Many thanks to our GitHub Sponsors ❤️
A big thank you to everyone supporting us on GitHub Sponsors—your contributions mean the world to us! Special thanks to the following amazing supporters:

### 100$/Month Sponsors
  <a href="https://www.colba.net/" target=_blank><img
    src="https://avatars.githubusercontent.com/u/204464723" height="58"
  /></a>
  <a href="https://www.maehdros.com/" target=_blank><img
    src="https://avatars.githubusercontent.com/u/173894712" height="58"
  /></a>
  <a href="https://garske-systems.de/" target=_blank><img
    src="https://garske-systems.de/img/logo_min.png" height="58"
  /></a>

### 50$/Month Sponsors
  <a href="https://github.com/vnukhr" target=_blank><img
    src="https://avatars.githubusercontent.com/u/7805987?s=52&v=4" height="58"
  /></a><a href="https://github.com/humiico" target=_blank><img
    src="https://avatars.githubusercontent.com/u/110134705?s=52&v=4" height="58"
  /></a>

## Info, documentation and support

Please see [the official documentation](https://docs.mailcow.email/) for installation and support instructions. 🐄

🐛 **If you found a critical security issue, please mail us to [info at servercow.de](mailto:info@servercow.de).**

## Cowmunity

[mailcow community](https://community.mailcow.email)

[Telegram mailcow channel](https://telegram.me/mailcow)

[Telegram mailcow Off-Topic channel](https://t.me/mailcowOfftopic)

[Official 𝕏 (Twitter) Account](https://twitter.com/mailcow_email)

---

# multi.ai.inbox — how the service works

This fork turns mailcow into **multi.ai.inbox**: an inbox-as-a-service layer for AI agents across multiple brands (5 domains, ~15 mailboxes), self-hosted on Hetzner (EU).

**What this fork adds on top of upstream mailcow:**

- **Landing page** (`landing/`) — public frontdoor with two login paths: mailcow credentials or **"Login with Namecheap"** (API-key connect; Namecheap has no OAuth for third-party apps)
- **Post-login connector choice** — *Login with Namecheap* (list domains, manage DNS) or *Use dedicated MCP* (the fork's native MCP server)
- **Per-domain Settings page** (`domain_settings` table) — mail FQDN, branding, PTR target, quotas + auto-generated DNS snippets per domain
- **Native MCP server** — tools `list_domains`, `get_domain_settings`, `set_domain_settings` on top of `json_api.php`, authenticated with the same `api` key table
- **Registrar connectors** — Namecheap (API key) and Infomaniak (OAuth v2, multiple organizations)

## Architecture

```mermaid
flowchart TB
    User["👤 Founder / Agent"]

    subgraph LANDING["multi.ai.inbox landing page"]
        L["login: mailcow · Namecheap API key<br/>post-login: registrar connector or dedicated MCP"]
    end

    subgraph HETZNER["Hetzner CX33 · 2.28.99.252 (EU)"]
        subgraph STACK["Mailcow stack — 18 containers"]
            NGINX["nginx"]
            MCP["MCP server (fork)<br/>list_domains · get/set_domain_settings"]
            API["json_api.php<br/>tables: api (key+IP whitelist) · domain_settings"]
            MAIL["postfix · dovecot · SOGo"]
            SEC["rspamd · clamd · netfilter"]
            ACME["acme — Let's Encrypt"]
        end
        DB[("MySQL")]
    end

    NC["Namecheap API<br/>domains · DNS hosts"]
    INF["Infomaniak API<br/>kSuite · domains · DNS (multi-org)"]
    DNS["DNS zone<br/>A mail → 2.28.99.252 · MX · SPF · DKIM · DMARC"]

    User --> L
    L --> API
    MCP --> API
    API --> DB
    L --> NC
    L --> INF
    NC --> DNS
    INF --> DNS
    DNS -- "A + MX" --> NGINX
    ACME -- "TLS cert" --> NGINX
    DNS -- "SPF/DKIM/DMARC" --> MAIL
```

## Domain provisioning flow

```mermaid
sequenceDiagram
    actor U as Founder
    participant L as Landing multi.ai.inbox
    participant MC as Mailcow API
    participant R as Registrar (Namecheap / Infomaniak)
    participant AC as acme (Let's Encrypt)

    U->>L: login (mailcow or registrar API key)
    L->>R: list domains
    R-->>L: domain list
    U->>L: pick domain + settings (FQDN, branding, quotas)
    L->>MC: add domain + domain_settings
    MC-->>L: DKIM key generated
    L->>R: set A (mail → 2.28.99.252), MX, SPF, DKIM, DMARC
    R-->>L: zone updated
    AC->>AC: issue TLS cert once A record propagates
    U->>L: create mailbox hello@domain
```

## Mail flow

```mermaid
flowchart LR
    IN["Internet"] <-->|":25 ⚠️ Hetzner ticket required"| PW["postfix"]
    PW --> RS["rspamd"] --> DV["dovecot"] --> SG["SOGo · /user"]
```

**Security model:** the Namecheap/Infomaniak keys live server-side (never in the repo or logs); DNS writes use single-record updates only (`update_dns_record`) — full-zone replaces (`setHosts`) would wipe SPF/DKIM/DMARC.

---

[mailcow community](https://community.mailcow.email)

## Misc

**Important**: mailcow makes use of various open-source software. Please assure you agree with their license before using mailcow.
Any part of mailcow itself is released under **GNU General Public License, Version 3**.

mailcow is a registered word mark of The Infrastructure Company GmbH, Parkstr. 42, 47877 Willich, Germany.

The project is managed and maintained by The Infrastructure Company GmbH.

Originated from @andryyy (André)
