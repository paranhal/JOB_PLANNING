(function () {
  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }
  function newKey(it) {
    var kw = (it && (it.keyword || it.label)) || '';
    var g = (it && (it.kw_group || it.group)) || '조치';
    return 'new:' + kw + '|' + g;
  }
  function itemSig(d) {
    var items = (d && d.items) || [];
    var news = (d && d.new) || [];
    var a = items.map(function (it) { return String(it.keyword_id || ''); }).join('\n');
    var b = news.map(function (it) { return newKey(it); }).join('\n');
    return a + '##' + b;
  }
  function collectKeep(root) {
    var keep = {};
    if (!root) return keep;
    root.querySelectorAll('input[type=checkbox]:checked').forEach(function (el) {
      var lab = el.closest('label');
      keep[el.value] = {
        label: lab ? (lab.getAttribute('data-label') || el.value) : el.value,
        group: lab ? (lab.getAttribute('data-group') || '') : ''
      };
    });
    return keep;
  }
  function labelHTML(field, id, label, group, checked, extraClass) {
    var on = checked ? ' checked' : '';
    var cls = extraClass || 'border-slate-200 bg-white';
    var g = group ? '<span class="text-slate-400">' + esc(group) + '</span>' : '';
    return '<label data-label="' + esc(label) + '" data-group="' + esc(group) +
      '" class="inline-flex items-center gap-1.5 px-2 py-1 rounded-lg border text-xs ' + cls + '">' +
      '<input type="checkbox" name="kw_' + field + '" value="' + esc(id) + '"' + on +
      ' class="rounded border-slate-300 text-blue-600">' +
      '<span>' + esc(label) + '</span>' + g + '</label>';
  }

  function bindASKeywords(opts) {
    opts = opts || {};
    var fetchFn = opts.fetch || (typeof fetch === 'function' ? fetch.bind(typeof window !== 'undefined' ? window : globalThis) : null);
    var box = document.getElementById('as-kw-box');
    var list = document.getElementById('as-kw-list');
    var empty = document.getElementById('as-kw-empty');
    var sug = document.getElementById('as-kw-suggested');
    var neu = document.getElementById('as-kw-new');
    var neuEmpty = document.getElementById('as-kw-new-empty');
    var manual = document.getElementById('as-kw-manual');
    var chosen = document.getElementById('as-kw-chosen');
    if (!box || !list) return null;
    var field = box.getAttribute('data-field') || 'action';
    var fetchCount = 0;
    var renderCount = 0;
    var lastSig = null;
    var t = null;

    function texts() {
      var s = box.getAttribute('data-extra') || '';
      document.querySelectorAll('.as-set-text, .as-set-cause').forEach(function (el) {
        s += ' ' + (el.value || '');
      });
      var hid = document.getElementById('as-action-taken');
      if (hid) s += ' ' + hid.value;
      return s;
    }
    function renderChosen() {
      var bits = [];
      list.querySelectorAll('input:checked').forEach(function (el) {
        var lab = el.closest('label');
        bits.push((lab && lab.getAttribute('data-label') || el.value) + '(' + (lab && lab.getAttribute('data-group') || '') + ')');
      });
      if (neu) neu.querySelectorAll('input:checked').forEach(function (el) {
        var lab = el.closest('label');
        bits.push((lab && lab.getAttribute('data-label') || '') + '(' + (lab && lab.getAttribute('data-group') || '') + ')');
      });
      if (manual && manual.value.trim()) bits.push(manual.value.trim());
      if (chosen) chosen.textContent = bits.length ? bits.join(', ') : '없음';
    }
    function render(d, force) {
      var sig = itemSig(d);
      if (!force && lastSig !== null && lastSig === sig) {
        renderChosen();
        return false;
      }
      var keepL = collectKeep(list);
      var keepN = collectKeep(neu);
      var items = (d && d.items) || [];
      var html = '';
      var ids = [];
      items.forEach(function (it) {
        var id = it.keyword_id;
        ids.push(id);
        var on = !!keepL[id];
        delete keepL[id];
        html += labelHTML(field, id, it.keyword, it.kw_group, on, on ? 'border-blue-300 bg-blue-50' : 'border-slate-200 bg-white');
      });
      Object.keys(keepL).forEach(function (id) {
        html += labelHTML(field, id, keepL[id].label, keepL[id].group, true, 'border-blue-300 bg-blue-50');
      });
      list.innerHTML = html;
      if (sug) sug.value = ids.join(',');
      if (empty) empty.classList.toggle('hidden', html !== '');
      var news = (d && d.new) || [];
      var nh = '';
      news.forEach(function (it) {
        var key = newKey(it);
        var on = !!keepN[key];
        delete keepN[key];
        nh += labelHTML(field, key, it.keyword, it.kw_group, on, 'border-dashed ' + (on ? 'border-blue-300 bg-blue-50' : 'border-slate-200'));
      });
      Object.keys(keepN).forEach(function (id) {
        nh += labelHTML(field, id, keepN[id].label, keepN[id].group, true, 'border-dashed border-blue-300 bg-blue-50');
      });
      if (neu) neu.innerHTML = nh;
      if (neuEmpty) neuEmpty.classList.toggle('hidden', (nh !== ''));
      lastSig = sig;
      renderCount += 1;
      renderChosen();
      return true;
    }
    function run() {
      if (!fetchFn) return;
      fetchCount += 1;
      var q = texts().trim();
      var body = new URLSearchParams();
      body.set('text', q);
      body.set('q', q);
      fetchFn('/as/keywords/suggest', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body.toString()
      }).then(function (r) { return r.json(); }).then(function (d) { render(d); }).catch(function () {});
    }
    document.addEventListener('input', function (e) {
      if (!e.target) return;
      if (e.target.id === 'as-kw-manual') {
        renderChosen();
        return;
      }
      if (e.target.classList && (e.target.classList.contains('as-set-text') || e.target.classList.contains('as-set-cause'))) {
        clearTimeout(t);
        t = setTimeout(run, 400);
      }
    });
    if (list) list.addEventListener('change', renderChosen);
    if (neu) neu.addEventListener('change', renderChosen);
    renderChosen();
    lastSig = itemSig({
      items: Array.prototype.map.call(list.querySelectorAll('input[type=checkbox]'), function (el) {
        return { keyword_id: el.value };
      }),
      new: Array.prototype.map.call((neu ? neu.querySelectorAll('input[type=checkbox]') : []), function (el) {
        var lab = el.closest('label');
        return { keyword: lab ? lab.getAttribute('data-label') : '', kw_group: lab ? lab.getAttribute('data-group') : '' };
      })
    });
    run();
    return {
      render: render,
      run: run,
      renderChosen: renderChosen,
      fetchCount: function () { return fetchCount; },
      renderCount: function () { return renderCount; }
    };
  }

  var api = {
    esc: esc,
    newKey: newKey,
    itemSig: itemSig,
    collectKeep: collectKeep,
    bind: bindASKeywords
  };
  if (typeof window !== 'undefined') window.ASKeywords = api;
  if (typeof document !== 'undefined' && document.getElementById && document.getElementById('as-kw-box') && !api._ui) {
    api._ui = bindASKeywords();
  }
})();
