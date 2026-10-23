# Registro delle attività di trattamento (Art. 30 GDPR) — bozza

Documento richiesto dall'Art. 30(1) per il titolare e dall'Art. 30(2) per il
responsabile. Questa è una bozza tecnica costruita sul codice: le qualificazioni
giuridiche e i dati societari sono `[lawyer]` / `[founder]`.

Titolare: `[ragione sociale]`, sede `[indirizzo]`, contatto privacy `[email]`.
DPO: `[da nominare / non obbligatorio — decisione lawyer]`.
Ultimo aggiornamento: 2026-10-23.

## A. Attività in cui siamo titolari

| # | Attività | Finalità | Categorie di interessati | Categorie di dati | Base giuridica | Destinatari | Trasferimenti | Retention | Misure |
|---|----------|----------|--------------------------|-------------------|----------------|-------------|---------------|-----------|--------|
| A1 | Contatto commerciale e supporto | Rispondere a richieste, gestire il rapporto | Referenti dei clienti e prospect | Nome, email, contenuto del messaggio | Contratto o misure precontrattuali (Art. 6(1)(b)) | Hetzner (hosting) | Nessuno (UE) | 12 mesi dall'ultimo contatto | Accesso ristretto, TLS, nessun tracker |
| A2 | Fatturazione e adempimenti fiscali | Fatturare, conservare per obbligo di legge | Referenti amministrativi | Ragione sociale, indirizzo, P.IVA, importi | Obbligo legale (Art. 6(1)(c)) | Stripe, commercialista | Stripe: USA, DPF/SCC secondo il fornitore `[lawyer]` | 10 anni | Dati carta mai sul nostro sistema |
| A3 | Sicurezza del servizio (log, audit, antiabuso) | Prevenire accessi illeciti, indagare incidenti | Utenti API, operatori, mittenti | IP, timestamp, azione, esito, request id | Legittimo interesse (Art. 6(1)(f)) | Hetzner | Nessuno (UE) | 90 giorni (`core.AuditLogRetention`) | RLS per organizzazione, log senza contenuti né segreti |
| A4 | Gestione domini dei clienti | Registrare e verificare i domini | Referenti tecnici, registranti WHOIS | Nome dominio, username/API key del registrar, IP autorizzati | Contratto (Art. 6(1)(b)) | Namecheap, Infomaniak (previsto) | USA: SCC + TIA `[lawyer]`; CH: adeguatezza | Durata del dominio + 30 giorni | Credenziali cifrate AES-256-GCM, mai nei log |

## B. Attività in cui siamo responsabili (per conto dei clienti)

| # | Attività | Finalità | Interessati | Dati | Istruzioni | Sub-responsabili | Retention | Misure |
|---|----------|----------|-------------|------|------------|------------------|-----------|--------|
| B1 | Hosting di mailbox e invio/ricezione | Erogare il servizio di posta per gli agenti AI del cliente | Clienti del cliente, corrispondenti esterni (raccolta indiretta) | Contenuti e metadati delle email, allegati, IP di consegna | Contratto di servizio + DPA (Art. 28) | Hetzner; LLM solo se attivato `[lawyer]` | Finché la mailbox è attiva; cancellazione su istruzione | TLS, antivirus (ClamAV), screening (gatekeeper), RLS, cifratura credenziali, backup cifrati |
| B2 | Screening e antispam | Bloccare phishing e prompt injection prima della notifica | Come B1 | Header, corpo, link, verdetto | Configurazione dell'organizzazione (`screen_before_notify`) | Nessuno | Verdetto conservato con il messaggio | Fail-closed: senza verdetto il messaggio resta in attesa |
| B3 | Audit delle azioni | Consentire al cliente di verificare chi ha fatto cosa | Utenti API del cliente | Attore, azione, target, IP, esito | DPA, allegato sicurezza | Nessuno | 90 giorni, esportabile con `GET /v0/export` | RLS, nessun segreto nei metadati |

## C. Attività **non** attive (da completare prima dell'attivazione)

| # | Attività | Stato | Cosa manca |
|---|----------|-------|------------|
| C1 | Agenti AI che leggono le mailbox | Non attiva: gli strumenti disponibili operano a livello di dominio | Scelta del fornitore LLM, DPIA, LIA, avviso Art. 14 ai corrispondenti, clausola DPA `[lawyer]` |
| C2 | Connettore Infomaniak | Non attivo | DPA del fornitore, verifica del trasferimento CH `[lawyer]` |
| C3 | Analytics di prodotto | Non attivo | Base giuridica e consenso, se mai introdotto |

## D. Misure di sicurezza comuni (riferimento per l'Allegato II del DPA)

TLS su tutte le superfici; credenziali e segreti cifrati lato server (AES-256-GCM),
mai in log, pagine o repository; isolamento dei tenant con Row Level Security in
Postgres; rate limiting per organizzazione e per IP; antivirus e screening fail-closed;
audit log senza segreti; backup cifrati client-side con Object Lock; accesso del
personale alle mailbox solo su richiesta autorizzata e registrata; segreti in Secret
k8s, non nel repository.
