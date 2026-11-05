(function () {
  function collapse(el) {
    if (!el) return;
    var v = String(el.value || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/\n/g, ' ');
    if (v !== el.value) el.value = v;
  }
  function resize(el) {
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = Math.max(el.scrollHeight, 40) + 'px';
  }
  function bind(el) {
    if (!el || el.dataset.autoBound) return;
    el.dataset.autoBound = '1';
    el.setAttribute('rows', el.getAttribute('rows') || '1');
    el.style.resize = 'none';
    el.style.overflowWrap = 'anywhere';
    el.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') e.preventDefault();
    });
    el.addEventListener('paste', function () {
      setTimeout(function () { collapse(el); resize(el); }, 0);
    });
    el.addEventListener('input', function () { collapse(el); resize(el); });
    collapse(el);
    resize(el);
  }
  window.bindAutoTextarea = bind;
  function scan() {
    document.querySelectorAll('textarea[data-auto-text]').forEach(bind);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', scan);
  } else {
    scan();
  }
  document.addEventListener('htmx:afterSwap', scan);
})();
