# Database Design — AImulti.inbox (landing + gateway)

**Data**: 21/10/2026 · **Stato**: design CTO/architect · **Vendor**: PostgreSQL 16 self-hosted EU/CH (Hetzner o Exoscale CH) — niente managed US (CLOUD Act).

---

## 1. Audit dello stato attuale (verificato 21/10, `landing/go/*.go`)

| Stato | Storage | Persistenza | Rischio |
|---|---|---|---|
| **Sessioni** (`sessions`, `sessionStore` in main.go) | **In-memory map** | ❌ persa a ogni restart/redeploy del container | Gli utenti si sconnettono a ogni deploy; CSRF perso |
| **Billing state** (`bill_<sha256[:16]>.json`) | File JSON 0600 su volume `landing-credentials-vol` montato in `/credentials` | ✅ persistente (verified: volume montato, `docker inspect`) | Niente query transazionali, niente concorrenza multi-writer |
| **Domini gestiti** (`dom_<hash>.json`) | File JSON 0600, stesso volume | ✅ | Idem |
| **Onboarding env** (`env_*.json`) | File JSON 0600, stesso volume | ✅ | Il node-tenant count (fase 0 IaaS) conta i file |
| **Gateway API keys** (MCP, `gwk_...`) | cred dir files | ✅ | Audit `gateway_key_created` su stdout |
| **Credenziali Namecheap** (`credFile`, 0600) | File JSON, stesso volume | ✅ | Segreti su disco — ok, perms 0600 |
| **Audit log** | stdout / log file | ⚠️ non interrogabile | Nessuna query, retention limitata |

**Conclusione onesta**: per demo e primissimi clienti il file-based va bene. Ma **prima della Fase 2 (secondo node + LB)** il modello file diventa **split-brain** (due nodi, due set di file, stati divergenti) e le sessioni in-memory rompono il login ad ogni deploy. Il DB diventa obbligatorio alla Fase 2, non oltre.

## 2. Scelta: PostgreSQL 16, self-hosted EU/CH

- **Perché Postgres**: transazioni ACID per billing, JSONB per raw verified payloads, row-level lock per la concorrenza multi-writer, replica streaming per DR
- **Dove**: servizio dedicato piccolo (CX23/CAX11 ~€5/mo) o lato nodo landing in rete privata Hetzner; Exoscale CH (PG managed EU, std. GDPR/CH-DPA) per i clienti che chiedono residenza dati esplicita
- **Esclusi**: managed DB US (RDS/Cloud SQL — CLOUD Act), MongoDB/NoSQL (nessun bisogno: il carico è OLTP relazionale)

## 3. Schema (DDL completo)

```sql
CREATE TABLE tenants (
  id         TEXT PRIMARY KEY,               -- username slug
  plan       TEXT NOT NULL DEFAULT 'trial',  -- trial|solo|studio|scale|dedicated|dedicated-xl
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,               -- sha256 del cookie (mai il cookie in chiaro)
  tenant_id  TEXT NOT NULL REFERENCES tenants(id),
  csrf       TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_expiry ON sessions(expires_at);

CREATE TABLE gateway_api_keys (              -- endpoint MCP
  id         TEXT PRIMARY KEY,               -- gwk_...
  tenant_id  TEXT NOT NULL REFERENCES tenants(id),
  key_hash   TEXT NOT NULL,                  -- sha256, mai la chiave in chiaro
  can_write  BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tenant_domains (
  tenant_id     TEXT NOT NULL REFERENCES tenants(id),
  domain        TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, domain)
);

CREATE TABLE subscriptions (
  id                 TEXT PRIMARY KEY,       -- sub_... Mollie
  tenant_id          TEXT NOT NULL REFERENCES tenants(id),
  mollie_customer_id TEXT NOT NULL,          -- cst_...
  plan               TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'open',  -- open|paid|canceled
  amount_eur         NUMERIC(8,2) NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE payments (
  id              TEXT PRIMARY KEY,          -- tr_... Mollie
  tenant_id       TEXT NOT NULL REFERENCES tenants(id),
  subscription_id TEXT REFERENCES subscriptions(id),
  status          TEXT NOT NULL,             -- open|paid|failed|canceled
  amount_eur      NUMERIC(8,2) NOT NULL,
  webhook_seen_at TIMESTAMPTZ,
  raw             JSONB NOT NULL,            -- payment verificato server-side (GET con la nostra chiave)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE provisioning_jobs (
  id              TEXT PRIMARY KEY,          -- prov_...
  tenant_id       TEXT NOT NULL REFERENCES tenants(id),
  plan            TEXT NOT NULL,
  instance_type   TEXT NOT NULL,             -- whitelist spend.go (CCX fuori budget → rifiuto)
  est_monthly_eur NUMERIC(8,2) NOT NULL,
  status          TEXT NOT NULL DEFAULT 'queued',  -- queued|running|done|failed
  error           TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id        BIGSERIAL PRIMARY KEY,
  ts        TIMESTAMPTZ NOT NULL DEFAULT now(),
  event     TEXT NOT NULL,
  tenant_id TEXT,
  ip        TEXT,
  data      JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_audit_ts ON audit_log(ts DESC);
CREATE INDEX idx_audit_event ON audit_log(event);
```

### Regole d'integrità (fail-closed, come il resto dello stack)

1. **Verifica webhook server-side**: la riga `payments.raw` si scrive SOLO dopo il GET del payment con la nostra chiave — mai dal body Mollie
2. **Subscription solo su paid**: `status='paid'` in `subscriptions` si setta in transazione con la riga `payments` corrispondente (idempotente per `payments.id`)
3. **Provisioning**: un solo job `queued/running` per tenant (unique partial index) e `spendGuard` nel trigger — budget account rispettato anche sotto concorrenza
4. **Segreti**: mai in chiaro — `sessions.id`/`gateway_api_keys.key_hash` sono hash; le cred Namecheap restano su file 0600 (non in DB) o in un secret store

## 4. Migration file → DB (zero downtime)

```
1. LANDING_DB_DSN env opzionale — senza DSN il codice resta file-based (stato attuale, zero cambiamento)
2. Dual-read: DB se DSN + riga esiste, altrimenti fallback file (transizione)
3. Import script (landing/infra/migrate.py): legge /credentials/*.json → INSERT … ON CONFLICT DO NOTHING
4. Cutover: DSN configurato + import verificato (row count = file count) → i file restano come backup read-only
```

## 5. Backup / DR

- `pg_dump` giornaliero su volume off-node + retention 30 giorni (cron esistente in ~/.hermes/cron)
- Restore test mensile (fase 2 IaaS): ripristino in container scratch + checksum row count
- Le mailbox NON sono nel DB: maildir su disco + MySQL/Redis mailcow (interno piattaforma) — i loro backup seguono la runbook mailcow esistente

## 6. Integration con lo spend guard ($20/mese)

La tabella `provisioning_jobs` è la source del budget: `est_monthly_eur` sommata sui job `queued/running` = spesa committed che `spendGuard()` verifica prima di ogni nuovo job. Soglia founder: **20 EUR/mese** (≈ $20; Hetzner fattura in EUR — 20 EUR è la lettura più severa). CCX fuori whitelist → provisioning Dedicated rifiutato finché il budget non viene alzato con il primo cliente.

---

*Ordine di lavoro: schema + migration script alla Fase 2 (quando il secondo node rende il file-based split-brain) · fino ad allora: file-based + volume montato è corretto e persistente (verified: volume landing-credentials-vol montato su /credentials).*
