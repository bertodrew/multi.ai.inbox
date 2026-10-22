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
  } else {
    const submit = mailcowForm && mailcowForm.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    if (mailcowHint) mailcowHint.textContent = 'The mail UI is temporarily unreachable. Please try again later — if it persists, contact support.';
    console.warn('Landing: mail origin unknown — set MAILCOW_UI_ORIGIN in the platform config and restart the landing services.');
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

  // Register form: client-side password confirmation, then POST to the
  // account endpoint — success rotates the session and lands on onboarding.
  const regForm = $('#register-form');
  const regError = $('#reg-error');
  if (regError) regError.setAttribute('role', 'alert');
  if (regForm && regError) {
    regForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      regError.hidden = true;
      const pass = $('#reg-pass').value;
      if (pass !== $('#reg-pass2').value) {
        regError.textContent = 'Passwords do not match';
        regError.hidden = false;
        return;
      }
      const body = new URLSearchParams();
      body.set('first_name', $('#reg-first').value.trim());
      body.set('last_name', $('#reg-last').value.trim());
      body.set('email', $('#reg-email').value.trim());
      body.set('country', $('#reg-country').value);
      body.set('password', pass);
      body.set('password_confirm', $('#reg-pass2').value);
      body.set('csrf', csrf);
      const submit = regForm.querySelector('button[type="submit"]');
      submit.disabled = true;
      submit.textContent = 'Creating account…';
      let res;
      try {
        res = await fetch('/landingpage/api/auth/register', {
          method: 'POST',
          body,
          headers: { 'X-Landing-CSRF': csrf },
        }).then((r) => r.json());
      } catch (err) {
        res = { ok: false, error: 'Network error: ' + err.message };
      }
      submit.disabled = false;
      submit.textContent = 'Create account';
      if (res.ok) {
        location.href = 'onboarding.html';
        return;
      }
      regError.textContent = res.error || 'Registration failed';
      regError.hidden = false;
    });
  }

  // Password visibility toggles: eye/eye-off inside the field wrapper.
  document.querySelectorAll('[data-toggle-pass]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const wrap = btn.closest('.field-glow');
      const input = wrap && wrap.querySelector('input');
      if (!input) return;
      const show = input.type === 'password';
      input.type = show ? 'text' : 'password';
      btn.setAttribute('aria-pressed', show ? 'true' : 'false');
      btn.setAttribute('aria-label', show ? 'Hide password' : 'Show password');
      btn.classList.toggle('showing', show);
    });
  });

  // Field glow: the radial highlight that follows the pointer across the
  // input wrapper (CSS custom properties, no animation library).
  document.querySelectorAll('.field-glow').forEach((wrap) => {
    wrap.addEventListener('pointermove', (e) => {
      const r = wrap.getBoundingClientRect();
      wrap.style.setProperty('--mx', (e.clientX - r.left) + 'px');
      wrap.style.setProperty('--my', (e.clientY - r.top) + 'px');
    });
    wrap.addEventListener('pointerenter', () => wrap.classList.add('glow-on'));
    wrap.addEventListener('pointerleave', () => wrap.classList.remove('glow-on'));
  });

  // OAuth entry points: the server answers with a redirect when the provider
  // is configured, or the honest 501 when it is not — surface the error in
  // the register panel, never a fake navigation.
  document.querySelectorAll('[data-oauth]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const provider = btn.dataset.oauth;
      let res = null;
      try {
        res = await fetch('/landingpage/api/auth/oauth/' + provider, { redirect: 'manual' });
      } catch (err) {
        res = null;
      }
      if (res && res.type === 'opaqueredirect') {
        location.href = '/landingpage/api/auth/oauth/' + provider;
        return;
      }
      let data = null;
      if (res) {
        try {
          data = await res.json();
        } catch (err) {
          data = null;
        }
      }
      if (regError) {
        regError.textContent =
          (data && data.error) || 'Sign-in with ' + provider + ' is unavailable right now.';
        regError.hidden = false;
      }
    });
  });

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
