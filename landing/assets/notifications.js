/* notifications.js — corner bell + notifications dropmenu.
   Adapted from a shadcn React notifications block to this static site's
   vanilla JS/CSS. Self-mounts: injects its stylesheet, builds the bell in the
   topbar corner (always visible, next to the menu toggle) and the dropmenu.
   Honest states: the panel is empty until the platform records events — no
   demo data. Read state is client-side (localStorage); production can move it
   server-side without changing the fetch contract. */
(async () => {
  'use strict';

  const API = '/landingpage/api/notifications';
  const READ_KEY = 'multiai.notif.read';
  const POLL_MS = 30000;

  // Stylesheet (CSP style-src 'self' — <link>, no inline <style> blocks).
  if (!document.querySelector('link[href$="notifications.css"]')) {
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    const src = document.currentScript ? document.currentScript.src : 'assets/notifications.js';
    link.href = src.replace(/notifications\.js.*$/, 'notifications.css');
    document.head.appendChild(link);
  }

  const bar = document.querySelector('.topbar .bar-inner');
  if (!bar || document.getElementById('notif-bell')) return;

  // --- Bell in the topbar corner (next to the menu toggle) ---
  const toggle = document.getElementById('bar-toggle');
  const bell = document.createElement('button');
  bell.id = 'notif-bell';
  bell.className = 'notif-bell';
  bell.type = 'button';
  bell.setAttribute('aria-haspopup', 'true');
  bell.setAttribute('aria-expanded', 'false');
  bell.setAttribute('aria-controls', 'notif-drop');
  bell.setAttribute('aria-label', 'Notifications');
  bell.innerHTML =
    '<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" ' +
    'stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
    '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/>' +
    '<path d="M13.7 21a2 2 0 0 1-3.4 0"/></svg>' +
    '<span class="notif-badge" id="notif-badge" hidden></span>';
  bar.insertBefore(bell, toggle || null);

  const badge = bell.querySelector('#notif-badge');

  // --- Dropmenu panel ---
  const drop = document.createElement('div');
  drop.id = 'notif-drop';
  drop.className = 'notif-drop';
  drop.setAttribute('role', 'dialog');
  drop.setAttribute('aria-label', 'Notifications panel');
  drop.hidden = true;
  drop.innerHTML =
    '<div class="notif-head">' +
    '<h2 class="notif-title">Notifications <span class="notif-count" id="notif-unread-head" hidden></span></h2>' +
    '<button type="button" class="notif-markall" id="notif-markall">Mark all read</button>' +
    '</div>' +
    '<div class="notif-tabs" role="tablist" aria-label="Notification filters">' +
    '<button type="button" class="notif-tab" data-filter="all" role="tab" aria-selected="true">All <span class="n" data-count>0</span></button>' +
    '<button type="button" class="notif-tab" data-filter="unread" role="tab" aria-selected="false">Unread <span class="n" data-count>0</span></button>' +
    '<button type="button" class="notif-tab" data-filter="mentions" role="tab" aria-selected="false">Mentions <span class="n" data-count>0</span></button>' +
    '</div>' +
    '<ul class="notif-list" id="notif-list"></ul>';
  document.body.appendChild(drop);

  const ICONS = {
    account: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M19 8v6"/><path d="M22 11h-6"/>',
    provisioning: '<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3l2 2-7 7-2-2Z"/>',
    security: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1 1 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>',
    mail: '<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/><path d="M8 12h.01"/><path d="M12 12h.01"/><path d="M16 12h.01"/>',
  };

  const state = { items: [], filter: 'all', authed: true };

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }
  function kindIcon(kind) {
    return '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" ' +
      'stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
      (ICONS[kind] || ICONS.mail) + '</svg>';
  }
  function relTime(ts) {
    if (!ts) return '';
    const t = new Date(ts);
    if (isNaN(t)) return String(ts);
    const s = Math.max(1, Math.round((Date.now() - t.getTime()) / 1000));
    if (s < 60) return s + 's ago';
    const m = Math.round(s / 60);
    if (m < 60) return m + ' min ago';
    const h = Math.round(m / 60);
    if (h < 24) return h + 'h ago';
    return Math.round(h / 24) + 'd ago';
  }

  function readIds() {
    try { return new Set(JSON.parse(localStorage.getItem(READ_KEY) || '[]')); }
    catch (e) { return new Set(); }
  }
  function saveRead(ids) {
    try { localStorage.setItem(READ_KEY, JSON.stringify([...ids].slice(-200))); }
    catch (e) { /* storage unavailable — read state just doesn't persist */ }
  }

  function unreadCount() {
    const read = readIds();
    return state.items.filter((it) => !read.has(String(it.id))).length;
  }
  function mentionsCount() {
    return state.items.filter((it) => it.mention).length;
  }

  function render() {
    const read = readIds();
    const unread = unreadCount();
    badge.hidden = unread === 0;
    badge.textContent = String(unread);
    const headCount = drop.querySelector('#notif-unread-head');
    headCount.hidden = unread === 0;
    headCount.textContent = unread + ' new';
    const markBtn = drop.querySelector('#notif-markall');
    markBtn.disabled = unread === 0;

    drop.querySelectorAll('.notif-tab').forEach((t) => {
      t.setAttribute('aria-selected', t.dataset.filter === state.filter ? 'true' : 'false');
    });
    const counts = drop.querySelectorAll('[data-count]');
    counts[0].textContent = String(state.items.length);
    counts[1].textContent = String(unread);
    counts[2].textContent = String(mentionsCount());

    const list = drop.querySelector('#notif-list');

    if (!state.authed) {
      list.innerHTML = '<div class="notif-empty"><p class="notif-empty-title">Sign in to see your notifications</p><p>No session on this page.</p></div>';
      return;
    }
    if (!state.items.length) {
      list.innerHTML = '<div class="notif-empty">' +
        '<svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.8" ' +
        'stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>' +
        '<p class="notif-empty-title">You&rsquo;re all caught up</p><p>No notifications yet.</p></div>';
      return;
    }
    const items = state.items.filter((it) => {
      if (state.filter === 'unread') return !read.has(String(it.id));
      if (state.filter === 'mentions') return it.mention;
      return true;
    });
    if (!items.length) {
      list.innerHTML = '<div class="notif-empty"><p class="notif-empty-title">Nothing in this view</p>' +
        '<p>No ' + state.filter + ' notifications.</p></div>';
      return;
    }
    list.innerHTML = items.map((it) => {
      const kind = ICONS[it.kind] ? it.kind : 'mail';
      const isUnread = !read.has(String(it.id));
      return '<li><button type="button" class="notif-item" data-id="' + escapeHtml(String(it.id)) +
        '" data-kind="' + kind + '" data-unread="' + isUnread + '">' +
        '<span class="notif-ico" aria-hidden="true">' + kindIcon(kind) + '</span>' +
        '<span class="notif-body"><span class="notif-row">' +
        '<span class="notif-title-text">' + escapeHtml(it.title || 'Event') + '</span>' +
        '<span class="notif-time">' + escapeHtml(relTime(it.ts)) + '</span></span>' +
        (it.detail ? '<span class="notif-snippet">' + escapeHtml(it.detail) + '</span>' : '') +
        '</span><span class="notif-dot" aria-hidden="true"></span></button></li>';
    }).join('');
  }

  function position() {
    const r = bell.getBoundingClientRect();
    drop.style.top = Math.round(r.bottom + 8) + 'px';
    drop.style.right = Math.max(12, Math.round(window.innerWidth - r.right)) + 'px';
  }

  function setOpen(open) {
    if (open) {
      position();
      drop.hidden = false;
      bell.setAttribute('aria-expanded', 'true');
      load();
    } else {
      drop.hidden = true;
      bell.setAttribute('aria-expanded', 'false');
      bell.focus();
    }
  }
  const isOpen = () => !drop.hidden;

  async function load() {
    try {
      const r = await fetch(API, { credentials: 'same-origin' });
      if (r.status === 401) {
        state.authed = false;
        state.items = [];
      } else if (!r.ok) {
        state.authed = true;
        state.items = [];
      } else {
        const data = await r.json().catch(() => null);
        state.authed = true;
        state.items = data && Array.isArray(data.items) ? data.items : [];
      }
    } catch (e) {
      state.authed = true;
      state.items = [];
    }
    render();
  }

  bell.addEventListener('click', () => setOpen(!isOpen()));

  drop.addEventListener('click', (e) => {
    const tab = e.target.closest('.notif-tab');
    if (tab) {
      state.filter = tab.dataset.filter;
      render();
      return;
    }
    if (e.target.closest('#notif-markall')) {
      const ids = readIds();
      state.items.forEach((it) => ids.add(String(it.id)));
      saveRead(ids);
      render();
      return;
    }
    const item = e.target.closest('.notif-item');
    if (item) {
      const ids = readIds();
      ids.add(item.dataset.id);
      saveRead(ids);
      render();
    }
  });

  document.addEventListener('click', (e) => {
    if (isOpen() && !drop.contains(e.target) && !bell.contains(e.target)) setOpen(false);
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && isOpen()) setOpen(false);
  });
  window.addEventListener('resize', () => { if (isOpen()) position(); });

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible' && isOpen()) load();
  });

  await load();
  // Poll only while the tab is visible (same rule as dashboard.js); the
  // visibilitychange handler above refreshes on return.
  setInterval(() => {
    if (document.visibilityState === 'visible') load();
  }, POLL_MS);
})();
