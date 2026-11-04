(function (global) {
  function parseCats() {
    if (typeof document === 'undefined') return [];
    var el = document.getElementById('as-cause-categories-json');
    if (!el) return [];
    try {
      var v = JSON.parse(el.textContent || '[]');
      return Array.isArray(v) ? v : [];
    } catch (e) {
      return [];
    }
  }

  function childrenOf(cats, parent, level) {
    parent = String(parent || '');
    return (cats || []).filter(function (c) {
      return c.level === level && c.parent_code === parent && c.is_active !== false;
    });
  }

  function fill(sel, items, current) {
    if (!sel) return;
    sel.innerHTML = '<option value="">선택</option>';
    (items || []).forEach(function (c) {
      var opt = document.createElement('option');
      opt.value = c.code;
      opt.textContent = c.label;
      if (c.code === current) opt.selected = true;
      sel.appendChild(opt);
    });
  }

  function bindRoot(root, cats) {
    if (!root || root.getAttribute('data-cause-bound') === '1') return;
    var cat1 = root.querySelector('select[name$="cause_cat1"]');
    var cat2 = root.querySelector('select[name$="cause_cat2"]');
    var cat3 = root.querySelector('select[name$="cause_cat3"]');
    var wrap2 = root.querySelector('[data-cause-wrap="2"]');
    var wrap3 = root.querySelector('[data-cause-wrap="3"]');
    var customWrap = root.querySelector('[data-cause-custom-wrap]');
    var customInp = root.querySelector('[name$="cause_cat1_custom"]');
    if (!cat1) return;
    root.setAttribute('data-cause-bound', '1');

    function sync(resetLower) {
      var isCustom = cat1.value === 'custom';
      if (customWrap) customWrap.classList.toggle('hidden', !isCustom);
      if (customInp) {
        customInp.required = false;
        customInp.removeAttribute('required');
        if (!isCustom) customInp.value = '';
      }
      if (isCustom) {
        if (wrap2) wrap2.classList.add('hidden');
        if (wrap3) wrap3.classList.add('hidden');
        if (cat2) { cat2.value = ''; cat2.required = false; cat2.removeAttribute('required'); }
        if (cat3) { cat3.value = ''; cat3.required = false; cat3.removeAttribute('required'); }
        return;
      }
      var l2 = childrenOf(cats, cat1.value, 2);
      if (wrap2) wrap2.classList.toggle('hidden', l2.length === 0);
      if (l2.length === 0) {
        if (cat2) { cat2.value = ''; cat2.required = false; cat2.removeAttribute('required'); }
        if (wrap3) wrap3.classList.add('hidden');
        if (cat3) { cat3.value = ''; cat3.required = false; cat3.removeAttribute('required'); }
        return;
      }
      var cur2 = resetLower ? '' : ((cat2 && (cat2.value || cat2.getAttribute('data-current'))) || '');
      fill(cat2, l2, cur2);
      if (cat2) { cat2.required = false; cat2.removeAttribute('required'); }
      var l3 = childrenOf(cats, cat2 ? cat2.value : '', 3);
      if (wrap3) wrap3.classList.toggle('hidden', l3.length === 0);
      if (l3.length === 0) {
        if (cat3) { cat3.value = ''; cat3.required = false; cat3.removeAttribute('required'); }
        return;
      }
      var cur3 = resetLower ? '' : ((cat3 && (cat3.value || cat3.getAttribute('data-current'))) || '');
      fill(cat3, l3, cur3);
      if (cat3) { cat3.required = false; cat3.removeAttribute('required'); }
    }
    cat1.addEventListener('change', function () { sync(true); });
    if (cat2) cat2.addEventListener('change', function () { sync(false); });
    sync(false);
  }

  function bindAll() {
    if (typeof document === 'undefined') return;
    var cats = parseCats();
    document.querySelectorAll('.as-action-set').forEach(function (root) {
      bindRoot(root, cats);
    });
  }

  global.ASActionSetCause = { childrenOf: childrenOf, fill: fill, bindRoot: bindRoot, bindAll: bindAll };

  function start() {
    bindAll();
    var box = document.getElementById('as-action-sets');
    if (!box || typeof MutationObserver === 'undefined') return;
    new MutationObserver(function () { bindAll(); }).observe(box, { childList: true });
  }
  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', start);
    } else {
      start();
    }
  }
})(typeof window !== 'undefined' ? window : globalThis);
