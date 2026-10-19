# Termini di Servizio — multi.ai.inbox (servizio mail hosting)

**Versione:** 1.0-draft · **Data:** 2026-10-19 · **Data di efficacia:** [DA FISSARE AL LANCIO] · **Lingua autentica:** italiano (traduzione inglese di cortesia: `tos-en.md`; in caso di divergenza prevale la versione italiana)

> ⚠️ **BOZZA INTERNA — NON PUBBLICARE TAL QUALE.** Documento redatto come parte del pacchetto legale del progetto multi.ai.inbox (review legale 2026-10-19, finding P0-1). Da passare a un legale abilitato (giurisdizione IT/CH) **prima del lancio commerciale e prima del primo cliente pagante**. I campi tra parentesi quadre `[...]` sono da completare prima della pubblicazione.

---

## 1. Soggetti, imprint e contatti

1.1 **Fornitore del servizio:** [FORMA GIURIDICA E DENOMINAZIONE COMPLETA — DA COMPLETARE], con sede legale in [INDIRIZZO SEDE — DA COMPLETARE], C.F./P.IVA [DA COMPLETARE], REA/Registro imprese [DA COMPLETARE] (il "**Fornitore**").

1.2 **Cliente:** la persona fisica o giuridica che si iscrive al servizio (il "**Cliente**" o, per il modello self-service, il "**Tenant**").

1.3 **Contatti:** supporto: [SUPPORT EMAIL — DA COMPLETARE] · questioni legali: [LEGAL EMAIL — DA COMPLETARE] · privacy: [PRIVACY EMAIL — DA COMPLETARE].

1.4 **Nota sul software e sul marchio.** Il servizio è erogato su **multi.ai.inbox, fork di mailcow-dockerized**, rilasciato in licenza GNU GPL-3.0. "mailcow" è un marchio registrato di The Infrastructure Company GmbH; multi.ai.inbox **non è affiliato a, sponsorizzato o approvato da** mailcow o The Infrastructure Company GmbH. I componenti software upstream (postfix, dovecot, SOGo, nginx, rspamd, ClamAV e altri) restano di rispettivi autori.

---

## 2. Oggetto del servizio

2.1 Il Fornitore fornisce in **self-service** (iscrizione e fatturazione per tenant tramite la piattaforma):

- **caselle email** con accesso SMTP/IMAP e **webmail SOGo**;
- **pannello di amministrazione** web (interfaccia standard del fork, basata su mailcow-dockerized);
- **gestione DNS dei domini** tramite connettore verso il registrar dell'account Cliente (attualmente connettore Namecheap; connettore Infomaniak pianificato), quando il Cliente collega la propria API key;
- **API / MCP** per l'automazione (uso soggetto a autenticazione e ai limiti di questo contratto);
- **antispam e antivirus** lato server (rspamd, ClamAV), **TLS** con certificati Let's Encrypt, gestione **SPF, DKIM, DMARC, MTA-STS** per i domini ospitati;
- **backup** (§9).

2.2 **Infrastruttura.** Il servizio è erogato su **istanza singola (single-tenant, single-instance)** ospitata presso **Hetzner Online GmbH (Germania, UE)**, senza ridondanza geografica. Questa caratteristica è riflessa nello SLA (§4) ed è dichiarata in modo espresso.

2.3 **Non incluso.** [DA COMPLETARE AL LANCIO — es.: migrazione di caselle esistenti di massa, consulenza deliverability dedicata, branding dell'interfaccia per singolo tenant (oggi l'interfaccia admin è standard e non personalizzabile per tenant), SLA superiore al 99,5%.] Ciò che non è espressamente incluso non è dovuto.

2.4 **Requisiti.** Per usare il connettore registrar il Cliente deve disporre di un proprio account presso il registrar supportato e di una **API key propria** che soddisfi i requisiti del registrar (per Namecheap: soglie di accesso API in produzione — tipicamente ≥ 20 domini registrati o ≥ 50 USD di saldo/spesa, whitelist **IPv4** dell'IP del server Fornitore, limiti di rate 50/min · 700/h · 8.000/giorno). Il Fornitore documenta questi requisiti nell'onboarding.

---

## 3. Iscrizione, durata, rinnovo, prezzi

3.1 L'iscrizione è self-service tramite la piattaforma; il contratto si perfeziona all'attivazione del primo servizio pago. [DA COMPLETARE: verifica identità/età minima, accettazione esplicita dei Termini.]

3.2 **Durata e rinnovo:** [DA COMPLETARE AL LANCIO — es.: contratto mensile/annuale con rinnovo tacito; disdetta con preavviso di 30 giorni.]"

3.3 **Prezzi e fatturazione:** secondo il listino pubblicato al momento dell'ordine [LISTINO — DA COMPLETARE]. I prezzi sono al netto di IVA/tasse applicabili; fatturazione [DA COMPLETARE: periodicità, metodi di pagamento]. I pagamenti di dominio/registrar restano sul rapporto diretto Cliente–registrar: il Fornitore non rivende domini e non incassa canoni registrar per conto terzi.

3.4 **Mancato pagamento:** sollecito entro [X giorni], sospensione del servizio (§6) dopo [Y giorni], cessazione dopo [Z giorni].

---

## 4. Livello di servizio (SLA)

4.1 Il Fornitore si impegna a un'**uptime mensile del 99,5%** per i servizi di posta (SMTP, IMAP) e webmail, **misurato** e corredato da crediti secondo lo **SLA allegato** (documento `SLA`), parte integrante e sostanziale del presente contratto.

4.2 **Natura dell'impegno.** Il 99,5% riflette l'erogazione su **istanza singola senza ridondanza geografica** (§2.2): non sono promessi 99,9% o superiori finché non sia implementata ridondanza. Lo SLA definisce misurazione, esclusioni, crediti e tempi di risposta.

---

## 5. Politica di uso accettabile (AUP)

5.1 **Divieti.** È vietato usare il servizio per: invio di **posta commerciale non sollecitata (UCE/UBE, "spam")** o **mail bombing**; **phishing** o Furto d'identità; distribuzione di **malware**; contenuti illegali ai sensi del diritto italiano ed europeo (anche diritto d'autore, od incitato all'odio, minori); violazione di diritti di terzi; **reverse engineering di API** o circumvention delle protezioni; **overload** deliberato di sistemi e API propri o di terzi; raccolta indirizzi per spam (harvesting). L'AUP è allineato alle politiche del registrar (Namecheap Universal ToS §7 e Hosting AUP §9) per non esporre a risoluzioni a catena.

5.2 **Opt-in documentato.** Per l'invio di newsletter/comunicazioni di massa il Cliente deve garantire consensi **opt-in** documentabili e conformità al Codice Privacy italiano, al GDPR e, se applicabile, al CAN-SPAM Act. Su richiesta il Fornitore può chiedere prova dei consensi.

5.3 **Consegne del reato/infrazione.** Lo spam o l'abuso di **un solo tenant** sull'IP condiviso del server può causare la **blacklisting dell'IP** con distruzione della deliverability di **tutti** i domini ospitati e la risoluzione dell'account registrar. Ne derivano le seguenti regole:

- **sospensione immediata** delle caselle responsabili (§6), anche senza preavviso in caso di spam attivo in corso;
- **addebito pass-through** al Cliente responsabile delle **penalità di terzi** risultanti, tra cui la **penale fino a USD 500** che il registrar (Namecheap, Hosting AUP §9) addebita sul caso di spam/blacklisting, oltre ai costi di delisting, monitoraggio e mitigazione documentati;
- il Fornitore segnala alle autorità competenti gli usi manifestamente illeciti.

5.4 **Monitoraggio.** Il Fornitore monitora il traffico in uscita (antispam rspamd) e i rate di invio; limiti di invio per casella/ora possono essere imposti per proteggere la reputazione condivisa [VALORE — DA COMPLETARE].

---

## 6. Sospensione

6.1 Il Fornitore può **sospendere** il servizio, in tutto o in parte, per: violazione dell'AUP (§5); **mancato pagamento**; minaccia concreta alla **sicurezza** dell'istanza o di terzi (compromissione, relaying aperto, botnet, blacklisting in corso); richiesta vincolante di autorità o registrar; manutenzione straordinaria urgente.

6.2 **Tempi ed effetti.** La sospensione per violazione AUP può essere **immediata e senza preavviso** se l'abuso è in corso. Il Fornitore notifica la sospensione [entro 24 ore] con la motivazione. La riattivazione avviene dopo rimozione della causa e, per i casi di pagamento, previo saldo; il Fornitore può richiedere garanzie ragionevoli di non reiterazione.

6.3 I giorni di sospensione **non** danno diritto a riduzione della fee se la sospensione deriva da violazione del Cliente; danno diritto a credito SLA se imputabile al Fornitore (salvo esclusioni di SLA).

---

## 7. Credenziali, API key del Cliente e connettori registrar

7.1 Le **API key del Cliente** (registrar e altri connettori) sono custodite lato server **cifrate (AES-256-GCM)**, sono usate **solo per le operazioni concordate** (lettura/gestione DNS e dominio dei domini del Cliente) e **non compaiono in log, pagine o repository**. Scope: raccomandata key di sola lettura dove il flusso lo consenta; revoca immediata da parte del Fornitore al disconnect e alla cessazione, e obbligo del Cliente di revocarla direttamente presso il registrar alla cessazione.

7.2 Il Cliente è responsabile della correttezza e dell'ambito della key fornita e **notifica immediatamente** al Fornitore ogni compromissione presunta. Il Fornitore notifica al Cliente ogni accesso anomalo rilevato sulle credenziali custodite.

7.3 **Proprietà del dominio.** I domini restano **registrati nell'account registrar del Cliente**: il Fornitore agisce come strumento di gestione via API, **non** come registrar reseller, non assume obblighi ICANN/RAA e non ha titolo per trattenere o trasferire i domini. In nessun caso il Fornitore sposta domini in account propri senza mandato scritto.

---

## 8. Agenti AI

8.1 Il servizio può prevedere l'uso di **agenti AI** per triage, estrazione di attività e proposte di bozze sulle caselle. Funzione soggetta a attivazione e a governance: gli agenti operano con chiavi per-agente auditabili; le azioni con effetti verso terzi richiedono **revisione umana** (human-in-the-loop).

8.2 **Nessuna decisione esclusivamente automatizzata** ai sensi dell'Art. 22 GDPR (effetti giuridici o altrettanto significativi: sospensione/cancellazione account, risposte vincolanti) è eseguita dagli agenti senza intervento umano. I contenuti delle caselle **non sono usati per addestrare modelli** senza base giuridica specifica e consenso del Cliente.

8.3 Per l'attivazione della funzione si applicano la Informativa Privacy (§11) e, per i tenant, il DPA allegato.

---

## 9. Backup e retention

9.1 Il Fornitore esegue **backup cifrati** delle caselle e della configurazione su volume dedicato, con **rotazione [30 giorni — pianificata]**. Gli obiettivi **RPO/RTO contrattuali sono dichiarati solo dopo l'esito del restore drill**: fino a quel momento il backup è impegno di mezzo, non di risultato. [RPO/RTO — DA FISSARE DOPO IL DRILL.]

9.2 **Retention dei dati del servizio:** caselle (maildir) finché attive; backup fino a rotazione; log di sistema e di audit [90 giorni — pianificato]; dati di fatturazione secondo obblighi fiscali [10 anni, normativa italiana]. Alla cessazione: export e cancellazione (§12).

---

## 10. Proprietà e trattamento dei dati del Cliente

10.1 **Proprietà.** Mail, contatti, allegati e dati del brand appartengono al **Cliente**. Il Fornitore li tratta esclusivamente per erogare il servizio e secondo le istruzioni documentate del Cliente; per le caselle dei tenant il Fornitore è **responsabile del trattamento** ex Art. 28 GDPR e il Cliente **titolare** (DPA allegato), fermo restando che per i dati di contatto e fatturazione del Cliente il Fornitore è titolare autonomo.

10.2 **Accesso al contenuto delle caselle** da parte del personale Fornitore avviene **solo su richiesta autorizzata del Cliente** (supporto, incidenti, richieste di diritti) e documentato in audit log. Nessun uso dei dati dei clienti — **nemmeno analytics aggregati** — senza consenso scritto.

10.3 Il trattamento dei dati personali è disciplinato dalla **Informativa Privacy** (documento `Privacy Policy`) e, per i tenant, dal **DPA** (documento `DPA`), parte integrante del presente contratto.

---

## 11. Software, garanzia e deliverability

11.1 **Software upstream senza garanzia.** I componenti software di base (mailcow-dockerized e i suoi componenti) sono forniti "**AS IS**" senza garanzia di alcun tipo ai sensi della GPLv3 §15. Ciò **non riduce** gli obblighi di servizio: la garanzia copre il **servizio gestito** erogato dal Fornitore (presente contratto + SLA), non il codice upstream in sé.

11.2 **Disclaimer deliverability.** L'IP del server è un IP datacenter: la consegna della posta dipende anche dalla reputazione dell'IP presso i destinatari, da policy dei riceventi, da blacklisting esterni e da pratiche di **warmup** del dominio. Il Fornitore gestisce SPF/DKIM/DMARC/MTA-STS e monitora la reputazione, ma **non garantisce l'ingresso in posta in arrivo** presso terzi; i blacklisting causati da contenuti o pratiche del Cliente sono esclusi dallo SLA e imputati al Cliente (§5.3).

---

## 12. Cessazione ed exit

12.1 In caso di cessazione (scadenza, disdetta, recesso o risoluzione), il Cliente ha diritto a: **export completo** delle caselle (formato standard Maildir/EML e/o IMAP migration), esportazione di domini/configurazioni, **assistenza alla migrazione** entro [30 giorni dalla cessazione]; trascorso tale termine i dati sono cancellati, con residuo nei backup fino a rotazione naturale (dichiarata nella Informativa Privacy).

12.2 Il Fornitore revoca le credenziali/API key custodite e assiste la rimozione/rielaborazione dei record DNS concordata. Il **dominio resta nell'account registrar del Cliente** (§7.3): nessun lock-in. Nessuna penalità di uscita oltre i canoni dovuti.

12.3 Il Fornitore può recedere con preavviso di [30 giorni] per giustificato motivo (cessazione attività, perdita requisiti infrastrutturali), garantendo export e assistenza migrazione pari a §12.1.

---

## 13. Limitazione di responsabilità

13.1 Salvo dolo o colpa grave, responsabilità per morte/lesioni personali, e ipotesi di responsabilità non escludibile per legge, la responsabilità complessiva del Fornitore è **limitata alle fee pagate dal Cliente negli ultimi 12 mesi**; sono **esclusi** i danni indiretti, la perdita di profitto, di dati imputabili al Cliente, di opportunità commerciali e di immagine.

13.2 **Sanzioni GDPR:** le sanzioni amministrative inflitte a una parte per la propria violazione restano a suo carico e non sono trasferibili né soggette al cap (§13.1); il cap non limita gli obblighi di risarcimento verso interessati nei limiti di legge. Il Fornitore sottoscriverà una **RC professionale** [DA ATTIVARE — DA COMPLETARE CON DATI POLIZZA].

13.3 **Casi fortuiti, forza maggiore** ( blacklisting esterna non imputabile, guasti del provider infrastrutturale Hetzner, interruzioni dei connettori/registrar terzi, attacchi DDoS di scala eccezionale) **escludono** la responsabilità del Fornitore per il periodo dell'evento e sono esclusi dallo SLA.

---

## 14. Modifiche ai Termini

14.1 Le modifiche sono notificate con [30 giorni] di preavviso via email/dashboard. Se il Cliente non accetta, può recedere con effetto immediato entro il preavviso; il silenzio oltre il termine vale accettazione. Le modifiche non retroagiscono sui contratti in corso.

---

## 15. Diritto applicabile e foro

15.1 Il presente contratto è disciplinato dalla **legge italiana**. Per le controversie è competente il foro di [SEDE LEGALE FORNITORE — DA COMPLETARE], salvo foro obbligatorio del consumatore ove applicabile.

15.2 **Versioni e prevalenza.** La versione italiana è quella autentica; la versione inglese è di cortesia. In caso di conflitto tra Termini, SLA, DPA e Informativa Privacy prevale: DPA (per il trattamento dati) → SLA (per misurazione e crediti) → Termini.

---

## Allegati (parte integrante)

- **SLA** (`sla.md`) — uptime 99,5% mensile, misurazione, esclusioni, crediti, response time.
- **DPA Art. 28 GDPR** (`dpa-template.md`) — per i tenant, trattamento dati personali delle caselle.
- **Informativa Privacy** (`privacy-it.md`) — titolare, basi giuridiche, destinatari, trasferimenti, retention, cookie, diritti.
- **AUP** — §5 del presente documento (vietato duplicare politiche divergenti).
