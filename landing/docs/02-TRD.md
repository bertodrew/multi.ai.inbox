# 02 — TRD (Technical Requirements Document) — AImulti.inbox

**Data**: 21/10/2026 · **Stack reale** (verificato su landing/html, landing/go, data/conf/nginx/site.landingpage.custom): static HTML/CSS + vanilla JS, proxy Go (landing-go, stdlib only, distroless), nginx-mailcow, checkout Mollie, login SOGo.

---

## 1. Architettura ad alto livello

```mermaid
flowchart LR
    U[Browser utente] --> N[nginx-mailcow<br>site.landingpage.custom]
    N -->|"statico<br>CSP stretta"| H[/landingpage-html/<br>11 pagine statiche]
    N -->|"proxy_pass<br>/landingpage/api/ → :8080/api/"| G[landing-go<br>binario Go distroless]
    G -->|Bearer| NC[Namecheap API<br>xml.response]
    G -->|REST| M[Mollie v2<br>checkout/webhook]
    G -->|HTTP interno| MC[Mailcow / SOGo<br>Mail UI su /home]
    A[Agente AI MCP] -->|"JSON-RPC 2025-06-18<br>Bearer gwk_…"| G
    G -->|Terraform worker<br>prov_*.json| HC[Hetzner / Exoscale<br>istanze dedicated]
```

## 2. Sitemap nginx (da site.landingpage.custom, commenti reali)

| Percorso | Cosa | Nota |
|---|---|---|
| `/` | landing AImulti.inbox (statico, root `/landingpage-html`) | CSP stretta, `form-action 'self'` |
| `/home` | Mail UI (login SOGo se non autenticato, dashboard se sì) | invariato |
| `/SOGo/*`, `/rspamd`, ActiveSync, autodiscover | piattaforma mail | invariati (template) |
| `/landingpage/api/` | **API landing (proxy Go)** | `proxy_pass http://landing-go:8080/api/` — prefisso nascosto per non collidere con l'API del pannello (`/api/v1/`) |
| `/landingpage/*` (resto) | 301 → root | vecchi link |
| `/*.html` | subpagine statiche | CSP + X-Frame-Options DENY |

CSP reale sul statico: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'` — nessuna risorsa esterna nel bundle.

## 3. Contenitore Go (landing-go)

- **Build**: `golang:1.24-alpine` (digest pinned) → `CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"` → **distroless** `static-debian12` (digest pinned), EXPOSE 8080, VOLUME `/credentials`, `LANDING_CRED_DIR=/credentials`
- **Dipendenze**: stdlib only (go.mod senza require esterni)
- **File**: main.go (bootstrap/connect/status/domains/dns/namecheap/mcp), checkout.go, gateway.go, provisioning.go, spend.go, testlogin.go + 5 file di test

## 4. Rotte API reali (mux.HandleFunc di main.go e gateway.go)

| Rotta | Metodo | Funzione |
|---|---|---|
| `/api/status` | GET | stato sessione + CSRF + origine posta |
| `/api/connect` | POST | connessione Namecheap (credenziali per sessione) |
| `/api/disconnect` | POST | logout + drop sessione |
| `/api/domains` | GET | domini del registrar |
| `/api/gateway/domains/{domain}` | GET | dettaglio dominio gateway |
| `/api/gateway/domains/{domain}` | PATCH | cap piano enforced (402 se oltre) |
| `/api/gateway/mailboxes` | GET | mailbox del dominio (query `domain=…`, quota MiB, last_imap_login) |
| `/api/env` | GET | origine posta per la UI |
| `/api/dns` | GET | record zona dal registrar |
| `/api/mcp` | POST | MCP interno (protocolOrigin landing-go) |
| `/api/checkout` | POST | self-serve subscriptions (Mollie) |
| `/api/checkout/webhook` | POST | webhook Mollie (verifica server-side) |
| `/api/checkout/state` | GET | piano/status per la UI |
| `/api/provisioning/state` | GET | stato provisioning (FASE 1, iaas-design.md §4) |
| `/api/gateway` | POST | MCP gateway JSON-RPC |
| `/api/gateway/keys` | POST | creazione chiavi `gwk_…` scoped |

## 5. Modello di sicurezza (header di main.go — fix delle 4 finding Strix sul PHP)

| Finding PHP | Fix implementato |
|---|---|
| S1 session fixation (CVSS 9.6) | token 32-byte **server-issued**; cookie client mai fidato come identificatore; **rotazione** su ogni connect (re-issue cookie, invalidazione vecchio store) |
| S2 global credential file (CVSS 9.9) | credenziali **per sessione**: `namecheap_<sha256(sid)[:16]>.json`, **AES-256-GCM** at rest con master key, AAD legato al sid — due utenti non condividono mai stato |
| S3 brute-force (CVSS 5.3) | failed connect limitati **per IP (5/10 min)** oltre che per sessione |
| S4 status oracle (CVSS 5.3) | `/api/status` non espone l'IP egress; client IP preso dalla connessione (X-Real-IP/RemoteAddr) |

CSRF: `ensureCSRF`/`requireCSRF` (header `X-Landing-CSRF`; checkout.js lo legge da `/api/status`). Reap degli orphan credential file dopo 24h (`credTTL`).

## 6. Checkout Mollie (checkout.go)

- `mollieCall`: REST Bearer `MOLLIE_API_KEY`, base `https://api.mollie.com/v2`
- **Fail-closed**: senza chiave tutto risponde **503** "Payments are not configured yet" — niente checkout finti
- `POST /api/checkout {plan}` → `POST /payments` (hosted checkout) → redirect `checkout_url`
- Webhook: il payment si **verifica con GET /payments/{id} con la nostra chiave** (mai fidarsi del body), solo `status=paid` procede
- Su paid: **subscription ricorrente** appesa al customer per-tenant (`cst_…`), primo payment incluso; stato persistito in `bill_<sha256(username)[:16]>.json`
- `planFor`: bill file `status=paid` → tier; altrimenti **trial caps** (1 dominio, 1 mailbox)
- Nodo pieno: `nodeTenantCount() >= maxTenantsPerNode()` (env `MAX_TENANTS_PER_NODE`, default 80) → 503

## 7. Gateway MCP (gateway.go)

- JSON-RPC versione protocollo `2025-06-18`, origin `landing-go`
- Chiavi `gwk_…` con scope: `can_write` + `domain` (chiave scoped → solo il suo dominio, senza toccare il registrar)
- Tool: `domains_list`, `zone_get`, `zone_set`, `zone_undo`
- **zone_set**: richiede `can_write`; `dry_run=true` = solo diff preview (con hash del set); apply accettato **SOLO se l'hash combacia con l'ultimo dry_run della chiave sul dominio** (decisone founder 19/10/2026); diff preview obbligatorio prima di ogni commit
- Rate limiter `gwLimiter` per chiave; audit log di ogni negazione (`gateway_scope_denied`)

## 8. Spend guard e provisioning (spend.go, provisioning.go)

- `MAX_MONTHLY_SPEND_EUR` default **20** (≈ founder: "limiti di spesa a $20/mese"); whitelist tipi istanza con prezzi Hetzner (CCX **assenti di proposito** → provisioning Dedicated bloccato finché il budget non viene alzato); override runtime `SPEND_INSTANCE_COSTS` (JSON)
- Spesa committed = somma dei job `prov_*.json` queued+running; rifiuto onesto con numeri
- **Il terraform apply gira solo in un worker esterno** (mai in-process): `triggerProvisioning` accoda il job; Dedicated = `ccx41→ccx42→ccx43` (finestra manutenzione), Dedicated XL = `ccx63`; infra in `landing/infra/` (lb.tf: Hetzner LB http 80 + tcp 443 passthrough, round_robin, health `/landingpage/api/status:8080`; shared-node.tf: cloud-init docker + install.sh)

## 9. Requisiti non funzionali

- **CSP-safe**: nessuna risorsa esterna, script-src 'self' (tutti i JS vanilla senza framework)
- **Accessibilità**: skip-link, aria-current nello stepper, role=alert sui messaggi, `<dialog>` per i modal
- **Test**: suite Go verde (main/checkout/gateway/provisioning/spend) — 5 file di test nel repo
- **Vendor screen**: Hetzner (DE) o Exoscale (CH) — fuori dal perimetro US CLOUD Act (iaas-design.md §6)
