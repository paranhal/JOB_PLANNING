(function (root) {
  var KEY = 'exec.dash.state';
  var TTL = 5 * 60 * 1000;

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

  function save(orgId, extra) {
    extra = extra || {};
    var ss = storage();
    if (!ss) return;
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

  function restore(extra) {
    extra = extra || {};
    try {
      if (root.history) root.history.scrollRestoration = 'manual';
    } catch (e) {}
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
    var apply = extra.scrollTo || function (v) {
      if (typeof root.scrollTo === 'function') root.scrollTo(0, v);
    };
    apply(y);
    if (root.requestAnimationFrame) {
      root.requestAnimationFrame(function () { apply(y); });
    }
    if (typeof root.setTimeout === 'function') {
      root.setTimeout(function () { apply(y); }, 50);
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

  function bind(opts) {
    opts = opts || {};
    var doc = opts.document || root.document;
    if (!doc) return;
    if (doc.getElementById('exec-org-cards') || doc.querySelector('[data-exec-dash]')) {
      restore(opts);
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
    root.addEventListener('pageshow', function (ev) {
      if (doc.getElementById('exec-org-cards') || doc.querySelector('[data-exec-dash]')) {
        restore(opts);
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
    refreshCards: refreshCards
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
