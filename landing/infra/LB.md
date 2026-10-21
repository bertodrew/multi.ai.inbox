# LB — Load balancing HTTP + MX multipli SMTP (FASE 2) — AImulti.inbox

**Stato**: config pronta, **nessun `terraform apply` eseguito** (regola repo) · **Vendor**: Hetzner (DE) + Namecheap (DNS) · Design: `landing/docs/iaas-design.md` §3
**Server attuale**: Hetzner 2.28.99.252 (nodo 1, fuori Terraform) · **File**: `landing/infra/lb.tf`, `landing/infra/shared-node.tf` (file FASE 2, non toccano i file del modulo Fase 1)

## 1. Architettura

```
HTTP   client ──> Hetzner LB (80/443, round_robin) ──> nginx nodo ──> landing-go :8080
                                                          └──> SOGo (webmail)
SMTP   mittente ──> DNS MX (10 mail1 / 20 mail2) ──> postfix del nodo
```

- **HTTP** (landing/API/webmail): Hetzner LB managed, 2 servizi — 80 http → 80 nginx, 443 **tcp passthrough** → 443 nginx (TLS resta sul nodo: SNI multi-brand, cert per-dominio già in carico al proxy nginx→landing-go/SOGo). Health check http su `/landingpage/api/status` :8080 (landing-go, `EXPOSE 8080` nel Dockerfile). `proxyprotocol = false`.
- **SMTP**: nessun LB device — **DNS MX multipli**: priorità 10 → `mail1.<zone>` (nodo 1 esistente), 20 → `mail2.<zone>` (nuovo shared node). Failover gratis con priorità.

## 2. File e variabili

| File | Contenuto |
|---|---|
| `lb.tf` | `hcloud_load_balancer` (lb11, round_robin), servizi 80/443, target dinamici `type = "ip"` da `var.target_ips`, DNS MX su Namecheap (`mode = MERGE`), output `lb_ipv4` |
| `shared-node.tf` | `hcloud_server` per i nodi 2+ (`var.node_count`, numerazione da 2), cloud-init docker + bootstrap stack, `lifecycle ignore_changes = [user_data]` (nodo stateful: mai rebuild), output `shared_node_ipv4` |

Variabili chiave: `target_ips` (lista IP, **primo = nodo 1 esistente**, default `["2.28.99.252"]`), `landingo_port` (8080), `zone` (default `aimulti.inbox`), `node_count` (1 = crea il secondo nodo), `server_type` (default `CX42` — vedi limite AX41-NVMe §7), `node_image` (debian-12), `mx_ttl` (3600), `ssh_keys` (default `[]`).

## 3. Deploy

```sh
terraform init
terraform plan    # revisionare: 1 LB, 2 servizi, N target, 1 resource Namecheap
terraform apply   # SOLO in finestra di manutenzione, con autorizzazione del founder
```

- Credenziali via env: `HCLOUD_TOKEN`, `NAMECHEAP_USER_NAME`, `NAMECHEAP_API_USER`, `NAMECHEAP_API_KEY`, `NAMECHEAP_CLIENT_IP` (l'IP che chiama l'API Namecheap va whitelisted nella console Namecheap).
- Il cutover DNS (A sito → `lb_ipv4`, MX) è §8-§9, non fa parte dell'apply.

## 4. Prerequisito nodo (install.sh)

Il cloud-init (`shared-node.tf`) installa Docker (script ufficiale get.docker.com) e cerca `/opt/aimulti-inbox/install.sh`:

- **Pipeline Fase 1**: dopo la creazione dell'istanza la pipeline copia `install.sh` via SSH in `/opt/aimulti-inbox/`; il cloud-init lo esegue, log in `/var/log/aimulti-install.log`.
- **install.sh assente**: il nodo scrive `/opt/aimulti-inbox/INSTALL_REQUIRED` e resta **fuori rotazione** — il health check LB su `/landingpage/api/status` fallisce finché lo stack non sale. Procedura manuale (già validata su Hetzner, ~15 min): deploy chiavi SSH → clone repo → generate → `docker compose up -d` → verifica `/landingpage/api/status` 200 su `:8080`.

Prerequisiti perché il health check LB funzionino: lo stack landing pubblica `8080` sull'host (`ports: "8080:8080"`) e il firewall del nodo accetta dall'IPv4 del LB (`output lb_ipv4`) su **80/443/8080** — i target `type = "ip"` sono raggiunti via percorso pubblico.

## 5. Health check

- **Cosa copre**: landing-go vivo su :8080 (200 su `/landingpage/api/status`). Parametri: interval 10s, timeout 5s, retries 3 → un nodo guasto esce dalla rotazione in **~30-45s**.
- **Limite noto**: se nginx è giù ma landing-go risponde, il nodo resta "healthy" e prende traffico HTTP fallito. Il runbook (§6) include la verifica attiva di nginx.

## 6. Runbook failover HTTP

**Nodo unhealthy (automatico)**
1. Console Hetzner → Load Balancer → Targets: stato del target.
2. Sul nodo: `docker ps`, `docker compose logs --tail 100` dello stack landing, spazio disco, load. Metriche nodo: monitoring `:8791` (mailbox count, storage %, queue depth, load).
3. Riparato il nodo, il health check lo rimette in rotazione da solo (~10-30s).

**Drain manuale (rimozione target)**
- Hetzner LB non ha una modalità drain esplicita: rimuovere il target ferma le **nuove** connessioni; quelle esistenti (keep-alive HTTP) chiudono da sole in pochi secondi/minuti.
- Terraform: rimuovere l'IP da `var.target_ips` → `terraform apply`. Oppure console: Load Balancer → Targets → Remove.
- **SMTP non toccare**: il failover mail è DNS (§8) — mai togliere l'MX di un nodo se non è dichiarato irrecuperabile.

**Aggiunta target**
1. `var.target_ips` += IP del nuovo nodo (output `shared_node_ipv4`) → `terraform apply`.
2. Attendere il health check **verde** PRIMA di ogni cutover DNS.
3. Trigger capacità (design §4): monitoring `:8791` — allarme a 70% del cap nodo (80 tenant), provisioning del prossimo a 85%: si aumenta `var.node_count`, mai accendere nodi sul load istantaneo.

## 7. Limiti

- **SOGo sticky session**: SOGo è stateful (sessioni di login) — round_robin puro rompe il webmail con 2 nodi. Hetzner LB supporta cookie sticky (`http { cookie_name, cookie_lifetime }`) **solo** su servizi protocol `http`/`https` — sul passthrough `tcp` (config attuale 443) non ci sono cookie gestiti. Opzioni: (a) passare il servizio 443 a `protocol = "https"` + `hcloud_managed_certificate` + `cookie_name`, oppure (b) instradare il webmail fuori dal LB (DNS per-tenant verso il nodo giusto). **Fino a Fase 3 (storage condiviso)** il failover del webmail perde comunque le sessioni: re-login sull'altro nodo = mailbox assente (stato non condiviso).
- **API stateless**: il design dichiara landing-go senza stato → LB classico ok. Prima del cutover multi-nodo verificare che lo stato checkout/billing persista fuori dal nodo (storage condiviso o DB esterno).
- **proxyprotocol off**: nginx vede l'IPv4 del LB nei log. Se serve il client reale: `proxyprotocol = true` + `set_real_ip_from <lb_ip>` lato nginx — **entrambi insieme**, mai uno solo.
- **Target IP**: i target `type = "ip"` devono essere IP Hetzner (cloud o Robot). **AX41-NVMe** (€59/mo, 64 GB, 2×512 GB NVMe RAID1) è un dedicated root server **Robot**: NON è un tipo hcloud — si ordina via Robot e si aggiunge il suo IP a `var.target_ips` (oppure provider hrobot). Per il provisioning gestito hcloud usare CX/CPX/CCX (default `CX42`: 8 vCPU dedicati/16 GB).
- **SSH**: `var.ssh_keys` default `[]` — passare le chiavi (pipeline Fase 1) PRIMA dell'apply: un nodo senza chiavi e senza password è irraggiungibile (solo rescue mode).
- **MX weight**: Namecheap espone solo la priorità (`mx_pref`); il weight MX è 0 (RFC 5321) e non è usato — il failover è per priorità 10/20 (primary/backup), non bilanciamento per peso.

## 8. MX TTL e propagazione

- TTL default 3600. **In cutover: abbassare a 60-300 su MX e A mail PRIMA** di qualunque modifica (il provider aggiorna il TTL con `var.mx_ttl`).
- Propagazione: i resolver cachano fino al TTL **vecchio** — con TTL 3600 serve fino a 1h prima che i mittenti vedano il nuovo MX. La propagazione tocca solo le **nuove** decisioni dei mittenti: chi ha già in coda ritrasmette con i propri retry (tipicamente ogni 5-30 min) → **failover SMTP reale in minuti-ore**, non istantaneo. Il failover veloce resta quello HTTP (health check ~30-45s).
- Verifica: `dig +short MX aimulti.inbox`, `dig +short mail1.aimulti.inbox`, `dig +short mail2.aimulti.inbox` — MX 10/20, A puntati ai nodi.

## 9. DNS su Namecheap

- La zone è **registrata su Namecheap** (nameserver Namecheap): il provider `namecheap` scrive gli host records via API.
- `mode = "MERGE"`: aggiorna **solo** i record dichiarati in `lb.tf` — gli altri record della zone (sito, www, SPF, DKIM) non vengono toccati. Se un domani la zone passa tutta a Terraform: `terraform import namecheap_domain_records.mx <zone>` prima.
- Record gestiti: `mail1.<zone>` A → `var.target_ips[0]` (nodo 1, MX 10) · `mail2.<zone>` A → primo shared node (MX 20) · `@` MX 10 → `mail1.<zone>.` · `@` MX 20 → `mail2.<zone>.`
- SPF/DKIM: se la zone ha SPF, aggiornare `include:`/`ip4:` con mail1/mail2 insieme al cutover.

## 10. Checklist cutover

1. Backup/snapshot del nodo 1 (stato mail).
2. `ssh_keys` in `var.ssh_keys` + installazione stack sul nodo 2 (`install.sh` o procedura manuale §4).
3. `terraform plan`: 1 LB, 2 servizi (80/443), target = nodi, resource Namecheap (MERGE).
4. DNS: abbassare TTL (MX + A mail + A sito) a 60s PRIMA delle modifiche.
5. `terraform apply` (finestra manutenzione, autorizzazione founder).
6. Health check nodo 1 **verde** su `/landingpage/api/status:8080` (console LB → Targets).
7. HTTP cutover: A/AAAA del sito (`aimulti.inbox`, www) → `output lb_ipv4`.
8. Verifica HTTPS, API (checkout) e webhook Mollie arrivino tramite LB, webmail accessibile.
9. SMTP: `dig` MX 10/20 e A mail1/mail2 corretti (§8), test invio/ricezione su un mailbox del nodo 1, SPF aggiornato se presente.
10. Test failover: stop landing-go sul nodo 1 → nodo fuori rotazione in ~30-45s → traffico su nodo 2 → rimettere su.
11. Rialzare i TTL a 3600.

## 11. Rollback

- Rimuovere il nodo 2 da `var.target_ips` (e `node_count = 0` se non serve) → apply.
- Ripristinare l'A del sito direttamente al nodo 1 (TTL 60 rende il rollback rapido).
- MX: la priorità 20 su un nodo irraggiungibile non ferma la posta — i mittenti ricadono sulla 10. Rimuovere il record `mail2`/MX 20 solo se il nodo è dichiarato irrecuperabile.
