(async () => {
  const $ = (sel, el = document) => el.querySelector(sel);

  let status;
  try {
    status = await fetch('/api/status.php').then((r) => r.json());
  } catch (e) {
    location.replace('index.html');
    return;
  }
  if (!status.ok || !status.connected) {
    location.replace('index.html');
    return;
  }

  const csrf = status.csrf;
  const endpoint = location.origin + '/api/mcp.php';

  $('#connected-as').textContent = 'Connected as ' + status.username + (status.sandbox ? ' (sandbox)' : '');
  $('#admin-link').href = status.mailcow_origin ? status.mailcow_origin + '/' : '#';
  $('#mcp-endpoint').textContent = endpoint;

  const config = JSON.stringify({
    mcpServers: {
      'multi-ai-inbox': {
        type: 'http',
        url: endpoint,
        headers: { Authorization: 'Bearer <MAILCOW_API_KEY>' },
      },
    },
  }, null, 2);
  $('#claude-config').textContent = config;

  document.querySelectorAll('[data-copy]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = $(btn.dataset.copy);
      if (!target) return;
      navigator.clipboard.writeText(target.textContent).then(() => {
        const prev = btn.textContent;
        btn.textContent = 'Copied';
        setTimeout(() => { btn.textContent = prev; }, 1200);
      });
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
      res = await fetch('/api/domains.php').then((r) => r.json());
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
      res = await fetch('/api/dns.php?domain=' + encodeURIComponent(domain)).then((r) => r.json());
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
    const body = new URLSearchParams();
    body.set('csrf', csrf);
    await fetch('/api/disconnect.php', {
      method: 'POST',
      body,
      headers: { 'X-Landing-CSRF': csrf },
    }).catch(() => null);
    location.href = 'index.html';
  });
})();
