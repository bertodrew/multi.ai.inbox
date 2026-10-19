/* webmail.js — full mail client embed + module launcher
   The groupware client (SOGo) ships with the platform and is same-origin with
   the landing, so the full client — mail, folders, compose, calendar, contacts,
   tasks — runs inside the frame. Module buttons deep-link into each view;
   "Open in new tab" is the honest escape hatch if the frame is blocked or the
   session drops mid-use. Reads GET api/status only (session + origin). */
(async () => {
  'use strict';

  const $ = (sel, el = document) => el.querySelector(sel);

  async function api(path, opts) {
    try {
      const r = await fetch('/landingpage' + path, opts);
      let data = null;
      try { data = await r.json(); } catch (e) { data = null; }
      return { httpOk: r.ok, status: r.status, data };
    } catch (e) {
      return { httpOk: false, status: 0, data: null };
    }
  }

  const frame = $('#sogo-frame');
  const sogoOpen = $('#sogo-open');
  const buttons = Array.from(document.querySelectorAll('.module-btn'));

  function setView(view) {
    const src = view === 'Mail' ? '/SOGo/so' : '/SOGo/so/view=' + view;
    if (frame.getAttribute('src') !== src) frame.setAttribute('src', src);
    buttons.forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.view === view)));
  }

  buttons.forEach((b) => b.addEventListener('click', () => setView(b.dataset.view)));

  const status = await api('api/status');

  // Session identity in the topbar (same convention as the other app pages).
  if (status.httpOk && status.data) {
    if (status.data.connected && status.data.username) {
      const ca = $('#connected-as');
      if (ca) ca.textContent = 'Connected as ' + status.data.username + (status.data.sandbox ? ' (sandbox)' : '');
    }
    // Keep the frame and the escape link on the same origin as the mail UI.
    const origin = status.data.mailcow_origin ? status.data.mailcow_origin.replace(/\/$/, '') : '';
    if (origin) {
      sogoOpen.href = origin + '/SOGo/';
      const adminLink = $('#admin-link');
      if (adminLink) adminLink.href = origin + '/home';
    }
  }
})();
