# Accessibility audit — WCAG 2.1 AA

Audit del sign-in modal + dashboard founder + notifiche (2026-10-22). Metodo:
analisi statica di HTML/CSS/JS + verifiche live; il contrast cromatico non è
misurabile senza rendering — segnato come da verificare col renderer.

## Esito: 13 punti coperti, 4 gap (0 critici, 2 medi, 2 bassi)

### Coperti (verificati)

| # | Punto | Dove | Criterio |
|---|-------|------|----------|
| 1 | Skip link presente su **tutte le 12 pagine** | `index.html:20` + 11 | 2.4.1 |
| 2 | Modal = `<dialog>` **nativo** → focus trap, ESC close, focus return e `role=dialog` gratuiti | `index.html:181`, `tour-webmail:291` | 2.1.2, 4.1.2 |
| 3 | Modal `aria-labelledby="login-modal-title"` | `index.html:181` | 4.1.2 |
| 4 | Tabs ARIA completi: `role=tablist` + `aria-label="Sign-in method"`, `role=tab` + `aria-selected` + `aria-controls`, `role=tabpanel` + `aria-labelledby` + `tabindex=0` | `index.html:201-221` | 4.1.2 |
| 5 | Arrow keys sui tab con `focus()` (pattern WAI-ARIA tabs) | `app.js:48-53` | 2.1.1 |
| 6 | Password toggle: `aria-pressed` + `aria-label` dinamico ("Show/Hide password") + SVG `aria-hidden` | `app.js:104-115` | 4.1.2 |
| 7 | Errori con `role="alert"` (`reg-error`) — annunciati dagli screen reader | `index.html:281` | 4.1.3 |
| 8 | Label associate a **tutti** i campi (`login_user`, `pass_user`, `reg-first/last/email/country/pass/pass2`) | `index.html:208-277` | 1.3.1, 3.3.2 |
| 9 | `autocomplete` completi: `given-name`, `family-name`, `email`, `username`, `new-password`×2, `current-password` | `index.html` | 1.3.5 |
| 10 | Campanella notifiche: `aria-haspopup`, `aria-expanded`, `aria-controls`, `aria-label="Notifications"`, badge anonunciato | `notifications.js:25-45` | 4.1.2 |
| 11 | `prefers-reduced-motion: reduce` su 2 blocchi — orbit-spin e animazioni disattivate | `style.css:168,236` | 2.3.3 |
| 12 | Touch target ≥44px: `oauth-btn`, `pass-toggle` (44×44 + padding input), `notif-bell` 44×44 | `style.css` ≤560px, `notifications.css` | 2.5.8 |
| 13 | Landmark + `lang="en"` su tutte le pagine, un solo `<h1>` per pagina | tutte le 12 | 1.3.1 |

### Gap

1. **MEDIA — Contrast AA non misurato** (1.4.3). I testi `.hint`, `.msg.error`,
   badge e muted su background non sono verificabili in analisi statica.
   Fix: pass col renderer (axe-core o Lighthouse) e portare tutto a ≥4.5:1
   (testo normale) / ≥3:1 (large text e bordi UI).

2. **MEDIA — `aria-disabled` su link non disabilitato davvero** (4.1.2).
   `#open-mailcow` e `#mailcow-link` (inbox, onboarding) hanno
   `aria-disabled="true"` da markup, ma restano `<a>` cliccabili finché JS non
   rimuove l'attributo. Fix consigliato: rimuovere `href` da markup (come fa
   già `onboarding.js` per l'origin mancante) così il link non è attivabile, e
   usare `aria-disabled` solo come announcement.

3. **BASSO — Il badge notifiche non ha announcement proprio** (4.1.3). Il
   `#notif-badge` è visibile ma il conteggio non viene annunciato quando arriva.
   Fix: `aria-label` dinamico sulla bell ("Notifications, 3 unread") quando il
   badge si popola.

4. **BASSO — Heading hierarchy nelle sezioni wizard** (1.3.1). I titoli step
   (`step-1-title`…) sono `<h2>` con `tabindex="-1"` per il focus — corretto —
   ma la pagina onboarding non ha un `<h1>` visibile prima del wizard (solo
   hero). Verificare che ogni pagina abbia esattamente un `<h1>` coerente con
   l'ordine di lettura.

## Note

- Il modal è un `<dialog>` nativo: focus trap, ESC, inert background e focus
  return sono garantiti dal browser, nessuna gestione JS manuale necessaria.
- Le 12 pagine sono coerenti (stesso header, skip link, lang, meta).
- Verifica live raccomandata dopo i fix: axe-core su `/`, `onboarding.html`,
  `dashboard.html` a 390px e 1280px.
