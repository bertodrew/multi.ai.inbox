# Production readiness, stato reale

Aggiornato durante la passata di QA del 24 ottobre 2026, su staging
(`162-55-213-24.sslip.io`, namespace `mai`). Ogni riga "verificato" ha dietro un
comando eseguito e il suo output; le righe "non testabile" dicono cosa serve.

## 1. Verificato, con prova

| Area | Cosa è stato eseguito | Esito |
|---|---|---|
| Contratto API | 36 casi su staging: parità `/v0` e `/v1`, header di versione, errori 400/401/403/404/409, paginazione, idempotenza, permessi a scadenza | 36/36 PASS |
| Isolamento tenant | seconda organizzazione reale, chiave sua, tentativi di lettura su inbox, thread, ticket, coda, export e chiavi della prima | 404 ovunque, nessun dato, org poi cancellata |
| Ticket | ciclo completo, transizioni invalide, nota interna assente dal thread e presente nell'export, contatori | PASS |
| MCP | handshake `initialize` (protocollo 2025-06-18), `tools/list` (23 tool, 7 dei ticket), `tools/call` su `list_tickets` | PASS |
| Posta inbound | SMTP su porta 25, archiviazione, thread, `screening: allowed`, ticket creato dall'intake | PASS |
| Backup | job forzato, dump nuovo, oggetto in `mai-staging-backup` con Object Lock | PASS |
| Restore drill | confronto dei conteggi di ogni tabella pubblica, ticketing compreso | PASS |
| Suite Go end to end | `go test ./internal/e2e/` con Postgres reale e i tre ruoli di produzione | PASS (dopo i fix sotto) |
| Gate statici | `gofmt`, `go build`, `go vet`, `go test ./...`, SDK Go, `npm run gen`, `tsc`, `eslint`, `next build`, `check-copy`, `check-pwa` | PASS |
| Deploy | immagine in cluster 1/1, `healthz` 204, sito e console 200, proxy same-origin `/v0` `/v1` attivo con CSP `connect-src 'self'` | PASS |
| Console nel browser | suite di accettazione Playwright con chiave reale, 26 spec su desktop e 390 px, axe su ogni pagina | 24 PASS, 0 FAIL, 2 saltati per configurazione |
| Push web | iscrizione, invio su `message.received`, isolamento fra due organizzazioni, servizio irraggiungibile senza perdere la posta | PASS (12 sottotest) |

## 2. Difetti trovati e corretti

1. **La posta trattenuta apriva un ticket.** L'intake girava prima del verdetto:
   la coda mostrava un thread non leggibile e un messaggio poi bloccato restava
   in coda. Ora il ticket nasce quando il messaggio diventa visibile (verdetto o
   rilascio). Test di regressione in `internal/e2e/tickets_test.go`, che sul
   codice vecchio fallisce con "held mail opened 1 tickets, want 0".
2. **Timestamp nel fuso del server.** pgx decodificava `timestamptz` in
   `time.Local`: lo stesso istante usciva `+01:00` su una macchina a Zurigo e `Z`
   in container. Ora la scansione è fissata a UTC.
3. **Otto codici di errore non documentati** (`invalid_transition`,
   `ticket_exists`, `ticket_required`, `note_required`, `no_inbound_message`,
   `thread_*`, `ticket_not_found`): aggiunti al catalogo e alla pagina errors.
4. **Design contro schema.** La doc prometteva un secondo numero sullo stesso
   thread dopo la chiusura; lo schema tiene un ticket per thread. Doc allineata,
   numerazione multipla spostata a Fase B.
5. **Migrazioni in race.** Due repliche che partono insieme potevano applicare la
   stessa migrazione: ora advisory lock con ricontrollo dentro la transazione.

## 3. Difetti trovati dalla suite di accettazione nel browser

Sette, tutti chiusi. Il dettaglio con stato e gravita sta in
`docs/qa/acceptance-report.md`. I due bloccanti erano lo stesso sintomo visto da
due lati: il form del magic link veniva rifiutato dalla CSP, prima perche
puntava a `api.<host>` e poi perche `MAI_WEB_ORIGIN` portava il 303 su un'altra
origine. Gli altri: la risposta del reply interpretata con lo schema sbagliato,
il secondo reply senza traccia nella timeline, gli attributi `style` della lista
virtualizzata, i campioni di codice non raggiungibili da tastiera, due pannelli
con lo stesso nome accessibile.

## 4. Non testabile oggi, e perché

| Cosa | Perché | Cosa serve |
|---|---|---|
| Recapito outbound reale (SPF, DKIM, DMARC allineati, spam) | **misurato: il relay Brevo risponde `525 5.7.1 Unauthorized IP address` all'IP del server**, e la porta 25 in uscita e bloccata da Hetzner, quindi oggi non esce posta verso l'esterno | autorizzare `162.55.213.24` in Brevo (SMTP e API, IP autorizzati) oppure ottenere l'unblock della 25, poi dominio reale con i record pubblicati |
| Iscrizione self service con email o Google | senza un mittente che il relay accetta nessun magic link arriva, e Google non ha client id | le due cose sopra piu le credenziali OAuth |
| Magic link | `MAI_AUTH_SECRET` assente: l'endpoint risponde `auth_disabled`, quindi il form ora funziona ma non manda nulla | generare il secret, poi `kubectl -n mai rollout restart deploy/mai` |
| Google, GitHub | client id e secret non configurati su staging | credenziali OAuth del founder |
| Stripe live (checkout, portal, webhook firmato) | chiavi di test assenti e prezzi non creati | chiavi Stripe e i tre price id |
| Installazione PWA su iPhone fisico e push iOS | serve un dispositivo e una chiave VAPID | chiave VAPID e un telefono |
| Firma legale | review v2 in attesa di un legale umano | firma del revisore |
| Restore drill come promessa di continuita | un solo server | secondo server in un'altra location |

## 5. Cosa manca per essere pronto a vendere

In ordine, e ognuna blocca la successiva.

1. **Dominio `aimulti.inbox`** con DNS, certificato, SPF, DKIM e DMARC. Senza
   dominio reale non si manda posta in modo credibile e non si sblocca la 25.
2. **Unblock porta 25** su Hetzner (richiesta di supporto, 1 a 3 giorni).
3. **Blob su Object Storage** (`MAI_BLOB_BACKEND=s3`): oggi stanno su un volume
   RWO legato al nodo, quindi una seconda replica su un altro nodo non li
   vedrebbe. Il commento in `deploy/k8s/base/mai.yaml` ha gia le variabili.
4. **Split dei ruoli e ridondanza**: `mai` gira con tutti i ruoli in un pod,
   `strategy: Recreate`, una replica. Dopo il punto 3: Deployment `mai-api`
   (ruoli api, mx, RollingUpdate, due repliche, PodDisruptionBudget, HPA su CPU)
   e Deployment `mai-worker` (sender, dispatcher, enrich, scheduler, una
   replica). Prima di allora, scalare significa duplicare scheduler e dispatcher.
5. **Chiavi Stripe** e prezzi, poi un acquisto di prova end to end.
6. **Firma legale** su privacy, termini, DPA e sub-processor.
7. **Healthchecks** `HC_RESTORE_DRILL_URL` e alerting, cosi un drill fallito non
   passa inosservato.
8. **Chiave VAPID** se il push deve essere acceso al lancio, altrimenti resta
   spento e dichiarato tale, che e uno stato onesto.

## 6. Cosa invece non blocca

- Cookie: non ce ne sono, nessun banner da fare.
- Analytics: nessuno, e la CSP `'self'` lo rende anche difficile da sbagliare.
- React Native: si fa quando un cliente pagante lo chiede.
- Portale pubblico dei ticket (Fase D): rinviato per decisione del CEO in review.
