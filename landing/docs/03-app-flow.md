# 03 — App Flow — AImulti.inbox

**Data**: 21/10/2026 · **Fonte**: flussi reali in landing/html/*.html, html/assets/*.js, landing/go/*.go — nessun flusso inventato.

---

## 1. Flusso principale: dal visitatore al dominio che riceve posta

```mermaid
flowchart TD
    V[Visitatore<br>index.html] -->|modal Sign in<br>POST /home| SOGO[Login SOGo<br>Mail UI]
    V -->|Use cases / Webmail / Pricing / Docs| NAV[Navigazione topbar]
    NAV --> OB[onboarding.html<br>Wizard Setup]
    OB --> ENV{Ambiente:<br>shared o dedicated}
    ENV --> S1[Step 1 — Mail origin<br>URL del pannello posta]
    S1 --> S2[Step 2 — Connector<br>credenziali Namecheap]
    S2 -->|POST /api/connect<br>+ rotazione sessione| NC[Namecheap API]
    S2 --> S3[Step 3 — First domain<br>GET /api/dns<br>record zona dal registrar]
    S3 --> DONE[Setup completato<br>next steps]
    DONE --> CK[pricing.html<br>Subscribe → Mollie]
    CK --> LIVE[Domini live<br>inbox.html, settings.html]
```

## 2. Wizard Setup (onboarding.html — struttura reale)

Stepper reale (`<ol class="stepper" aria-label="Setup progress">`):

1. **Mail** — Step 1 "Mail origin": URL del pannello posta (Mail origin)
2. **Connector** — Step 2 "Connect a provider": credenziali API Namecheap
3. **First domain** — Step 3 "First domain": "Pick the first domain to inspect; DNS records load from Namecheap."

Prima dei tre step: scelta **ambiente** (radio-card "Your environment" — shared vs dedicated), che determina il provisioning (fase 0 IaaS). Ogni step ha `aria-current="step"` sul dot attivo e messaggi `role="alert"` (`step3-error`).

## 3. Flusso di connessione (handleConnect, main.go)

```mermaid
sequenceDiagram
    participant UI as onboarding.js
    participant G as landing-go :8080
    participant NC as Namecheap
    UI->>G: POST /landingpage/api/connect {api_key, api_user…}
    G->>G: verifica credenziali con NC (ncCall)
    G->>G: store per-sessione<br>namecheap_<sha256(sid)[:16]>.json<br>AES-256-GCM (AAD = sid)
    G->>G: rotateSession — nuovo cookie,<br>vecchio store invalidato (fix S1)
    G-->>UI: ok + domini
    UI->>G: GET /api/dns?domain=…
    G->>NC: namecheap.domains.dns.getHosts
    G-->>UI: record zona
```

## 4. Flusso checkout Mollie (pricing.html → checkout.go — sequenza reale)

```mermaid
sequenceDiagram
    participant P as pricing.html (checkout.js)
    participant G as landing-go
    participant M as Mollie v2
    P->>G: GET /landingpage/api/status (CSRF)
    P->>G: POST /api/checkout {plan} + X-Landing-CSRF
    G->>G: sessione valida? nodo pieno? (MAX_TENANTS_PER_NODE→503)
    G->>M: POST /customers (uno per tenant, cst_…)
    G->>M: POST /payments → hosted checkout
    G-->>P: 201 {checkout_url} → redirect
    M->>G: POST /api/checkout/webhook {payment_id}
    G->>M: GET /payments/{id} con la NOstra chiave<br>(mai fidarsi del body)
    M-->>G: status=paid (verificato)
    G->>G: POST /customers/{cst}/subscriptions<br>(ricorrente, primo payment incluso)
    G->>G: bill_<sha256(username)[:16]>.json<br>{status:"paid", plan, subscription_id}
    G->>G: triggerProvisioning (se dedicated:<br>job prov_*.json → terraform worker)
```

Errori reali gestiti da checkout.js: `401 → redirect onboarding.html` (prima il sign-in); `503 → messaggio inline onesto` ("Payments are not configured yet on this server"); nodo pieno → 503 "contact sales".

## 5. Login e webmail (app.js, webmail.html — flusso attuale)

- Ogni bottone `[data-open-login]` apre il `<dialog>` Sign in (backdrop click e close button chiudono); il form posta a **/home** (SOGo autentica con le credenziali mailbox, separate dalla sessione landing)
- L'origine posta arriva da `/api/env` / `/api/status` (`MAILCOW_UI_ORIGIN`); se sconosciuta → warning in console per l'operatore (mai dettaglio tecnico all'utente)
- **webmail.html non incornicia SOGo** (SAMEORIGIN): la CTA "Open the webmail client" porta a /home in tab nuova — flusso attuale dopo il commit 93c79631 che ha rimosso iframe e module switch

## 6. Operare i domini (inbox, settings, connectors)

```mermaid
flowchart LR
    subgraph UI app pages
        I[inbox.html] -->|GET api/gateway/mailboxes?domain=…| A1[righe mailbox reali<br>quota MiB, last_imap_login]
        S[settings.html] -->|record dominio| A2[lettura/scrittura zona]
        C[connectors.html] -->|POST api/gateway/keys| A3[chiave gwk_… scoped<br>can_write + domain]
    end
    A2 --> Z[Gateway MCP:<br>zone_get / zone_set / zone_undo]
    A3 --> Z
    Z -->|dry_run=true → diff preview + hash<br>apply SOLO se hash combacia| MC[Mailcow registrar]
```

## 7. Stato di errore (502.html)

Fallback statico di cortesia quando la piattaforma non risponde (`proxy_intercept_errors on` su nginx): nessuna interazione, solo rassicurazione e retry manuale.
