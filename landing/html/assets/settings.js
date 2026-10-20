/* settings.js — per-domain settings page
   READS (existing landing API):
     GET api/status, api/connect (session), api/domains, api/dns
   WRITES (live since 20/10): Save PATCHes GATEWAY_WRITE_BASE
   ('api/gateway/domains/<domain>', session + X-Landing-CSRF). Only fields the
   mail platform accepts are written (branding, defquota, maxquota); ptr_target
   is network-level rDNS — the gateway answers with a warning, never a fake
   success. UseGatewayWrites must stay true in production. */
(async () => {
  'use strict';

  const USE_GATEWAY_WRITES = true;
  const GATEWAY_WRITE_BASE = 'api/gateway/domains/';

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

  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function showMsg(el, text) { el.textContent = text; el.hidden = false; }
  function hideMsg(el) { el.hidden = true; }

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

  const page = $('#page');
  const domainSelect = $('#domain-select');
  const btnRefresh = $('#btn-refresh');
  const dnsOutput = $('#dns-output');

  /* ---------------- status guard ---------------- */

  let status = await api('api/status');
  if (!status.data || !status.data.ok) {
    serviceDown(page);
    return;
  }
  status = status.data;

  if (!status.connected) {
    // No Namecheap session: settings need the domain list; route to login.
    location.replace('index.html');
    return;
  }

  /* topbar */
  $('#connected-as').textContent = status.username || '';
  if (status.mailcow_origin) $('#admin-link').href = status.mailcow_origin + '/home';

  /* confirm-modal helper (B8): Escape cancels, focus returns to the opener */
  function confirmModal({ title, body, confirmLabel }) {
    return new Promise((resolve) => {
      const prevFocus = document.activeElement;
      const overlay = el('div', 'modal-overlay');
      const box = el('div', 'modal');
      box.setAttribute('role', 'dialog');
      box.setAttribute('aria-modal', 'true');
      const h = el('h2', null, title);
      h.tabIndex = -1;
      const p = el('p', null, body);
      const actions = el('div', 'form-actions');
      const no = el('button', 'btn ghost', 'Cancel');
      no.type = 'button';
      const yes = el('button', 'btn danger', confirmLabel);
      yes.type = 'button';
      actions.append(no, yes);
      box.append(h, p, actions);
      overlay.appendChild(box);
      document.body.appendChild(overlay);

      const close = (val) => {
        document.removeEventListener('keydown', onKey);
        overlay.remove();
        if (prevFocus && prevFocus.focus) prevFocus.focus();
        resolve(val);
      };
      const onKey = (e) => { if (e.key === 'Escape') close(false); };
      document.addEventListener('keydown', onKey);
      no.addEventListener('click', () => close(false));
      yes.addEventListener('click', () => close(true));
      overlay.addEventListener('click', (e) => { if (e.target === overlay) close(false); });
      h.focus();
    });
  }

  /* disconnect — confirm first (UX review B8), then the dedicated endpoint */
  $('#btn-disconnect').addEventListener('click', async () => {
    const ok = await confirmModal({
      title: 'Disconnect Namecheap?',
      body: 'The stored API key will be removed from this session. You can reconnect at any time.',
      confirmLabel: 'Disconnect',
    });
    if (!ok) return;
    const body = new URLSearchParams();
    body.set('csrf', status.csrf || '');
    await api('api/disconnect', {
      method: 'POST',
      body,
      headers: { 'X-Landing-CSRF': status.csrf || '' },
    });
    location.replace('index.html');
  });

  /* ---------------- domains + per-domain state ---------------- */

  let domains = [];
  let current = null; // { domain, record }

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
    hideMsg($('#dns-error'));
  }

  /* gateway read: per-domain mail settings — placeholder until the gateway ships.
     Falls back silently to nothing (fields stay empty + notice). */
  async function loadGatewayRecord(domain) {
    const res = await api(GATEWAY_WRITE_BASE + encodeURIComponent(domain));
    if (res.data && res.data.ok && res.data.domain) return res.data.domain;
    if (res.status === 404 || res.status === 501 || res.status === 503) return null; // gateway not live
    return null;
  }

  function fillForm(rec) {
    $('#f-fqdn').value = rec ? (rec.fqdn || current.domain) : current.domain;
    $('#f-branding').value = rec ? (rec.branding || '') : '';
    $('#f-ptr').value = rec ? (rec.ptr_target || '') : '';
    $('#f-defquota').value = rec && rec.defquota != null ? rec.defquota : '';
    $('#f-maxquota').value = rec && rec.maxquota != null ? rec.maxquota : '';
  }

  async function selectDomain(domain) {
    current = { domain, record: null };
    $('#mail-meta').textContent = domain;
    $('#dns-meta').textContent = domain;

    // per-domain fields: gateway record (placeholder) + honest notice
    const rec = await loadGatewayRecord(domain);
    if (rec) current.record = rec;
    const note = $('#gateway-read-note');
    if (rec) {
      note.hidden = true;
    } else {
      showMsg(note, 'Mail-domain fields are empty \u2014 this domain may not be provisioned on the mail platform yet.');
    }
    fillForm(current.record);

    // DNS snippets: EXISTING endpoint
    hideMsg($('#dns-error'));
    dnsOutput.replaceChildren();
    const skel = el('div', 'skeleton');
    skel.append(el('span'), el('span'), el('span'), el('span'));
    dnsOutput.appendChild(skel);
    const res = await api('api/dns?domain=' + encodeURIComponent(domain));
    skel.remove();
    if (!res.data || !res.data.ok) {
      showMsg($('#dns-error'), (res.data && res.data.error) || 'Failed to load DNS records.');
      return;
    }
    const hosts = res.data.hosts || [];
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

  domainSelect.addEventListener('change', () => {
    const idx = domainSelect.value;
    if (idx !== '' && domains[idx]) selectDomain(domains[idx].name);
  });
  btnRefresh.addEventListener('click', async () => {
    btnRefresh.disabled = true;
    await loadDomains();
    btnRefresh.disabled = false;
  });

  /* ---------------- save (gateway write, feature-flagged) ---------------- */

  $('#domain-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    hideMsg($('#save-ok'));
    hideMsg($('#save-error'));
    if (!current) return;

    if (!USE_GATEWAY_WRITES) {
      showMsg($('#save-error'), 'Writes are not live yet \u2014 the API gateway for domain settings is still being built. Changes here are read-only for now.');
      return;
    }
    const payload = {
      branding: $('#f-branding').value.trim(),
      ptr_target: $('#f-ptr').value.trim(),
      defquota: $('#f-defquota').value === '' ? null : Number($('#f-defquota').value),
      maxquota: $('#f-maxquota').value === '' ? null : Number($('#f-maxquota').value),
    };
    const btn = $('#btn-save');
    btn.disabled = true;
    btn.textContent = 'Saving\u2026';
    const res = await api(GATEWAY_WRITE_BASE + encodeURIComponent(current.domain), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json', 'X-Landing-CSRF': status.csrf || '' },
      body: JSON.stringify(payload),
    });
    btn.disabled = false;
    btn.textContent = 'Save changes';
    if (res.data && res.data.ok) {
      current.record = res.data.domain || payload;
      fillForm(current.record);
      let msg = 'Changes saved.';
      if (res.data.warnings && res.data.warnings.length) msg += ' Note: ' + res.data.warnings[0];
      showMsg($('#save-ok'), msg);
    } else {
      showMsg($('#save-error'), (res.data && res.data.error) || 'Save failed \u2014 the gateway did not answer.');
    }
  });

  /* ---------------- copy DNS records ---------------- */

  $('#btn-copy-dns').addEventListener('click', async () => {
    const lines = [];
    dnsOutput.querySelectorAll('table tbody tr').forEach((tr) => {
      const cells = [...tr.children].map((td) => td.textContent.trim());
      lines.push(cells.join('\t'));
    });
    if (!lines.length) return;
    try {
      await navigator.clipboard.writeText(lines.join('\n'));
      showMsg($('#copy-ok'), 'Records copied.');
    } catch (err) {
      showMsg($('#copy-ok'), 'Copy failed \u2014 select the table and copy manually.');
    }
    setTimeout(() => { hideMsg($('#copy-ok')); }, 3000);
  });

  /* ---------------- entry point ---------------- */

  await loadDomains();
  if (domains.length) {
    await selectDomain(domains[0].name);
  } else {
    $('#mail-meta').textContent = 'no domains';
  }
})();
