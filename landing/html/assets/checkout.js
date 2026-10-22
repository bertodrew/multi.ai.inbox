// checkout.js — wire the tier Subscribe buttons on the pricing page:
// POST api/checkout {plan} with the session cookie + CSRF, then redirect to
// the Mollie hosted checkout. 401 → onboarding (sign in first); 501 → honest
// inline message (payments not configured on this server yet). No framework,
// CSP-safe (script-src 'self').
(function () {
  'use strict';
  var msg = document.getElementById('checkout-msg');

  function say(text) {
    if (!msg) { return; }
    msg.textContent = text;
    msg.hidden = false;
  }

  function wire(btn) {
    btn.addEventListener('click', function () {
      var plan = btn.getAttribute('data-checkout');
      btn.disabled = true;
      fetch('/landingpage/api/status', { credentials: 'same-origin' })
        .then(function (r) { return r.json(); })
        .then(function (status) {
          return fetch('/landingpage/api/checkout', {
            method: 'POST',
            credentials: 'same-origin',
            headers: {
              'Content-Type': 'application/json',
              'X-Landing-CSRF': status && status.csrf ? status.csrf : ''
            },
            body: JSON.stringify({ plan: plan })
          });
        })
        .then(function (res) {
          return res.json().catch(function () { return {}; }).then(function (data) {
            return { res: res, data: data };
          });
        })
        .then(function (out) {
          if (out.res.ok && out.data && out.data.checkout_url) {
            window.location.href = out.data.checkout_url;
            return;
          }
          if (out.res.status === 401) {
            window.location.href = 'onboarding.html';
            return;
          }
          say((out.data && out.data.error) || 'Checkout unavailable — try again.');
        })
        .catch(function () {
          say('Checkout unavailable — check your connection and try again.');
        })
        .then(function () { btn.disabled = false; });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () {
      Array.prototype.forEach.call(document.querySelectorAll('[data-checkout]'), wire);
    });
  } else {
    Array.prototype.forEach.call(document.querySelectorAll('[data-checkout]'), wire);
  }
})();
