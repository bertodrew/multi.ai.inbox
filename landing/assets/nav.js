/* nav.js — mobile topbar toggle (hamburger).
   Row 1 of the bar (brand + toggle) stays one row on mobile; the nav opens
   as a full-width drawer below it. Desktop ≥ 561px: toggle hidden, nav inline. */
(() => {
  'use strict';

  const btn = document.getElementById('bar-toggle');
  const header = btn ? btn.closest('.topbar') : null;
  if (!btn || !header) return;

  const setOpen = (open) => {
    header.classList.toggle('nav-open', open);
    btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    btn.setAttribute('aria-label', open ? 'Close menu' : 'Open menu');
  };

  btn.addEventListener('click', () => {
    setOpen(!header.classList.contains('nav-open'));
  });

  // Follow a link from the drawer → close it (topbar resets for the next page).
  header.addEventListener('click', (e) => {
    if (e.target.closest('a')) setOpen(false);
  });

  // Esc closes the drawer (same convention as the sign-in dialog).
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') setOpen(false);
  });
})();
