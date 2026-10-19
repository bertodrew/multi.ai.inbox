(async () => {
  const $ = (sel, el = document) => el.querySelector(sel);

  let status = null;
  try {
    status = await fetch('api/status').then((r) => r.json());
  } catch (e) {
    status = null;
  }

  if (status && status.ok && status.connected) {
    location.replace('connectors.html');
    return;
  }

  const csrf = status && status.ok ? status.csrf : '';
  const origin = status && status.ok ? status.mailcow_origin : '';
  const mailcowForm = $('#mailcow-form');
  const mailcowHint = $('#mailcow-hint');
  mailcowHint.setAttribute('aria-live', 'polite'); // M4 fix: origin status announced

  if (origin) {
    mailcowForm.action = origin + '/';
    $('#admin-link').href = origin + '/';
  } else {
    const submit = mailcowForm.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    mailcowHint.textContent = 'Mail origin unknown: set MAILCOW_UI_ORIGIN in the platform config and restart the landing services.';
  }

  // S4: the server no longer discloses its egress IP to the browser — leave the
  // field empty and the API uses the server's own egress IP (auto-detected).
  $('#nc-ip-hint').setAttribute('aria-live', 'polite'); // M4 fix: status update announced
  $('#nc-ip-hint').textContent =
    'Optional: leave empty to use this server\u2019s egress IP, or type the IP whitelisted in Namecheap (Profile > Tools > API Access).';

  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach((t) => {
        t.classList.toggle('active', t === tab);
        t.setAttribute('aria-selected', t === tab ? 'true' : 'false');
      });
      document.querySelectorAll('.panel').forEach((p) => {
        p.classList.toggle('active', p.id === 'panel-' + tab.dataset.tab);
      });
    });
  });

  const ncError = $('#nc-error');
  ncError.setAttribute('role', 'alert'); // M4 fix: submit errors announced to screen readers
  const ncSubmit = $('#nc-submit');

  $('#nc-form').addEventListener('submit', async (e) => {
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
      res = await fetch('api/connect', {
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

  // Scroll reveal: elements marked data-reveal fade/slide in on first view.
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
