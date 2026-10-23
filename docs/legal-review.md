# Legal review — AImulti.inbox

Registro di revisione legale: 20 gap e 18 domande aperte, con proprietario e stato.
Documento di lavoro interno, **non** è consulenza legale. Le voci marcate `[lawyer]`
richiedono un avvocato qualificato (IT/CH) e non sono chiudibili da codice o da
documentazione tecnica.

Legenda stato:

- **CLOSED (code)** — risolto nel repository con codice o configurazione.
- **CLOSED (docs)** — risolto nel repository con un documento o una pagina pubblica.
- **OPEN** — non chiudibile qui: decisione legale, dato societario o azione del founder.
- **PARTIAL** — la parte tecnica/documentale è chiusa, la parte legale resta aperta.

Tag proprietario: `[code]`, `[docs]`, `[ops]` (azione manuale su infrastruttura),
`[lawyer]` (richiede avvocato), `[founder]` (dato o decisione d'impresa).

Ultimo aggiornamento: 2026-10-23. Riferimenti di codice al commit di lavoro
(`master` + modifiche non committate di questa revisione).

---

## 1. Gap (20)

| ID | Gap | Owner | Stato | Evidenza / fix |
|----|-----|-------|-------|----------------|
| G01 | I Terms promettono un DPA "allegato" ma il link punta alla privacy policy: non esiste né un DPA né un template. | [docs] + [lawyer] | CLOSED (docs) | `docs/legal/dpa-template.md` (template Art. 28 con allegati) e pagina pubblica `landing/legal/dpa.html`. Validità giuridica: `[lawyer]`. |
| G02 | Nessuna lista sub-processor pubblicata né interna, benché la privacy policy ne citi alcuni in una tabella. | [docs] | CLOSED (docs) | `docs/legal/subprocessors.md` (registro interno con procedura di modifica) e `landing/legal/subprocessors.html` (lista pubblica). |
| G03 | Privacy policy con segnaposto non compilati: forma e denominazione del titolare, sede, email privacy, obbligo di DPO "da verificare". | [lawyer] + [founder] | OPEN | Sezione 1 di `landing/legal/privacy.html`. Restano `[lawyer]`: la forma giuridica e l'obbligo di DPO dipendono dalla struttura societaria. |
| G04 | Retention dell'audit log dichiarata "90 giorni (planned)" senza alcuna applicazione tecnica: la tabella cresceva senza limite. | [code] | CLOSED (code) | `core.AuditLogRetention` + `PurgeExpiredAudit` (`internal/core/retention.go`), invocata dal scheduler a ogni tick (`internal/scheduler/scheduler.go`). |
| G05 | Retention dei backup: "30 giorni (planned)" nei documenti, mentre la configurazione reale è Object Lock governance 30 giorni + lifecycle `crypt/dumps/` 45 giorni + blob mai espirati. | [ops] + [lawyer] | PARTIAL | Comportamento reale descritto in `docs/legal/retention-and-deletion.md`. Applicazione del lifecycle e del bucket: azione del founder su Hetzner (`[ops]`), vedi `docs/agentmail-re/14-disaster-recovery-backup.md` §4.1. La cancellazione nei backup e i blob immutabili restano `[lawyer]` (Q06, Q07). |
| G06 | Nessun endpoint di export: l'unico export era la summary della CLI operatore, usata in fase di cancellazione, non disponibile al cliente. | [code] | CLOSED (code) | `GET /v0/export` (`internal/api/api.go`, `internal/core/export.go`), permesso `data_export`, documentato in `openapi/openapi.yaml`. |
| G07 | La cancellazione di una organizzazione era possibile solo via CLI operatore: il diritto all'cancellazione non aveva un percorso self-service né una procedura scritta. | [code] + [docs] | CLOSED (code) | `DELETE /v0/organization` con `confirm` = nome organizzazione, permesso `data_delete` (mai implicito in una chiave full-access), riusa `core.DeleteOrg`. Procedura in `docs/legal/dsar-runbook.md`. |
| G08 | La quarantena di 90 giorni degli indirizzi cancellati (anti-riuso) non era divulgata: un cliente poteva aspettarsi la riassegnazione immediata dell'indirizzo. | [docs] | CLOSED (docs) | `core.AddressQuarantine` documentata in privacy §6, in `docs/legal/retention-and-deletion.md` e nell'OpenAPI (`DELETE /v0/organization`). |
| G09 | La tabella cookie dichiarava un cookie `MULTIAI_LANDING` che il codice non imposta; nessun inventario verificato di cookie, localStorage e tracker. | [docs] | CLOSED (docs) | Verifica sul codice: nessun `SetCookie`, nessun `document.cookie`, nessun `localStorage`/`sessionStorage` in `landing/assets/*.js`. Sezione 3 di privacy riscritta sull'evidenza; il cookie di sessione webmail è di SOGo, non nostro. |
| G10 | Nessun registro delle attività di trattamento (Art. 30 GDPR), che è il primo documento richiesto in un controllo o in una due diligence. | [docs] | CLOSED (docs) | `docs/legal/records-of-processing.md` (bozza con finalità, basi giuridiche, categorie, destinatari, retention, misure). Revisione `[lawyer]`. |
| G11 | Nessuna procedura per le richieste degli interessati (DSAR): chi risponde, con quale canale, entro quale termine, con quale prova. | [docs] | CLOSED (docs) | `docs/legal/dsar-runbook.md`, allineato agli endpoint `GET /v0/export` e `DELETE /v0/organization` e alla CLI `mai admin org-delete`. |
| G12 | Nessun runbook per la violazione di dati personali (Art. 33/34, 72 ore, catena di notifica al cliente-controller). | [docs] | CLOSED (docs) | `docs/legal/incident-response-breach.md` (bozza: rilevazione, contenimento, valutazione del rischio, notifica, registro). Contenuto e soglie: `[lawyer]`. |
| G13 | La sezione "Your rights" della privacy policy elencava i diritti senza dire come esercitarli tecnicamente (endpoint, export, cancellazione, tempi). | [docs] | CLOSED (docs) | Sezione 9 di `landing/legal/privacy.html` riscritta: export self-service, cancellazione con conferma, ruolo del processor per le mailbox dei tenant. |
| G14 | La privacy cita "SCC + TIA documentata" per Namecheap (USA) ma nel repository non esiste né la TIA né il riferimento al DPA del fornitore. | [lawyer] | OPEN | `[lawyer]`: la TIA è un documento di valutazione legale, non un artefatto tecnico. Nel repo resta solo il riferimento in `docs/legal/subprocessors.md` (nota "TIA da produrre"). |
| G15 | Il fornitore LLM per gli agenti AI è "to be defined", quindi il trasferimento e la base giuridica non sono determinabili. | [lawyer] + [founder] | OPEN | Privacy §4 e §8 dichiarano la funzione non attiva. Scelta del fornitore e valutazione: `[lawyer]`. Vincolo tecnico già rispettato: nessun agente legge mailbox oggi. |
| G16 | Doppio ruolo: per le mailbox dei tenant siamo processor, per i dati di contatto e fatturazione controller. La ripartizione non è scritta in un documento vincolante. | [lawyer] | OPEN | Bozza della ripartizione in `docs/legal/dpa-template.md` §2 e §3; qualificazione e clausole: `[lawyer]`. |
| G17 | Le email contengono per natura categorie particolari (Art. 9): manca una valutazione d'impatto e la LIA per il trattamento. | [lawyer] | OPEN | Privacy §2 e §8 dichiarano la minimizzazione. DPIA e LIA: `[lawyer]` (Q11, Q12). |
| G18 | I Terms non citavano la lista sub-processor, il preavviso di modifica e l'ordine di prevalenza con il DPA in modo completo. | [docs] + [lawyer] | CLOSED (docs) | `landing/legal/terms.html` §10, §14 e §15 aggiornati: link a DPA e sub-processor, preavviso 30 giorni, allegati completi. Il preavviso contrattuale definitivo: `[lawyer]`. |
| G19 | L'audit log non aveva un elenco pubblico delle azioni registrate né un export completo per il cliente (l'API restituiva al massimo 200 righe recenti). | [code] + [docs] | CLOSED (code) | `GET /v0/export` include l'intero audit log dell'organizzazione; elenco azioni e retention in `docs/legal/retention-and-deletion.md` §4. |
| G20 | La privacy dichiarava che la versione italiana (`privacy-it.md`) è quella autentica, ma il file non esiste: affermazione non veritiera verso i clienti italiani. | [docs] + [lawyer] | CLOSED (docs) | Nota in fondo a `landing/legal/privacy.html` corretta: l'inglese è la versione di riferimento, la traduzione italiana è dichiarata in preparazione. Quale versione prevale: `[lawyer]` (Q18). |

---

## 2. Domande per il legale (18)

Tutte richiedono un avvocato: nessuna è chiudibile da codice. Le risposte cambiano
testi pubblicati, non implementazione.

| ID | Domanda | Perché è bloccante |
|----|---------|--------------------|
| Q01 | Quale forma societaria e sede vanno indicate come titolare del trattamento, e con quale rappresentante UE se la società è svizzera? | Determina la sezione 1 della privacy, il DPA e le clausole di legge applicabile. |
| Q02 | Siamo obbligati a nominare un DPO ai sensi dell'Art. 37? Il trattamento di mailbox di terzi su larga scala integra i criteri EDPB? | Se sì, cambiano privacy, DPA e registro Art. 30. |
| Q03 | La qualificazione delle mailbox dei tenant come "processor" è corretta anche quando un agente AI del cliente agisce tramite la nostra API? | Decide chi risponde alle richieste degli interessati e chi è responsabile delle azioni dell'agente. |
| Q04 | Per i dati di contatto e fatturazione possiamo essere controller autonomo, o serve una qualificazione mista (controller congiunto)? | Determina la struttura del DPA e l'informativa ai contatti. |
| Q05 | Il trasferimento verso Namecheap (USA, non DPF) regge con SCC + TIA, o serve un fornitore UE per la gestione domini? | `[lawyer]`: rischio di trasferimento non conforme, con impatto diretto sul contratto. |
| Q06 | La cancellazione nei backup per rotazione naturale (30-45 giorni) è accettabile come "cancellazione" ai sensi dell'Art. 17, o serve un meccanismo di cancellazione mirata? | Determina se possiamo promettere cancellazione "entro 30 giorni" o dobbiamo dire "entro il ciclo di backup". |
| Q07 | I blob conservati a lungo termine nel bucket immutabile (Object Lock) sono compatibili con l'Art. 17 e con il principio di limitazione della conservazione? | Può richiedere di cambiare il lifecycle dei blob: decisione legale prima che tecnica. |
| Q08 | Per le email dei corrispondenti esterni (raccolta indiretta) l'avviso ex Art. 14 può stare nella privacy pubblica, o serve una notifica attiva al primo contatto? | L'esenzione "sforzo sproporzionato" va motivata per iscritto. |
| Q09 | Possiamo trattare le email per finalità di sicurezza (filtro spam, screening, antivirus) sulla base del legittimo interesse, o serve consenso del cliente-controller? | Toccano il flusso principale del prodotto (screen before notify). |
| Q10 | La conservazione dei metadati di consegna (IP, timestamp, esiti) per 90 giorni è proporzionata, o va ridotta/estesa? | Determina il valore in `core.AuditLogRetention` e il testo pubblicato. |
| Q11 | Serve una DPIA per il trattamento delle mailbox e per gli agenti AI? Con quale metodologia e con quale coinvolgimento dei clienti? | Se sì, è un documento obbligatorio prima dell'attivazione degli agenti. |
| Q12 | La LIA per gli agenti AI che leggono le mailbox richiede il consenso del cliente-controller o basta l'istruzione documentata nel DPA? | Cambia il flusso di onboarding e il contratto. |
| Q13 | Le azioni dell'agente verso terzi (invio email) richiedono sempre revisione umana per evitare decisioni automatizzate ex Art. 22? | Determina se il prodotto può inviare senza approvazione. |
| Q14 | Il limite di responsabilità nei Terms è opponibile verso consumatori e piccole imprese, e come si coordina con le penalità dell'SLA? | `[lawyer]`: clausole vessatorie e ordine di prevalenza. |
| Q15 | Il preavviso di 30 giorni per la modifica dei sub-processor è adeguato, e serve un diritto di obiezione con recesso? | Determina il testo contrattuale e la procedura di notifica. |
| Q16 | Quale legge applicabile e quale foro scegliamo (IT, CH o altro) per contratti con clienti UE e non UE? | Determina Terms e DPA; impatta anche l'esecuzione delle richieste degli interessati. |
| Q17 | La versione inglese o quella italiana dei documenti legali prevale in caso di conflitto, e serve una traduzione autentica per il mercato italiano? | Chiude G20 e la nota di prevalenza linguistica. |
| Q18 | Che tempi e che forma deve avere la notifica al cliente-controller in caso di violazione (Art. 33(2)), e come si coordina con le 72 ore verso l'autorità? | Determina la procedura in `docs/legal/incident-response-breach.md` e la clausola DPA. |

---

## 3. Cosa non è stato toccato (deliberatamente)

- Nessuna dichiarazione di conformità (ISO 27001, SOC 2, DPF): non sono fatti verificati.
- Nessuna scelta di fornitore LLM, nessuna TIA, nessuna DPIA: richiedono un legale.
- Nessuna modifica a dati societari, email di contatto o ragione sociale: sono dati reali,
  non segnaposto da inventare. Restano `[lawyer]`/`[founder]`.
