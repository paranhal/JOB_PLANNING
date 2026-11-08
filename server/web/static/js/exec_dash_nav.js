(function (root) {
  var KEY = 'exec.dash.state';
  var TTL = 5 * 60 * 1000;
  var lastFocusOrg = '';
  var lastScrollY = 0;

  function now() {
    return Date.now();
  }

  function storage() {
    try {
      return root.sessionStorage;
    } catch (e) {
      return null;
    }
  }

  function docOf(extra) {
    extra = extra || {};
    return extra.document || root.document;
  }

  function save(orgId, extra) {
    extra = extra || {};
    var ss = storage();
    if (!ss) return;
    if (orgId) lastFocusOrg = orgId;
    var y = extra.scrollY;
    if (typeof y !== 'number') {
      y = (root.pageYOffset || (root.document && root.document.documentElement && root.document.documentElement.scrollTop) || 0);
    }
    try {
      ss.setItem(KEY, JSON.stringify({
        scrollY: y,
        orgId: orgId || '',
        ts: extra.ts || now()
      }));
    } catch (e) {}
  }

  function focusOrgCard(orgId, extra) {
    extra = extra || {};
    orgId = orgId || '';
    if (!orgId) return null;
    lastFocusOrg = orgId;
    var doc = docOf(extra);
    if (!doc || typeof doc.querySelector !== 'function') return null;
    var el = doc.querySelector('a[data-exec-org="' + orgId + '"]');
    if (!el) return null;
    if (typeof el.focus === 'function') {
      try { el.focus({ preventScroll: !!extra.skipScrollIntoView }); } catch (e1) { el.focus(); }
    }
    if (!extra.skipScrollIntoView && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' });
    }
    if (el.classList && el.classList.add) el.classList.add('exec-card-focus');
    var later = extra.setTimeout || root.setTimeout;
    if (typeof later === 'function') {
      later(function () {
        if (el.classList && el.classList.remove) el.classList.remove('exec-card-focus');
      }, 2000);
    }
    if (typeof extra.focusCard === 'function') extra.focusCard(orgId, el);
    return el;
  }

  function restore(extra) {
    extra = extra || {};
    var ss = storage();
    if (!ss) return null;
    var raw;
    try {
      raw = ss.getItem(KEY);
    } catch (e) {
      return null;
    }
    if (!raw) return null;
    var st;
    try {
      st = JSON.parse(raw);
    } catch (e) {
      return null;
    }
    var clock = extra.now || now();
    if (!st || typeof st.ts !== 'number' || clock - st.ts > TTL) {
      try { ss.removeItem(KEY); } catch (e2) {}
      return null;
    }
    try { ss.removeItem(KEY); } catch (e3) {}
    var y = typeof st.scrollY === 'number' ? st.scrollY : 0;
    lastScrollY = y;
    var apply = extra.scrollTo || function (v) {
      try {
        if (typeof root.scrollTo === 'function') root.scrollTo(0, v);
      } catch (e4) {}
      var se = root.document && (root.document.scrollingElement || root.document.documentElement);
      if (se) se.scrollTop = v;
    };
    apply(y);
    if (root.requestAnimationFrame) {
      root.requestAnimationFrame(function () { apply(y); });
    }
    if (st.orgId) {
      lastFocusOrg = st.orgId;
      var focusOpts = {};
      for (var k in extra) { if (Object.prototype.hasOwnProperty.call(extra, k)) focusOpts[k] = extra[k]; }
      focusOpts.skipScrollIntoView = true;
      focusOrgCard(st.orgId, focusOpts);
    }
    apply(y);
    if (typeof root.setTimeout === 'function') {
      root.setTimeout(function () { apply(y); }, 50);
      root.setTimeout(function () { apply(y); }, 120);
      root.setTimeout(function () { apply(y); }, 300);
    }
    return st;
  }

  function pathIsExecHome(href) {
    if (!href) return false;
    try {
      var u = new URL(href, 'http://local.invalid');
      var p = u.pathname.replace(/\/+$/, '') || '/';
      return p === '/exec';
    } catch (e) {
      return /(^|\/)exec\/?(\?|$)/.test(String(href));
    }
  }

  function bindBack(opts) {
    opts = opts || {};
    var doc = opts.document || root.document;
    if (!doc) return;
    var a = doc.getElementById('exec-dash-back');
    if (!a) return;
    var hist = opts.history || root.history;
    var getPrev = opts.referrer;
    a.addEventListener('click', function (ev) {
      if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey || ev.button) return;
      var prev = typeof getPrev === 'function' ? getPrev() : (doc.referrer || '');
      if (pathIsExecHome(prev) && hist && typeof hist.back === 'function') {
        ev.preventDefault();
        hist.back();
      }
    });
  }

  function refreshCards() {
    var el = root.document && root.document.getElementById('exec-org-cards');
    if (!el || !root.htmx || typeof root.htmx.ajax !== 'function') return;
    var date = el.getAttribute('data-exec-date') || '';
    var url = '/exec/cards';
    if (date) url += '?date=' + encodeURIComponent(date);
    root.htmx.ajax('GET', url, { source: el, target: '#exec-org-cards', swap: 'innerHTML' });
  }

  function takeFocusQuery(extra) {
    extra = extra || {};
    var loc = extra.location || root.location;
    if (!loc || !loc.search) return '';
    var q;
    try {
      q = new URLSearchParams(loc.search);
    } catch (e) {
      return '';
    }
    var org = q.get('focus') || '';
    if (!org) return '';
    lastFocusOrg = org;
    if (extra.replaceState) {
      extra.replaceState(org);
      return org;
    }
    if (root.history && typeof root.history.replaceState === 'function') {
      q.delete('focus');
      var next = loc.pathname + (q.toString() ? '?' + q.toString() : '') + (loc.hash || '');
      var strip = function () {
        try { root.history.replaceState({}, '', next); } catch (e2) {}
        if (lastScrollY && typeof root.scrollTo === 'function') root.scrollTo(0, lastScrollY);
      };
      if (typeof root.setTimeout === 'function') root.setTimeout(strip, 350);
      else strip();
    }
    return org;
  }

  function consumeFocusQuery(extra) {
    extra = extra || {};
    var org = takeFocusQuery(extra);
    if (!org) return;
    var o = extra || {};
    var run = function () { focusOrgCard(org, o); };
    run();
    if (root.requestAnimationFrame) root.requestAnimationFrame(run);
  }

  function bind(opts) {
    opts = opts || {};
    var doc = opts.document || root.document;
    if (!doc) return;
    if (doc.getElementById('exec-org-cards') || doc.querySelector('[data-exec-dash]')) {
      var dash = doc.querySelector('[data-exec-dash]');
      var marked = dash && dash.getAttribute('data-exec-focus');
      if (marked) lastFocusOrg = marked;
      var qOrg = takeFocusQuery(opts);
      if (qOrg) lastFocusOrg = qOrg;
      var restored = restore(opts);
      var focusOpts = {};
      for (var k in opts) { if (Object.prototype.hasOwnProperty.call(opts, k)) focusOpts[k] = opts[k]; }
      if (restored && typeof restored.scrollY === 'number') focusOpts.skipScrollIntoView = true;
      if (lastFocusOrg) focusOrgCard(lastFocusOrg, focusOpts);
      if (lastScrollY && typeof root.scrollTo === 'function') root.scrollTo(0, lastScrollY);
    }
    bindBack(opts);
    if (!doc.body || doc.body.getAttribute('data-exec-nav')) return;
    doc.body.setAttribute('data-exec-nav', '1');
    doc.addEventListener('click', function (ev) {
      var t = ev.target;
      var a = t && t.closest ? t.closest('a[data-exec-org]') : null;
      if (!a) return;
      if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey || ev.button === 1) return;
      save(a.getAttribute('data-exec-org'));
    }, true);
    doc.body.addEventListener('htmx:afterSwap', function (ev) {
      var tgt = ev && ev.detail && ev.detail.target;
      if (!tgt) return;
      var id = tgt.id || '';
      if (id !== 'exec-org-cards' && id !== 'exec-panel') return;
      var focus = (tgt.getAttribute && tgt.getAttribute('data-exec-focus')) || lastFocusOrg;
      if (focus) {
        var swapOpts = {};
        for (var k2 in opts) { if (Object.prototype.hasOwnProperty.call(opts, k2)) swapOpts[k2] = opts[k2]; }
        swapOpts.skipScrollIntoView = true;
        focusOrgCard(focus, swapOpts);
      }
    });
    root.addEventListener('pageshow', function (ev) {
      if (doc.getElementById('exec-org-cards') || doc.querySelector('[data-exec-dash]')) {
        restore(opts);
        if (lastFocusOrg) {
          var showOpts = {};
          for (var k3 in opts) { if (Object.prototype.hasOwnProperty.call(opts, k3)) showOpts[k3] = opts[k3]; }
          showOpts.skipScrollIntoView = true;
          focusOrgCard(lastFocusOrg, showOpts);
        }
        if (lastScrollY && typeof root.scrollTo === 'function') root.scrollTo(0, lastScrollY);
      }
      if (ev.persisted) refreshCards();
    });
    root.setInterval(function () {
      if (root.document && root.document.hidden) return;
      if (!root.document.getElementById('exec-org-cards')) return;
      refreshCards();
    }, TTL);
  }

  var api = {
    KEY: KEY,
    TTL: TTL,
    save: save,
    restore: restore,
    bindBack: bindBack,
    bind: bind,
    pathIsExecHome: pathIsExecHome,
    refreshCards: refreshCards,
    focusOrgCard: focusOrgCard
  };

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = api;
  } else {
    root.ExecDashNav = api;
    if (root.document) {
      if (root.document.readyState === 'loading') {
        root.document.addEventListener('DOMContentLoaded', function () { bind(); });
      } else {
        bind();
      }
    }
  }
})(typeof window !== 'undefined' ? window : global);
