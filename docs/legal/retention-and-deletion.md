# Retention e cancellazione — matrice tecnica

Fonte di verità tecnica per la sezione 6 della privacy policy e per il DPA
(Allegato II, misure di cancellazione). Ogni riga indica **dove** il dato vive e
**cosa** lo cancella davvero: se la colonna "applicato da" è vuota, la retention
è solo una promessa e va marcata come tale nella pagina pubblica.

Verificato sul codice al 2026-10-23.

## 1. Matrice

| Dato | Dove vive | Retention | Applicato da | Note |
|------|-----------|-----------|--------------|------|
| Contenuto delle mailbox (maildir, blob `.eml` e allegati) | Volume dei blob + indice Postgres | Finché la mailbox è attiva; cancellazione alla rimozione della mailbox o dell'organizzazione | `internal/core/deletion.go` (`ProcessBlobDeletions`, coda `blob_deletions`, lease 5 min, backoff crescente) + trigger della migration `0006_deletion.sql` | La coda è drenata dal scheduler a ogni tick. Un blob non cancellabile resta in coda con `last_error`, non viene perso silenziosamente. |
| Messaggi, thread, allegati, bozze (righe DB) | Postgres | Finché la mailbox è attiva | `ON DELETE CASCADE` da `inboxes`/`organizations` | Le bozze non spedite non finiscono mai nel blob store (allegati inline, max 6 MB): cancellare la bozza non lascia residui. |
| Indirizzi di mailbox cancellate | Tabella `address_quarantine` | **90 giorni** | `core.AddressQuarantine` + `PurgeExpiredQuarantine` (scheduler) | Impedisce che un indirizzo cancellato venga riregistrato da un altro cliente. Divulgato in privacy §6 e nell'OpenAPI. |
| Audit log (azioni di sicurezza su pannello, API e MCP) | Postgres, RLS per organizzazione | **90 giorni** | `core.AuditLogRetention` + `PurgeExpiredAudit` (scheduler, ogni tick) | Prima era un target non applicato: ora la tabella viene potata. Il cliente può esportarne il contenuto con `GET /v0/export`. |
| Eventi webhook (`events`) | Postgres | Nessun TTL: seguono la vita dell'organizzazione | `ON DELETE CASCADE` da `organizations` | **Aperto**: valutare una retention (es. 30 giorni) per non far crescere la tabella. Non è un dato necessario al servizio dopo la consegna. |
| Backup cifrati (dump Postgres, blob, `caddy-data`) | Hetzner Object Storage, bucket con Object Lock, cifratura client-side rclone crypt | Object Lock **governance 30 giorni**; lifecycle `crypt/dumps/` **45 giorni**; blob conservati | Configurazione del bucket (fuori dal repository) `[ops]`; job in `deploy/k8s/base/backup.yaml` (ogni 6 ore) | La retention è una **impostazione del bucket**, non del codice: finché non è applicata, la promessa "30 giorni" non è verificabile. Vedi §3. |
| Dati di fatturazione e contatto | Postgres + Stripe | **10 anni** (obblighi fiscali, legge italiana); contatti fino a 12 mesi dopo la cessazione | Nessun job: cancellazione manuale dopo il termine `[ops]` | Stripe è controller autonomo per i dati di pagamento: non conserviamo numeri di carta. |
| Sessioni webmail (SOGo) | Server SOGo | Sessione, TTL del cookie di SOGo | Configurazione SOGo | Non impostiamo cookie nostri: vedi privacy §3. |
| Log applicativi (stdout, `slog`) | Log del container | Rotazione del runtime k8s | Configurazione del cluster | Non contengono contenuti di email né segreti: verificato in `internal/api/api.go` (`s.log` con campi espliciti). |

## 2. Cosa fa `DELETE /v0/organization` (e `mai admin org-delete`)

Ordine reale, verificato in `internal/core/orgdelete.go`:

1. tutte le API key vengono revocate e si scrive una riga `org.delete` nell'audit, in
   un'unica transazione: nessuna richiesta può più agire per l'organizzazione;
2. la summary (organizzazione, inbox, domini, conteggi e **intero audit log**) viene
   consegnata a chi ha chiesto la cancellazione: se l'export fallisce non si cancella
   nulla e il comando si può ripetere;
3. la riga dell'organizzazione viene cancellata: il cascade rimuove inbox, thread,
   messaggi, allegati, domini, webhook, eventi, bozze, chiavi e audit log; i trigger
   accodano ogni blob e mettono in quarantena ogni indirizzo per 90 giorni.

Tempi osservabili: righe DB immediate; blob entro pochi minuti dal tick del scheduler;
copie nei backup entro il ciclo di rotazione (30-45 giorni, §3).

## 3. Backup: cosa possiamo promettere

Comportamento reale (non quello che i documenti dicevano):

- backup ogni 6 ore, cifrati **prima** dell'upload (rclone crypt): senza la chiave il
  bucket non è leggibile;
- Object Lock in modalità governance 30 giorni: nessuno può cancellare un oggetto prima;
- lifecycle sui dump a 45 giorni: dopo, il dump sparisce;
- i **blob** non hanno lifecycle: restano finché non vengono rimossi con la chiave del
  vault, quindi una cancellazione GDPR non li rimuove dai backup.

Conseguenza da scrivere in chiaro nella privacy: la cancellazione nei backup si esaurisce
per rotazione naturale (entro 45 giorni per i dump, fino a 30 giorni di immutabilità), e i
blob nei backup immutabili richiedono una procedura manuale con la chiave del vault.
La formulazione contrattuale e l'accettabilità dell'Art. 17 sono `[lawyer]` (vedi
`docs/legal-review.md` Q06, Q07).

## 4. Azioni registrate nell'audit log

Elenco verificato nel codice (non esaustivo per i tool MCP, che registrano `mcp.<tool>`):
`api_key.create`, `api_key.revoke`, `inbox.create`, `inbox.delete`, `message.release`,
`domain.create`, `domain.delete`, `webhook.create`, `webhook.delete`,
`billing.checkout`, `billing.portal`, `billing.update` (webhook Stripe),
`billing.downgrade` (fine grace period), `org.export`, `org.delete`.
Il campo `actor` distingue `api_key:<id>`, `mcp:<id>`, `operator`, `stripe` e
`system:billing`; `metadata` non contiene mai segreti né contenuti di email.

Letture e invii **non** sono auditati: `message.send` non scrive una riga di audit (il
record di consegna è in `send_outcomes`). Se serve un audit di invio per i clienti
regolamentati è una voce `[code]` aperta, non un fatto già disponibile.

## 5. Voci aperte (non chiuse da questa revisione)

| Voce | Chi |
|------|-----|
| Applicare davvero il lifecycle del bucket e verificare l'Object Lock in produzione | `[ops]` / founder |
| Definire una retention per `events` (oggi nessun TTL) | `[code]`, decisione prodotto |
| Procedura di cancellazione mirata nei backup immutabili (chiave del vault) | `[lawyer]` + `[ops]` |
| Retention dei contatti commerciali dopo la cessazione: job o manuale | `[ops]` |
