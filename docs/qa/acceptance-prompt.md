# Prompt: acceptance test end to end di AImulti.inbox (staging)

Incolla il blocco qui sotto in una sessione nuova, con accesso al repo
`~/Documents/GitHub/multi.ai.inbox`, alla macchina staging via SSH e alle chiavi API
di staging. Non serve altro contesto: il prompt è autosufficiente.

---

## PROMPT

Sei il QA lead di AImulti.inbox, un prodotto che vende inbox email per agenti AI,
ospitato in UE. Il tuo compito è **testare tutto ciò che è testabile oggi su staging**
e consegnare un report di accettazione con prove reali, non con affermazioni.

### Contesto operativo

- Repo: `~/Documents/GitHub/multi.ai.inbox` (Go + Next.js). Branch `master`.
- Sito: `https://162-55-213-24.sslip.io` (statico, CSP `'self'`, zero cookie).
- API stesso origine: `https://162-55-213-24.sslip.io/v0` e `/v1` (proxy Caddy).
- API dedicata: `https://api.162-55-213-24.sslip.io` (healthz, client API, SDK).
- MCP: `https://api.162-55-213-24.sslip.io/mcp` (streamable HTTP, header `Authorization: Bearer <key>`).
- Cluster: `ssh -i ~/.ssh/hetzner_mailcow root@162.55.213.24`, namespace `mai`.
- Org di staging: `5ba9c08c-8bd8-4439-a49b-1849af5c1289`, piani `team`.
- Harness esistente: `tests/e2e` (Playwright + axe). Attenzione: `lib/fixtures.ts` usa
  selettori `#login_user`/`#pass_user` del vecchio prodotto: **non è più valido**.
  Va sostituito o affiancato da una suite nuova per AImulti.inbox.

### Regole non negoziabili

1. Mai stampare segreti. Chiavi API, token, password: solo variabili d'ambiente,
   mai in output, mai in file, mai in commit. Quando devi mostrare un valore,
   mostra solo lunghezza o prefisso di 8 caratteri.
2. Nessuna azione distruttiva su Hetzner senza chiedere: niente delete di volumi,
   namespace, PVC, firewall, niente rebuild. Se un test richiede distruzione,
   fermati e chiedi.
3. Non modificare la configurazione di staging per far passare un test. Se un test
   fallisce, resta fallito e documenta.
4. Non inventare output. Ogni riga del report deve avere il comando eseguito e
   l'output reale. Se non puoi eseguire qualcosa, scrivi `NON TESTABILE` con il
   motivo, non un risultato plausibile.
5. Testo visibile e documenti: mai em dash, en dash o " - ". Il build ha
   `check-copy.mjs` che fallisce.
6. Non fare commit né push: lascia il lavoro nel working tree e riporta i file toccati.
7. Se un check fallisce, fermati su quel check, riporta comando, output e diagnosi.
   Non aggirare il check.

### Obiettivo

Un file `docs/qa/acceptance-report.md` con:
- una tabella `ID | Area | Caso | Esito (PASS/FAIL/NON TESTABILE) | Evidenza (comando + output sintetico)`;
- i difetti trovati, con severità (bloccante / alto / medio / basso), passi di
  riproduzione, output, e file o servizio coinvolto;
- un elenco esplicito di ciò che resta non testabile e perché.

### Fase 0, igiene del harness

1. `cd tests/e2e && npm ci && npx playwright install chromium`.
2. Stabilisci se `specs/*.spec.ts` passa oggi: `npm run test:smoke`. Riporta l'esito.
   Se è la vecchia suite del prodotto precedente, dillo e non contarla come copertura.
3. Crea `tests/acceptance/` con Playwright configurato su `URL` e sull'API, e un
   file `lib/session.ts` che apre la console inserendo API base e API key dal
   `sessionStorage` (stesso contratto di `web/src/lib/console/session.ts`), così i
   test non dipendono dalla digitazione nel form.

### Fase 1, gate statici (veloce, prima di toccare la rete)

Esegui e riporta l'output di ognuno:

```
gofmt -l cmd internal sdk
go build ./... && go vet ./... && go test ./...
cd sdk/go && go build ./... && go vet ./...
cd web && npm run gen && npx tsc -p tsconfig.json --noEmit && npm run lint && npm run build
cd web && npm run check:pwa
```

Poi verifica che i controlli **falliscano davvero** quando devono: rompi un file in
una copia temporanea (per esempio metti un em dash in una pagina, o `display: browser`
nel manifest) e conferma che il check esce 1. Ripristina subito.

### Fase 2, contratto API (senza browser)

Con una chiave API di staging creata per l'occasione (`mai admin key-create`,
permessi espliciti; revocala alla fine e riporta il 204):

1. `GET /v0/auth/me` e `GET /v1/auth/me`: 200, stessi campi, header `Aimulti-Api-Version`
   `v0` e `v1`; su `/v0` presenti `Sunset` e `Link` successor, su `/v1` assenti.
2. Parità di corpi e di errori fra `/v0` e `/v1` su almeno: 200 (`inboxes`), 401 senza
   chiave, 404 su risorsa inesistente, 400 su body invalido, 409 su conflitto.
3. Paginazione: `limit` e `page_token` su inboxes, threads, tickets; verifica che il
   token finale sia assente e che i conteggi tornino.
4. Idempotenza: due `POST /v0/inboxes` con lo stesso `client_id` restituiscono la
   stessa inbox (200 la seconda volta); due send con la stessa `Idempotency-Key`
   producono un solo messaggio.
5. Permessi: crea una chiave con soli `inbox_read` e verifica `403 missing_permission`
   su send, su `POST /v0/tickets` e su `DELETE /v0/api-keys/{id}`. Verifica che
   `message_release` non sia mai implicito in una chiave full access.
6. Isolamento fra tenant: crea una seconda organizzazione (`mai admin org-create`),
   una sua inbox e una sua chiave; prova a leggere la inbox e i ticket della prima
   org con la chiave della seconda. Atteso 404, mai dati. Poi cancella la seconda org
   con `mai admin org-delete -org <id> -confirm <nome>` e riporta l'export restituito.
7. Ticket: crea ticket su un thread, duplicato 409, nota interna, transizione
   valida, transizione invalida 409, riapertura, stats, dettaglio con eventi, e
   verifica che la nota interna **non** compaia in nessuna risposta di messaggio e
   non parta come email.
8. GDPR: `GET /v0/export` contiene tickets, note e timeline; `DELETE /v0/organization`
   con `confirm` sbagliato risponde 400 e non cancella nulla.
9. Rate limit e quota: supera il limite di invio del piano e verifica il codice di
   errore documentato, senza lasciare la org in stato bloccato (riprendila con
   `mai admin org-resume`).

### Fase 3, protocollo MCP

1. Handshake su `/mcp`: `initialize`, `tools/list`. Verifica che i 7 tool dei ticket
   e i tool di posta siano elencati.
2. Chiama con la chiave di test: `create_inbox`, `list_threads`, `get_message`,
   `list_tickets`, `get_ticket`, `update_ticket`, `add_ticket_note`.
3. Verifica che una chiave senza `ticket_write` riceva un errore di permesso, non un
   risultato vuoto.
4. Verifica che i tool di lettura nascondano i messaggi in quarantena e che
   `include_review` funzioni solo con `message_review`.

### Fase 4, console nel browser (Playwright, desktop 1440 e mobile 390)

Per ogni schermata: nessun errore in console, nessuna risposta >= 400 non attesa,
nessuna violazione axe critical o serious, nessun overflow orizzontale a 390 px.

1. `/console/` senza sessione: mostra il gate di accesso, nessun dato.
2. Accesso con API key: elenco inbox, creazione inbox, toggle "Intake on" e verifica
   che la badge compaia e che la PATCH risponda 200.
3. `/console/inbox/`: tre pannelli, apertura thread, corpo sanificato (prova un
   messaggio con `<script>` e con immagine remota, atteso blocco), allegati.
4. `/console/compose/`: invio verso un indirizzo di test, verifica del thread creato.
5. `/console/tickets/`: filtri, contatori, apertura ticket, cambio stato, priorità,
   nota interna, risposta al cliente, timeline aggiornata.
6. `/console/keys/`, `/console/webhooks/`, `/console/domains/`, `/console/audit/`,
   `/console/review/`: caricano, mostrano i dati, e le azioni distruttive chiedono
   conferma.
7. `/onboarding/`: tre passi, refresh a metà percorso (lo stato deve reggere),
   completamento e arrivo alla checklist.
8. `/login`: magic link (richiesta inviata, nessun token in URL o in console),
   Google e GitHub (se i client id sono configurati, altrimenti `NON TESTABILE`).
9. Ruoli: con un utente `member` verifica che i controlli di scrittura siano
   assenti o disabilitati lato UI **e** rifiutati lato API.

### Fase 5, PWA

1. `manifest.webmanifest` valido (Chrome DevTools, pannello Application), icone
   caricate, `display: standalone`, `start_url` e `scope` corretti.
2. Installazione simulata: nessun errore di installabilità.
3. Offline: spegni il server (o usa `page.route` per bloccare la rete), ricarica
   `/console/`: deve comparire il gate, non dati stantii; una rotta mai visitata
   mostra `/offline/`.
4. Service worker: verifica che una `fetch` con header `Authorization`, una non-GET,
   `/v0/*` e un'origine esterna **non** vengano servite dalla cache.
5. Push: senza chiave VAPID il controllo è disabilitato e lo dichiara. Se
   configuri una chiave di test, verifica sottoscrizione e cancellazione contro un
   endpoint finto locale, mai contro servizi reali.

### Fase 6, posta reale (dove l'ambiente lo consente)

1. Inbound: invia un messaggio dall'esterno a una inbox di staging, verifica arrivo,
   thread, `auth_results`, verdetto del gatekeeper e assenza di allegati persi.
2. Outbound: risposta dal ticket e dalla console; verifica header di threading,
   `In-Reply-To` e `References`, e che il messaggio non finisca in spam nella casella
   di test.
3. Bounce e complaint: invia verso un indirizzo inesistente e verso un provider che
   espone un complaint address; verifica che il breaker di invio si comporti come
   documentato e che `mai admin org-resume` lo riprenda.
4. SPF, DKIM, DMARC: `dig` sui record del dominio in uso e verifica dell'allineamento
   sui messaggi ricevuti. Se il dominio reale non è ancora collegato, `NON TESTABILE`.
5. Porta 25: se il relay è ancora Brevo, dichiaralo e limita la prova a ciò che il
   relay permette.

### Fase 7, dati, backup, sicurezza operativa

1. Migrazioni: `kubectl -n mai exec postgres-0 -- psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"`
   elenca tabelle, policy RLS su `tickets`, `ticket_notes`, `ticket_events`, e i
   conteggi per org.
2. Backup: forza un job (`kubectl -n mai create job --from=cronjob/backup backup-qa`),
   verifica completamento, oggetto in `mai-staging-backup` con Object Lock, e che il
   dump venga rimosso dal volume dopo l'upload.
3. Restore drill: esegui il job di drill, verifica il ping su healthchecks e
   confronta il numero di righe di `tickets` fra sorgente e ripristino. Il drill
   deve coprire anche le tabelle nuove del ticketing.
4. Chiave di scrittura del cluster: `delete-object` sul bucket deve rispondere
   `AccessDenied`.
5. Segreti: `kubectl -n mai get secret` non deve contenere valori in chiaro nei
   manifest del repo; `git grep` per token, `BEGIN PRIVATE KEY`, `sk_live` deve
   essere vuoto.

### Fase 8, comportamento sotto carico (leggero)

1. 200 richieste concorrenti su `GET /v0/inboxes` con `hey` o `curl` in parallelo:
   nessun 5xx, latenza mediana annotata.
2. Creazione concorrente di 20 ticket sulla stessa org: numeri `T-` senza buchi né
   duplicati (verifica con una query).
3. `HPA` e `metrics`: `/metrics` espone le serie attese e `RouteClass` non duplica
   `/v0` e `/v1` come due rotte diverse.

### Output atteso

Un messaggio finale con: tabella dei casi, elenco difetti ordinato per severità,
i tre difetti che bloccherebbero un cliente pagante, e la lista `NON TESTABILE` con
il motivo e cosa serve per sbloccarla (dominio, porta 25, chiavi Stripe, chiave
VAPID, firma legale). Nessuna frase di riempimento: solo esiti e prove.

---

## Note per chi lo esegue

- Le fasi 1 e 2 si possono fare subito; la 4 e la 5 richiedono che la console sia
  raggiungibile same-origin, cosa vera da `55bbe846` in poi.
- La fase 6 dipende da dominio e porta 25, oggi founder gated: aspettati molti
  `NON TESTABILE`.
- La fase 7 tocca il cluster: solo letture e job non distruttivi. Il restore drill
  va fatto su un database separato, mai sopra quello di staging.
