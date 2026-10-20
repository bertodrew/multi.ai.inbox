/* onboarding.js — AImulti.inbox setup wizard (3 steps)
   Consumes the EXISTING landing API only:
     GET  api/status     — session, csrf, mailcow_origin
     POST api/connect    — Namecheap API key (form-encoded + X-Landing-CSRF)
     GET  api/domains    — Namecheap domain list (requires the session)
     GET  api/dns        — DNS hosts for one domain (requires the session)
   No gateway endpoints are called here: the wizard only walks what exists today.
   Backend-down is a first-class state (UX review A2): in-block message with a
   Retry action — never a blind redirect and never an operations-only message. */
(async () => {
  'use strict';

  const $ = (sel, el = document) => el.querySelector(sel);

  /* fetch wrapper: JSON responses only; HTML error pages (nginx 502) and
     network failures both resolve to ok:false instead of throwing */
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

  function showMsg(el, text) { el.textContent = text; el.hidden = false; }

  function serviceDown(container) {
    container.replaceChildren();
    const card = document.createElement('section');
    card.className = 'card';
    const h = document.createElement('h2');
    h.textContent = 'Service temporarily unavailable';
    const p = document.createElement('p');
    p.textContent = 'The landing API is not responding. This is usually brief \u2014 wait a few minutes and retry.';
    const actions = document.createElement('div');
    actions.className = 'form-actions';
    const retry = document.createElement('button');
    retry.className = 'btn primary';
    retry.type = 'button';
    retry.textContent = 'Retry';
    retry.addEventListener('click', () => location.reload());
    actions.appendChild(retry);
    card.append(h, p, actions);
    container.appendChild(card);
  }

  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  /* tables render inside .table-scroll (390px scrolls inside the frame) */
  function table(captionText, headers, rows) {
    const scroll = el('div', 'table-scroll');
    const t = el('table');
    const cap = el('caption', 'sr-only', captionText);
    t.appendChild(cap);
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

  /* ---------------- wizard state ---------------- */

  const wizard = $('#wizard');

  function showStep(n) {
    for (let i = 1; i <= 3; i++) {
      $('#step-' + i).hidden = i !== n;
      const ind = $('#step-ind-' + i);
      ind.classList.toggle('done', i < n);
      if (i === n) ind.setAttribute('aria-current', 'step');
      else ind.removeAttribute('aria-current');
    }
    const heading = $('#step-' + n + '-title');
    if (heading) heading.focus();
  }

  /* ---------------- status guard ---------------- */

  let status = await api('api/status');
  if (!status.data || !status.data.ok) {
    serviceDown(wizard);
    return;
  }
  status = status.data;

  /* topbar: mail UI link */
  const adminLink = $('#admin-link');
  if (status.mailcow_origin) adminLink.href = status.mailcow_origin + '/home';

  /* ---------------- environment choice (persisted per account) ---------------- */

  const envFieldset = $('#env-choice fieldset.env-pick');
  if (envFieldset) {
    let envState = envFieldset.nextElementSibling;
    if (!envState || !envState.classList.contains('env-save-state')) {
      envState = el('p', 'hint env-save-state', '');
      envFieldset.after(envState);
    }
    // saved choice pre-selected on load
    api('api/env').then((res) => {
      if (res.data && res.data.ok && res.data.env) {
        const want = envFieldset.querySelector('input[name="env"][value="' + res.data.env + '"]');
        if (want) {
          envFieldset.querySelectorAll('input[name="env"]').forEach((i) => { i.checked = i === want; });
          envState.textContent = 'Saved: ' + res.data.env + '.';
        }
      }
    });
    envFieldset.addEventListener('change', async (e) => {
      if (e.target.name !== 'env') return;
      envState.textContent = 'Saving\u2026';
      const res = await api('api/env', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Landing-CSRF': status.csrf || '' },
        body: JSON.stringify({ env: e.target.value }),
      });
      if (res.httpOk && res.data && res.data.ok) {
        envState.textContent = 'Saved: ' + res.data.env + '.';
      } else if (res.status === 401) {
        envState.textContent = 'Connect your API key to save this choice.';
      } else {
        envState.textContent = (res.data && res.data.error) || 'Could not save the choice \u2014 try again.';
      }
    });
  }

  /* ---------------- step 1 — mailcow origin ---------------- */

  $('#origin-value').textContent = status.mailcow_origin || 'not configured';
  $('#api-state').textContent = 'reachable';
  $('#origin-state').textContent = status.mailcow_origin ? 'origin resolved' : 'origin missing';
  if (status.mailcow_origin) $('#open-mailcow').href = status.mailcow_origin + '/home';

  $('#to-step-2').addEventListener('click', () => showStep(2));

  /* ---------------- step 2 — connect Namecheap ---------------- */

  const ncError = $('#nc-error');
  const ncSubmit = $('#nc-submit');

  $('#nc-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    ncError.hidden = true;

    const body = new URLSearchParams();
    body.set('username', $('#nc-username').value.trim());
    body.set('api_key', $('#nc-api-key').value.trim());
    body.set('client_ip', '');
    body.set('use_sandbox', $('#nc-sandbox').checked ? '1' : '0');
    body.set('csrf', status.csrf || '');

    ncSubmit.disabled = true;
    ncSubmit.textContent = 'Connecting\u2026';
    const res = await api('api/connect', {
      method: 'POST',
      body,
      headers: { 'X-Landing-CSRF': status.csrf || '' },
    });
    ncSubmit.disabled = false;
    ncSubmit.textContent = 'Connect Namecheap';

    if (!res.data || !res.data.ok) {
      showMsg(ncError, (res.data && res.data.error) || 'Connection failed \u2014 the landing API did not answer.');
      return;
    }
    await loadDomains();
    showStep(3);
  });

  $('#skip-connect').addEventListener('click', () => {
    showStep(3);
    loadDomains(); // will surface the honest 401 state if skipped without a key
  });

  /* ---------------- step 3 — first domain ---------------- */

  const domainSelect = $('#domain-select');
  const step3Error = $('#step3-error');
  const inspectBtn = $('#inspect-dns');
  let domains = [];

  async function loadDomains() {
    const res = await api('api/domains');
    if (!res.data || !res.data.ok) {
      domainSelect.replaceChildren();
      const opt = el('option', null, 'No connection');
      opt.value = '';
      domainSelect.appendChild(opt);
      showMsg(step3Error, (res.data && res.data.error) || 'Could not load domains \u2014 connect a Namecheap API key in step 2.');
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
    step3Error.hidden = true;
  }

  function dnsTable(hosts) {
    return table(
      'DNS records',
      [{ label: 'Host' }, { label: 'Type' }, { label: 'Address' },
       { label: 'MX pref', num: true }, { label: 'TTL', num: true }],
      hosts.map((r) => [
        { text: r.name }, { text: r.type }, { text: r.address },
        { text: r.mx_pref, num: true }, { text: r.ttl, num: true },
      ])
    );
  }

  inspectBtn.addEventListener('click', async () => {
    step3Error.hidden = true;
    const idx = domainSelect.value;
    if (idx === '' || !domains[idx]) {
      showMsg(step3Error, 'Pick a domain first \u2014 or connect an API key in step 2.');
      return;
    }
    const domain = domains[idx].name;
    inspectBtn.disabled = true;
    inspectBtn.textContent = 'Loading\u2026';
    const res = await api('api/dns?domain=' + encodeURIComponent(domain));
    inspectBtn.disabled = false;
    inspectBtn.textContent = 'Inspect DNS';

    if (!res.data || !res.data.ok) {
      showMsg(step3Error, (res.data && res.data.error) || 'Failed to load DNS records.');
      return;
    }
    const out = $('#dns-output');
    out.replaceChildren();
    $('#dns-title').textContent = 'DNS records \u2014 ' + res.data.domain;
    if (!res.data.hosts.length) {
      const p = el('p', 'empty-state', 'No host records \u2014 the domain may use other nameservers.');
      out.appendChild(p);
    } else {
      out.appendChild(dnsTable(res.data.hosts));
    }
    $('#dns-result').hidden = false;
    $('#next-steps').hidden = false;
  });

  /* ---------------- entry point ---------------- */

  if (status.connected) {
    $('#origin-state').textContent = 'already connected';
    await loadDomains();
    showStep(3);
  } else {
    showStep(1);
  }
})();
