# Product review: GUI, mobile, onboarding

Data: 2026-10-23. Skill usate: `ui-ux-pro-max`, `mobile-design`, `onboarding-cro`, `signup-flow-cro`.
Metodo: audit del repo (`web/src/app/console/*`, `web/src/app/*`, `internal/api`, `internal/mcpserver`).

## Stato reale (verificato nel codice)

| Componente | Stato | Evidenza |
|---|---|---|
| Console gestionale | **Completa** | `console/`: audit, compose, domains, inbox, keys, review, webhooks |
| Lettura email da umano | **STUB da 15 righe** | `console/inbox/page.tsx` = solo `metadata.title` |
| Auth utente | **Assente** | solo `Authorization: Bearer <api key>` su `/v0/*` |
| Signup / login / OAuth | **Assente** | nessun `signup`, `login`, `oauth` nel backend |
| API per app mobile | **Parziale** | REST `/v0/*` + MCP tools, nessun client SDK |
| Landing → docs | **Link diretto** | "Quickstart" porta a `/docs`, non a un funnel |

---

## 1. GUI per leggere le email da umano

**Il problema è reale e bloccante**: oggi un umano non può leggere una mailbox. Il
`console/inbox` è uno stub; senza GUI il prodotto è solo per agenti, e tu come
titolare non puoi né verificare né assistere un cliente.

### Opzione A — Webmail completo in-house (consigliata)

Costruire `console/inbox` come client email 3-pane (lista thread | messaggio | dettagli):

- **Lista**: thread con mittente, oggetto, snippet, timestamp, non-letti. Virtualizzata
  (`FlatList`-equivalente web: `react-virtuoso`), non renderizzare 500 righe.
- **Lettura**: HTML sanificato (`DOMPurify`, già mitigato dalla CSP `'self'`), immagini
  remote bloccate di default con "Mostra immagini".
- **Azioni**: rispondi, inoltra, archivia, etichetta, elimina. Compose già in embrione
  (`console/compose`).
- **Allegati**: download streaming con token a scadenza, mai URL diretti al blob.
- **Cerca**: full-text su oggetto+mittente+corpo, paginata.

Costo: ~2-3 settimane di lavoro agente. È il pezzo con più alto impatto percepito:
senza webmail il cliente umano non compra.

### Opzione B — Delegare a SOGo / Roundcube

Montare SOGo (server-side, IMAP su postgres) accanto al gateway e lasciare la webmail
a lui. Meno lavoro (1 settimana), ma: seconda UI da mantenere, stack pesante (SOGo =
Java/Tomcat), branding da rifare, e due sistemi di permessi da tenere allineati.

**Consiglio**: A. Sei già padrone di thread/message/attachments nell'API; il webmail è
solo un client su quell'API. La B ti dà una webmail completa ma fuori controllo.

---

## 2. App mobile (iOS + Android)

**Non partire da due app native.** L'87% del valore di un client email sta in
lista + lettura + risposta; il resto è notifica push.

### Fase 1 — PWA installabile (2 settimane)

- `manifest.json` + service worker + `display: standalone`: la webmail diventa
  installabile da Safari/Chrome senza store.
- **Offline**: cache dell'ultimo thread aperto, coda di invio con retry.
- **Deep link**: `aimulti.inbox/console/inbox?id=<thread>` per link condivisi.
- **Push web** (iOS 16.4+ supporta le web push via PWA): notifica "nuovo messaggio".
- Costo: €0 di store, zero review Apple, aggiornamenti immediati.

### Fase 2 — Wrapper nativo quando serve (React Native + Expo)

Solo se la PWA non basta (push affidabili su iOS, biometria, condivisione file):

- **Expo** (OTA updates, un codebase, web team già su React/Next).
- **Mobile API da preparare ora**: versionare `/v1/*` per non rompere le app quando
  cambi l'API. Aggiungere: `GET /v1/threads?cursor=`, `POST /v1/messages` idempotente,
  token a breve scadenza + refresh, `GET /v1/me`.
- **Sicurezza mobile** (dalla skill `mobile-design`): token in `SecureStore`/
  Keychain, mai `AsyncStorage`; SSL pinning in produzione; deep link per le push.

**Consiglio**: PWA ora, React Native in Fase 2. Non due codebase native prima di
avere clienti che le chiedono.

---

## 3. Onboarding funnel (il vero buco)

Oggi: "Quickstart" → `/docs`. È un vicolo cieco: un visitatore che vuole provare
finisce in documentazione tecnica. **Il funnel manca del tutto.**

### Flusso proposto (dalla skill `signup-flow-cro`)

```
Landing → "Start free" → Auth (email magic link | Google | GitHub)
        → Onboarding wizard 3 step → Prima inbox → Aha moment
```

**Step 1 — Auth, 3 opzioni, nell'ordine giusto per il pubblico:**
- **Google** (primario per B2B AI): un click, nessuna password
- **GitHub** (per chi costruisce agenti: il pubblico naturale)
- **Email magic link** (no password: meno frizione, meno supporto password reset)
- Niente Apple in web (solo se fai l'app iOS), niente password classica al lancio.

**Step 2 — Onboarding wizard (3 step, non 7):**
1. "Come si chiama la tua organizzazione?" (1 campo)
2. "Che dominio vuoi usare?" → campo dominio + "oppure usa un indirizzo di test gratis"
   (la scelta "test gratis" evita il blocco DNS e porta subito al valore)
3. "Invita il tuo team?" (skip-able)

Poi atterra **dentro** la console con la prima inbox creata e un messaggio di
benvenuto già in cassetta: il valore si vede prima di configurare.

**Step 3 — Checklist persistente (dalla skill `onboarding-cro`):**
```
☑ Crea la tua prima inbox            (fatto)
☐ Verifica il dominio                 (10 min)
☐ Invia la prima email                (2 min)
☐ Collega un agente via MCP/API       (5 min)
```
Progress bar, ogni voce apre il contesto giusto, dismissibile. La voce "collega un
agente" è il tuo differenziale: va resa un quick win, non un capitolo di docs.

### Auth tecnica (backend)

Serve un layer di identità sopra l'attuale API key:
- `users` (email, provider, provider_id) + `sessions` (JWT breve + refresh)
- l'API key resta per gli **agenti**; l'umano usa la sessione
- mapping: utente → organizzazione → ruoli (`owner`, `admin`, `member`)
- magic link: token monouso a 15 min via email (riusa il relay Brevo)
- OAuth: Google/GitHub con PKCE

Costo: ~2 settimane. Prerequisito di tutto il resto.

---

## Priorità (RICE grezzo)

| # | Cosa | Impatto | Sforzo | Quando |
|---|---|---|---|---|
| 1 | **Auth + signup** (Google/GitHub/magic link) | Bloccante: senza non vendi | 2 sett | Subito |
| 2 | **Webmail** (`console/inbox` 3-pane) | Bloccante: senza non usi | 2-3 sett | Subito dopo auth |
| 3 | **Onboarding wizard + checklist** | Alto: attivazione | 1 sett | Con il webmail |
| 4 | **PWA** (installabile + push) | Medio: mobile senza store | 2 sett | Dopo il webmail |
| 5 | **`/v1` versionata + SDK** | Abilitante per app native | 1 sett | Con la PWA |
| 6 | **React Native** | Basso finché non lo chiedono | 4-6 sett | Dopo i primi clienti |

## Ordine consigliato

**Auth → Webmail → Onboarding** in un unico blocco da ~5 settimane: sono
interdipendenti (l'onboarding atterra nel webmail, il webmail richiede la sessione).
Poi PWA. La mobile nativa solo su richiesta esplicita di clienti paganti.

## Cosa NON fare

- Non montare SOGo "per avere subito qualcosa": doppio stack, doppia UI, doppia auth.
- Non partire da due app native: costo 3x, store review, e il 90% del valore sta in PWA.
- Non mettere password classiche al lancio: magic link + OAuth riducono supporto e rischio.
- Non chiedere il dominio obbligatorio all'onboarding: "indirizzo di test gratis" porta
  all'aha moment senza DNS.
