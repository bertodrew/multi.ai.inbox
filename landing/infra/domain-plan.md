# Domain plan — dominio + TLS + LANDING_BASE_URL (AImulti.inbox landing)

**Stato**: indagine eseguita e **riverificata sul server (2026-10-22, 2° passaggio)** — **nessuna modifica applicata** (dominio non in DNS, decisione founder mancante)
**Server**: Hetzner `2.28.99.252` (nodo 1, fuori Terraform) · stack in `/opt/mailcow-dockerized` (mailcow `2026-09a-71-g0743ae6`) · nginx del pannello serve root e `/landingpage/`

## 1. Stato attuale (verificato via SSH + dig + RDAP, 2026-10-22)

- `mailcow.conf`: `MAILCOW_HOSTNAME=mail.example.org` (placeholder IANA, **non risolve**), `ADDITIONAL_SAN=` (vuoto), `ADDITIONAL_SERVER_NAMES=` (vuoto), `ADDITIONAL_SSL_HOST` inesistente in questa versione (il var corretto è `ADDITIONAL_SERVER_NAMES`), `SKIP_LETS_ENCRYPT=n`.
- DB (mysql-mailcow, via `DBROOT` da mailcow.conf): tabella `domain` **0 righe**, `mailbox` **0 righe** — nessun dominio configurato nel pannello, nessun candidato in mailcow.
- DNS: `aimulti.inbox`, `www.aimulti.inbox`, `mail.aimulti.inbox`, `aimulti.it` — A e NS tutti vuoti; **nulla punta a 2.28.99.252** (rDNS = `static.252.99.28.2.clients.your-server.de`, default Hetzner).
- Registrazione: `aimulti.inbox` **non registrata** (IANA whois: 0 oggetti; RDAP rdap.org: **404**); anche `aimulti.it` non registrata (RDAP 404).
- Cert: **self-signed** `CN=mail.example.org` (emesso 2026-10-18, valido 1 anno); `acme-mailcow` attivo ma in loop: log `No A or AAAA record found for hostname mail.example.org` → `Cannot validate any hostnames, skipping Let's Encrypt for 1 hour`.
- Sito live via nginx (self-signed, su IP): `/` → 200, `/landingpage/` → 301 → root, `/landingpage/api/status` → 200, `/landingpage/api/env` → 401 (storico, ancora non risolto).
- nginx conf: solo `site.landingpage.custom` (incluso nel server block del pannello): il landing è servito alla **root** (`/`) e l'API su `/landingpage/api/` → proxy `landing-go:8080`. Con il cert del dominio non serve alcuna modifica nginx: `https://<DOMINIO>/` serve direttamente landing + API.
- `docker-compose.override.yml`: servizio `landing-go` **senza `LANDING_BASE_URL`** (verificato 2026-10-22).
- Codice (`landing/go`):
  - `auth.go:166` — `origin := env("LANDING_BASE_URL", "https://"+r.Host)`; redirect_uri = `origin + "/landingpage/api/auth/oauth/<provider>/callback"`.
  - `checkout.go:187` — `strings.TrimRight(env("LANDING_BASE_URL", "https://2.28.99.252"), "/")` (base URL per webhook Mollie; commento nel codice: impostarla solo con dominio+cert reali).
  - **Formato atteso**: origin **senza slash finale** (es. `https://DOMINIO`), il path è aggiunto dal codice.

## 2. Percorso scelto

**(a) `ADDITIONAL_SAN` + `ADDITIONAL_SERVER_NAMES` in `mailcow.conf` + riavvio `acme-mailcow`** — scelto, ma **non eseguibile ora**: il dominio non risolve, la validazione HTTP-01 fallirebbe (il log acme lo conferma già per `mail.example.org`). I cert finiscono in `data/assets/ssl/<dominio>/` e nginx li serve per tutto (root e `/landingpage/` inclusi); `ADDITIONAL_SERVER_NAMES=<dominio>` registra il vhost nginx per il nome extra.
**(b) certbot standalone** — scartato: doppio stack ACME, port 80 già in carico al pannello, nessun vantaggio.

## 3. Comandi da eseguire (founder, dopo la decisione del dominio)

`mailcow.conf` e `docker-compose.override.yml` sono gitignored: queste modifiche vivono solo sul server, documentate qui.

```sh
# 1) DNS (registrar, TTL 60) — SOLO se il dominio risolve si prosegue
#    A  @    -> 2.28.99.252
#    A  www  -> 2.28.99.252
dig +short A <DOMINIO>            # deve restituire 2.28.99.252

# 2) mailcow.conf (sul server, /opt/mailcow-dockerized)
ADDITIONAL_SAN=<DOMINIO>          # cert LE per il dominio del landing
ADDITIONAL_SERVER_NAMES=<DOMINIO> # vhost nginx

# 3) riavvio e verifica emissione
docker compose restart acme-mailcow
docker compose logs --tail 50 acme-mailcow
ls data/assets/ssl/<DOMINIO>/

# 4) verifica HTTPS con dominio reale
curl -sI https://<DOMINIO>/landingpage/api/status   # 200
curl -sI https://<DOMINIO>/                         # 200
openssl s_client -connect <DOMINIO>:443 -servername <DOMINIO> </dev/null 2>/dev/null | openssl x509 -noout -issuer -subject  # issuer Let's Encrypt

# 5) LANDING_BASE_URL (solo a questo punto) — docker-compose.override.yml,
#    servizio landing-go, environment:
#      - LANDING_BASE_URL=https://<DOMINIO>     # SENZA slash finale
docker compose up -d landing-go
```

`LANDING_BASE_URL` **non impostata finché il dominio non esiste**: il valore documentato è `https://<DOMINIO>` (senza slash finale) e dipende dalla decisione del founder; oggi i fallback del codice (r.Host per OAuth, `https://2.28.99.252` per i webhook checkout) restano invariati. Impostarla ora punterebbe redirect_uri OAuth e webhook Mollie a un host non risolvente e senza cert.

## 4. Decisione founder (mancante)

- **Quale dominio**: registrare `aimulti.inbox` (consigliato — combacia col brand; la zone Namecheap è già prevista da `landing/infra/lb.tf` default `zone = "aimulti.inbox"` e `LB.md` §9, target `2.28.99.252` già default in `target_ips`) oppure scegliere/registrare un dominio alternativo (es. `aimulti.it`, anch'esso libero).
- **Quando aggiornare il DNS**: dopo la registrazione, subito (TTL 60) — senza DNS il cert LE non può essere emesso né il sito raggiunto via dominio.
- Al variare della decisione cambiano solo i valori `<DOMINIO>` in §3, nessun altro punto del piano.
