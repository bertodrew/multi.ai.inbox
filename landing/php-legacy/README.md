# php-legacy — RITIRATO 2026-10-21

Endpoint PHP del landing sostituiti dal rewrite Go (stdlib-only, distroless):
`landing/go` — stessi endpoint su `/api/*` (status, connect, disconnect, domains, dns, mcp) + checkout/gateway/provisioning.

- La nginx live (`data/conf/nginx/site.landingpage.custom`) instrada `/landingpage/api/` → `landing-go:8080`.
- Tutti gli asset HTML chiamano `/landingpage/api/*` — nessun riferimento a landing-php nel percorso live.
- `nginx/landing.conf` qui dentro è la vecchia config FastCGI → landing-php:9000, superata.

Motivazione (CERT-EU 2026-018): 2 CVE PHP CVSS 9.2, RCE non autenticato — la superficie
PHP di cui il progetto è owner è azzerata; resta solo il PHP upstream (core mailcow),
mitigato in `data/conf/phpfpm/` e patchato via `./update.sh`.
