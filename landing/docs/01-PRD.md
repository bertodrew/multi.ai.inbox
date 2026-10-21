# 01 — PRD (Product Requirements Document) — AImulti.inbox

**Data**: 21/10/2026 · **Prodotto**: AImulti.inbox — hosting mailcow gestito (fork Mailcow GPL-3.0) con rivendita mail hosting multi-brand · **Branch**: feature/landing-page · **Fonte**: landing/html/*.html, landing/html/assets/*, landing-go/*.go (codice reale, niente feature inventate)

---

## 1. Problema e posizione

Chi gestisce più brand non vuole una casella per brand su Gmail/Workplace: vuole **ogni brand sul proprio dominio, un'inbox sola, e agenti che operano via MCP** — con residenza dati EU/CH e inferenza sovrana. Il prodotto è il **hosting gestito di quella piattaforma** (mailcow fork) venduto self-serve: connetti il tuo registrar (Namecheap), scegli l'ambiente (shared/dedicated), paga con Mollie, e il tuo dominio riceve posta.

**Regola ferrea di brand** (decisione founder, verbatim storico): *"ricorda che mailcow non deve mai apparire"* — nel testo visibile non compare mai "mailcow"; il fork resta GPL-3.0 a livello codice, il brand utente è AImulti.inbox.

## 2. Utenti target

| Persona | Bisogno | Pagina di riferimento |
|---|---|---|
| **Freelance/agenzia multi-brand** | Una casella per ogni brand cliente, domini separati, branding per brand | index, use-cases, pricing |
| **Tecnico/owner** | Setup guidato (connessione registrar, DNS), gestione domini e mailbox | onboarding (Setup), settings, inbox |
| **Agente AI / dev** | Operare DNS e zona via MCP con chiavi scoped, senza toccare la UI | docs, connectors, gateway |
| **Acquirente enterprise** | Istanza dedicata, SLA, contatto vendite | pricing (Enterprise tier), legal/sla |

## 3. Proposta di valore (dalla hero reale di index.html)

> **"Every brand. One inbox."** — Self-hosted · multi-brand · agent-native

Quattro pilastri (sezioni reali di index.html):

1. **Admin auth** — login sul pannello posta (/home, SOGo) senza esci dal landing (modal Sign in)
2. **Namecheap connector** — le zone DNS si leggono/scrivono dal registrar senza trascrivere record a mano
3. **Dedicated MCP server** — gateway JSON-RPC con chiavi scoped per dominio, dry-run obbligatorio prima di ogni commit zona
4. **Groupware webmail** — client groupware (SOGo) sullo stesso dominio

## 4. Funzionalità per pagina (11 pagine statiche, verificato sui `<title>`)

| Pagina | Scopo | Interazioni reali |
|---|---|---|
| `index.html` | Landing + login + tour | modal Sign in (POST /home), tour web-inbox e agent-native |
| `use-cases.html` | Casi d'uso multi-brand | contenuto statico |
| `webmail.html` | Launcher webmail onesto | CTA "Open the webmail client" → /home (SOGo non incorniciabile, SAMEORIGIN) |
| `pricing.html` | Piani e billing | Subscribe → POST /landingpage/api/checkout → Mollie hosted |
| `docs.html` | Guide integrate | contenuto statico |
| `onboarding.html` | Wizard Setup 3 step | ambiente (shared/dedicated) → Mail origin → Connector → First domain |
| `inbox.html` | Mailboxes del dominio | GET /landingpage/api/gateway/mailboxes?domain=… |
| `settings.html` | Domain settings | lettura/scrittura record dominio |
| `connectors.html` | Config MCP/agent | chip endpoint, chiavi gateway |
| `admin.html` | Pannello admin | gestione piattaforma |
| `502.html` | Fallback servizio | pagina statica di cortesia |

## 5. Piani (pricing.html + checkout.go `checkoutPlans` — prezzi reali nel codice)

| Tier | Prezzo | Domini | Mailboxes | Ambiente |
|---|---|---|---|---|
| Trial (non a pagamento) | — | 1 | 1 | shared, enforced server-side |
| Solo | €9/mo | 1 | 3 | shared |
| Studio | €25/mo | 3 | 10 | shared |
| Scale | €55/mo | 10 | 25 | shared |
| Dedicated | €149/mo | illimitati | 50 | istanza dedicata (CCX) |
| Dedicated XL | €249/mo | illimitati | 150 | root server dedicato |
| Enterprise | — | — | — | Contact sales |

I cap sono **enforced nel write path** (`PATCH api/gateway/domains/{domain}` → 402 con messaggio piano; nodo pieno → 503 "contact sales" oltre `MAX_TENANTS_PER_NODE`, default 80). Il billing è **per tenant** (sezione "Billing per tenant" di pricing.html): un customer Mollie per tenant (`cst_…`), una subscription ricorrente creata solo dopo il primo pagamento verificato.

## 6. Metriche di successo (proposte)

- **Onboarding completato** (step 3 → primo dominio ispezionato) / visite onboarding
- **Trial → paid**: conversione checkout Mollie per tier
- **Time-to-first-domain** dal connect registrar
- **Adozione gateway MCP**: chiavi create, zone_set con dry-run+apply
- **Churn**: cancellazioni subscription per tier

## 7. Non-obiettivi (v1)

- Autoscaling reattivo della posta (mail = stateful: capacity planning, vedi docs/iaas-design.md)
- Cliente SMTP/IMAP raw al di fuori della piattaforma
- Multi-valuta (EUR only, Mollie)
- Mobile app
