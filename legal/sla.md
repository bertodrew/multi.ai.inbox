# SLA — Service Level Agreement — multi.ai.inbox (99,5% mensile)

**Versione:** 1.0-draft · **Data:** 2026-10-19 · **File bilingue:** versione italiana autentica + English translation (convenience)

> ⚠️ **BOZZA INTERNA — NON PUBBLICARE TAL QUALE.** Da passare a un legale abilitato (giurisdizione IT/CH) prima del lancio. Gli scaglioni di credito e i tempi di risposta sono **proposte di mercato da validare** con il legale. Parte integrante dei Termini di Servizio (`tos-it.md` / `tos-en.md`).

---

## Versione italiana

### 1. Impegno di disponibilità

Il Fornitore si impegna a un'**uptime mensile del 99,5%** per i servizi di **posta (SMTP: 25/465/587, IMAP: 143/993)** e **webmail (HTTPS)**. In un mese di 30 giorni il 99,5% corrisponde a un massimo di **~216 minuti (3h 36m)** di indisponibilità imputabile.

⚠️ **Natura dell'impegno:** il servizio è erogato su **istanza singola (single-tenant, single-instance)** su Hetzner (DE) **senza ridondanza geografica**. Il 99,5% riflette questa architettura: percentuali superiori (es. 99,9%, riferimento API Infomaniak) non sono promesse finché non sia implementata ridondanza/DR.

### 2. Misurazione

- **Uptime mensile** = (minuti totali del mese − minuti di downtime imputabile) / minuti totali del mese, calcolato a fine mese sul fuso CEST.
- **Downtime** = indisponibilità continua o ripetuta di uno dei servizi in §1 accertata dal monitoring del Fornitore o segnalata dal Cliente e verificata. [SISTEMA DI MONITORING — DA DEFINIRE]
- Report mensile di disponibilità su richiesta del Cliente.

### 3. Esclusioni

Non concorrono al calcolo dell'uptime, né danno diritto a crediti:

1. **manutenzione programmata** comunicata con **≥ 48h di preavviso**, max **4h/mese**, in finestra preferenziale 02:00–06:00 CEST;
2. **forza maggiore** (eventi non prevedibili e insuperabili);
3. **guasti/failover del provider infrastrutturale** (Hetzner) e dei suoi servizi (network, storage box, alimentazione);
4. **interruzioni di servizi di terzi** non controllati dal Fornitore: connettori/registrar (Namecheap API), DNS del Cliente gestito esternamente, provider LLM, blacklist pubbliche esterne;
5. **blacklisting dell'IP causata da contenuti o pratiche del Cliente** o di un suo utente (violazione AUP);
6. sospensione del servizio per **violazione AUP o mancato pagamento** (§6 Termini);
7. **problemi lato Cliente**: dispositivi, connettività del Cliente, configurazioni errate impostate dal Cliente, credenziali non funzionanti per cause del Cliente;
8. servizi **fuori perimetro** (es. consegna in posta in arrivo presso terzi — disclaimer deliverability nei Termini §11.2).

### 4. Crediti di servizio

Se l'uptime mensile scende sotto il 99,5%, il Cliente ha diritto a un **credito** sul canone del mese successivo, da richiedere entro [30 giorni] dal report mensile:

| Uptime mensile | Credito sul canone mensile |
|---|---|
| < 99,5% – ≥ 99,0% | 10% |
| < 99,0% – ≥ 98,0% | 25% |
| < 98,0% – ≥ 95,0% | 50% |
| < 95,0% | 100% |

Il credito è l'**unico rimedio** per il mancato raggiungimento dell'uptime (salvo dolo/colpa grave), non copre danni indiretti, ed è null'oltre il canone del mese. In caso di sospensione per violazione del Cliente non sono dovuti crediti.

### 5. Tempi di risposta (supporto)

| Severità | Definizione | Primo tempo di risposta |
|---|---|---|
| S1 — Critico | Servizio di posta/webmail irraggiungibile o perdita di dati | **1 ora** (h24) |
| S2 — Alto | Degradazione significativa (lentezza, delivery parziale, quota) | **4 ore** (h24) |
| S3 — Normale | Richieste standard, configurazioni, domande | **1 giorno lavorativo** |
| S4 — Basso | Richieste informative, documentazione | **3 giorni lavorativi** |

Tempi di risposta (non di risoluzione); orario [orari supporto — DA COMPLETARE]. **Right-to-suspend**: violazione AUP → sospensione immediata come da Termini §6.

### 6. Variazioni

Modifiche allo SLA con [30 giorni] di preavviso come da Termini §14. I crediti maturati prima della modifica restano acquisiti.

---

## English version (convenience translation)

### 1. Availability commitment

The Provider commits to a **monthly uptime of 99.5%** for **mail (SMTP: 25/465/587, IMAP: 143/993)** and **webmail (HTTPS)**. Over a 30-day month, 99.5% equals at most **~216 minutes (3h 36m)** of attributable unavailability.

⚠️ **Nature of the commitment:** the service runs on a **single instance (single-tenant, single-instance)** on Hetzner (DE) **without geographic redundancy**. 99.5% reflects this architecture: higher figures (e.g. 99.9%, the Infomaniak API reference) are not promised until redundancy/DR is implemented.

### 2. Measurement

- **Monthly uptime** = (total minutes in month − attributable downtime minutes) / total minutes in month, computed at month-end on CEST.
- **Downtime** = continuous or repeated unavailability of any service in §1 ascertained by the Provider's monitoring or reported by the Customer and verified. [MONITORING SYSTEM — TO BE DEFINED]
- Monthly availability report on the Customer's request.

### 3. Exclusions

The following do not count against uptime and grant no credits:

1. **scheduled maintenance** notified with **≥ 48h notice**, max **4h/month**, in the preferred window 02:00–06:00 CEST;
2. **force majeure**;
3. **failures/failover of the infrastructure provider** (Hetzner) and its services (network, storage box, power);
4. **outages of third-party services** not controlled by the Provider: connectors/registrar (Namecheap API), Customer DNS managed externally, LLM providers, external public blacklists;
5. **IP blacklisting caused by the Customer's content or practices** or those of their users (AUP violation);
6. suspension for **AUP violation or non-payment** (§6 of the Terms);
7. **customer-side issues**: devices, the Customer's connectivity, misconfigurations set by the Customer, credentials failing for Customer-side causes;
8. services **out of scope** (e.g. inbox placement at third parties — deliverability disclaimer in the Terms §11.2).

### 4. Service credits

If monthly uptime falls below 99.5%, the Customer is entitled to a **credit** on the next month's fee, to be claimed within [30 days] of the monthly report:

| Monthly uptime | Credit on the monthly fee |
|---|---|
| < 99.5% – ≥ 99.0% | 10% |
| < 99.0% – ≥ 98.0% | 25% |
| < 98.0% – ≥ 95.0% | 50% |
| < 95.0% | 100% |

The credit is the **sole remedy** for missed uptime (except intent/gross negligence), does not cover indirect damages, and never exceeds the monthly fee. No credits are due for suspension caused by the Customer's violation.

### 5. Response times (support)

| Severity | Definition | First response time |
|---|---|---|
| S1 — Critical | Mail/webmail unreachable or data loss | **1 hour** (24/7) |
| S2 — High | Significant degradation (slowness, partial delivery, quota) | **4 hours** (24/7) |
| S3 — Normal | Standard requests, configuration, questions | **1 business day** |
| S4 — Low | Informational requests, documentation | **3 business days** |

Response (not resolution) times; support hours [SUPPORT HOURS — TO BE COMPLETED]. **Right-to-suspend**: AUP violation → immediate suspension per the Terms §6.

### 6. Amendments

SLA changes with [30 days'] notice per the Terms §14. Credits accrued before the change remain earned.
