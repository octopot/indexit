// The page works without this script; it keeps the page current, writes
// recent times as relative ones and copies commands.
(() => {
  'use strict';

  const main = document.getElementById('main');
  const status = document.getElementById('status');
  if (!main || !status) return;

  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const relative = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
  const number = new Intl.NumberFormat(undefined);
  const full = new Intl.DateTimeFormat(undefined, { dateStyle: 'full', timeStyle: 'short' });
  const day = new Intl.DateTimeFormat(undefined, { dateStyle: 'full' });
  const clock = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
  const interval = Number(main.dataset.refresh || 15) * 1000;
  let lastOK = Date.now();

  function ago(date) {
    const seconds = Math.min(0, Math.round((date.getTime() - Date.now()) / 1000));
    if (seconds > -60) return relative.format(seconds, 'second');
    if (seconds > -3600) return relative.format(Math.round(seconds / 60), 'minute');
    return relative.format(Math.round(seconds / 3600), 'hour');
  }

  function times(root) {
    for (const node of root.querySelectorAll('time[datetime]')) {
      const date = new Date(node.getAttribute('datetime'));
      if (Number.isNaN(date.getTime())) continue;
      node.title = (node.hasAttribute('data-date') ? day : full).format(date);
      if (node.hasAttribute('data-rel') || node.hasAttribute('data-updated')) {
        if (Date.now() - date.getTime() < 86400000) node.textContent = ago(date);
      }
    }
  }

  function countUp(root) {
    if (reduced) return;
    for (const node of root.querySelectorAll('[data-count]')) {
      const target = Number(node.dataset.count);
      if (!Number.isFinite(target) || target < 2) continue;
      // the last frame shows the number exactly as the server wrote it
      const text = node.textContent;
      const start = performance.now();
      const duration = 500;
      const frame = (now) => {
        const t = Math.min(1, (now - start) / duration);
        const eased = 1 - Math.pow(1 - t, 4);
        node.textContent = t < 1 ? number.format(Math.round(target * eased)) : text;
        if (t < 1) requestAnimationFrame(frame);
      };
      requestAnimationFrame(frame);
    }
  }

  const serving = status.querySelector('[data-status]')?.textContent || '';
  // The service looks at its data directory every 30 seconds; a snapshot
  // much older than that means it no longer does, even if it still answers.
  const scanEvery = 30000;
  let scanned = Date.parse(status.querySelector('time[data-updated]')?.getAttribute('datetime') || '') || Date.now();
  let state = 'fresh';
  function freshness() {
    const command = status.dataset.restart;
    const restart = command ? '; start it again with ' + command + '.' : '.';
    let next = 'fresh';
    let text = serving;
    if (Date.now() - lastOK > 2 * interval) {
      next = 'offline';
      text = 'Stopped updating at ' + clock.format(new Date(lastOK)) + '. The service may have stopped' + restart;
    } else if (Date.now() - scanned > 4 * scanEvery) {
      next = 'stuck';
      text = 'Counts last updated at ' + clock.format(new Date(scanned)) + '. The service answers but no longer counts' + restart;
    }
    if (next === state) return;
    state = next;
    status.classList.toggle('stale', next !== 'fresh');
    const live = status.querySelector('[data-status]');
    if (live) live.textContent = text;
  }

  // Tiles are matched by their heading. A tile is replaced only when the
  // server sends it changed and nobody is selecting text in it; what the
  // reader did in it (focus, an open table, scrolled commands and calendar)
  // carries over.
  const key = (node) => node.getAttribute('aria-labelledby') || node.className;
  const served = new Map([...main.children].map((node) => [key(node), node.outerHTML]));
  const focusable = 'a[href], button, summary, [tabindex]';

  function selecting(node) {
    const selection = window.getSelection();
    return selection && !selection.isCollapsed && node.contains(selection.anchorNode);
  }

  function swap(old, fresh) {
    const node = document.adoptNode(fresh);
    const open = [...old.querySelectorAll('details')].map((d) => d.open);
    node.querySelectorAll('details').forEach((d, i) => { if (open[i]) d.open = true; });
    // scrolled boxes are matched by id or class, so one that comes or goes
    // does not hand its position to another
    const box = (el) => el.id || el.className;
    const scrolled = new Map([...old.querySelectorAll('pre, .calendar, .table-scroll')].map((el) => [box(el), {
      left: el.scrollLeft, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 2,
    }]));
    // focus follows the same link, button or box, or else its position
    const targets = [...old.querySelectorAll(focusable)];
    const focused = targets.indexOf(document.activeElement);
    const active = focused >= 0 ? document.activeElement : null;
    const identity = (el) => el.getAttribute('href') || el.id || el.dataset.copy || el.tagName + ':' + el.textContent.trim();
    old.replaceWith(node);
    times(node);
    node.querySelectorAll('pre, .calendar, .table-scroll').forEach((el) => {
      const was = scrolled.get(box(el));
      if (was) el.scrollLeft = was.end && el.classList.contains('calendar') ? el.scrollWidth : was.left;
    });
    if (active) {
      const candidates = [...node.querySelectorAll(focusable)];
      const same = candidates.find((el) => identity(el) === identity(active)) || candidates[focused];
      same?.focus({ preventScroll: true });
    }
    return node;
  }

  function reconcile(fresh) {
    const current = new Map([...main.children].map((node) => [key(node), node]));
    let cursor = main.firstElementChild;
    for (const node of fresh) {
      const k = key(node);
      const html = node.outerHTML;
      const old = current.get(k);
      current.delete(k);
      if (!old) {
        // a tile that was not there, such as one that needs a look
        const added = document.adoptNode(node);
        main.insertBefore(added, cursor);
        times(added);
        latest(added);
        served.set(k, html);
        continue;
      }
      if (served.get(k) !== html && !selecting(old)) {
        served.set(k, html);
        cursor = swap(old, node).nextElementSibling;
      } else {
        cursor = old.nextElementSibling;
      }
    }
    for (const [k, node] of current) {
      served.delete(k);
      node.remove();
    }
  }

  let busy = false;
  async function refresh() {
    if (document.hidden || busy) return;
    busy = true;
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), 10000);
    try {
      const response = await fetch('/', { headers: { Accept: 'text/html' }, cache: 'no-store', credentials: 'same-origin', signal: abort.signal });
      if (!response.ok) throw new Error(String(response.status));
      const page = new DOMParser().parseFromString(await response.text(), 'text/html');
      const next = page.getElementById('main');
      const nextStatus = page.getElementById('status');
      if (!next || !nextStatus) throw new Error('unexpected page');
      reconcile([...next.children]);
      // The live region stays in place, so a screen reader hears only what
      // freshness() writes into it; the scan time changes beside it.
      const time = status.querySelector('time[data-updated]');
      const nextTime = nextStatus.querySelector('time[data-updated]');
      if (time && nextTime) {
        if (time.getAttribute('datetime') !== nextTime.getAttribute('datetime')) {
          time.setAttribute('datetime', nextTime.getAttribute('datetime'));
          time.textContent = nextTime.textContent;
          times(status);
        }
      } else if (nextTime) {
        const live = status.querySelector('[data-status]');
        // childNodes is live: copy it before adoptNode takes the nodes away
        const nodes = [...nextStatus.childNodes].map((node) => document.adoptNode(node));
        status.replaceChildren(...nodes.map((node) => (live && node.matches?.('[data-status]') ? live : node)));
        times(status);
      }
      scanned = Date.parse(status.querySelector('time[data-updated]')?.getAttribute('datetime') || '') || scanned;
      lastOK = Date.now();
    } catch {
      // freshness() below tells the reader
    } finally {
      clearTimeout(timer);
      busy = false;
      freshness();
    }
  }

  document.addEventListener('click', async (event) => {
    const button = event.target instanceof Element ? event.target.closest('button[data-copy]') : null;
    if (!button) return;
    const source = document.getElementById(button.dataset.copy);
    if (!source) return;
    const text = source.textContent.trim();
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(source);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      button.textContent = 'Selected';
      setTimeout(() => { button.textContent = 'Copy'; }, 2500);
      return;
    }
    button.textContent = 'Copied';
    setTimeout(() => { button.textContent = 'Copy'; }, 2500);
  });

  // the calendar opens on the latest weeks where it scrolls
  function latest(root) {
    for (const node of root.querySelectorAll('.calendar')) node.scrollLeft = node.scrollWidth;
  }

  document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });

  // the entrance plays once; tiles that later polls bring stay still
  setTimeout(() => document.body.classList.add('settled'), 1000);
  times(document);
  latest(document);
  countUp(main);
  setInterval(refresh, interval);
  setInterval(() => {
    if (!document.hidden && !busy) freshness();
    for (const node of document.querySelectorAll('time[data-updated], time[data-rel]')) {
      const date = new Date(node.getAttribute('datetime'));
      if (Date.now() - date.getTime() < 86400000) node.textContent = ago(date);
    }
  }, 5000);
})();
