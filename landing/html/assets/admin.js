/* admin.js — operator / domain-admin page
   READS today (existing landing API):
     GET api/status, api/domains, api/dns
   Gateway endpoints (audit trail, access keys) are being built in parallel:
   the page fetches GATEWAY_READ_BASE and, on 404/501/503 or network error,
   shows the honest in-card notice — never fake rows. */
(async () => {
  'use strict';

  const GATEWAY_READ_BASE = 'api/gateway/';

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

  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function showMsg(el, text) { el.textContent = text; el.hidden = false; }

  function serviceDown(container) {
    container.replaceChildren();
    const card = el('section', 'card');
    const h = el('h2', null, 'Service temporarily unavailable');
    const p = el('p', null, 'The landing API is not responding. Wait a few minutes and retry.');
    const actions = el('div', 'form-actions');
    const retry = el('button', 'btn primary', 'Retry');
    retry.type = 'button';
    retry.addEventListener('click', () => location.reload());
    actions.appendChild(retry);
    card.append(h, p, actions);
    container.appendChild(card);
  }

  function notAuthorized(container) {
    container.replaceChildren();
    const card = el('section', 'card');
    const h = el('h2', null, 'Not authorized');
    const p = el('p', null, 'This view needs an admin session. If you believe this is a mistake, ask the operator to enable your account.');
    const actions = el('div', 'form-actions');
    const back = el('a', 'btn ghost', 'Back to the site');
    back.href = 'use-cases.html';
    actions.appendChild(back);
    card.append(h, p, actions);
    container.appendChild(card);
    // Operator-facing hint stays out of the visible UI.
    console.warn('Admin gate: set LANDING_ADMIN_USER on the server to the connected username to grant admin access.');
  }

  function table(captionText, headers, rows) {
    const scroll = el('div', 'table-scroll');
    const t = el('table');
    t.appendChild(el('caption', 'sr-only', captionText));
    const thead = el('thead');
    const trh = el('tr');
    headers.forEach((h) => {
      const th = el('th', h.num ? 'num' : null, h.label);
      th.scope = 'col';
      trh.appendChild(th);
    });
    thead.appendChild(trh);
    t.appendChild(thead);
    const tbody = el('tbody');
    rows.forEach((cells) => {
      const tr = el('tr');
      cells.forEach((c) => {
        const td = el('td', c.num ? 'num' : null);
        if (c.node) td.appendChild(c.node);
        else td.textContent = c.text;
        tr.appendChild(td);
      });
      tbody.appendChild(tr);
    });
    t.appendChild(tbody);
    scroll.appendChild(t);
    return scroll;
  }

  function badge(text, cls) {
    return el('span', 'badge' + (cls ? ' ' + cls : ''), text);
  }

  const page = $('#page');
  const domainSelect = $('#domain-select');
  const dnsOutput = $('#dns-output');

  /* ---------------- status guard ---------------- */

  let status = await api('api/status');
  if (!status.data || !status.data.ok) {
    serviceDown(page);
    return;
  }
  status = status.data;

  if (!status.connected) {
    // Admin view is domain-scoped; it needs the Namecheap session.
    location.replace('index.html');
    return;
  }

  /* topbar: who is connected (visible in every state) */
  $('#connected-as').textContent = status.username || '';

  /* disconnect — two-step confirm (no modal on this page), then the endpoint.
     The nav is visible in every state, so this wires before the admin gate. */
  const btnDisconnect = $('#btn-disconnect');
  if (btnDisconnect) {
    let armed = false;
    let resetTimer = null;
    btnDisconnect.addEventListener('click', async () => {
      if (!armed) {
        armed = true;
        btnDisconnect.textContent = 'Confirm disconnect';
        resetTimer = setTimeout(() => {
          armed = false;
          btnDisconnect.textContent = 'Disconnect';
        }, 4000);
        return;
      }
      clearTimeout(resetTimer);
      armed = false;
      btnDisconnect.textContent = 'Disconnect';
      btnDisconnect.disabled = true;
      const body = new URLSearchParams();
      body.set('csrf', status.csrf || '');
      await api('api/disconnect', {
        method: 'POST',
        body,
        headers: { 'X-Landing-CSRF': status.csrf || '' },
      });
      location.replace('index.html');
    });
  }

  /* admin gate — fail-closed: nav entries + scoped data only for admins */
  if (!status.admin) {
    notAuthorized(page);
    return;
  }
  $('#nav-admin').hidden = false;
  $('#nav-dashboard').hidden = false;

  /* session card */
  $('#kv-user').textContent = status.username || 'unknown';
  $('#kv-sandbox').textContent = status.sandbox ? 'sandbox' : 'production';
  $('#kv-origin').textContent = status.mailcow_origin || 'not configured';
  $('#kv-api').textContent = 'reachable';

  /* ---------------- domains + scoped views ---------------- */

  let domains = [];

  async function loadDomains() {
    const res = await api('api/domains');
    if (!res.data || !res.data.ok) {
      domainSelect.replaceChildren();
      const opt = el('option', null, 'No connection');
      opt.value = '';
      domainSelect.appendChild(opt);
      showMsg($('#dns-error'), (res.data && res.data.error) || 'Could not load domains \u2014 reconnect the Namecheap API key.');
      return;
    }
    domains = res.data.domains || [];
    domainSelect.replaceChildren();
    if (!domains.length) {
      const opt = el('option', null, 'No domains in this account');
      opt.value = '';
      domainSelect.appendChild(opt);
      return;
    }
    domains.forEach((d, i) => {
      const opt = el('option', null, d.name);
      opt.value = String(i);
      domainSelect.appendChild(opt);
    });
    $('#dns-error').hidden = true;
  }

  async function loadDnsScope(domain) {
    $('#scope-meta').textContent = domain;
    $('#dns-error').hidden = true;
    dnsOutput.replaceChildren();
    const skel = el('div', 'skeleton');
    skel.append(el('span'), el('span'), el('span'), el('span'));
    dnsOutput.appendChild(skel);
    const res = await api('api/dns?domain=' + encodeURIComponent(domain));
    skel.remove();
    if (!res.data || !res.data.ok) {
      showMsg($('#dns-error'), (res.data && res.data.error) || 'Failed to load DNS records.');
      $('#stat-dns').textContent = '\u2013';
      return;
    }
    const hosts = res.data.hosts || [];
    $('#stat-dns').textContent = String(hosts.length);
    if (!hosts.length) {
      dnsOutput.appendChild(el('p', 'empty-state', 'No host records \u2014 the domain may use other nameservers.'));
      return;
    }
    dnsOutput.appendChild(table(
      'DNS records for ' + domain,
      [{ label: 'Host' }, { label: 'Type' }, { label: 'Address' },
       { label: 'MX pref', num: true }, { label: 'TTL', num: true }],
      hosts.map((r) => [
        { text: r.name }, { text: r.type }, { text: r.address },
        { text: r.mx_pref, num: true }, { text: r.ttl, num: true },
      ])
    ));
  }

  /* ---------------- audit trail (gateway, in costruzione) ---------------- */

  async function loadAudit(domain) {
    $('#audit-meta').textContent = domain;
    const note = $('#audit-note');
    const out = $('#audit-output');
    out.replaceChildren();
    const res = await api(GATEWAY_READ_BASE + 'audit?domain=' + encodeURIComponent(domain));
    if (res.data && res.data.ok && Array.isArray(res.data.entries)) {
      note.hidden = true;
      const entries = res.data.entries;
      $('#stat-audit').textContent = String(entries.length);
      if (!entries.length) {
        out.appendChild(el('p', 'empty-state', 'No audit entries for this domain yet.'));
        return;
      }
      out.appendChild(table(
        'Audit entries for ' + domain,
        [{ label: 'When' }, { label: 'Actor' }, { label: 'Action' }, { label: 'Detail' }],
        entries.map((r) => [
          { text: r.when || r.at || '' }, { text: r.actor || '' },
          { node: r.action ? badge(r.action, r.action.includes('delete') || r.action.includes('remove') ? 'warn' : 'info') : el('span') },
          { text: r.detail || r.what || '' },
        ])
      ));
      return;
    }
    // gateway not live (404/501/503) or any other failure: honest notice
    $('#stat-audit').textContent = '\u2013';
    showMsg(note, 'The audit trail is not available yet \u2014 the audit API ships with the API gateway.');
  }

  /* ---------------- access keys (gateway, in costruzione) ---------------- */

  async function loadKeys(domain) {
    $('#keys-meta').textContent = domain;
    const note = $('#keys-note');
    const out = $('#keys-output');
    out.replaceChildren();
    const res = await api(GATEWAY_READ_BASE + 'keys?domain=' + encodeURIComponent(domain));
    if (res.data && res.data.ok && Array.isArray(res.data.keys)) {
      note.hidden = true;
      const keys = res.data.keys;
      if (!keys.length) {
        out.appendChild(el('p', 'empty-state', 'No access keys for this domain.'));
        return;
      }
      out.appendChild(table(
        'Access keys for ' + domain,
        [{ label: 'Label' }, { label: 'Scope' }, { label: 'Created' }, { label: 'Last used' }],
        keys.map((r) => [
          { text: r.label || '' }, { text: r.scope || '' },
          { text: r.created || '' }, { text: r.last_used || 'never' },
        ])
      ));
      return;
    }
    showMsg(note, 'Access keys are not available yet \u2014 key management ships with the API gateway.');
  }

  domainSelect.addEventListener('change', () => {
    const idx = domainSelect.value;
    if (idx === '' || !domains[idx]) return;
    const d = domains[idx].name;
    loadDnsScope(d);
    loadAudit(d);
    loadKeys(d);
  });

  /* ---------------- entry point ---------------- */

  await loadDomains();
  $('#stat-domains').textContent = String(domains.length);
  if (domains.length) {
    const d = domains[0].name;
    loadDnsScope(d);
    loadAudit(d);
    loadKeys(d);
  } else {
    $('#scope-meta').textContent = 'no domains';
  }
})();
