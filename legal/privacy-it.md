# Informativa Privacy — multi.ai.inbox (Artt. 13/14 GDPR)

**Versione:** 1.0-draft · **Data:** 2026-10-19 · **Lingua autentica:** italiano (traduzione inglese di cortesia: `privacy-en.md`)

> ⚠️ **BOZZA INTERNA — NON PUBBLICARE TAL QUALE.** Redatta come parte del pacchetto legale del progetto multi.ai.inbox (review privacy 2026-10-19, finding P0-1). Da passare a un legale abilitato (giurisdizione IT/CH) **prima della pubblicazione sul sito**. I periodi di retention indicati sono **valori pianificati**: vanno implementati tecnicamente (§7) **prima** della pubblicazione, pena un'informativa inesatta. Campi `[...]` da completare.

---

## 1. Chi siamo, ruoli e contatti

1.1 **Titolare del trattamento:** [FORMA GIURIDICA E DENOMINAZIONE COMPLETA — DA COMPLETARE], con sede in [INDIRIZZO — DA COMPLETARE] (di seguito "**multi.ai.inbox**" o il "**Fornitore**"). Contatto privacy: [PRIVACY EMAIL — DA COMPLETARE].

1.2 **DPO:** non nominato; obbligo di nomina da verificare a scala attuale con un legale abilitato. **Riferente privacy:** [NOME — DA COMPLETARE].

1.3 **Ruoli per servizio e contenuti.** Per i dati di contatto, fatturazione e uso del servizio, multi.ai.inbox è **titolare**. Per le caselle email dei tenant (hosting rivenduto) il **titolare è il cliente** (che decide le finalità della propria corrispondenza) e multi.ai.inbox è **responsabile del trattamento** ex Art. 28 GDPR, con contratto DPA dedicato.

1.4 **Trasferimenti e applicazione territoriale.** Trattiamo dati di persone nell'UE e, ove applicabile, in Svizzera: si applica il GDPR e, per i dati di persone in CH, la **nLPD** svizzera (2023) — trattati in coerenza con entrambe.

---

## 2. Quali dati trattiamo e perché

| # | Finalità | Dati personali | Interessati | Base giuridica |
|---|---|---|---|---|
| T1 | Erogazione del servizio di posta delle caselle ospitate (ricezione, invio, conservazione, webmail) | Contenuti delle mail, intestazioni (mittenti, destinatari, oggetto), IP nei percorsi di consegna, quote | Clienti/tnt e i loro collaboratori; **mittenti e destinatari esterni** | Contratto (per il cliente); **interesse legittimo + informativa Art. 14** per i corrispondenti esterni (raccolta indiretta: scrivono nelle caselle) |
| T2 | Accesso al sito e ai pannelli (login, form di connessione) | Credenziali di accesso, username account registrar, API key, IP client, timestamp | Operatori, clienti | Esecuzione del contratto (accesso); interesse legittimo (sicurezza) |
| T3 | Log di sicurezza e audit delle azioni (pannello, API/MCP) | Evento, timestamp, tool usato, dominio, esito, IP client | Operatori, clienti | Interesse legittimo (sicurezza, Accountability Art. 5(2)) |
| T4 | Gestione domini tramite connettori registrar | Dati dell'account registrar (username, API key, IP whitelistato), dati registrant/WHOIS ove trattati | Clienti | Esecuzione del contratto |
| T5 | Contatto e fatturazione | Referenti del cliente, dati di fatturazione/uso | Clienti | Contratto; obblighi legali (fiscali) |
| T6 | **Agenti AI sulle caselle (pianificato — oggi non attivo)** | Contenuti e metadati delle mail, categorie particolari incidentali | Mittenti/destinatari (anche terzi) | Interesse legittimo + **LIA scritta** (da completare prima dell'attivazione); informativa Art. 14 |
| T7 | Hosting rivenduto a tenant | Contenuti delle mailbox dei tenant | Collaboratori dei clienti | Per i contenuti il **titolare è il cliente**; noi = responsabile (DPA) |

Le **categorie particolari** (Art. 9 GDPR) non sono trattate attivamente; possono comparire incidentalmente nei contenuti delle mail: minimizziamo (estrazione solo del necessario al task), niente logging dei contenuti, **niente addestramento di modelli sui contenuti delle caselle** senza base specifica.

---

## 3. Cookie e tecnologie simili

Usiamo **solo cookie tecnici strettamente necessari** — nessun cookie di profilazione, nessun tracker di terze parti, nessun banner (eccezione ePrivacy per cookie strictly necessary).

| Cookie | Finalità | Durata | Tipo |
|---|---|---|---|
| `MULTIAI_LANDING` | Sessione della landing (login/connessione) | Di sessione | Tecnico, HttpOnly, Secure, SameSite=Lax |
| Sessione di autenticazione pannello (mailcow) | Autenticazione admin | Di sessione | Tecnico |
| Sessione webmail (SOGo) | Autenticazione webmail | Di sessione | Tecnico |

---

## 4. Destinatari e responsabili del trattamento

| Destinatario | Paese | Ruolo | Dati condivisi | Base del rapporto |
|---|---|---|---|---|
| **Hetzner Online GmbH** | Germania (UE) | Responsabile (hosting server, database, backup) | Tutti i dati trattati sull'istanza | DPA Art. 28 — **da attivare nell'account Hetzner** (non si conclude automaticamente, T&C §6.2); processing riservato a UE/SEE |
| **Namecheap Inc.** | **USA** (extra-UE) | Responsabile per gestione domini/WHOIS; titolare autonomo per i propri account | Username account, API key, IP del server | DPA con **SCC incorporate** + **TIA documentata**; rappresentante UE (EDPO, Bruxelles, Art. 27); **non certificata EU-US Data Privacy Framework** |
| **Infomaniak** | **Svizzera** (pianificato) | Responsabile (quando il connettore è attivo) | Dati dominio/account | Adeguatezza UE→CH (decisione 2000/518/CE, confermata 2024) — nessuna SCC necessaria; DPA generato dal Manager |
| **Let's Encrypt** (ISRG) | USA | Titolare autonomo (emissione certificati TLS) | Nome dominio | Necessità tecnica del servizio |
| **Provider LLM** (pianificato, solo se gli agenti AI leggono le caselle) | Da definire | Responsabile | Contenuti mail solo se il flusso lo richiede | DPA + SCC/DPF secondo vendor, **oppure** LLM in UE — da definire **prima** dell'attivazione |
| Autorità giudiziarie/amministrative | UE | Titolari autonomi | Quanto richiesto per legge | Obbligo legale |

Nota tecnica: l'IP in uscita del server è rilevato tramite un servizio pubblico esterno (attualmente `api.ipify.org`, USA; dato riferito al server, non a una persona); sostituzione pianificata con metadata Hetzner.

---

## 5. Trasferimenti extra-UE

- **USA (Namecheap):** trasferimento di dati personali (username se l'account è personale, API key come dato pseudonimo, IP) con meccanismo **SCC** incorporate nel DPA Namecheap, corredato da **TIA documentata**. Namecheap non risulta certificata DPF.
- **USA (Let's Encrypt):** nome dominio per l'emissione dei certificati TLS.
- **CH (Infomaniak, pianificato):** paese con **decisione di adeguatezza** — nessuna garanzia aggiuntiva richiesta.
- Nessun altro trasferimento extra-UE è effettuato; eventuali nuovi destinatari (inclusi provider LLM) saranno aggiunti a questa sezione **prima** dell'attivazione, con il relativo meccanismo.

---

## 6. Conservazione (retention)

| Dato | Conservazione |
|---|---|
| Contenuti delle caselle (maildir) | Finché la casella è attiva; cancellazione alla rimozione della casella o alla cessazione |
| Backup delle caselle e del database | Rotazione **30 giorni** (pianificata) |
| Log di sistema e audit (pannello, MCP) | **90 giorni** (pianificato) |
| Sessioni di connessione landing | Di sessione — TTL **8–12 ore** (da impostare) |
| Dati di fatturazione e contatti cliente | **10 anni** (obblighi fiscali, normativa italiana); contatti fino a 12 mesi dalla cessazione |

⚠️ I valori segnati "pianificato/da impostare" sono **target**: la loro implementazione tecnica (lTrim/expire su Redis API log, TTL sessioni, logrotate, rotazione backup) è condizione **precedente** alla pubblicazione di questa informativa. Il diritto di cancellazione sui backup si esaurisce alla rotazione naturale (prassi accettata).

---

## 7. Sicurezza del trattamento

TLS su tutte le superfici; credenziali API custodite cifrate **AES-256-GCM** lato server e mai in log/pagine/repository; isolamento container (read-only rootfs, cap-drop), rate limiting, audit log senza segreti; backup cifrati su volume dedicato; accesso del personale ai contenuti delle caselle **solo su richiesta autorizzata del cliente** e documentato. In caso di **violazione dei dati** con rischio per gli interessati, notifica al Garante senza ingiustificato ritardo (Art. 33) e comunicazione agli interessati quando a alto rischio (Art. 34); per le mailbox dei tenant, notifica immediata al cliente-titolare come da DPA.

---

## 8. Agenti AI

Oggi **nessun agente AI legge le caselle** (i tool disponibili operano a livello dominio). Se la funzione sarà attivata: minima estrazione per task, **nessun addestramento di modelli sui contenuti**, azioni verso terzi con **revisione umana** (human-in-the-loop), **nessuna decisione esclusivamente automatizzata** con effetti giuridici o significativi (Art. 22 GDPR — es. sospensione/cancellazione account, risposte vincolanti), chiavi per-agente auditabili, LIA e informativa Art. 14 verso i corrispondenti prima dell'attivazione.

---

## 9. Diritti degli interessati

9.1 Hai i diritti degli Artt. **15–22 GDPR**: accesso, rettifica, cancellazione, limitazione, portabilità, opposizione; per trattamenti basati su interesse legittimo puoi opporti in qualsiasi momento (Art. 21). Esercizio dei diritti: [PRIVACY EMAIL — DA COMPLETARE]; risposta entro 1 mese.

9.2 **Mittenti/destinatari esterni** (persone che scrivono nelle caselle ospitate): contatta il [PRIVACY EMAIL] — per le caselle di clienti-tenant siamo responsabili e assisteremo il titolare-cliente nel gestire la tua richiesta (Art. 28(3)(e)).

9.3 **Reclamo:** puoi proporre reclamo al **Garante per la protezione dei dati personali** (www.garanteprivacy.it) o, in Svizzera, al **FDPIC**; e in ogni caso al titolare del trattamento.

---

## 10. Modifiche all'informativa

Le modifiche sono pubblicate su questa pagina con data di aggiornamento; le variazioni sostanziali sono notificate ai clienti via email. Versione corrente: 1.0-draft, 2026-10-19.

---

*Documento redatto su base review privacy 2026-10-19 (dati verificati sul codice: landing, backend Go, docker-compose) e documenti vendor verificati (DPA Hetzner, T&C §6.2, privacy/DPA Namecheap, DPA Infomaniak, decisione adeguatezza UE→CH). Da revisionare con legale abilitato IT/CH prima della pubblicazione.*
