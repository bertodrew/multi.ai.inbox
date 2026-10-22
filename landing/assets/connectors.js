(async () => {
  const $ = (sel, el = document) => el.querySelector(sel);

  let status;
  try {
    status = await fetch('/landingpage/api/status').then((r) => r.json());
  } catch (e) {
    location.replace('index.html');
    return;
  }
  if (!status || !status.ok) {
    location.replace('index.html');
    return;
  }

  /* Admin gating: the Admin and Dashboard nav links stay hidden by default and
     are shown only for admin sessions — every other nav entry stays public. */
  if (status.admin === true) {
    const na = $('#nav-admin');
    const nd = $('#nav-dashboard');
    if (na) na.hidden = false;
    if (nd) nd.hidden = false;
  }

  const csrf = status.csrf;
  const endpoint = location.origin + '/landingpage/api/mcp';

  // M4 fix: error box and session status announced to screen readers
  const domainsError = $('#domains-error');
  if (domainsError) domainsError.setAttribute('role', 'alert');
  const connectedAs = $('#connected-as');
  if (connectedAs) connectedAs.setAttribute('aria-live', 'polite');

  /* Connect registrar panel: visible only while NOT connected. Connected
     sessions keep the existing page (Browse domains + MCP) with the panel
     hidden; without a session the page shows the connect form instead. */
  const ncPanel = $('#nc-panel');
  const ncError = $('#nc-error');
  if (ncError) ncError.setAttribute('role', 'alert');
  if (status.connected) {
    if (ncPanel) ncPanel.hidden = true;
  } else {
    if (ncPanel) ncPanel.hidden = false;
    const browse = $('#btn-load-domains');
    if (browse) browse.hidden = true; // needs the Namecheap session
    const disc = $('#btn-disconnect');
    if (disc) disc.hidden = true; // nothing to disconnect yet
    wireConnectForm(csrf);
  }

  if (status.connected) {
    $('#connected-as').textContent = 'Connected as ' + status.username + (status.sandbox ? ' (sandbox)' : '');
  }
  $('#mcp-endpoint').textContent = endpoint;

  const config = JSON.stringify({
    mcpServers: {
      'multi-ai-inbox': {
        type: 'http',
        url: endpoint,
        headers: { Authorization: 'Bearer <read-write API key>' },
      },
    },
  }, null, 2);
  $('#claude-config').textContent = config;

  document.querySelectorAll('[data-copy]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = $(btn.dataset.copy);
      if (!target) return;
      const prev = btn.textContent;
      const flash = (label) => {
        btn.textContent = label;
        setTimeout(() => { btn.textContent = prev; }, 1200);
      };
      const text = target.textContent;
      const fallback = () => {
        // clipboard API unavailable or denied: transient textarea + execCommand, as in app.js
        try {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.setAttribute('readonly', '');
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.select();
          const ok = document.execCommand('copy');
          ta.remove();
          flash(ok ? 'Copied' : 'Copy failed');
        } catch (e) {
          flash('Copy failed');
        }
      };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(() => flash('Copied')).catch(fallback);
      } else {
        fallback();
      }
    });
  });

  function table(headers, rows, rowClass) {
    const t = document.createElement('table');
    const thead = document.createElement('thead');
    const trh = document.createElement('tr');
    headers.forEach((h) => {
      const th = document.createElement('th');
      th.textContent = h;
      trh.appendChild(th);
    });
    thead.appendChild(trh);
    t.appendChild(thead);
    const tbody = document.createElement('tbody');
    rows.forEach((cells) => {
      const tr = document.createElement('tr');
      if (rowClass) tr.className = rowClass;
      cells.forEach((c) => {
        const td = document.createElement('td');
        if (c.node) td.appendChild(c.node);
        else td.textContent = c;
        tr.appendChild(td);
      });
      tbody.appendChild(tr);
    });
    t.appendChild(tbody);
    return t;
  }

  function badge(text, cls) {
    const b = document.createElement('span');
    b.className = 'badge' + (cls ? ' ' + cls : '');
    b.textContent = text;
    return b;
  }

  function showError(el, msg) {
    el.textContent = msg;
    el.hidden = false;
  }

  /* Connect registrar form → POST /api/connect (form-encoded + X-Landing-CSRF).
     Honest states: button disabled + "Connecting…" while the key is validated,
     the backend error verbatim in nc-error on failure; on success the session
     is rotated server-side, so the page reloads and re-reads the status with
     the new session (form hidden, connected page shown). */
  function wireConnectForm(csrfToken) {
    const form = $('#nc-form');
    const submit = $('#nc-submit');
    const err = $('#nc-error');
    if (!form || !submit || !err) return;
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      err.hidden = true;

      const body = new URLSearchParams();
      body.set('username', $('#nc-username').value.trim());
      body.set('api_key', $('#nc-api-key').value.trim());
      body.set('client_ip', $('#nc-client-ip').value.trim());
      body.set('use_sandbox', $('#nc-sandbox').checked ? '1' : '0');
      body.set('csrf', csrfToken);

      submit.disabled = true;
      submit.textContent = 'Connecting…';
      let res;
      try {
        res = await fetch('/landingpage/api/connect', {
          method: 'POST',
          body,
          headers: { 'X-Landing-CSRF': csrfToken },
        }).then((r) => r.json());
      } catch (e2) {
        res = { ok: false, error: 'Network error: ' + e2.message };
      }

      if (res.ok) {
        location.reload(); // session rotated server-side: re-read the status
        return;
      }
      submit.disabled = false;
      submit.textContent = 'Connect registrar';
      err.textContent = res.error || 'Connection failed';
      err.hidden = false;
    });
  }

  $('#btn-load-domains').addEventListener('click', async () => {
    const btn = $('#btn-load-domains');
    const err = $('#domains-error');
    const out = $('#domains-output');
    const dnsOut = $('#dns-output');
    err.hidden = true;
    dnsOut.hidden = true;
    btn.disabled = true;
    btn.textContent = 'Loading…';
    let res;
    try {
      res = await fetch('/landingpage/api/domains').then((r) => r.json());
    } catch (e) {
      res = { ok: false, error: 'Network error: ' + e.message };
    }
    btn.disabled = false;
    btn.textContent = 'Browse domains';
    if (!res.ok) { showError(err, res.error || 'Failed to load domains'); return; }

    out.replaceChildren();
    out.hidden = false;
    if (!res.domains.length) {
      const p = document.createElement('p');
      p.className = 'hint';
      p.textContent = 'No domains found in this Namecheap account.';
      out.appendChild(p);
      return;
    }
    out.appendChild(table(
      ['Domain', 'Expires', 'Status'],
      res.domains.map((d) => [
        d.name,
        d.expires,
        { node: d.expired ? badge('expired', 'warn') : d.locked ? badge('locked') : badge('active', 'ok') },
      ]),
      'domain-row'
    ));
    const p = document.createElement('p');
    p.className = 'hint';
    p.textContent = 'Click a domain to inspect its DNS records.';
    out.appendChild(p);

    out.querySelectorAll('tr.domain-row').forEach((tr, i) => {
      tr.addEventListener('click', () => {
        out.querySelectorAll('tr.domain-row').forEach((r) => r.classList.remove('selected'));
        tr.classList.add('selected');
        loadDns(res.domains[i].name);
      });
    });
  });

  async function loadDns(domain) {
    const dnsOut = $('#dns-output');
    dnsOut.hidden = true;
    let res;
    try {
      res = await fetch('/landingpage/api/dns?domain=' + encodeURIComponent(domain)).then((r) => r.json());
    } catch (e) {
      res = { ok: false, error: 'Network error: ' + e.message };
    }
    if (!res.ok) { showError($('#domains-error'), res.error || 'Failed to load DNS records'); return; }

    dnsOut.replaceChildren();
    dnsOut.hidden = false;
    const h = document.createElement('p');
    h.className = 'table-title';
    h.textContent = 'DNS records — ' + res.domain;
    dnsOut.appendChild(h);
    if (!res.hosts.length) {
      const p = document.createElement('p');
      p.className = 'hint';
      p.textContent = 'No host records (the domain may use other nameservers).';
      dnsOut.appendChild(p);
      return;
    }
    dnsOut.appendChild(table(
      ['Host', 'Type', 'Address', 'MX pref', 'TTL'],
      res.hosts.map((r) => [r.name, r.type, r.address, r.mx_pref, r.ttl])
    ));
  }

  $('#btn-disconnect').addEventListener('click', async () => {
    // M12 fix: destructive action confirmed before credentials are removed
    if (!window.confirm('Disconnect Namecheap? The stored API key will be removed from this session. You can reconnect at any time.')) return;
    const body = new URLSearchParams();
    body.set('csrf', csrf);
    await fetch('/landingpage/api/disconnect', {
      method: 'POST',
      body,
      headers: { 'X-Landing-CSRF': csrf },
    }).catch(() => null);
    location.href = 'index.html';
  });
})();
