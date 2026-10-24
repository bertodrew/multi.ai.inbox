# Prompt: QA completa, fix dei gap e dei bug (AImulti.inbox, staging)

Incolla il blocco sotto in una sessione nuova. È autosufficiente: contiene il
contesto, le skill da caricare per ogni fase, le regole e il criterio di "fatto".

---

## PROMPT

Sei il QA lead **e** il fixer di AImulti.inbox: un prodotto che vende inbox email
per agenti AI, ospitato in UE, con console umana (webmail, coda ticket, onboarding)
e API/MCP per gli agenti. Il prodotto è su staging e deve diventare vendibile.

Il tuo compito non è "fare i test": è **testare ciò che finora nessuno ha testato,
trovare i difetti, e correggerli**, con prove reali. Un difetto trovato e non
corretto vale solo se scrivi perché non lo hai corretto.

### Contesto operativo

- Repo: `~/Documents/GitHub/multi.ai.inbox`, branch `master`, Go + Next.js.
- Sito e console: `https://162-55-213-24.sslip.io` (statico, CSP `'self'`, zero cookie).
- API stesso origine: `/v0` e `/v1` sul sito; API dedicata `https://api.162-55-213-24.sslip.io`.
- MCP: `https://api.162-55-213-24.sslip.io/mcp` (streamable HTTP, Bearer).
- Cluster: `ssh -i ~/.ssh/hetzner_mailcow root@162.55.213.24`, namespace `mai`.
- Org staging: `5ba9c08c-8bd8-4439-a49b-1849af5c1289` (piano `team`).
- Harness esistente: `tests/e2e` (Playwright + axe). **Attenzione**: `lib/fixtures.ts`
  usa `#login_user`/`#pass_user`, selettori del vecchio prodotto. Va sostituito.
- Tool `browser_*` su questa macchina sono inaffidabili: **usa Playwright** per il
  browser, non il toolset browser.

### Regole non negoziabili

1. **Mai stampare segreti**: chiavi, token, password solo in variabili d'ambiente.
   In output mostra prefisso di 8 caratteri o lunghezza. Mai in file, mai in commit.
2. **Nessuna azione distruttiva su Hetzner** senza chiedere: niente delete di volumi,
   namespace, PVC, firewall, niente rebuild. Il restore drill va su un database
   separato, mai sopra staging.
3. **Non piegare l'ambiente al test.** Se un test fallisce, resta fallito e lo documenti.
4. **Niente output inventati.** Ogni riga del report ha comando e output reale. Se non
   puoi eseguire, scrivi `NON TESTABILE` con il motivo. Un test verde inventato è
   peggio di un test rosso.
5. **Testo visibile e documenti**: mai em dash, en dash o " - ". `check-copy.mjs`
   fa fallire il build.
6. **Ogni fix ha un test di regressione** che fallisce prima del fix. Se non riesci a
   scriverlo, il fix non è finito.
7. **Non toccare master direttamente**: lavori su `fix/qa-hardening`. Commit piccoli,
   uno per difetto o per gruppo coerente, messaggi che spiegano causa e rimedio.
   Push e merge solo con il gate completo verde, e mai force push.

### Fase 0, arruola le skill e prepara il campo

Esegui `skills_list` e carica con `skill_view` le skill della tabella. Se un nome non
esiste, usa `skills_list` per trovare l'equivalente e annota la sostituzione nel report.

| Fase | Skill da caricare |
|---|---|
| Metodo QA e report | `dogfood` (tassonomia e template di report), `qa-test-planner`, `senior-qa` |
| Browser E2E | `playwright-e2e-builder`, `playwright`, `webapp-testing`, `e2e-testing-patterns` |
| Accessibilità e qualità UI | `accessibility`, `accessibility-auditor`, `ux-laws`, `ui-design-review`, `web-design-guidelines` |
| Performance e mobile | `web-quality-audit`, `core-web-vitals`, `performance`, `mobile-design`, `mobile-native` |
| Debug | `systematic-debugging`, `debugging-strategies`, `find-bugs` |
| Codice e API | `golang-pro`, `clean-code`, `api-design-principles`, `code-review`, `requesting-code-review` |
| Sicurezza | `security-review`, `owasp-top-10-testing`, `web-security-testing`, `api-security-testing`, `security-audit` |
| Dati | `postgres-best-practices`, `postgresql-optimization` |
| MCP | `mcp-builder` |
| Posta reale | `himalaya` (IMAP e SMTP da terminale), `smtp-penetration-testing` solo per gli header |
| Privacy e conformità | `gdpr-dsgvo-expert`, `data-privacy-compliance`, `regulatory-threat-model` |
| K8s e operations | `senior-devops`, `observability-engineer`, `incident-responder` |
| Chiusura | `verification-before-completion`, `test-fixing`, `caveman` per l'output terse |

Poi: `cd tests/e2e && npm ci && npx playwright install chromium`, e stabilisci se la
suite attuale passa (`npm run test:smoke`). Se è la vecchia suite, dillo e sostituiscila
con `tests/acceptance/`, con `lib/session.ts` che apre la console iniettando API base e
chiave nel `sessionStorage` (stesso contratto di `web/src/lib/console/session.ts`), così
i test non dipendono dalla digitazione nel form.

### Fase 1, gate statici e loro potere di fallire

Esegui e riporta: `gofmt -l cmd internal sdk`, `go build ./...`, `go vet ./...`,
`go test ./...`, `cd sdk/go && go build ./... && go vet ./...`,
`cd web && npm run gen && npx tsc -p tsconfig.json --noEmit && npm run lint && npm run build`,
`cd web && npm run check:pwa`.

Poi prova che i controlli **falliscano davvero**: rompi una copia temporanea (un em dash
in una pagina, `display: browser` nel manifest, un test Go sbagliato) e verifica exit 1.
Ripristina.

### Fase 2, contratto API (nessun browser)

Con una chiave creata per l'occasione (`mai admin key-create`, permessi espliciti,
revocata alla fine con 204 documentato):

1. Parità `/v0` e `/v1`: stessi corpi, header di versione corretti, `Sunset` e `Link`
   solo su `/v0`.
2. Errori: 400, 401, 403, 404, 409, 413, 429 confrontati fra le due versioni e con
   `docs/errors`. Ogni codice documentato deve esistere, ogni codice reale deve essere
   documentato: **se manca nel catalogo, è un gap da chiudere**.
3. Paginazione su inboxes, threads, messages, tickets; idempotenza su inbox e send.
4. Permessi: chiave con soli `inbox_read` deve dare 403 su send, su `POST /tickets`,
   su `DELETE /api-keys/{id}`. `message_release` mai implicito.
5. **Isolamento tenant**: crea una seconda org con inbox, thread e ticket; con la sua
   chiave prova a leggere le risorse della prima. Atteso 404, mai dati, in ogni
   endpoint (inboxes, threads, messages, tickets, notes, export). Questo è il test che
   vale di più: fallo per primo e in modo esaustivo.
6. Ticket: ciclo completo, transizioni invalide, nota interna che non esce mai come
   email, export GDPR che la contiene.
7. Quote e rate limit: superamento con il codice documentato, poi ripresa con
   `mai admin org-resume`, senza lasciare l'org bloccata.

### Fase 3, MCP

Handshake su `/mcp`, `tools/list`, poi chiamate reali a `create_inbox`, `list_threads`,
`get_message`, `create_ticket`, `list_tickets`, `get_ticket`, `update_ticket`,
`assign_ticket`, `add_ticket_note`, `reply_ticket`. Verifica: chiave senza permesso
riceve errore di permesso e non un risultato vuoto; i tool di lettura nascondono la
posta in quarantena; `include_review` funziona solo con `message_review`; le
descrizioni dei tool bastano a un modello per usarli senza indovinare.

### Fase 4, console nel browser (Playwright, 1440 px e 390 px)

Per ogni schermata: zero errori console, zero risposte >= 400 non attese, zero
violazioni axe critical o serious, zero overflow orizzontale a 390 px, target di tap
almeno 44 px, navigazione da tastiera con focus visibile.

1. `/console/` senza sessione: gate, nessun dato.
2. Accesso con API key, elenco inbox, creazione, toggle intake.
3. `/console/inbox/`: tre pannelli, apertura thread, corpo sanificato (`<script>` e
   immagine remota bloccati), allegati, ricerca.
4. `/console/compose/`: invio, thread creato, allegati.
5. `/console/tickets/`: filtri, contatori, apertura, cambio stato, priorità, nota,
   risposta, timeline.
6. `/console/keys/`, `webhooks/`, `domains/`, `audit/`, `review/`: dati corretti,
   conferme sulle azioni distruttive.
7. `/onboarding/`: tre passi, refresh a metà (lo stato regge), completamento.
8. `/login`: magic link (nessun token in URL o console), Google e GitHub se i client
   id sono configurati, altrimenti `NON TESTABILE`.
9. Ruoli: con `member` i controlli di scrittura sono assenti in UI **e** rifiutati
   dall'API.

### Fase 5, PWA, offline, push (gap noto)

1. Manifest, icone, `display`, `start_url`, `scope`, installabilità senza errori.
2. Offline: rete bloccata, `/console/` mostra il gate e non dati stantii, rotta mai
   visitata mostra `/offline/`.
3. Service worker: non serve dalla cache richieste con `Authorization`, non-GET,
   `/v0/*`, origini esterne.
4. **Gap da chiudere**: il push oggi è solo client e disabilitato, perché l'endpoint
   non esiste. `web/docs/push-design.md` ne descrive uno. Implementalo end-to-end:
   tabella, `POST` e `DELETE /v0/push/subscriptions`, permesso dedicato, chiavi VAPID
   lato server, invio su evento `message.received`, test Go e test Playwright contro
   un endpoint finto locale. Aggiorna `openapi.yaml`, rigenera, aggiorna la console.
   Se una scelta è di prodotto (chi può iscriversi, payload), scrivila e prendi la
   decisione più prudente, motivandola.

### Fase 6, posta reale

1. Inbound: messaggio dall'esterno a un inbox di staging; thread, `auth_results`,
   verdetto del gatekeeper, allegati.
2. Outbound: risposta da ticket e da console; `In-Reply-To` e `References` corretti,
   niente spam nella casella di test.
3. IMAP: con `himalaya` verifica cartelle, ricerca, allegati, e che la posta di un
   tenant non sia visibile da un altro.
4. Bounce e complaint: indirizzo inesistente e complaint address; il breaker deve
   comportarsi come documentato e `mai admin org-resume` deve riprendere.
5. SPF, DKIM, DMARC con `dig` e allineamento sui messaggi ricevuti. Se il dominio
   reale non è collegato: `NON TESTABILE` con la causa (dominio, porta 25).

### Fase 7, dati, backup, sicurezza operativa

1. Migrazioni e RLS: tabelle e policy presenti, `tickets`, `ticket_notes`,
   `ticket_events` incluse, e nessuna tabella senza policy.
2. Backup forzato: job completato, oggetto in `mai-staging-backup` con Object Lock,
   dump rimosso dal volume dopo l'upload.
3. **Gap da chiudere**: il restore drill deve coprire anche le tabelle dei ticket.
   Verifica il conteggio righe prima e dopo su un database separato e, se il drill
   non le include, correggi la procedura e il runbook.
4. Chiave del cluster: `delete-object` deve rispondere `AccessDenied`.
5. Segreti: `git grep` per token, `BEGIN PRIVATE KEY`, `sk_live` deve essere vuoto;
   nessun segreto nei log del pod.

### Fase 8, carico leggero e code path

200 richieste concorrenti su `GET /v0/inboxes`: nessun 5xx, latenza mediana annotata.
20 creazioni concorrenti di ticket: numeri `T-` senza buchi né duplicati. `/metrics`
espone le serie attese e `/v0` e `/v1` non sono duplicati come rotte.

### Come correggi

Per ogni difetto:

1. **Riproduci** e scrivi il test che fallisce. Mostra che fallisce.
2. **Diagnosi prima del fix**: usa `systematic-debugging`, dichiara la causa, non la
   probabilità.
3. **Fix minimo** che risolve la causa, non il sintomo. Niente refactor non richiesti.
4. **Test verde**, poi gate completo.
5. **Classifica**: bloccante (impedisce a un cliente di pagare o di usare il prodotto),
   alto (dati sbagliati, permessi, perdita di mail), medio (UX che confonde), basso
   (cosmetico). Bloccanti e alti li correggi sempre. I medi se il fix è contenuto.
   I bassi li elenchi e basta.
6. Se il fix cambia l'API: aggiorna `openapi/openapi.yaml`, `npm run gen`, gli enum di
   `web/src/lib/console/schemas.ts`, i test di contratto e `docs/`.
7. Se il fix richiede una decisione di prodotto o di prezzo, non decidere: scrivi la
   domanda nel report e lascia il codice invariato.

### Criterio di "fatto"

- `docs/qa/acceptance-report.md` con tabella `ID | Area | Caso | Esito | Evidenza`,
  difetti per severità con passi di riproduzione, e la lista `NON TESTABILE` con cosa
  serve per sbloccarla.
- Tutti i bloccanti e gli alti chiusi, ognuno col suo test di regressione.
- Gate completo verde: `gofmt`, `go build`, `go vet`, `go test ./...`, SDK Go,
  `npm run gen`, `tsc`, `eslint`, `next build`, `check-copy`, `check-pwa`,
  e la nuova suite Playwright verde su desktop e mobile.
- Nessun segreto nel repo, nei log o nel report.
- Un solo messaggio finale: numeri, non racconto. Quanti casi, quanti PASS, quanti
  FAIL, quanti fix, cosa resta aperto, e i tre difetti che avrebbero fermato un
  cliente pagante.

---

## Note per chi lo esegue

- Fasi 2, 3, 5 e 7 si possono parallelizzare in worktree separati (`dispatching-parallel-agents`),
  una fase per agente, con un integratore solo che fa i merge. Nessun file condiviso:
  `openapi.yaml`, `schemas.ts` e `caddy.yaml` li tocca un solo agente.
- La fase 4 richiede che la console sia raggiungibile same-origin, cosa vera dal commit
  `55bbe846`.
- La fase 6 dipende da dominio e porta 25: aspettati molti `NON TESTABILE`, e non
  aggirarli.
- Le fasi che toccano il cluster sono in sola lettura più job non distruttivi.
