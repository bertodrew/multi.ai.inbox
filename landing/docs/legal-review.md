# Legal Review — Terms / Privacy / SLA (AImulti.inbox) — preparazione firma umana

**Data:** 2026-10-22 · **Scope:** `landing/html/legal/{terms,privacy,sla}.html` (commit 79785998 + nav 0743ae6d) e pacchetto autentico `legal/` root (`tos-it.md`, `tos-en.md`, `privacy-it.md`, `privacy-en.md`, `sla.md`, `dpa-template.md`) · **Versioni revisionate:** 1.0-draft, 2026-10-19

> ⚠️ **Questo documento NON è consulenza legale.** Prepara la revisione umana: elenca ciò che i documenti coprono oggi, i gap rispetto a GDPR + nLPD (rivFA svizzera) e le domande da chiudere col legale abilitato (giurisdizione IT/CH). Nessun gap qui è "coperto": vanno scritti da un legale abilitato prima della pubblicazione e comunque **prima del primo cliente pagante**.

---

## 1. Cosa copre ogni documento oggi

### Terms (`tos-it.md` autentica / `tos-en.md` / `terms.html` pubblicata)
15 sezioni: parti e imprint (placeholder identità/contatti) · oggetto del servizio (mailbox SMTP/IMAP + webmail SOGo, pannello admin, DNS connector, API/MCP, antispam/AV, TLS, backup) · infrastruttura dichiarata **istanza singola Hetzner DE senza ridondanza geografica** · iscrizione/durata/prezzi (dove tutto è `[DA COMPLETARE]`) · SLA 99.5% per rimando · AUP (spam, phishing, malware, opt-in documentato per bulk, addebito pass-through penale Namecheap fino a USD 500, sospensione immediata) · sospensione (immediata senza preavviso per AUP) · credenziali API key (AES-256-GCM, uso concordato, domini restano nel registrar del cliente — no lock-in) · **agenti AI** (human-in-the-loop, no decisioni esclusivamente automatizzate ex art. 22, no training sui contenuti) · backup/retention (30gg pianificata, log 90gg, fatturazione 10 anni) · proprietà dati + ruoli (processor ex art. 28 per caselle tenant, titolare autonomo per contatto/fatturazione) · garanzia (AS IS upstream GPLv3, coperto il servizio gestito) + disclaimer deliverability (no inbox placement garantita) · cessazione/exit (export completo, 30gg, no penalty) · limitazione di responsabilità (fee 12 mesi, sanzioni GDPR fuori dal cap, forza maggiore) · modifiche (30gg, silenzio = accettazione) · legge italiana/foro + precedenza DPA → SLA → Termini.

### Privacy (`privacy-it.md` autentica / `privacy-en.md` / `privacy.html` pubblicata)
Chi siamo/ruoli (titolare per contatto/fatturazione/uso; **responsabile ex art. 28 per caselle tenant**, DPO non nominato) · tabella finalità/basi giuridiche (T1–T7: erogazione posta con raccolta indiretta di mittenti/destinatari esterni, accesso/credenziali, log audit, DNS connector, contatto/fatturazione, **agenti AI pianificati**) · categorie particolari art. 9 incidentali con minimizzazione · cookie strettamente necessari (no banner) · tabella destinatari/processor (Hetzner DE, Namecheap USA con SCC incorporate + TIA documentata **non DPF**, Infomaniak CH pianificato con adeguatezza UE→CH, Let's Encrypt, LLM provider da definire, autorità; nota tecnica api.ipify.org) · trasferimenti extra-EEA · retention (maildir attive, backup 30gg, log 90gg, sessioni 8–12h, fatturazione 10 anni) · sicurezza (AES-256-GCM, TLS, isolamento container, breach art. 33/34) · **agenti AI: oggi nessuno legge le caselle** (tool a livello dominio) con governance pre-attivazione (LIA scritta, informativa art. 14, revisione umana) · diritti artt. 15–22 + reclamo Garante/FDPIC.

### SLA (`sla.md` bilingue / `sla.html` pubblicata)
99.5% uptime mensile su SMTP (25/465/587), IMAP (143/993), webmail HTTPS (~216 min su 30 gg — calcolo verificato) · misurazione a fine mese su CEST con monitoring `[DA DEFINIRE]` · 8 esclusioni (manutenzione ≥48h max 4h/mese, forza maggiore, **guasti Hetzner**, terzi, blacklist da violazione AUP, sospensioni, problemi cliente, fuori perimetro) · crediti 10/25/50/100% come **unico rimedio** · support S1 1h h24 / S2 4h h24 / S3 1gg lav. / S4 3gg lav. (orari `[DA COMPLETARE]`) · variazioni con 30gg preavviso.

### DPA (`dpa-template.md` — template, NON pubblicato sul sito)
Art. 28(3) GDPR completo: oggetto/durata/natura/scopo, tipi dati/interessati, istruzioni documentate, riservatezza, sicurezza, sub-responsabili (stesso elenco privacy) con informativa preventiva e obiezione `[X giorni]`, assistenza (violazione notificata entro `[48 ore]`), trasferimenti extra-UE, agenti AI §9, restituzione/cancellazione con certificato scritto, audit/inspezioni, responsabilità, firme. **Stato: bozza interna "NON USARE PER FIRMARE", solo italiano, solo GDPR** — la pagina EN non esiste.

---

## 2. Gap rispetto a GDPR + nLPD (rivFA svizzera) — servizio mail B2B con agenti AI

**GDPR (UE):**

- **G1 — Titolare del trattamento non identificato.** Entità giuridica, sede, C.F./P.IVA, contatti privacy/support/legal sono placeholder in tutti i documenti (art. 13(1)(a)): senza identità non c'è informativa valida né imprint.
- **G2 — DPA non pubblicato né firmato.** `terms.html` §10/§15 e `privacy.html` §1 rimandano a un DPA "allegato" che esiste solo come template bozza interno; il template stesso dichiara che un generico ToS **non** soddisfa la sostanza dell'art. 28(3): il DPA va pubblicato, firmato e incorporato in forma vincolante prima del primo dato personale tenant.
- **G3 — DPA Hetzner non attivato.** La privacy dichiara "da attivare nell'account Hetzner (non si conclude automaticamente)": il trattamento partirebbe senza contratto art. 28 in essere con l'unico sub-processor che vede tutti i dati dell'istanza.
- **G4 — Informativa art. 14 senza meccanismo.** Per i corrispondenti esterni (mittenti/destinatari che scrivono nelle caselle ospitate — raccolta indiretta) le basi citano "art. 14 notice" ma non esiste testo/meccanismo pubblicato da nessuna parte: trasparenza obbligatoria attualmente mancante.
- **G5 — Mollie assente dai destinatari.** Il checkout (customer_id, username/plan nei metadata, subscription) gira su Mollie NL dal commit c4990fc5, ma Mollie non compare nella tabella destinatari di privacy §4: disclosure incompleta (informativa vs realtà del trattamento).
- **G6 — Dati di registrazione e OAuth assenti dall'informativa.** Il register reale (nome, paese, hash password — `POST /api/auth/register`, commit 0743ae6d) e gli entry point OAuth (Google/GitHub/Microsoft, oggi 501 fail-closed) non hanno finalità/dati nella tabella privacy né IDP tra i destinatari: aggiornare l'informativa **prima** di abilitare il flusso.
- **G7 — Meccanismo sub-processor solo placeholder.** L'informativa preventiva/obiezione ai nuovi sub-responsabili esiste solo come `[X giorni]` nel DPA §6.2 e è assente dalle pagine pubblicate (art. 28(2)).
- **G8 — Informativa inesatta sui fatti (TTL sessioni).** `privacy.html` §6 dichiara "8–12h TTL (to be set)" ma il codice impone **24h** (`main.go`: `sessionTTL = 24 * time.Hour`): pubblicarla così è inesatta ex art. 13 — decidere quale valore fa fede.
- **G9 — Retention "pianificate" senza enforcement tecnico.** Backup 30gg e log 90gg sono target: l'informativa stessa li pone come condizione precedente alla pubblicazione — da verificare tecnicamente (rotazione reale, logrotate) prima di pubblicarla.
- **G10 — Registro dei trattamenti e DPIA non menzionati.** Art. 30 (registro) non compare in nessun documento; per gli agenti AI sulle caselle (contenuti di terzi, categorie particolari incidentali) la necessità di DPIA è da valutare col legale.
- **G11 — DPO: decisione non chiusa.** "Non nominato; obbligo da verificare" — serve una decisione formale (nomina o determinazione documentata della non-necessità).

**nLPD / rivFA svizzera (clienti target CH):**

- **G12 — Clausola di applicazione territoriale nLPD assente dalla pagina pubblicata.** La versione autentica `privacy-it.md` §1.4 dichiara "GDPR e, per i dati di persone in CH, nLPD svizzera (2023)"; la `privacy.html` pubblicata **non contiene questa clausola** (la cita solo nel lede): la pagina EN-pubblicata droppa la copertura CH presente nell'originale.
- **G13 — DPA solo-GDPR.** Il template non ha clausole per l'incaricato del trattamento (art. 9 nLPD) né per la comunicazione transfrontaliera ex artt. 16–17 nLPD (rilevante per Namecheap USA quando serviranno clienti CH): da estendere prima della firma.
- **G14 — Reclamo FDPIC senza riferimento operativo.** `privacy.html` cita il FDPIC ma senza indirizzo/canale (www.edoeb.admin.ch) e senza precisare il regime per le caselle tenant (CH).

**Residenza dati / regola venditore (fornitori EU/CH non soggetti al CLOUD Act):**

- **G15 — Namecheap è l'unica deviazione dalla regola venditore.** Registrar USA (extra-UE, non DPF, soggetta al CLOUD Act): mitigata da SCC incorporate + TIA documentata + rappresentante UE art. 27, ma resta in tensione con il requisito venditore "EU/CH non CLOUD Act". Alternative minori nella stessa condizione: Let's Encrypt (USA) e `api.ipify.org` (USA, nota tecnica con sostituzione pianificata).
- **G16 — Residenza dati CH non coperta dai documenti legali.** Exoscale CH (per residenza CH esplicita, iaas-design §6) è solo pianificata e assente da terms/privacy: `terms.html` §2 dichiara solo "istanza singola Hetzner DE" — per clienti CH che chiedono residenza CH non c'è oggi copertura contrattuale.

**Agenti AI sulle caselle (trasparenza obbligatoria):**

- **G17 — Inferenza LLM non definita.** Il provider LLM è "da definire" (privacy §4/§5 richiedono l'aggiunta **prima** dell'attivazione) e la promessa di prodotto "inferenza sovrana" (PRD) non è riflessa in nessun documento legale: dove gira l'inferenza e con quale vendor è decisione da chiudere prima dell'attivazione.
- **G18 — Divergenza di tono Terms vs Privacy sugli agenti.** `terms.html` §8 descrive gli agenti "may be used … on mailboxes"; `privacy.html` §8 dichiara "Today no AI agent reads mailboxes" (tool solo a livello dominio). Allineare la trasparenza: oggi nessun agente legge le caselle.

**Contratto:**

- **G19 — Nessuna accettazione dei Termini nel flusso di registrazione.** Il register (nome/paese/password) non cattura consenso ai Termini (nessun checkbox): `tos-it.md` §3.1 lo segna `[DA COMPLETARE]` — senza accettazione esplicita non c'è prova della formazione del contratto né base documentabile.
- **G20 — Eligibilità B2B e recesso consumatore non affrontati.** Nessuna clausola di eligibilità business-only; se si iscrive un consumatore UE scattano le regole consumer (recesso 14gg servizi digitali) non considerate.

**Totale gap: 20.**

---

## 3. Domande chiuse per il firmatario (founder + legale abilitato)

1. Che entità firma i documenti (srl IT / ditta individuale o GmbH CH)? — determina titolare, foro, e regime nLPD.
2. La legge italiana + foro della sede va bene anche per i clienti CH, oppure serve opzione legge svizzera?
3. DPO: nominiamo o formalizziamo per iscritto la non-necessità alla scala attuale?
4. Il DPA (`dpa-template.md`) si approva e pubblica come allegato firmato prima del primo dato tenant — con clausole nLPD (art. 9 + trasferimenti)?
5. Attiviamo il DPA Hetzner nell'account prima di mettere online dati reali (non si conclude automaticamente)?
6. Dov'è l'informativa art. 14 per corrispondenti esterni (pagina dedicata + link nel footer/onboarding)?
7. Namecheap USA: accettata come eccezione documentata alla regola "EU/CH non CLOUD Act", o anticipiamo il connettore Infomaniak?
8. Inseriamo Mollie (NL) nella tabella destinatari di `privacy.html` §4 con lo scope reale dei dati billing?
9. Aggiorniamo l'informativa con i dati di registrazione (nome, paese, hash password) e gli IDP OAuth **prima** di abilitare register/OAuth?
10. TTL sessioni landing: fa fede 24h (codice attuale, da scrivere nell'informativa) o portiamo il codice a 8–12h?
11. Backup 30gg e log 90gg: confermi l'enforcement tecnico (rotazione, logrotate) come condizione di pubblicazione dell'informativa?
12. Quando eseguiamo il restore drill per dichiarare RPO/RTO contrattuali?
13. SLA: teniamo l'esclusione dei guasti Hetzner dai crediti (renderebbe il 99.5% quasi vuoto su istanza singola) o adottiamo crediti di mercato?
14. Penale pass-through fino a USD 500 e sospensione immediata senza preavviso: validiamo l'enforceabilità IT/CH col legale?
15. RC professionale: attiviamo la polizza prima del lancio (oggi "to be activated")?
16. Quale dominio di produzione per canonical/og (oggi `multi.ai`) e per `LANDING_BASE_URL` (oggetto richiesto da Mollie webhook)?
17. Rebrandiamo le versioni autentiche `legal/*.md` da "multi.ai.inbox" a "AImulti.inbox" (oggi divergono dalle HTML pubblicate)?
18. Aggiungiamo checkbox di accettazione Termini nel register e clausola di eligibilità B2B (con gestione del caso consumatore)?

---

## 4. Red flags (bloccanti o gravi)

- **R1 — DPA assente dall'ecosistema pubblicato:** un generico ToS non soddisfa l'art. 28(3) (lo dichiara il template stesso): **bloccante per il primo cliente pagante**.
- **R2 — Art. 14 senza meccanismo:** trasparenza obbligatoria mancante per la raccolta indiretta (le mail di terzi arrivano nelle caselle) — la lacuna più esposta del pacchetto.
- **R3 — Informativa privacy inesatta sui fatti:** TTL dichiarato 8–12h vs 24h nel codice; Mollie assente dai destinatari — pubblicarla così espone a inesattezza ex art. 13.
- **R4 — Trattamento senza contratto art. 28 con Hetzner:** DPA "da attivare" mentre tutta l'istanza (mail, DB, backup) gira lì.
- **R5 — SLA quasi vuoto:** i guasti Hetzner (causa principale di downtime su istanza singola) sono esclusi dai crediti e il monitoring non è definito — l'impegno 99.5% non è misurabile né opponibile in modo solido.
- **R6 — Clausole aggressive da validare:** penale pass-through fino a USD 500 + sospensione immediata senza preavviso: rischio vessatoria/consumatori da verificare IT/CH.
- **R7 — Contratto senza consenso:** nessuna accettazione dei Termini nel flusso register — nessuna prova della formazione del contratto.
- **R8 — Placeholder identitari ovunque:** entità, P.IVA, contatti, listino, foro, polizza: nessun documento è pubblicabile o firmabile così come è.
- **R9 — Nome prodotto obsoleto nelle versioni autentiche:** `legal/*.md` root usano "multi.ai.inbox" mentre le HTML pubblicate usano "AImulti.inbox" — divergenza tra versione autentica e versione pubblicata.
- **R10 — Canonical/og su dominio non confermato:** `https://multi.ai/legal/…` con dominio di produzione non ancora scelto (sito servito su IP): identifier legale/SEO da sistemare al lancio.

---

## 5. Fix tipografici effettuati nelle pagine legal (questa revisione)

- **F1 — `landing/html/legal/terms.html` §9 (retention):** rimando incrociato errato `(&sect;13)` → `(&sect;12)`: l'export/cancellazione alla cessazione è la sezione 12 "Termination and exit", non la 13 "Limitation of liability"; allineato a `tos-it.md`/`tos-en.md` §9.2 che citano correttamente §12.

**Non-fix voluti (segnalati, non toccati — sostanza legale):** canonical `multi.ai` (serve il dominio di produzione), TTL 8–12h vs 24h (decisione di valore, Q10), vecchio nome nelle `.md` autentiche (Q17), stray quote in `tos-it.md` §3.2 e typo in §13.3 (file fuori dal perimetro di fix assegnato). Nessun link morto (tutti gli asset esistono), nessun escape HTML rotto (`&minus;`/`&rarr;` sono entità valide), HTML bilanciato su tutte e 3 le pagine (verificato).

---

*Review preparata sul codice e sui documenti reali del repo (branch `feature/landing-page`): commit legal 79785998, nav 0743ae6d, checkout Mollie c4990fc5, `main.go sessionTTL = 24 * time.Hour`, `landing/docs/{01-PRD,iaas-design,database-design,05-backend-schema}.md`. Da revisionare con legale abilitato IT/CH prima della pubblicazione e comunque prima del primo cliente pagante.*
