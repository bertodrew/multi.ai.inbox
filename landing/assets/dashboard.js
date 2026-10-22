/* dashboard.js — founder control dashboard (read-only)
   Consumes the landing API contract only (the admin endpoints are being
   built in parallel):
     GET /landingpage/api/status        → {ok, connected, username, csrf, admin}
     GET /landingpage/api/admin/metrics → {ok, mail:{domains, mailboxes,
         mailboxes_by_domain:[{domain, mailboxes}]}, sessions:{active},
         provisioning:{enabled, pending, states:[…]},
         audit:{recent:[{ts, event, detail}] max 20}}
     GET /landingpage/api/admin/infra   → {ok, uptime_s,
         disk:{used_gb, total_gb, pct}, memory:{used_mb, total_mb, pct},
         services:[{name, state, health}]}
   Both admin endpoints answer 403 {error:"Admin session required"} for
   non-admin sessions: the page shows the honest not-authorized gate and
   never fetches data. Auto-refresh every 15s, paused while the tab is
   hidden (visibilitychange). Honest states only: skeleton → data → error
   with Retry — never fake rows. */
(async () => {
  'use strict';

  const REFRESH_MS = 15000;
  const STAT_IDS = ['stat-domains', 'stat-mailboxes', 'stat-sessions', 'stat-pending'];

  const $ = (sel, root = document) => root.querySelector(sel);

  const els = {
    dashRoot: $('#dash-root'),
    gate: $('#not-authorized'),
    downGate: $('#down-gate'),
    mailOutput: $('#mail-output'),
    mailError: $('#mail-error'),
    mailMeta: $('#mail-meta'),
    infraOutput: $('#infra-output'),
    infraError: $('#infra-error'),
    infraMeta: $('#infra-meta'),
    auditOutput: $('#audit-output'),
    auditError: $('#audit-error'),
    auditMeta: $('#audit-meta'),
  };

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

  function showMsg(node, text) { node.textContent = text; node.hidden = false; }
  function hideMsg(node) { node.hidden = true; }

  function skeleton(lines) {
    const sk = el('div', 'skeleton');
    sk.setAttribute('aria-hidden', 'true');
    for (let i = 0; i < lines; i++) sk.appendChild(el('span'));
    return sk;
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

  /* service-health badges: running → accent (ok), healthy/unknown → blue
     (info), failed/error/down → --warn */
  function healthBadge(health) {
    const h = String(health === undefined || health === null || health === '' ? 'unknown' : health).toLowerCase();
    if (h === 'running') return badge(h, 'ok');
    if (h === 'failed' || h === 'error' || h === 'down') return badge(h, 'warn');
    return badge(h, 'info');
  }

  /* ---------- formatting ---------- */

  function fmtUptime(s) {
    const n = Number(s);
    if (!Number.isFinite(n) || n < 0) return '\u2013';
    const d = Math.floor(n / 86400);
    const h = Math.floor((n % 86400) / 3600);
    if (d > 0) return d + 'd ' + h + 'h';
    if (h > 0) return h + 'h';
    return Math.floor((n % 3600) / 60) + 'm';
  }

  function fmtGb(v) { const n = Number(v); return Number.isFinite(n) ? n.toFixed(1) + ' GB' : '\u2013'; }
  function fmtMb(v) { const n = Number(v); return Number.isFinite(n) ? n.toFixed(1) + ' MB' : '\u2013'; }
  function fmtPct(v) { const n = Number(v); return Number.isFinite(n) ? Math.round(n) + '%' : '\u2013'; }
  function fmtInt(v) { const n = Number(v); return Number.isFinite(n) ? String(n) : '\u2013'; }

  function fmtTs(t) {
    if (t === null || t === undefined || t === '') return '';
    const n = Number(t);
    if (Number.isFinite(n) && String(t).trim() !== '') {
      const d = new Date(n < 1e12 ? n * 1000 : n);
      if (!Number.isNaN(d.getTime())) return d.toLocaleString();
    }
    const d = new Date(t);
    if (!Number.isNaN(d.getTime())) return d.toLocaleString();
    return String(t);
  }

  /* ---------- render helpers ---------- */

  function setStatValue(id, text) {
    const node = document.getElementById(id);
    if (node) node.textContent = text;
  }

  function setStatLoading() {
    STAT_IDS.forEach((id) => {
      const node = document.getElementById(id);
      if (!node) return;
      const sk = el('span', 'skeleton-line');
      sk.setAttribute('aria-hidden', 'true');
      node.replaceChildren(sk, el('span', 'sr-only', 'Loading\u2026'));
    });
  }

  function progressBar(label, pct) {
    const p = Math.max(0, Math.min(100, Math.round(Number(pct) || 0)));
    const bar = el('div', 'bar');
    bar.setAttribute('role', 'progressbar');
    bar.setAttribute('aria-label', label);
    bar.setAttribute('aria-valuemin', '0');
    bar.setAttribute('aria-valuemax', '100');
    bar.setAttribute('aria-valuenow', String(p));
    const fill = el('div', 'bar-fill');
    fill.style.width = p + '%'; // CSSOM only — no inline style attribute (CSP 'self')
    bar.appendChild(fill);
    return bar;
  }

  function infraRow(label, valueText, pct) {
    const row = el('div', 'infra-row');
    const head = el('div', 'infra-row-head');
    head.appendChild(el('span', 'k', label));
    head.appendChild(el('span', 'v num', valueText));
    row.appendChild(head);
    if (pct !== null && pct !== undefined) row.appendChild(progressBar(label, pct));
    return row;
  }

  function retryInto(outNode, loader) {
    outNode.replaceChildren();
    const actions = el('div', 'form-actions');
    const retry = el('button', 'btn primary small-btn', 'Retry');
    retry.type = 'button';
    retry.addEventListener('click', loader);
    actions.appendChild(retry);
    outNode.appendChild(actions);
  }

  /* ---------- full-page honest gates ---------- */

  let authorized = false;
  let refreshTimer = null;

  function stopRefresh() {
    if (refreshTimer !== null) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  }

  function showNotAuthorized() {
    authorized = false;
    stopRefresh();
    if (els.dashRoot) els.dashRoot.hidden = true;
    if (els.downGate) els.downGate.hidden = true;
    if (els.gate) els.gate.hidden = false;
  }

  function showDown() {
    authorized = false;
    stopRefresh();
    if (els.gate) els.gate.hidden = true;
    if (els.dashRoot) els.dashRoot.hidden = true;
    if (els.downGate) els.downGate.hidden = false;
  }

  const gateReload = $('#gate-reload');
  if (gateReload) gateReload.addEventListener('click', () => location.reload());
  const downRetry = $('#down-retry');
  if (downRetry) downRetry.addEventListener('click', () => location.reload());

  /* ---------- sections ---------- */

  function renderStats(metrics) {
    setStatValue('stat-domains', fmtInt(metrics.mail && metrics.mail.domains));
    setStatValue('stat-mailboxes', fmtInt(metrics.mail && metrics.mail.mailboxes));
    setStatValue('stat-sessions', fmtInt(metrics.sessions && metrics.sessions.active));
    setStatValue('stat-pending', fmtInt(metrics.provisioning && metrics.provisioning.pending));
  }

  function renderMail(metrics) {
    const byDomain = metrics.mail && Array.isArray(metrics.mail.mailboxes_by_domain)
      ? metrics.mail.mailboxes_by_domain : [];
    hideMsg(els.mailError);
    els.mailOutput.replaceChildren();
    els.mailMeta.textContent = byDomain.length ? String(byDomain.length) : '';
    if (!byDomain.length) {
      els.mailOutput.appendChild(el('p', 'empty-state', 'No domains provisioned yet'));
      return;
    }
    els.mailOutput.appendChild(table(
      'Mailboxes per domain',
      [{ label: 'Domain' }, { label: 'Mailboxes', num: true }],
      byDomain.map((r) => [
        { text: r && r.domain ? String(r.domain) : '' },
        { text: fmtInt(r && r.mailboxes), num: true },
      ])
    ));
  }

  function renderAudit(metrics) {
    const recent = metrics.audit && Array.isArray(metrics.audit.recent)
      ? metrics.audit.recent.slice(0, 20) : [];
    hideMsg(els.auditError);
    els.auditOutput.replaceChildren();
    els.auditMeta.textContent = recent.length ? recent.length + ' events' : '';
    if (!recent.length) {
      els.auditOutput.appendChild(el('p', 'empty-state', 'No audit entries yet.'));
      return;
    }
    els.auditOutput.appendChild(table(
      'Recent audit events',
      [{ label: 'When' }, { label: 'Event' }, { label: 'Detail' }],
      recent.map((r) => {
        const ev = r && r.event ? String(r.event) : '';
        const warn = /delete|remove|revoke/i.test(ev);
        return [
          { text: fmtTs(r && r.ts) },
          { node: ev ? badge(ev, warn ? 'warn' : 'info') : el('span') },
          { text: r && r.detail ? String(r.detail) : '' },
        ];
      })
    ));
  }

  function renderInfra(infra) {
    const disk = infra.disk || {};
    const memory = infra.memory || {};
    const services = Array.isArray(infra.services) ? infra.services : [];
    hideMsg(els.infraError);
    els.infraOutput.replaceChildren();
    els.infraMeta.textContent = services.length ? services.length + ' services' : '';
    els.infraOutput.appendChild(infraRow(
      'Disk',
      fmtGb(disk.used_gb) + ' / ' + fmtGb(disk.total_gb) + ' (' + fmtPct(disk.pct) + ')',
      disk.pct
    ));
    els.infraOutput.appendChild(infraRow(
      'Memory',
      fmtMb(memory.used_mb) + ' / ' + fmtMb(memory.total_mb) + ' (' + fmtPct(memory.pct) + ')',
      memory.pct
    ));
    els.infraOutput.appendChild(infraRow('Uptime', fmtUptime(infra.uptime_s), null));

    const servicesBlock = el('div', 'infra-services');
    servicesBlock.appendChild(el('h3', 'sub-title', 'Services'));
    if (!services.length) {
      servicesBlock.appendChild(el('p', 'empty-state', 'No services reported.'));
    } else {
      servicesBlock.appendChild(table(
        'Service states',
        [{ label: 'Name' }, { label: 'State' }, { label: 'Health' }],
        services.map((s) => [
          { text: s && s.name ? String(s.name) : '' },
          { text: s && s.state ? String(s.state) : '' },
          { node: healthBadge(s && s.health) },
        ])
      ));
    }
    els.infraOutput.appendChild(servicesBlock);
  }

  async function loadMetrics(opts) {
    const silent = opts && opts.silent;
    if (!silent) {
      hideMsg(els.mailError);
      hideMsg(els.auditError);
      setStatLoading();
      els.mailOutput.replaceChildren(skeleton(4));
      els.auditOutput.replaceChildren(skeleton(4));
      els.mailMeta.textContent = '';
      els.auditMeta.textContent = '';
    }
    const res = await api('api/admin/metrics');
    if (res.status === 403) { showNotAuthorized(); return; }
    if (!res.data || !res.data.ok) {
      const msg = (res.data && res.data.error) ||
        'Could not load mail metrics \u2014 the admin API is not responding.';
      STAT_IDS.forEach((id) => setStatValue(id, '\u2013'));
      showMsg(els.mailError, msg);
      retryInto(els.mailOutput, loadMetrics);
      showMsg(els.auditError, msg);
      retryInto(els.auditOutput, loadMetrics);
      return;
    }
    renderStats(res.data);
    renderMail(res.data);
    renderAudit(res.data);
  }

  async function loadInfra(opts) {
    const silent = opts && opts.silent;
    if (!silent) {
      hideMsg(els.infraError);
      els.infraOutput.replaceChildren(skeleton(3));
      els.infraMeta.textContent = '';
    }
    const res = await api('api/admin/infra');
    if (res.status === 403) { showNotAuthorized(); return; }
    if (!res.data || !res.data.ok) {
      showMsg(els.infraError, (res.data && res.data.error) ||
        'Could not load infrastructure state \u2014 the admin API is not responding.');
      retryInto(els.infraOutput, loadInfra);
      return;
    }
    renderInfra(res.data);
  }

  function loadAll(silent) {
    loadMetrics({ silent });
    loadInfra({ silent });
  }

  /* ---------- auto-refresh: 15s, paused while the tab is hidden ---------- */

  function startRefresh() {
    stopRefresh();
    refreshTimer = setInterval(() => {
      if (!document.hidden) loadAll(true);
    }, REFRESH_MS);
  }

  document.addEventListener('visibilitychange', () => {
    if (!authorized) return;
    if (document.hidden) {
      stopRefresh(); // honest pause: no polling while the tab is hidden
    } else {
      loadAll(true);
      startRefresh();
    }
  });

  /* ---------- entry point: status guard ---------- */

  const statusRes = await api('api/status');
  if (!statusRes.data || !statusRes.data.ok) {
    showDown();
    return;
  }
  const status = statusRes.data;

  const connectedAs = $('#connected-as');
  if (connectedAs) connectedAs.textContent = status.username || '';

  if (!status.admin) {
    // Fail-closed: no admin session → honest gate, no data is fetched.
    showNotAuthorized();
    return;
  }

  authorized = true;
  loadAll(false); // first paint: skeleton → data
  startRefresh();
})();
