# Acceptance report, multi.ai.inbox console

Ultima esecuzione: 24 ottobre 2026, contro staging, con una chiave
dell'organizzazione di staging, quindi con tutti gli spec attivi.

| Item | Value |
| --- | --- |
| Site under test | https://162-55-213-24.sslip.io |
| Console entry | /console/ |
| Suite | tests/acceptance, Playwright 1.56.1 con axe 4.10.2 |
| Esito | 26 spec: **24 passati, 0 falliti, 2 saltati** |
| Saltati, con motivo | accesso umano disattivato (`MAI_AUTH_SECRET` assente) e provider OAuth non configurati |

Comando:

```
cd tests/acceptance
MAI_E2E_API_KEY=<chiave dell'organizzazione> MAI_E2E_BASE=https://162-55-213-24.sslip.io npx playwright test
```

## Difetti trovati in questa passata e chiusi

| Difetto | Gravita | Stato |
| --- | --- | --- |
| Il form del magic link puntava a `api.<host>` e la CSP `form-action 'self'` lo rifiutava: il pulsante non faceva nulla | bloccante | corretto: form e link OAuth sull'origine della pagina |
| Anche sull'origine giusta il browser rifiutava, perche `MAI_WEB_ORIGIN` era `https://aimulti.inbox` e il 303 portava su un'altra origine | bloccante | corretto: il valore e per ambiente, staging torna al suo host |
| La risposta del reply veniva interpretata come un Ticket: ogni risposta riuscita mostrava "unexpected shape" e la timeline non si aggiornava | alto | corretto: si interpreta `SendResult` |
| Il secondo reply non lasciava traccia nella timeline | alto | corretto: ogni reply scrive un evento |
| La lista thread virtualizzata imposta le righe con attributi `style`, rifiutati da `style-src` | medio | corretto: `style-src-attr 'unsafe-inline'`, blocchi `<style>` ancora vietati |
| I campioni di codice scrollabili non erano raggiungibili da tastiera (axe serious) | medio | corretto: `tabIndex` |
| Due pannelli con lo stesso nome accessibile, e un pulsante "Take it" che non assegnava nulla | basso | corretti: nome distinto e etichetta "Open" |

Il report originale della prima esecuzione (senza chiave, 6 passati, 5 falliti,
15 saltati) resta nella storia del branch; i file dei difetti con screenshot e
tracce sono in `tests/acceptance/test-results/defects/`.

## Cosa copre la suite

Gate senza sessione e con chiave rifiutata, creazione inbox e toggle intake,
lettore a tre pannelli con sanificazione (`<script>`, immagini remote bloccate,
allegati), composer con round trip nella inbox di destinazione, coda ticket con
filtri, contatori, cambio stato, priorita, nota, risposta e timeline, chiavi,
webhook, domini, audit, coda di revisione, onboarding con reload a meta,
manifest e service worker, offline con rete tagliata da un proxy locale, axe su
ogni pagina e layout a 390 px.

## What the first run proved


The 6 passing specs cover the gate without a session, a rejected key, the
onboarding funnel end to end, the web app manifest with every icon, the service
worker registration, and the offline behaviour of the console with the network
cut. The 4 defects below are proven with a failing assertion, a screenshot and
the console or network evidence captured at the moment of failure.

The run wrote 9 finding files for those 4 defects: defect 1 caught four
assertions (wrong form action, refused submission, nothing visible, a console
error) and defect 2 caught two, one per provider, because each check fails
independently. The findings are grouped here by root cause.

