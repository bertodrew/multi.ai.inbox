# 04 — Design Brief — AImulti.inbox

**Data**: 21/10/2026 · **Fonte**: landing.css (435 righe), style.css, theme.css, index.html (markup reale), audit UX `ux_audit_5progetti_2026-10-21.json → projects.multi_ai`.

---

## 1. Principio visivo

Il landing è la vetrina self-serve della piattaforma: deve comunicare **multi-brand + agent-native** in pochi secondi e restare leggibile senza JS e senza dipendenze esterne. Le decisioni prese nel codice reale:

- **Zero dipendenze esterne**: 3 fogli di stile locali (style.css base, theme.css tema, landing.css specifico del landing), nessun font/CDN esterno — imposto dalla CSP `default-src 'self'`
- **Un solo file CSS specifico**: landing.css (435 righe) estende il tema condiviso senza duplicarlo
- **Formato**: HTML statico + vanilla JS (nessun framework, `script-src 'self'`)

## 2. Sistema tipografico e colore (dal markup/CSS reale)

| Elemento | Scelta reale | Dove |
|---|---|---|
| Tema | scuro, vetro/gradiente tecnico | theme.css, landing.css |
| Accento testuale | `.grad` — "One inbox." in gradiente | index.html hero: `<h1 data-reveal>Every brand.<br><span class="grad">One inbox.</span></h1>` |
| Brand | logo SVG in-stack: rettangoli sovrapposti blu/verde/ink + busta — **le tre caselle/brand che condividono una inbox** | topbar index.html |
| Eyebrow | "Self-hosted · multi-brand · agent-native" | index.html |
| Bottoni | `.btn primary` (azione) / `.btn ghost` (navigazione) | topbar, CTA |
| Card | `.card` con `.card-head` per sezioni feature/step | tutto il landing |

## 3. Struttura di pagina (convenzione condivisa dalle 11 pagine)

```
<a class="skip-link" href="#main">Skip to content</a>
<header class="topbar"> brand + nav (Use cases, Webmail, Pricing, Docs, Mail UI, Sign in) </header>
<main id="main">
  section.page-head → h1 + lede
  …sezioni…
</main>
<footer> legal/privacy, legal/terms, legal/sla </footer>
```

Convenzioni verificate: skip-link, `aria-current="page"` sul brand attivo, nav etichettata (`aria-label="Main"`), footer con link alle pagine legali.

## 4. Motion

- `[data-reveal]` per l'ingresso a cascata di hero/feature/CTA (app.js assegna `.in` all'IntersectionObserver)
- **Vincolo non negoziabile**: `opacity:0` di default **rompe il no-JS** — audit alto: senza JS quasi tutto il main resta invisibile. Regola: **contenuto visibile di default, nascondere solo con JS attivo** (es. classe `.js` su `<html>` o regola dentro `@media (scripting: enabled)`)

## 5. Accessibilità (requisiti già nel markup + gap dell'audit)

- skip-link ✓, stepper `aria-current="step"` ✓, messaggi `role="alert"` ✓, `<dialog>` per modal ✓
- **Gap da chiudere** (audit): bottoni `.btn` 40px e `.small-btn` 28px sotto i 44px mobile; tablist del modal login senza `aria-controls`/navigazione da frecce; focus visibile sui target touch

## 6. Registro di scrittura

- Titolo prodotto: "AImulti.inbox — every brand, one inbox"; lede corte, una promessa per sezione
- **Mai "mailcow" nel testo visibile** (regola ferrea di brand)
- Messaggi d'errore **onesti e numerati** (checkout: "Payments are not configured yet on this server"; nodo pieno: "contact sales") — mai copy di operatore (`set MAILCOW_UI_ORIGIN…` esposta nel modal login è una finding media dell'audit: messaggio per utente, dettaglio in log)
- CTA webmail onesta: "Open the webmail client" → /home (niente promesse di iframe)

## 7. Lavoro di design da fare (audit UX multi_ai → tasks UI, vedi 06-implementation-plan)

| Priorità | Finding audit | Fix |
|---|---|---|
| **Critical** | pricing.html Solo card: 5 bottoni identici "Subscribe" con data-checkout diversi | un solo Subscribe per card (refuso di template) |
| **Alto** | `data-reveal` opacity:0 di default → no-JS invisibile | visibile di default, nascondere solo con JS |
| **Alto** | docs.html: 2 link topbar incollati dentro la frase Step 1 | rimuovere |
| **Alto** | docs.html Guida 3 descrive iframe/toolbar rimossi da webmail.html | riscrivere sul flusso attuale (CTA → login SOGo) |
| **Medio** | href "Mail UI" incoerenti ('/', '/home', JS) | hardcodare '/home' ovunque |
| **Medio** | `.env-pick` radio-card CSS solo inline in onboarding | spostare in style.css |
| **Medio** | copy operator nel modal login | messaggio utente + dettaglio in log |
| **Medio** | "Contact sales" → onboarding.html | mailto o form dedicato |
| **Medio** | nessun Sign in nella topbar di use-cases/pricing/docs | bottone ghost |
| **Minor** | `.btn` < 44px mobile; inbox.js copy non onesta; webmail CTA `href='#'`; tablist aria; placeholder rotto docs.html | vedi piano |

## 8. Deliverable di design

1. Fix critici/alto (niente UI invisibile, niente doppio Subscribe, docs coerente col flusso reale)
2. Consolidamento CSS (env-pick in style.css, target touch 44px)
3. Coerenza topbar (Sign in ovunque, Mail UI → /home)
4. Pass di copia onesta sui messaggi d'errore
