# Legal review v2 — seconda opinione sistematica

Metodo: checklist di `gdpr-dsgvo-expert` (Art. 5/6/9/13-22/24/28/30/32/33-35, Chapter V),
`data-privacy-compliance` (rights, retention, breach, vendor DPA), `incident-reporting-navigator`
(NIS2/GDPR/DORA/CRA screening), `cra-vulnerability-obligations` (scope/Annex I/Art. 14).
Applicate a `docs/legal/*` e `landing/legal/*` al commit `8f6185ad`.

**Documento di lavoro interno, non è consulenza legale.** Le voci `[lawyer]` restano aperte.

## Esito in una riga

La v1 copre bene il GDPR core (diritti, retention, DPA, breach, Art. 30). Le checklist
rivelano **3 buchi strutturali non tracciati** + 4 integrazioni puntuali. Nessuna delle
18 domande è chiusa da questa review.

---

## Buchi strutturali (non presenti in v1)

### V1 — Stripe non è nella lista sub-processor `[docs]` + `[lawyer]`

`docs/legal/subprocessors.md` elenca Hetzner, Namecheap, Infomaniak, Let's Encrypt,
ipify — **Stripe assente**, benché il billing sia in roadmap attiva
(`docs/billing-enable.md`, scaffold pronto). Stripe è **USA** con trasferimento
extra-UE su pagamenti: richiede DPA + SCC/TIA come Namecheap (G14), in più è
**controller autonomo** per i dati di pagamento (PCI: la carta non passa da noi).

**Fix:** riga Stripe in `subprocessors.md` + `landing/legal/subprocessors.html`;
TIA `[lawyer]` insieme a quella Namecheap.

### V2 — Brevo non è nella lista sub-processor `[docs]` + `[lawyer]`

Il relay SMTP è in produzione (`MAI_SMTP_RELAY` nel secret `mai-secrets`) e
**nessun documento legale lo cita**. Brevo (Sendinblue SAS, **Francia = UE**) tratta
indirizzi email e contenuto dei messaggi in transito: è un sub-processor a tutti
gli effetti, Art. 28.

**Fix:** riga Brevo in entrambe le liste; nel DPA la finalità "consegna email
in uscita". Se si passa alla porta 25 diretta (unblock Hetzner), la riga va rimossa.

### V3 — EU AI Act non valutato `[lawyer]` + `[founder]`

Il prodotto fa girare **agenti AI che leggono mailbox e inviano email**. La v1
tratta il fornitore LLM (G15) ma non la **classificazione AI Act**: gli agenti
sono quasi certamente "sistemi AI a rischio limitato" → obbligo di **trasparenza
Art. 50** (l'utente deve sapere che interagisce con un sistema AI) e Art. 4
(AI literacy). Se un agente decide autonomamente su invii, si avvicina all'Art. 22
GDPR (già Q13) e va documentato.

**Fix:** sezione "AI Act" nel registro Art. 30 e nel DPA; checklist Art. 50 prima
dell'attivazione agenti.

---

## Integrazioni puntuali

### V4 — NIS2: screening negativo documentato `[docs]`
Nessun documento menziona NIS2. Il prodotto è SaaS email: **non** è nei settori
Allegato I/II, quindi molto probabilmente fuori scope — ma lo screening va
**scritto** (una riga nel registro), altrimenti in due diligence risulta "non
valutato" invece di "valutato e fuori scope".

### V5 — CRA: fuori scope oggi, dentro dal 11/12/2027 `[docs]`
`cra-vulnerability-obligations`: AImulti è un "prodotto con elementi digitali"
messo sul mercato UE. Se venduto come **servizio** (SaaS) il CRA non si applica
al software in sé (Art. 3(1) esclude i servizi non incorporati in un prodotto),
ma se in futuro si distribuisce il **connettore/agente on-prem** il CRA si applica.
Da annotare come decisione di prodotto, non come gap legale.

### V6 — Hetzner DPA: verificare che sia firmato `[ops]`
Il registro assume Hetzner come sub-processor con DPA. Hetzner lo fornisce
automaticamente nei Terms, ma va **scaricato e archiviato** con data, altrimenti
manca la prova Art. 28(3).

### V7 — Matrice retention incompleta sui metadati `[docs]`
`retention-and-deletion.md` copre mailbox, blob, audit, backup. **Non copre**:
`events` (3 righe oggi, nessun TTL), `webhook_deliveries` (crescita illimitata),
`idempotency_keys` (nessuna scadenza). Tre tabelle che crescono senza limite =
violazione Art. 5(1)(e) per eccesso di conservazione.

**Fix:** righe nella matrice + TTL/purge nello scheduler (come già fatto per audit).

---

## Cosa questa review NON chiude

Le 18 domande di v1 restano tutte aperte: richiedono un avvocato IT/CH.
Le due skill Ansvar (`cra-vulnerability-obligations`, `incident-reporting-navigator`)
richiedono il **gateway MCP `https://gateway.ansvar.eu/mcp`**, non connesso:
le affermazioni CRA/NIS2 qui sopra sono screening qualitativo, **non** citazioni
da testo ufficiale. Per un parere citato va connesso il gateway.

## Priorità suggerita

1. **V1 + V2** (sub-processor mancanti): 30 minuti, chiude un buco Art. 28 reale.
2. **V7** (TTL su events/webhook_deliveries/idempotency_keys): codice, chiude Art. 5(1)(e).
3. **V6** (archiviare DPA Hetzner): 5 minuti.
4. **V3** (AI Act): prima di attivare agenti con accesso mailbox.
5. **V4/V5**: una riga ciascuno nel registro, per la due diligence.
