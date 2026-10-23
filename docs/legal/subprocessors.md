# Sub-processor — registro interno

Registro dei sub-responsabili del trattamento (Art. 28(2) e (4) GDPR) per il
servizio AImulti.inbox. La lista pubblica è `landing/legal/subprocessors.html`:
le due copie devono restare allineate, questa è la fonte.

Stato: **bozza tecnica**, da validare `[lawyer]` prima della pubblicazione come
allegato contrattuale. Ogni riga "DPA" indica se l'accordo con il fornitore è
attivo o solo previsto.

| # | Fornitore | Paese / sede | Servizio | Dati trattati | Ruolo | Trasferimento extra-EEA | DPA |
|---|-----------|--------------|----------|---------------|-------|-------------------------|-----|
| 1 | Hetzner Online GmbH | Germania (UE) | Server, volumi, database, backup, object storage | Tutti i dati del servizio (mailbox, metadati, audit log, backup cifrati) | Sub-processor | Nessuno (UE) | **Da attivare nell'account Hetzner**: non si conclude automaticamente con l'ordine. `[ops]` |
| 2 | Namecheap Inc. | USA | Registrazione e gestione domini, WHOIS | Nome dominio, username e API key del connettore, IP autorizzati; dati registrante/WHOIS se trattati | Sub-processor per il connettore; controller autonomo per il proprio account | USA — SCC nel DPA Namecheap + TIA `[lawyer]`; fornitore **non** DPF | Presente nel DPA del fornitore; TIA da produrre `[lawyer]` |
| 3 | Infomaniak Network SA | Svizzera | Connettore domini (previsto, non attivo) | Come Namecheap, quando attivo | Sub-processor | Svizzera: decisione di adeguatezza UE→CH, nessuna SCC | Da attivare prima dell'uso `[ops]` |
| 4 | Internet Security Research Group (Let's Encrypt) | USA | Emissione certificati TLS | Nome dominio (nessun dato personale ulteriore) | Controller autonomo | USA — necessità tecnica, solo nome dominio | Non applicabile |
| 5 | Fornitore LLM (da definire) | Da definire | Agenti AI che leggono le mailbox (previsto, **non attivo**) | Contenuti e metadati delle email, solo dove il flusso lo richiede | Sub-processor | Da determinare con il fornitore | Da firmare **prima** dell'attivazione `[lawyer]` |
| 6 | Hetzner Object Storage (stesso fornitore, bucket separato) | Germania/UE | Bucket immutabile per i backup cifrati | Backup cifrati (rclone crypt) | Sub-processor | Nessuno (UE) | Coperto dal DPA Hetzner |
| 7 | api.ipify.org | USA | Rilevamento dell'IP pubblico di uscita del server | IP del server, non della persona | Non tratta dati personali dei clienti | USA (irrilevante: dato non personale) | Non applicabile |

Note tecniche verificabili nel repository:

- L'hosting e i backup restano su Hetzner: `deploy/k8s/base/*.yaml`, `deploy/hetzner/`.
- Il rilevamento dell'IP di uscita è documentato in privacy §4 ed è usato per i record SPF.
- Nessun servizio di analytics, nessun CDN di terze parti, nessun tracker: verificato in
  `landing/assets/*.js` (nessun `document.cookie`, nessun `localStorage`, nessuna chiamata
  a domini terzi oltre a quelle applicative).

## Procedura di modifica (impegno contrattuale, `[lawyer]` per la clausola)

1. Nuovo sub-processor: si aggiunge **prima** dell'attivazione, in questo registro e nella
   pagina pubblica, con paese, servizio, dati e base del trasferimento.
2. Preavviso ai clienti: 30 giorni, via email agli amministratori dell'organizzazione.
3. Diritto di obiezione: il cliente può opporsi motivatamente; se l'obiezione non è
   risolvibile, può recedere senza penale (clausola da validare `[lawyer]`).
4. Rimozione di un fornitore: si aggiorna il registro lo stesso giorno e si annota nel
   changelog qui sotto.

## Changelog

| Data | Modifica |
|------|----------|
| 2026-10-23 | Prima versione del registro (Hetzner, Namecheap, Infomaniak previsto, ISRG, LLM previsto, object storage, ipify). |
