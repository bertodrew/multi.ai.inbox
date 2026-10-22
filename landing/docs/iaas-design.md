# IaaS Design — AImulti.inbox (mail hosting multi-brand)

**Data**: 21/10/2026 · **Stato**: design approvato dal founder · **Infra target**: Hetzner (DE) o Exoscale (CH) — vendor EU/CH, non US CLOUD Act.

---

## 1. Principio: la mail non fa autoscaling reattivo

La mail è **stateful**: maildir su disco, MySQL, Redis, code postfix. Un autoscaler che accende/spegne nodi sul load distrugge lo stato (mailbox perdute, code perse, DKIM sign su nodi sbagliati). Il modello giusto è:

> **Capacity planning + provisioning pipeline**, non autoscaling.

- Ogni nodo ha un **cap di capacità noto** (tenant/nodo, dal piano).
- Lo scale UP = **provisioning del prossimo nodo**, su trigger di metriche (pre-allarme), non sul load istantaneo.
- Lo scale DOWN = consolidamento pianificato (migrazione tenant), mai automatico.

## 2. Modello di scala per piano

| Tier | Prezzo | Cap promessi | Come scala |
|---|---|---|---|
| **Trial** (non paid) | — | 1 dominio · 1 mailbox | Enforced server-side (`planFor` + cap nel write path) |
| **Solo** | €9/mo | 1 dominio · 3 mailboxes · 1 agent | Shared pool: cap 1 dominio enforced nel gateway write path |
| **Studio** | €25/mo | 3 domini · 10 mailboxes · 3 agent | Shared pool: cap 3 domini |
| **Scale** | €55/mo | 10 domini · 25 mailboxes · pool 1 TB | Shared pool: cap 10 domini |
| **Dedicated** | €149/mo | ∞ domini · 50 mailboxes · 8 vCPU/32 GB · 10 TB | **Istanza propria**: resize verticale CCX41→CCX42→CCX43 (finestra manutenzione) |
| **Dedicated XL** | €249/mo | ∞ domini · 150 mailboxes · 16 core/128 GB · 20 TB | **Root server dedicato** |

**Enforcement implementato (21/10)**: `planFor` risolve il piano attivo dal billing state (paid → tier, altrimenti trial); il cap domini è **enforced nel write path** (`PATCH api/gateway/domains/{domain}` → 402 con messaggio piano). Il cap mailboxes si enforce dove le mailboxes si creano (panel/API mailcow — fuori dal landing per design). Il **nodo pieno** rifiuta nuovi purchase: `nodeTenantCount() >= MAX_TENANTS_PER_NODE` (env, default 80) → 503 "contact sales".

## 3. Load balancing

| Layer | Meccanismo | Perché |
|---|---|---|
| **SMTP** | **DNS MX multipli** (priorità/weight) su più nodi postfix | La mail è nativamente distribuita: nessun LB device serve, il DNS fa il bilanciamento. Failover gratis con priorità |
| **HTTP** (landing/API/webmail) | **Hetzner Load Balancer managed** (o HAProxy + Floating IP) → upstream multipli | Istanze senza stato (landing-go distroless) → LB classico; Hetzner LB è EU, health check integrati |
| **Storage** | NFS (prima) → Ceph (a scala) condiviso tra i nodi mail | Prerequisito dello scale orizzontale postfix/dovecot: maildir su storage condiviso, MySQL master/replica |

Il mailcow su singolo nodo scala verticalmente finché ~50 tenant/shared; oltre: secondo nodo + storage.

## 4. Gestione IaaS: la provisioning pipeline (la parte più mancante)

Il flusso che manca — oggi il webhook paid persiste lo stato ma non triggera nulla:

```
Mollie webhook (paid)
  → subscription attiva (già fatto, 21/10)
  → [FASE 1] provisioning trigger:
       Dedicated: Terraform crea CCX → install mailcow → DNS write → onboarding email
       Shared: allocazione tenant sul nodo con più capacità libera
```

- **Terraform** (Hetzner/Exoscale provider): istanza, volume, IP, firewall, rDNS/PTR
- **Install**: script mailcow esistente (procedura già validata su Hetzner, ~15 min)
- **DNS write path**: già costruito (api/gateway/domains/{domain}, 21/10)
- **Osservabilità**: metriche nodo nel monitoring esistente (:8791) — mailbox count, storage %, queue depth, load → **trigger provisioning pre-cap** (allarme a 70% cap, provisioning a 85%)

### Stato implementazione (22/10, Fase 1)

**Wired** (in `landing/go/provisioning.go`, test `provisioning_test.go`):

```
Mollie webhook (paid, Dedicated/Dedicated XL)  [checkout.go, idempotente]
  → queueProvisioning → job prov_*.json 0600 (state=queued)  [21/10]
      con re-check spendGuard (whitelist + budget) — seconda porta
  → runProvScan (una pass in-process, nel goroutine del webhook, SOLO se
    il token esiste — fail-closed):
       queued → creating (POST https://api.hetzner.cloud/v1/servers, 201)
              → active  (GET /servers/{id}: status running → IP salvato)
              → failed  (token mancante → 503 onesto; errore provider → 503;
                         provider non implementato (exoscale) → 501)
```

- Token da env `HCLOUD_TOKEN` (viaggia solo nell'header `Authorization` — mai nel job file, mai nelle risposte); `PROVISIONING_PROVIDER` default `hetzner` (unico path implementato — la procedura mailcow è validata su Hetzner); Exoscale → job failed con errore "non implementato" (501) anche con `EXOSCALE_API_KEY` impostata.
- Endpoint `GET /api/provisioning/state`: 200 per queued/creating/active/none; **503** per job failed (token mancante, errore provider, budget wall) e **501** per provider non implementato — il corpo porta comunque il job per la UI.
- Poll bounded: `PROVISIONING_POLL_TIMEOUT` (default 120 s) / `PROVISIONING_POLL_INTERVAL` (default 2 s); un job ancora `initializing` al timeout resta onestamente `creating` (la pass successiva lo riprende) — mai fake success/fake failure.
- Scan serializzato (`provScanMu`): nessuna doppia creazione per lo stesso job; ogni job è riletto sotto lock. Race detector pulito.
- Idempotenza invariata: job queued/creating/active non viene mai resettato da un pagamento ricorrente; solo i failed vengono ri-accodati.

**Nota budget (punto di aggancio documentato)**: `monthlyCommittedEUR()` in `spend.go` conta gli stati `queued`+`running`; lo stato `creating` non è incluso nel committed spend. Finestra minuta, mitigata dal re-check `spendGuard` alla creazione (sotto mutex), dal check nel webhook e dai Limits Console (§7). Patch di una riga a spend.go (aggiungere `creating` al conteggio) quando il file è sbloccato.

**Resta manuale per il founder** (fuori dal web process):

| Step | Cosa | Chi |
|---|---|---|
| Hetzner Console → progetto → **Limits** | "Servers" tenuto BASSO (2-3 pre-lancio) — l'hard cap reale (§7) | founder (2 min) |
| Hetzner Console → **Security → SSH keys** | caricare la chiave; passare gli id via `PROVISIONING_SSH_KEY_IDS` | founder (una volta) |
| Install mailcow sull'istanza creata | lo script validato (~15 min) — l'API crea l'istanza, l'install resta esterna; `PROVISIONING_IMAGE` (default ubuntu-24.04) | founder / worker |
| DNS + rDNS/PTR | il write path DNS esiste (api/gateway/domains); rDNS/PTR via Console o API separata | founder |
| Alerting spesa a 70% budget | monitoring :8791 (Fase 0 IaaS, §7 livello 3) | fase successiva |

## 5. Fasi

| Fase | Cosa | Sforzo |
|---|---|---|
| **0 — FATTO (21/10)** | Cap piani enforced server-side (402 domini, 503 nodo pieno) + `MAX_TENANTS_PER_NODE` env + piano nel checkout state | ✓ |
| **1** | Provisioning pipeline Dedicated (Terraform + install + DNS) — purchase → istanza pronta | 2–3 gg |
| **2** | Secondo shared node + Hetzner LB (HTTP) + MX weight (SMTP) | 1–2 gg |
| **3** | Storage condiviso (NFS → Ceph) + scale orizzontale postfix/dovecot + MySQL replica | 3–5 gg |

## 6. Decisioni prese (per riferimento)

1. **Autoscaling reattivo: NO** per la mail — capacity planning + provisioning pipeline.
2. **LB: DNS-based per SMTP** (nativo), Hetzner LB managed per HTTP (EU, no US CLOUD Act).
3. **Cap shared pool: 80 tenant/nodo** (env-tunable) — il piano Scale (10 domini/25 mailboxes) è il consumatore tipico: ~3 Scale + mix Solo/Studio per nodo.
4. **Il cap è enforced nel write path**, non solo nel pricing — il pricing senza enforcement è un rischio di revenue (un Solo senza limiti crea domini illimitati a €9).
5. **Vendor screen**: Hetzner (DE, delegato EU) o Exoscale (CH) — entrambi fuori dal perimetro US CLOUD Act; Exoscale CH per i clienti che chiedono residenza dati svizzera esplicita.

---

*Implementato in `landing/go/checkout.go` + `landing/go/main.go` (commit cap enforcement 21/10) · test: TestTrialCapOneDomain, TestPaidPlanCapEnforced, TestDedicatedUnlimitedDomains, TestNodeCapacityRefusesPurchase, TestPlanForResolution — 5/5 PASS + suite verde.*

## 7. Spend guard dell'account Hetzner (21/10, founder: "limiti di spesa a $20/mese")

**Hetzner NON ha un hard spending cap nativo** (verificato ott. 2026: "no built-in hard spending cap, set your own alerting") — l'esposizione si lega su tre livelli:

| Livello | Meccanismo | Chi |
|---|---|---|
| **1 — Console** | Tab **Limits** del progetto: tenere il limite "Servers" BASSO (default 5). Ogni istanza ha un price cap mensile (`"your server's bill will never exceed its monthly price cap"`); 5×CX23 €5.49 ≈ €27.45 = esposizione massima del progetto | founder (2 min) |
| **2 — Codice (fail-closed)** | `spendGuard()` in `landing/go/spend.go`: budget mensile `MAX_MONTHLY_SPEND_EUR` (**default 20**), whitelist tipi istanza con prezzi (CPX12 €11.99 il più economico ordinabile, **CCX deliberatamente assenti** → il provisioning Dedicated è bloccato finché il budget non viene alzato); spesa committed = job prov_*.json queued+running; rifiuto onesto con numeri | implementato 21/10 |
| **3 — Alerting** | monitoring :8791: spesa committed EUR/mese + notifica a 70% del budget | fase 0 IaaS |

Console (fondatore): Hetzner Console → progetto → **Limits** → verifica "Servers" (abbassare a 2-3 pre-lancio); Billing → notification settings → alert email.

Il tier Dedicated (€149/mo su CCX €59+) **non entra** nel budget di 20 EUR: quando arriva il primo cliente Dedicated si alza `MAX_MONTHLY_SPEND_EUR` (o si aggiunge il CCX alla whitelist) — decisione di revenue, non di sicurezza.

*Test: TestSpendGuardAllowsEntryLevel, TestSpendGuardRejectsUnknownType, TestSpendGuardRejectsOverBudget, TestSpendGuardBudgetFromEnv, TestMonthlyCommittedFieldTolerance — 5/5 PASS.*
