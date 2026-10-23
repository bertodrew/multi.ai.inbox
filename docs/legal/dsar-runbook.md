# DSAR — runbook per le richieste degli interessati

Procedura operativa per accesso, rettifica, cancellazione, limitazione, portabilità e
opposizione (Artt. 15-22 GDPR). Non è consulenza legale: i casi limite e la valutazione
delle eccezioni sono `[lawyer]`.

## 1. Canale e presa in carico

- Canale unico: l'indirizzo privacy pubblicato nella policy (`[lawyer]`/`[founder]`: il
  valore reale va inserito, oggi è un segnaposto).
- Ogni richiesta apre un ticket con: data di arrivo, identità dichiarata, diritto
  esercitato, organizzazione/mailbox coinvolta, esito, data di chiusura.
- Termine: **1 mese** dall'arrivo (prorogabile di 2 mesi per richieste complesse, con
  comunicazione motivata entro il primo mese).

## 2. Verifica dell'identità (prima di qualsiasi azione)

| Richiedente | Come si verifica |
|-------------|------------------|
| Cliente/tenant (amministratore) | Chiave API con permesso adeguato, oppure richiesta firmata dall'indirizzo di contatto dell'organizzazione già registrato. |
| Interessato terzo (destinatario/mittente di email) | Email al nostro indirizzo privacy **da** o **verso** l'indirizzo coinvolto; in caso di dubbio, conferma su un canale indipendente. Mai dati a chi non può dimostrare il controllo dell'indirizzo. |
| Dipendente di un cliente | Si rinvia al cliente-controller: siamo processor. |

## 3. Accesso e portabilità (Art. 15, 20)

1. Il cliente esporta da solo: `GET /v0/export` con una chiave che ha `data_export`.
   Il documento contiene organizzazione, inbox, domini, webhook, metadati delle chiavi,
   conteggi e **intero audit log**; i segreti sono esclusi per costruzione.
2. I contenuti delle mailbox non sono duplicati nell'export: si esportano via IMAP
   (cartelle complete, formato standard) oppure messaggio per messaggio con
   `GET /v0/inboxes/{inbox}/messages/{message}/raw`.
3. Se la richiesta arriva da un terzo e riguarda mailbox di un tenant: risposta al
   richiedente con rinvio al titolare, e assistenza al cliente-controller (Art. 28(3)(e)).
   `[lawyer]`: testo standard della risposta di rinvio.

## 4. Cancellazione (Art. 17)

| Ambito | Azione | Effetto |
|--------|--------|---------|
| Singola inbox | `DELETE /v0/inboxes/{inbox}` (permesso `inbox_write`) | Righe DB subito, blob in coda, indirizzo in quarantena 90 giorni |
| Intera organizzazione (self-service) | `DELETE /v0/organization` con `{"confirm":"<nome organizzazione>"}`, permesso `data_delete` | Revoca chiavi, restituisce l'export, cancella tutto; blob in coda |
| Intera organizzazione (operatore) | `mai admin org-delete -org <uuid> -confirm <nome>` | Identico; la summary con l'audit log va su stdout e **va conservata** |
| Mailbox di un terzo che scrive a un cliente | Non cancelliamo: il titolare è il cliente. Si inoltra la richiesta al cliente-controller | `[lawyer]`: tempi e prova dell'inoltro |

Dopo la cancellazione: righe DB immediate, blob entro il tick del scheduler, copie nei
backup per rotazione (30-45 giorni). La cancellazione nei backup immutabili richiede la
procedura con la chiave del vault (`[ops]`, `[lawyer]`).

## 5. Rettifica, limitazione, opposizione

- Rettifica dei dati di contatto: richiesta all'operatore, modifica a DB con audit.
- Opposizione al trattamento per legittimo interesse (Art. 21): si valuta caso per caso;
  la decisione motivata è `[lawyer]`.
- Limitazione: non esiste oggi un flag applicativo che sospenda il trattamento di una
  singola mailbox mantenendone i dati. **Voce aperta `[code]`**: se il volume di richieste
  lo giustifica, serve un campo `restricted_until` su `inboxes` e un filtro in lettura.
  Fino ad allora la limitazione va realizzata con un intervento manuale sull'infrastruttura
  (il breaker dei bounce sospende l'invio da solo, `mai admin inbox-resume` lo riattiva, ma
  nessuno dei due è un comando di limitazione), documentando la scelta `[lawyer]`.

## 6. Prova e registro

- Ogni export viene registrato nell'audit log come `org.export` (attore, IP, request id).
- Ogni cancellazione scrive `org.delete` (attore `operator` o `api_key:<id>`).
- Il registro delle richieste (ticket) è la prova esterna: l'audit log si cancella con
  l'organizzazione, il ticket no. Conservare i ticket per 3 anni `[lawyer]`.
