# DPA — Contratto di trattamento dati (Art. 28 GDPR) — template per i clienti tenant

**Versione:** 1.0-draft · **Data:** 2026-10-19 · **Lingua:** italiano [versione EN da produrre se/quando i tenant lo richiedono — punto aperto del pacchetto]

> ⚠️ **BOZZA INTERNA — NON USARE PER FIRMARE.** Template art. 28(3) GDPR per il modello "hosting rivenduto a tenant": **Cliente = titolare**, **Fornitore (multi.ai.inbox) = responsabile**. Da passare a un legale abilitato (giurisdizione IT/CH) **prima della prima vendita hosting e comunque prima del primo dato personale trattato per un tenant**. Nei sistemi UE un generico ToS **non** soddisfa la sostanza dell'art. 28: il DPA deve essere separato o incorporato in forma vincolante e firmato. Campi `[...]` da completare.

---

**CONTRATTO DI TRATTAMENTO DATI PERSONALI ("DPA")** ai sensi dell'Art. 28 del Regolamento (UE) 2016/679 ("GDPR") tra:

- **[FORMA GIURIDICA E DENOMINAZIONE DEL CLIENTE — DA COMPLETARE]** ("**Titolare**"), e
- **[FORMA GIURIDICA E DENOMINAZIONE DEL FORNITORE — DA COMPLETARE]**, multi.ai.inbox ("**Responsabile**").

Il DPA fa parte integrante e sostanziale del contratto di servizio ("Termini di Servizio"). In caso di conflitto sui trattamenti dati prevale questo DPA.

---

## 1. Oggetto, durata, natura e scopo del trattamento (Art. 28(3)(a))

1.1 **Oggetto:** erogazione del servizio di mail hosting gestito per il Titolare — caselle email (SMTP/IMAP, webmail SOGo), pannello di amministrazione, gestione DNS dei domini tramite connettori, antispam/antivirus, TLS, backup — come descritto nei Termini di Servizio.

1.2 **Durata:** la durata del contratto di servizio, inclusa la finestra di export/post-cessazione di [30 giorni — DA COMPLETARE].

1.3 **Natura del trattamento:** conservazione, trasmissione, memorizzazione, backup, gestione tecnica della posta e — solo se attivato dal Titolare — triage/estrazione di attività da parte di agenti AI come da §9.

1.4 **Scopo:** esclusivamente l'erogazione del servizio per conto del Titolare secondo le sue **istruzioni documentate**. Nessuna finalità propria del Responsabile (nemmeno analytics, benchmarking o miglioramento di prodotti) senza mandato scritto.

## 2. Tipi di dati e categorie di interessati (Art. 28(3)(b))

2.1 **Tipi di dati:** contenuti delle caselle (mail, allegati), metadati (intestazioni, mittenti/destinatari, oggetti), indirizzi IP, identificativi account, credenziali gestite, dati tecnici di log e audit, dati di configurazione dei domini.

2.2 **Categorie di interessati:** collaboratori e utenti del Titolare; mittenti e destinatari esterni della corrispondenza. Le categorie particolari (Art. 9) possono comparire incidentalmente nei contenuti: nessun trattamento attivo al di là della conservazione/delivery.

## 3. Istruzioni documentate del Titolare (Art. 28(3)(a))

Il Responsabile tratta i dati **solo su istruzioni documentate** del Titolare, incluse quelle relative ai trasferimenti (§8). Il Titolare può emettere istruzioni scritte (anche email) coerenti con i Termini; il Responsabile segnala immediatamente istruzioni che ritiene violino il GDPR.

## 4. Riservatezza (Art. 28(3)(b))

Il Responsabile garantisce che le persone autorizzate al trattamento si impegnano alla **riservatezza** o agiscono sotto opportuna riservatezza statutaria/contrattuale. L'**accesso al contenuto delle caselle** da parte del personale del Responsabile avviene **solo su richiesta autorizzata e documentata del Titolare** (supporto, incidenti, richieste di diritti) e resta tracciato in audit log.

## 5. Misure di sicurezza (Art. 28(3)(c); Art. 32)

Misure di riferimento, aggiornabili: cifratura in transito (TLS su tutte le superfici); credenziali/API key custodite cifrate **AES-256-GCM** lato server e mai in log/repository; autenticazione pannelli; isolamento container; rate limiting; audit log senza segreti; **backup cifrati** su volume dedicato con rotazione [30 giorni — pianificata]; gestione delle vulnerabilità (scansione periodica; divulgazione coordinata, non pubblicazione di report dettagliati sul servizio in produzione). Il Responsabile documenta il modello di rischio e lo aggiorna a cambiamenti rilevanti.

## 6. Sub-responsabili (Artt. 28(2), 28(4))

6.1 Il Titolare **autorizza** l'impiego dei seguenti sub-responsabili, elencati con obbligo di informativa preventiva sui cambiamenti:

| Sub-responsabile | Paese | Servizio | Meccanismo trasferimento |
|---|---|---|---|
| Hetzner Online GmbH | Germania (UE) | Hosting server, DB, backup | Nessuno (UE/SEE) — DPA attivato nell'account |
| Namecheap Inc. | USA | Registrar/gestione domini (solo se il Titolare usa il connettore) | SCC (DPA) + TIA |
| Infomaniak | Svizzera (pianificato) | Hosting/domini alternativi | Adeguatezza UE→CH |
| Let's Encrypt (ISRG) | USA | Certificati TLS (nome dominio) | Necessità tecnica |
| [Provider LLM — DA DEFINIRE PRIMA DELL'ATTIVAZIONE] | Da definire | Solo se agenti AI leggono le caselle | SCC/DPF o LLM in UE |

6.2 Elenco aggiornato: [URL PAGINA SUB-PROCESSORS — DA PUBBLICARE]. Il Titolare riceve informativa preventiva di nuovi sub-responsabili con [X giorni] per obiezione motivata; in mancanza di accordo, recesso con rimborso pro-rata.

6.3 Il Responsabile **rimane pienamente responsabile verso il Titolare** per l'adempimento degli obblighi del DPA da parte dei sub-responsabili (Art. 28(4)) e impone loro gli stessi obblighi contrattuali (in particolare Hetzner: DPA attivato nell'account — non si conclude automaticamente).

## 7. Assistenza al Titolare

7.1 **Diritti degli interessati (Art. 28(3)(e)):** il Responsabile assiste il Titolare, con mezzi appropriati, nel soddisfare le richieste degli interessati (Artt. 15–22), segnalando le richieste ricevute direttamente **entro [48 ore]** e non rispondendo al posto del Titolare salvo istruzioni.

7.2 **Sicurezza e notifica (Art. 28(3)(f); Artt. 33/34):** il Responsabile **notifica al Titolare ogni violazione dei dati personali senza ingiustificato ritardo** e in ogni caso **entro [48 ore]** dalla conoscenza, con la documentazione disponibile (natura, categorie, numero interessati, misure adottate). Il Titolare gestisce la notifica al Garante (Art. 33) e agli interessati (Art. 34).

7.3 **DPIA e consultazioni:** il Responsabile fornisce le informazioni necessarie per DPIA e consultazioni previe (Artt. 35/36), inclusa la descrizione delle misure di §5.

## 8. Trasferimenti extra-UE

Il Responsabile non trasferisce i dati del Titolare fuori da UE/SEE, salvo i sub-responsabili autorizzati in §6 e i relativi meccanismi (SCC+TIA per Namecheap; adeguatezza per CH). Qualsiasi nuovo trasferimento richiede istruzioni documentate e meccanismo adeguato.

## 9. Agenti AI (solo se attivati dal Titolare)

9.1 Se il Titolare attiva la funzione agenti AI: estrazione **minima** per task; **nessun addestramento di modelli sui contenuti** delle caselle; azioni con effetti verso terzi **solo con revisione umana**; **nessuna decisione esclusivamente automatizzata** con effetti giuridici o significativi (Art. 22 GDPR); chiavi per-agente auditabili; log dei contenuti vietati.

9.2 L'attivazione richiede informativa aggiornata verso i corrispondenti esterni (Art. 14) e valutazione congiunta della base giuridica.

## 10. Restituzione e cancellazione (Art. 28(3)(g))

Alla cessazione, a scelta del Titolare: **restituzione/export** (formato Maildir/EML, IMAP migration, export configurazioni/domini) e **cancellazione** dei dati entro [30 giorni], comprese le copie; residuo nei backup fino a rotazione naturale (max [30 giorni], dichiarata nella informativa privacy), dopo il quale è definitivamente cancellato. Il Responsabile certifica per iscritto la cancellazione su richiesta.

## 11. Audit e documentazione (Art. 28(3)(h))

Il Responsabile rende disponibili al Titolare le informazioni necessarie a dimostrare l'adempimento e consente **audit/inspezioni**: [annuali] su appuntamento, oppure su report di sicurezza e attestazioni messe a disposizione; audit on-site del Titolare o di terzo da lui incaricato con preavviso di [X giorni], nel rispetto della riservatezza di altri clienti e della sicurezza.

## 12. Responsabilità e indennizzi

Coerentemente con i Termini: il cap di responsabilità è quello dei Termini; le sanzioni amministrative GDPR inflitte a una parte per la propria violazione restano a suo carico. Il Responsabile risponde dei danni derivanti da violazione del DPA nei limiti di legge.

## 13. Clausole finali

Legge applicabile e foro come da Termini (legge italiana, foro [DA COMPLETARE]). Modifiche solo per iscritto. Nulla in questo DPA limita gli obblighi più favorevoli agli interessati già presenti nella informativa privacy.

**Firme:**

| | Titolare | Responsabile |
|---|---|---|
| Denominazione | [___] | [___] |
| Legale rappresentante | [___] | [___] |
| Luogo, data | [___] | [___] |
| Firma | [___] | [___] |
