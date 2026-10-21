/* webmail.js — webmail launcher
   The groupware client (SOGo) ships with the platform. It authenticates with
   the mailbox address + password — separate from this app's Namecheap session
   — so the client opens in a new tab instead of an embed: SOGo's own login
   page cannot run inside a frame (SAMEORIGIN), and this session doesn't carry
   mailbox credentials. Reads GET api/status (session + origin). */
(async () => {
  'use strict';

  const $ = (sel, el = document) => el.querySelector(sel);

  async function api(path, opts) {
    try {
      const r = await fetch('/landingpage/' + String(path).replace(/^\/+/, ''), opts);
      let data = null;
      try { data = await r.json(); } catch (e) { data = null; }
      return { httpOk: r.ok, status: r.status, data };
    } catch (e) {
      return { httpOk: false, status: 0, data: null };
    }
  }

  const status = await api('api/status');

  // Keep the escape link on the same origin as the mail UI.
  const origin = status.httpOk && status.data && status.data.mailcow_origin
    ? status.data.mailcow_origin.replace(/\/$/, '') : '';
  const open = $('#sogo-open');
  const gate = $('#sogo-gate');
  if (open && origin) open.href = origin + '/SOGo/';
  else if (open && gate) {
    // Origin unknown: disable the CTA (no empty tab) and tell the user honestly.
    open.removeAttribute('href');
    open.setAttribute('aria-disabled', 'true');
    const note = document.createElement('p');
    note.className = 'msg error';
    note.textContent = 'The webmail client is temporarily unreachable — please try again later.';
    gate.appendChild(note);
  }
  const adminLink = $('#admin-link');
  // The Mail UI entry points at the webmail that actually answers (/SOGo/):
  // /home was the wrong URL — it lands in the platform's raw admin UI.
  if (adminLink && origin) adminLink.href = origin + '/SOGo/';

  // Session identity in the topbar (same convention as the other app pages).
  if (status.httpOk && status.data && status.data.connected && status.data.username) {
    const ca = $('#connected-as');
    if (ca) ca.textContent = 'Connected as ' + status.data.username + (status.data.sandbox ? ' (sandbox)' : '');
  }
})();
