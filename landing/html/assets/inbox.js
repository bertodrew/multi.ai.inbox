/* inbox.js — mailbox list + state + basic actions
   READS today (existing landing API):
     GET api/status, api/domains, api/dns
   MAILBOX LISTING: there is no browser-facing mailbox endpoint today. The
   mailcow MCP tools (list_domains / get_domain_settings / set_domain_settings)
   authenticate with the Bearer API key, which never reaches the browser — so
   the listing is fetched from the gateway proxy being built in parallel:
     GATEWAY_MAILBOXES = 'api/gateway/mailboxes?domain=<domain>'
   On 404/501/503 or any failure the page shows an honest error notice with a
   Retry action — never fake rows. Basic actions work today:
   prepare + copy the address, open the mail UI. */
(async () => {
  'use strict';

  const GATEWAY_MAILBOXES = 'api/gateway/mailboxes';

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

  /* ---------------- status guard ---------------- */

  let status = await api('api/status');
  if (!status.data || !status.data.ok) {
    serviceDown(page);
    return;
  }
  status = status.data;

  if (!status.connected) {
    // The domain list needs the Namecheap session; route to login.
    location.replace('index.html');
    return;
  }

  /* topbar + mailcow links */
  $('#connected-as').textContent = status.username || '';
  if (status.mailcow_origin) {
    $('#admin-link').href = status.mailcow_origin + '/home';
    $('#mailcow-link').href = status.mailcow_origin + '/home';
    $('#open-mailcow').href = status.mailcow_origin + '/home';
  }

  /* ---------------- domains + per-domain state ---------------- */

  let domains = [];

  async function loadDomains() {
    const res = await api('api/domains');
    if (!res.data || !res.data.ok) {
      domainSelect.replaceChildren();
      const opt = el('option', null, 'No connection');
      opt.value = '';
      domainSelect.appendChild(opt);
      showMsg($('#state-error'), (res.data && res.data.error) || 'Could not load domains \u2014 reconnect the Namecheap API key.');
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
    $('#state-error').hidden = true;
  }

  async function loadState(domain, rec) {
    $('#state-meta').textContent = domain;
    $('#kv-registrar').replaceChildren(
      rec.expired ? badge('expired', 'warn') : rec.locked ? badge('locked', 'info') : badge('active', 'ok')
    );
    $('#kv-expires').textContent = rec.expires || 'unknown';

    // DNS record count from the existing endpoint
    $('#kv-dns').textContent = '\u2026';
    const res = await api('api/dns?domain=' + encodeURIComponent(domain));
    if (res.data && res.data.ok) {
      $('#kv-dns').textContent = String((res.data.hosts || []).length);
    } else {
      $('#kv-dns').textContent = '\u2013';
    }
  }

  /* ---------------- mailbox list (live via the API gateway) ---------------- */

  async function loadMailboxes(domain) {
    $('#list-meta').textContent = domain;
    const note = $('#list-note');
    const out = $('#list-output');
    out.replaceChildren();

    const res = await api(GATEWAY_MAILBOXES + '?domain=' + encodeURIComponent(domain));
    if (res.data && res.data.ok && Array.isArray(res.data.mailboxes)) {
      note.hidden = true;
      const mailboxes = res.data.mailboxes;
      if (!mailboxes.length) {
        out.appendChild(el('p', 'empty-state', 'No mailboxes on this domain yet \u2014 create the first one in the mail UI.'));
        return;
      }
      out.appendChild(table(
        'Mailboxes on ' + domain,
        [{ label: 'Address' }, { label: 'Quota used', num: true }, { label: 'Messages', num: true },
         { label: 'State' }, { label: 'Last login' }],
        mailboxes.map((m) => [
          { text: m.address || m.username || '' },
          { text: m.quota_used != null ? m.quota_used : '', num: true },
          { text: m.messages != null ? m.messages : '', num: true },
          { node: m.active === false ? badge('inactive', 'warn') : badge('active', 'ok') },
          { text: m.last_login || 'never' },
        ])
      ));
      return;
    }
    // gateway error (404/501/503) or any failure: honest notice + retry, no fake rows
    note.replaceChildren();
    note.appendChild(document.createTextNode('Could not load the mailbox listing right now. Today mailbox operations also run agent-side via MCP \u2014 see the Connectors page for the setup. '));
    const retry = el('button', 'btn small-btn', 'Retry');
    retry.type = 'button';
    retry.addEventListener('click', () => loadMailboxes(domain));
    note.appendChild(retry);
    note.hidden = false;
  }

  /* ---------------- basic actions (work today) ---------------- */

  const aliasInput = $('#mb-alias');
  const previewInput = $('#mb-preview');

  function currentDomain() {
    const idx = domainSelect.value;
    return idx !== '' && domains[idx] ? domains[idx].name : '';
  }

  function updatePreview() {
    const d = currentDomain();
    previewInput.value = d ? aliasInput.value.trim() + '@' + d : '';
  }

  aliasInput.addEventListener('input', updatePreview);

  $('#btn-copy-address').addEventListener('click', async () => {
    updatePreview();
    if (!previewInput.value) return;
    try {
      await navigator.clipboard.writeText(previewInput.value);
      showMsg($('#copy-ok'), 'Address copied \u2014 paste it in the mail UI to create the mailbox.');
    } catch (err) {
      showMsg($('#copy-ok'), 'Copy failed \u2014 select the preview field and copy manually.');
    }
    setTimeout(() => { $('#copy-ok').hidden = true; }, 4000);
  });

  domainSelect.addEventListener('change', () => {
    const d = currentDomain();
    if (!d) return;
    loadState(d, domains[Number(domainSelect.value)]);
    loadMailboxes(d);
    updatePreview();
  });

  $('#btn-refresh').addEventListener('click', async () => {
    const btn = $('#btn-refresh');
    btn.disabled = true;
    await loadDomains();
    btn.disabled = false;
  });

  /* ---------------- entry point ---------------- */

  await loadDomains();
  if (domains.length) {
    const d = domains[0].name;
    await loadState(d, domains[0]);
    loadMailboxes(d);
    updatePreview();
  } else {
    $('#state-meta').textContent = 'no domains';
  }
})();
