/* 502.js — dedicated error page (served by nginx on 502/503/504).
   CSP is script-src 'self': no inline handlers — the Retry action is wired
   here and simply reloads the page the user was trying to reach. */
(function () {
  'use strict';
  var btn = document.getElementById('btn-retry');
  if (btn) btn.addEventListener('click', function () { location.reload(); });
})();
