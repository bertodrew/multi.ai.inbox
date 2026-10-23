# Violazione di dati personali — runbook (bozza)

Procedura per un incidente con dati personali (Art. 33 e 34 GDPR e Art. 33(2) verso il
cliente-controller). Bozza tecnica: soglie, testo delle notifiche e valutazione del rischio
sono `[lawyer]`.

## 1. Rilevazione

Fonti: audit log per organizzazione, metriche Prometheus (`internal/metrics`), health
endpoint, dead man's switch dei backup, allarmi dell'hosting, segnalazione di un cliente o
di un ricercatore.

Segnali tipici: accesso con una chiave API non riconosciuta, raffica di 401/403, export
anomalo (`org.export` su organizzazioni non attese), cancellazioni di massa, accesso al
bucket dei backup, alterazione dei deployment.

## 2. Contenimento (prime ore)

1. Revocare le chiavi coinvolte: `DELETE /v0/api-keys/{key}` o revoca d'emergenza a DB
   (`update api_keys set revoked_at = now()`).
2. Sealare l'accesso: ruotare i segreti k8s (`mai-backup`, Stripe, registrar), togliere
   il token Hetzner/GitHub all'agente operativo.
3. Conservare le prove **prima** di pulire: export di `audit_log`, log del pod, snapshot
   del DB. L'audit log ha retention 90 giorni: non aspettare.
4. Se il sospetto è un difetto applicativo, congelare il deployment (rollback all'immagine
   precedente in `deploy/k8s/staging/kustomization.yaml`).

## 3. Valutazione (entro 24 ore)

Compilare una scheda con: cosa è successo, quali dati e quante persone, categorie (email e
allegati sono contenuto, quindi potenzialmente Art. 9), se i dati erano cifrati o
accessibili, durata, contenimento efficace, rischio per diritti e libertà.
La qualificazione "rischio" e la decisione di notificare sono `[lawyer]`.

## 4. Notifiche

| Destinatario | Quando | Contenuto minimo | Chi |
|--------------|--------|------------------|-----|
| Cliente-controller (per le mailbox dei tenant) | Senza indebito ritardo, **prima** di ogni altra comunicazione | Natura dell'incidente, categorie e volume, conseguenze, misure adottate, referente | `[lawyer]` per il testo, operatore per l'invio |
| Autorità di controllo (Garante / FDPIC) | Entro **72 ore** dalla conoscenza, se sussiste rischio | Art. 33(3): natura, categorie e numero, DPO/referente, conseguenze, misure | `[lawyer]` |
| Interessati | Se rischio elevato (Art. 34) | Linguaggio semplice, cosa fare per limitare il danno | `[lawyer]` |

Il canale di notifica ai clienti è l'email degli amministratori dell'organizzazione; per
i terzi corrispondenti, se necessario, la pubblicazione sulla pagina di stato `[lawyer]`.

## 5. Chiusura

1. Registro degli incidenti: data, causa, dati coinvolti, notifiche inviate, prove.
2. Post-mortem tecnico senza colpevoli: causa radice, test di regressione, modifica al
   codice o alla configurazione.
3. Aggiornare `docs/legal/records-of-processing.md` se cambiano misure o trattamento.
4. Se l'incidente nasce da un sub-processor, verificare l'obbligo contrattuale di notifica
   (Annex III del DPA) e i tempi dichiarati dal fornitore `[lawyer]`.

## 6. Cosa il prodotto offre già per l'indagine

- audit log per organizzazione, con `actor`, `action`, `target`, `ip`, `request_id`, 90 giorni;
- export completo del log via `GET /v0/export` (permesso `data_export`);
- `send_outcomes` per gli invii, `events` per i webhook;
- backup immutabili: un attaccante con la chiave S3 del cluster non può cancellarli.

## 7. Vuoti noti

- Nessun alert automatico su "export di massa" o "revoca di molte chiavi": oggi serve
  leggerli dall'audit log. Voce `[code]` aperta.
- Nessun registro incidenti nel repository: va tenuto fuori (dati personali), con un
  riferimento interno.
