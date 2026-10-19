(async () => {
  const $ = (sel, el = document) => el.querySelector(sel);

  let status = null;
  try {
    status = await fetch('api/status.php').then((r) => r.json());
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

  if (origin) {
    mailcowForm.action = origin + '/';
    $('#admin-link').href = origin + '/';
  } else {
    const submit = mailcowForm.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    mailcowHint.textContent = 'Mailcow origin unknown: set MAILCOW_UI_ORIGIN in mailcow.conf and restart the landing services.';
  }

  if (status && status.ok && status.public_ip) {
    $('#nc-client-ip').value = status.public_ip;
    $('#nc-ip-hint').innerHTML =
      'Whitelist this server IP in Namecheap &rarr; Profile &rarr; Tools &rarr; API Access: ' +
      '<code>' + status.public_ip + '</code>';
  } else {
    $('#nc-ip-hint').textContent =
      'Could not auto-detect the server IP: open the Namecheap panel and add this server IP under Profile > Tools > API Access.';
  }

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
      res = await fetch('api/connect.php', {
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
})();
