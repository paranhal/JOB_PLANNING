(function (root) {
  'use strict';

  function itemID(h) {
    if (!h) return '';
    return String(h.id || h.ID || h.asset_id || h.AssetID || '').trim();
  }
  function itemName(h) {
    if (!h) return '';
    var name = String(h.name || h.Name || h.product_name || h.ProductName || '').trim();
    var model = String(h.model_name || h.ModelName || '').trim();
    if (name && model && name.indexOf(model) < 0) name += ' · ' + model;
    return name || itemID(h);
  }
  function itemSub(h) {
    if (!h) return '';
    return String(h.sub || h.install_location || h.InstallLocation || '').trim();
  }
  function assetFamily(a) {
    if (!a) return '';
    var cat = String(a.product_category || a.ProductCategory || '').toLowerCase();
    var blob = ((a.product_name || a.ProductName || '') + ' ' + (a.product_type || a.ProductType || '') + ' ' + cat).toLowerCase();
    if (cat === 'rfid') return 'rfid';
    if (cat === 'homepage' || cat === 'elibrary' || cat === 'mobile') return 'web';
    if (cat === 'materials') return 'klas';
    if (blob.indexOf('rfid') >= 0) return 'rfid';
    if (blob.indexOf('klas') >= 0 || blob.indexOf('k-las') >= 0) return 'klas';
    if (blob.indexOf('홈페이지') >= 0 || blob.indexOf('전자도서') >= 0 || blob.indexOf('elibrary') >= 0) return 'web';
    return 'common';
  }
  function cloneItems(list) {
    return (list || []).map(function (h) {
      return { id: h.id, name: h.name, sub: h.sub || '', family: h.family || '' };
    });
  }
  function normalizeSelected(list) {
    if (!Array.isArray(list)) return [];
    var out = [];
    var seen = {};
    list.forEach(function (h) {
      var id = itemID(h);
      if (!id || seen[id]) return;
      seen[id] = true;
      out.push({ id: id, name: itemName(h), sub: itemSub(h), family: h.family || assetFamily(h) || '' });
    });
    return out;
  }
  function runeLen(s) {
    return Array.from(String(s || '').trim()).length;
  }

  function createTargetPicker(opts) {
    opts = opts || {};
    var selected = normalizeSelected(opts.selected);
    var state = {
      multi: opts.multi !== false,
      customerId: String(opts.customerId || '').trim(),
      applied: cloneItems(selected),
      saved: cloneItems(selected),
      draft: {},
      hits: [],
      q: '',
      open: false,
      hint: '',
      hi: -1,
      rowHint: ''
    };
    selected.forEach(function (h) { state.draft[h.id] = true; });

    function checkedIDs() {
      return Object.keys(state.draft).filter(function (id) { return state.draft[id]; });
    }
    function lookup(id) {
      var found = null;
      state.hits.forEach(function (h) { if (itemID(h) === id) found = h; });
      if (found) {
        return { id: id, name: itemName(found), sub: itemSub(found), family: assetFamily(found) };
      }
      var prev = null;
      state.applied.concat(state.saved).forEach(function (h) { if (h.id === id) prev = h; });
      return prev || { id: id, name: id, sub: '', family: '' };
    }

    var api = {
      state: state,
      setQuery: function (q) { state.q = q || ''; },
      setCustomer: function (id) {
        id = String(id || '').trim();
        if (id === state.customerId) return;
        var first = !state.customerId;
        state.customerId = id;
        if (!first) {
          state.applied = [];
          state.saved = [];
          state.draft = {};
          state.hits = [];
          state.open = false;
          state.q = '';
          state.rowHint = '';
          state.hint = id ? '' : '거래처를 먼저 선택하세요';
        }
      },
      canSearch: function () {
        var q = String(state.q || '').trim();
        if (!state.customerId) {
          state.hint = '거래처를 먼저 선택하세요';
          state.open = false;
          state.hits = [];
          return false;
        }
        if (runeLen(q) < 2) {
          state.hint = q ? '2글자 이상 입력하세요' : '';
          state.open = false;
          state.hits = [];
          return false;
        }
        return true;
      },
      receiveHits: function (list) {
        if (!api.canSearch()) return;
        var cid = state.customerId;
        state.hits = (list || []).filter(function (h) {
          var hc = String(h.customer_id || h.CustomerID || '').trim();
          return !hc || hc === cid;
        });
        state.open = true;
        state.hi = state.hits.length ? 0 : -1;
        state.hint = state.hits.length ? '' : '검색 결과가 없습니다';
        state.rowHint = '';
      },
      toggle: function (id) {
        id = String(id || '').trim();
        if (!id) return;
        if (!state.multi && !state.draft[id]) {
          var n = checkedIDs().length;
          if (n >= 1) {
            state.rowHint = '한 행에는 자산 1개만 연결됩니다. 더 있으면 장비 추가를 누르세요.';
            return;
          }
        }
        state.draft[id] = !state.draft[id];
        state.rowHint = '';
      },
      selectAll: function () {
        if (!state.multi && state.hits.length > 1) {
          state.rowHint = '한 행에는 자산 1개만 연결됩니다. 더 있으면 장비 추가를 누르세요.';
          return;
        }
        state.hits.forEach(function (h) {
          var id = itemID(h);
          if (id) state.draft[id] = true;
        });
        state.rowHint = '';
      },
      clearDraft: function () {
        state.hits.forEach(function (h) {
          var id = itemID(h);
          if (id) state.draft[id] = false;
        });
        state.rowHint = '';
      },
      apply: function () {
        if (!state.multi && checkedIDs().length > 1) {
          state.rowHint = '한 행에는 자산 1개만 연결됩니다. 더 있으면 장비 추가를 누르세요.';
          return false;
        }
        state.applied = checkedIDs().map(lookup);
        state.open = false;
        state.hits = [];
        state.q = '';
        state.rowHint = '';
        state.hint = '';
        return true;
      },
      close: function () {
        state.draft = {};
        state.applied.forEach(function (h) { state.draft[h.id] = true; });
        state.open = false;
        state.hits = [];
        state.q = '';
        state.rowHint = '';
      },
      cancel: function () {
        state.applied = cloneItems(state.saved);
        api.close();
        state.hint = '';
      },
      remove: function (id) {
        state.applied = state.applied.filter(function (h) { return h.id !== id; });
        state.draft[id] = false;
      },
      markSaved: function () {
        state.saved = cloneItems(state.applied);
      },
      clickOutside: function () {
        if (!state.open) return false;
        api.close();
        return true;
      },
      reclickBox: function () {
        if (!state.open) return false;
        api.close();
        return true;
      },
      onKey: function (key) {
        var prevent = key === 'Enter' || key === 'Escape' || key === 'ArrowDown' || key === 'ArrowUp';
        if (key === 'Escape') api.close();
        else if (key === 'ArrowDown' && state.open && state.hits.length) {
          state.hi = Math.min(state.hits.length - 1, state.hi + 1);
        } else if (key === 'ArrowUp' && state.open && state.hits.length) {
          state.hi = Math.max(0, state.hi - 1);
        } else if (key === 'Enter' && state.open && state.hi >= 0 && state.hits[state.hi]) {
          api.toggle(itemID(state.hits[state.hi]));
        }
        return prevent;
      }
    };
    return api;
  }

  function csrfToken() {
    if (typeof document === 'undefined') return '';
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? (m.getAttribute('content') || '') : '';
  }

  function paint(rootEl, api) {
    var state = api.state;
    var chips = rootEl.querySelector('.tp-chips');
    var empty = rootEl.querySelector('.tp-empty');
    var hint = rootEl.querySelector('.tp-hint');
    var rowHint = rootEl.querySelector('.tp-row-hint');
    var panel = rootEl.querySelector('.tp-panel');
    var list = rootEl.querySelector('.tp-list');
    var box = rootEl.querySelector('.tp-hiddens');
    var q = rootEl.querySelector('.tp-q');
    var saveBtn = rootEl.querySelector('.tp-save');
    if (empty) empty.classList.toggle('hidden', state.applied.length > 0);
    if (hint) hint.textContent = state.hint || '';
    if (rowHint) {
      rowHint.textContent = state.rowHint || '';
      rowHint.classList.toggle('hidden', !state.rowHint);
    }
    if (panel) panel.classList.toggle('hidden', !state.open);
    if (saveBtn) saveBtn.classList.toggle('hidden', rootEl.getAttribute('data-standalone') !== '1');
    if (q && document.activeElement !== q) q.value = state.q || '';
    if (chips) {
      chips.textContent = '';
      state.applied.forEach(function (h) {
        var chip = document.createElement('span');
        chip.className = 'inline-flex items-center gap-1 rounded-full bg-hover border border-line px-2 py-0.5 text-xs';
        var label = document.createElement('span');
        label.textContent = h.name || h.id;
        var rm = document.createElement('button');
        rm.type = 'button';
        rm.className = 'tp-chip-x text-ink-muted';
        rm.setAttribute('data-id', h.id);
        rm.textContent = '×';
        rm.addEventListener('click', function () {
          api.remove(h.id);
          paint(rootEl, api);
        });
        chip.appendChild(label);
        chip.appendChild(rm);
        chips.appendChild(chip);
      });
    }
    if (list) {
      list.textContent = '';
      if (state.open && !state.hits.length) {
        var none = document.createElement('li');
        none.className = 'px-3 py-2 text-xs text-ink-subtle';
        none.textContent = '검색 결과가 없습니다';
        list.appendChild(none);
      }
      state.hits.forEach(function (h, i) {
        var id = itemID(h);
        var li = document.createElement('li');
        var lab = document.createElement('label');
        lab.className = 'flex items-start gap-2 px-3 py-2 cursor-pointer hover:bg-hover' + (i === state.hi ? ' bg-primary-soft' : '');
        var cb = document.createElement('input');
        cb.type = 'checkbox';
        cb.className = 'mt-0.5 tp-check';
        cb.checked = !!state.draft[id];
        cb.addEventListener('change', function () {
          if (cb.checked === !!state.draft[id]) return;
          api.toggle(id);
          if (!state.draft[id]) cb.checked = false;
          paint(rootEl, api);
        });
        var text = document.createElement('span');
        var line = itemName(h);
        var sub = itemSub(h);
        text.textContent = sub ? (line + ' · ' + sub) : line;
        lab.appendChild(cb);
        lab.appendChild(text);
        li.appendChild(lab);
        list.appendChild(li);
      });
    }
    if (box) {
      var name = rootEl.getAttribute('data-name') || 'asset_id';
      var cls = rootEl.getAttribute('data-input-class') || '';
      var inputID = rootEl.getAttribute('data-input-id') || '';
      var keep = rootEl.getAttribute('data-keep-empty') === '1';
      box.textContent = '';
      var items = state.applied.slice();
      if (!items.length && keep) items = [{ id: '', name: '' }];
      items.forEach(function (h, i) {
        var input = document.createElement('input');
        input.type = 'hidden';
        input.name = name;
        input.value = h.id || '';
        if (cls) input.className = cls;
        if (inputID && i === 0) input.id = inputID;
        if (h.family) input.setAttribute('data-family', h.family);
        box.appendChild(input);
      });
    }
  }

  function readCustomer(rootEl) {
    var from = rootEl.getAttribute('data-customer-from') || 'customer';
    var form = rootEl.closest ? rootEl.closest('form') : null;
    if (from === 'site') {
      if (!form) return '';
      var site = form.querySelector('[name="site_id"]');
      return site ? String(site.value || '').trim() : '';
    }
    var preset = String(rootEl.getAttribute('data-customer') || '').trim();
    if (!form) return preset;
    var cust = form.querySelector('[name="customer_id"]');
    var v = cust ? String(cust.value || '').trim() : '';
    return v || preset;
  }

  function mountTargetPicker(rootEl) {
    if (!rootEl || rootEl._tp) return rootEl && rootEl._tp;
    var selected = [];
    var raw = rootEl.querySelector('.tp-selected');
    if (raw && raw.textContent.trim()) {
      try { selected = JSON.parse(raw.textContent); } catch (e) { selected = []; }
    }
    var api = createTargetPicker({
      multi: rootEl.getAttribute('data-multi') !== '0',
      customerId: rootEl.getAttribute('data-customer') || '',
      selected: selected
    });
    rootEl._tp = api;
    if (!api.state.customerId) {
      var initial = readCustomer(rootEl);
      if (initial) api.setCustomer(initial);
    }
    var q = rootEl.querySelector('.tp-q');
    var timer = 0;
    function search() {
      api.setQuery(q ? q.value : '');
      if (!api.canSearch()) { paint(rootEl, api); return; }
      if (typeof fetch !== 'function') { paint(rootEl, api); return; }
      var url = '/api/assets/' + encodeURIComponent(api.state.customerId) + '?q=' + encodeURIComponent(String(api.state.q || '').trim());
      fetch(url, { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (list) {
        api.receiveHits(list);
        paint(rootEl, api);
      }).catch(function () {
        api.state.hint = '검색에 실패했습니다';
        paint(rootEl, api);
      });
    }
    if (q) {
      q.addEventListener('input', function () {
        clearTimeout(timer);
        timer = setTimeout(search, 200);
      });
      q.addEventListener('keydown', function (e) {
        if (api.onKey(e.key)) e.preventDefault();
        paint(rootEl, api);
      });
      q.addEventListener('click', function () {
        if (api.reclickBox()) paint(rootEl, api);
      });
    }
    var all = rootEl.querySelector('.tp-all');
    var none = rootEl.querySelector('.tp-none');
    var applyBtn = rootEl.querySelector('.tp-apply');
    var cancelBtn = rootEl.querySelector('.tp-cancel');
    var saveBtn = rootEl.querySelector('.tp-save');
    if (all) all.addEventListener('click', function () { api.selectAll(); paint(rootEl, api); });
    if (none) none.addEventListener('click', function () { api.clearDraft(); paint(rootEl, api); });
    if (applyBtn) applyBtn.addEventListener('click', function () { api.apply(); paint(rootEl, api); });
    if (cancelBtn) cancelBtn.addEventListener('click', function () { api.cancel(); paint(rootEl, api); });
    if (saveBtn) saveBtn.addEventListener('click', function () {
      if (api.state.open && api.apply() === false) { paint(rootEl, api); return; }
      paint(rootEl, api);
      var url = rootEl.getAttribute('data-save-url') || '';
      if (!url || typeof fetch !== 'function') return;
      var body = new URLSearchParams();
      if (!api.state.applied.length) body.append('asset_id', '');
      api.state.applied.forEach(function (h) { body.append('asset_id', h.id); });
      fetch(url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
          'X-CSRF-Token': csrfToken()
        },
        body: body.toString()
      }).then(function (r) {
        if (!r.ok) throw new Error('save');
        api.markSaved();
        api.state.hint = '저장했습니다';
        paint(rootEl, api);
      }).catch(function () {
        api.state.hint = '저장하지 못했습니다';
        paint(rootEl, api);
      });
    });
    var form = rootEl.closest ? rootEl.closest('form') : null;
    if (form && !rootEl._tpSubmit) {
      rootEl._tpSubmit = true;
      form.addEventListener('submit', function (e) {
        if (!api.state.open) return;
        if (api.apply() === false) {
          e.preventDefault();
          paint(rootEl, api);
          return;
        }
        paint(rootEl, api);
      });
    }
    paint(rootEl, api);
    return api;
  }

  function bootTargetPickers() {
    if (typeof document === 'undefined') return;
    document.querySelectorAll('.js-target-picker').forEach(mountTargetPicker);
    if (document.documentElement.getAttribute('data-tp-booted') === '1') return;
    document.documentElement.setAttribute('data-tp-booted', '1');
    document.addEventListener('change', function (e) {
      var t = e.target;
      if (!t || t.name !== 'customer_id') return;
      var form = t.form;
      if (!form) return;
      form.querySelectorAll('.js-target-picker[data-customer-from="customer"]').forEach(function (el) {
        if (!el._tp) return;
        el._tp.setCustomer(t.value || '');
        paint(el, el._tp);
      });
    });
    document.addEventListener('customer-picked', function (e) {
      var id = (e.detail && e.detail.id) || '';
      var form = e.target && e.target.closest ? e.target.closest('form') : null;
      if (!form) return;
      form.querySelectorAll('.js-target-picker[data-customer-from="customer"]').forEach(function (el) {
        if (!el._tp) return;
        el._tp.setCustomer(id);
        paint(el, el._tp);
      });
    });
    document.addEventListener('sites-changed', function (e) {
      var ids = (e.detail && e.detail.ids) || [];
      var form = e.target && e.target.closest ? e.target.closest('form') : null;
      if (!form) return;
      form.querySelectorAll('.js-target-picker[data-customer-from="site"]').forEach(function (el) {
        if (!el._tp) return;
        el._tp.setCustomer(ids[0] || '');
        paint(el, el._tp);
      });
    });
    document.addEventListener('pointerdown', function (e) {
      document.querySelectorAll('.js-target-picker').forEach(function (el) {
        if (!el._tp || !el._tp.state.open) return;
        if (el.contains(e.target)) return;
        el._tp.clickOutside();
        paint(el, el._tp);
      });
    });
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(function () {
        document.querySelectorAll('.js-target-picker[data-customer-from="site"]').forEach(function (el) {
          if (!el._tp || el._tp.state.customerId) return;
          var id = readCustomer(el);
          if (!id) return;
          el._tp.setCustomer(id);
          paint(el, el._tp);
        });
      });
    }
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { createTargetPicker: createTargetPicker, mountTargetPicker: mountTargetPicker };
  }
  root.createTargetPicker = createTargetPicker;
  root.mountTargetPicker = mountTargetPicker;
  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bootTargetPickers);
    else bootTargetPickers();
  }
})(typeof window !== 'undefined' ? window : globalThis);
