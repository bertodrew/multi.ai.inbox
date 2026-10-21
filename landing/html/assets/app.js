(async () => {
  const $ = (sel, el = document) => el.querySelector(sel);

  let status = null;
  try {
    status = await fetch('/landingpage/api/status').then((r) => r.json());
  } catch (e) {
    status = null;
  }

  // Connected-first redirect: only on the sign-in landing itself — the other
  // marketing pages (use cases, pricing, docs) stay browsable while connected.
  if ((document.body.dataset.page || 'index') === 'index' && status && status.ok && status.connected) {
    location.replace('connectors.html');
    return;
  }

  const csrf = status && status.ok ? status.csrf : '';
  const origin = status && status.ok ? status.mailcow_origin : '';
  const mailcowForm = $('#mailcow-form');
  const mailcowHint = $('#mailcow-hint');
  if (mailcowHint) mailcowHint.setAttribute('aria-live', 'polite'); // M4 fix: origin status announced

  if (origin) {
    if (mailcowForm) mailcowForm.action = origin + '/home';
    // Mail UI → /home hardcoded on every page — one canonical fallback across
    // all pages (the hardcoded hrefs were incoherent: /SOGo/ qui, /home altrove),
    // upgraded with the runtime origin when the status answers.
    const adminLink = $('#admin-link');
    if (adminLink) adminLink.href = origin + '/home';
  } else {
    const submit = mailcowForm && mailcowForm.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    if (mailcowHint) mailcowHint.textContent = 'The mail UI is temporarily unreachable. Please try again later — if it persists, contact support.';
    console.warn('Landing: mail origin unknown — set MAILCOW_UI_ORIGIN in the platform config and restart the landing services.');
  }

  // S4: the server no longer discloses its egress IP to the browser — leave the
  // field empty and the API uses the server's own egress IP (auto-detected).
  const ncIpHint = $('#nc-ip-hint');
  if (ncIpHint) {
    ncIpHint.setAttribute('aria-live', 'polite'); // M4 fix: status update announced
    ncIpHint.textContent =
      'Optional: leave empty to use this server\u2019s egress IP, or type the IP whitelisted in Namecheap (Profile > Tools > API Access).';
  }

  // ARIA tablist: click activates, arrow keys move focus + activate,
  // roving tabindex (active tab 0, the others -1).
  const tabs = Array.from(document.querySelectorAll('.tab'));
  const activateTab = (active) => {
    tabs.forEach((t) => {
      t.classList.toggle('active', t === active);
      t.setAttribute('aria-selected', t === active ? 'true' : 'false');
      t.tabIndex = t === active ? 0 : -1;
    });
    document.querySelectorAll('.panel').forEach((p) => {
      p.classList.toggle('active', p.id === 'panel-' + active.dataset.tab);
    });
  };
  tabs.forEach((tab) => {
    tab.addEventListener('click', () => activateTab(tab));
    tab.addEventListener('keydown', (e) => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
      e.preventDefault();
      const idx = tabs.indexOf(tab);
      const next = tabs[(idx + (e.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length];
      next.focus();
      activateTab(next);
    });
  });

  const ncError = $('#nc-error');
  const ncSubmit = $('#nc-submit');
  const ncForm = $('#nc-form');
  if (ncError) ncError.setAttribute('role', 'alert'); // M4 fix: submit errors announced to screen readers
  if (ncForm && ncSubmit && ncError) {
    ncForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    ncError.hidden = true;

    const body = new URLSearchParams();
    body.set('username', $('#nc-username').value.trim());
    body.set('api_key', $('#nc-api-key').value.trim());
    body.set('client_ip', $('#nc-client-ip').value.trim());
    body.set('use_sandbox', $('#nc-sandbox').checked ? '1' : '0');
    body.set('csrf', csrf);

    ncSubmit.disabled = true;
    ncSubmit.textContent = 'Connecting…';
    let res;
    try {
      res = await fetch('/landingpage/api/connect', {
        method: 'POST',
        body,
        headers: { 'X-Landing-CSRF': csrf },
      }).then((r) => r.json());
    } catch (err) {
      res = { ok: false, error: 'Network error: ' + err.message };
    }
    ncSubmit.disabled = false;
    ncSubmit.textContent = 'Connect Namecheap';

    if (res.ok) {
      location.href = 'connectors.html';
      return;
    }
    ncError.textContent = res.error || 'Connection failed';
    ncError.hidden = false;
    });
  }

  // Sign-in modal: every [data-open-login] opens the <dialog>; backdrop and
  // close button dismiss it (Esc is native to <dialog>).
  const loginModal = $('#login-modal');
  if (loginModal) {
    document.querySelectorAll('[data-open-login]').forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        loginModal.showModal();
      });
    });
    loginModal.addEventListener('click', (e) => {
      if (e.target === loginModal) loginModal.close();
    });
    const closeBtn = $('#login-close');
    if (closeBtn) closeBtn.addEventListener('click', () => loginModal.close());
  }

  // Product tour: every [data-open-tour] opens its <dialog id>; backdrop and
  // close buttons dismiss it (Esc is native to <dialog>).
  document.querySelectorAll('[data-open-tour]').forEach((el) => {
    el.addEventListener('click', () => {
      const dlg = document.getElementById(el.dataset.openTour);
      if (dlg && typeof dlg.showModal === 'function') dlg.showModal();
    });
  });
  document.querySelectorAll('.tour-modal').forEach((dlg) => {
    dlg.addEventListener('click', (e) => {
      if (e.target === dlg) dlg.close();
    });
    const closer = dlg.querySelector('[data-close-tour]');
    if (closer) closer.addEventListener('click', () => dlg.close());
  });

  // Copy buttons ([data-copy] → CSS selector of the value to copy): same
  // pattern as connectors.js, wired here too because docs.html loads only
  // app.js — no-op on pages without [data-copy] buttons.
  document.querySelectorAll('[data-copy]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = $(btn.dataset.copy);
      if (!target) return;
      const text = target.textContent;
      const done = () => {
        const prev = btn.textContent;
        btn.textContent = 'Copied';
        setTimeout(() => { btn.textContent = prev; }, 1200);
      };
      const fallback = () => {
        // Non-secure contexts and denied permissions (file:// previews):
        // clipboard API unavailable — transient textarea + execCommand copy.
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.setAttribute('readonly', '');
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        try {
          if (document.execCommand('copy')) done();
        } catch (e) { /* clipboard unavailable — leave the button as-is */ }
        ta.remove();
      };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(done).catch(fallback);
        return;
      }
      fallback();
    });
  });

  // Scroll reveal: progressive enhancement — mark the document only when JS
  // runs (landing.css hides [data-reveal] under .js-reveal), elements marked
  // data-reveal fade/slide in on first view. Without JS, content stays visible.
  document.documentElement.classList.add('js-reveal');
  if ('IntersectionObserver' in window) {
    const io = new IntersectionObserver((entries) => {
      entries.forEach((en) => {
        if (en.isIntersecting) {
          en.target.classList.add('in');
          io.unobserve(en.target);
        }
      });
    }, { threshold: 0.15 });
    document.querySelectorAll('[data-reveal]').forEach((el) => io.observe(el));
  } else {
    document.querySelectorAll('[data-reveal]').forEach((el) => el.classList.add('in'));
  }
})();
