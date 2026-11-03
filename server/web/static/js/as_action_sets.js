(function (global) {
  var missingLabels = {
    result_code: '결과',
    work_place: '근무구분',
    process_type: '처리유형',
    work_date: '작업일',
    work_date_required: '작업일',
    visit_date: '작업일',
    cause_cat1: '원인분류',
    action_taken: '조치내용'
  };

  function needsResult(opts, result) {
    var requireResult = !opts || opts.requireResult !== false;
    return requireResult && !String(result || '').trim();
  }

  function appendDraftFlag(fd, opts) {
    if (opts && opts.requireResult === false) fd.append('draft', '1');
  }

  function copyWorkDateToVisit(workVal, visitEl) {
    if (visitEl) visitEl.value = workVal || '';
  }

  function missingLine(codes) {
    var labs = (codes || []).map(function (c) { return missingLabels[c] || c; });
    if (!labs.length) return '';
    return '최종 저장 전 필요: ' + labs.join(', ');
  }

  global.ASActionSets = {
    needsResult: needsResult,
    appendDraftFlag: appendDraftFlag,
    copyWorkDateToVisit: copyWorkDateToVisit,
    missingLine: missingLine,
    toggleOpen: null
  };

  var form = document.getElementById('as-action-form');
  var box = document.getElementById('as-action-sets');
  if (!form || !box) return;
  var addBtn = document.getElementById('as-add-set');
  var addErr = document.getElementById('as-add-set-err');
  var maxSets = 20;
  var slots = collectOptions(box.querySelector('.as-set-start'));
  var durs = collectOptions(box.querySelector('.as-set-dur'));

  function collectOptions(sel) {
    var out = [];
    if (!sel) return out;
    Array.prototype.forEach.call(sel.options, function (o) {
      out.push({ v: o.value, l: o.textContent });
    });
    return out;
  }
  function lastSet() {
    var all = box.querySelectorAll('.as-action-set');
    return all[all.length - 1] || null;
  }
  function setSeq(el) { return parseInt(el.getAttribute('data-action-set') || '0', 10); }
  function prefix(seq) { return 's' + seq + '_'; }
  function toast(msg) {
    var t = document.getElementById('as-set-toast');
    if (!t) {
      t = document.createElement('div');
      t.id = 'as-set-toast';
      t.className = 'fixed left-1/2 bottom-6 -translate-x-1/2 bg-slate-900 text-white text-sm px-4 py-2 rounded-full z-50 max-w-[90vw] text-center';
      document.body.appendChild(t);
    }
    t.textContent = msg;
    t.style.opacity = '1';
    clearTimeout(toast.t);
    toast.t = setTimeout(function () { t.style.opacity = '0'; }, 2800);
  }
  function field(root, cls) { return root.querySelector(cls); }
  function endLabel(start, dur) {
    if (!start || !dur) return { t: '—', v: '' };
    var p = start.split(':');
    var total = parseInt(p[0], 10) * 60 + parseInt(p[1], 10) + parseInt(dur, 10);
    if (total >= 24 * 60) return { t: '작업이 자정을 넘을 수 없습니다', v: '', err: 1 };
    var hh = String(Math.floor(total / 60)).padStart(2, '0');
    var mm = String(total % 60).padStart(2, '0');
    var v = hh + ':' + mm;
    return { t: koTime(v), v: v };
  }
  function koTime(hhmm) {
    var p = (hhmm || '').split(':');
    if (p.length < 2) return hhmm || '—';
    var h = parseInt(p[0], 10), m = p[1];
    var per = '오전', h12 = h;
    if (h === 0) h12 = 12;
    else if (h === 12) per = '오후';
    else if (h > 12) { per = '오후'; h12 = h - 12; }
    return per + ' ' + String(h12).padStart(2, '0') + ':' + m;
  }
  function syncTimes(root) {
    var st = field(root, '.as-set-start');
    var du = field(root, '.as-set-dur');
    var en = field(root, '.as-set-end');
    var lab = field(root, '.as-set-end-label');
    var same = field(root, '.as-set-leave-same');
    var lv = field(root, '.as-set-leave');
    var hint = field(root, '.as-set-leave-hint');
    var r = endLabel(st && st.value, du && du.value);
    if (lab) lab.textContent = r.t;
    if (en) en.value = r.v || '';
    if (same && same.checked) {
      if (lv) { lv.value = r.v || lv.value; lv.disabled = true; }
      if (hint) hint.textContent = '완료 시간이 철수 시간이 됩니다';
    } else if (lv) {
      lv.disabled = false;
      if (!lv.value && r.v) lv.value = r.v;
      if (hint) hint.textContent = '';
    }
  }
  function toggleOpen(root, open) {
    var body = root.querySelector('.as-set-body');
    if (!body) return;
    body.classList.toggle('hidden', !open);
    root.setAttribute('data-open', open ? '1' : '0');
    root.classList.toggle('border-blue-300', open);
    var btns = root.querySelectorAll('[data-set-toggle]');
    btns.forEach(function (b) {
      if (b.tagName === 'BUTTON') b.textContent = open ? '접기 ▲' : '펼치기 ▼';
    });
  }
  global.ASActionSets.toggleOpen = toggleOpen;
  function serializeSet(root) {
    var seq = setSeq(root);
    var p = prefix(seq);
    var fd = new FormData();
    fd.append('seq', String(seq));
    root.querySelectorAll('input,select,textarea').forEach(function (el) {
      if (!el.name || el.disabled) return;
      if (el.type === 'checkbox' || el.type === 'radio') {
        if (el.checked) fd.append(el.name, el.value || '1');
      } else fd.append(el.name, el.value);
    });
    var same = field(root, '.as-set-leave-same');
    if (same && !same.checked) fd.append(p + 'leave_same', '0');
    return fd;
  }
  function focusField(root, code) {
    var sel = '.as-set-result';
    if (code === 'work_date_required' || code === 'visit_date' || code === 'work_date') sel = '.as-set-work-date';
    if (code === 'process_type') sel = '.as-set-ptype';
    if (code === 'action_taken') sel = '.as-set-text';
    var el = field(root, sel);
    if (!el) return;
    el.classList.add('border-red-500');
    el.focus();
    if (el.scrollIntoView) el.scrollIntoView({ block: 'center' });
  }
  function showMissing(root, codes) {
    var el = field(root, '.as-set-missing');
    var line = missingLine(codes);
    if (!el) return;
    if (!line) { el.textContent = ''; el.classList.add('hidden'); return; }
    el.textContent = line;
    el.classList.remove('hidden');
  }
  function saveSet(root, then, opts) {
    opts = opts || {};
    var seq = setSeq(root);
    var fd = serializeSet(root);
    var res = field(root, '.as-set-result');
    if (needsResult(opts, res && res.value)) {
      showSetErr(root, '조치 결과를 먼저 선택해 주세요');
      return;
    }
    appendDraftFlag(fd, opts);
    var asId = (form.getAttribute('action') || '').split('/')[2];
    fetch('/as/' + encodeURIComponent(asId) + '/action-sets', {
      method: 'POST', credentials: 'same-origin', body: fd
    }).then(function (r) {
      return r.json().then(function (j) { return { ok: r.ok, j: j }; });
    }).then(function (x) {
      var errEl = field(root, '.as-set-err');
      if (!x.ok) {
        showSetErr(root, (x.j && (x.j.error || x.j.code)) || '저장하지 못했습니다');
        if (res && opts.requireResult !== false) res.value = '';
        toggleOpen(root, true);
        focusField(root, x.j && x.j.code);
        return;
      }
      if (errEl) { errEl.textContent = ''; errEl.classList.add('hidden'); }
      var sid = root.querySelector('input[name="' + prefix(seq) + 'set_id"]');
      if (sid && x.j.set_id) sid.value = x.j.set_id;
      showMissing(root, x.j.missing);
      var draft = opts.requireResult === false;
      toggleOpen(root, draft);
      toast(x.j.message || ('조치 ' + seq + ' 임시 저장됨'));
      if (then) then();
    }).catch(function () {
      showSetErr(root, '네트워크 오류');
      toggleOpen(root, true);
    });
  }
  function showSetErr(root, msg) {
    var errEl = field(root, '.as-set-err');
    if (errEl) { errEl.textContent = msg; errEl.classList.remove('hidden'); }
    toast(msg);
  }
  function syncVisitHint(root) {
    var ptype = field(root, '.as-set-ptype');
    var hint = root.querySelector('.as-set-visit');
    if (hint) hint.classList.toggle('hidden', !ptype || ptype.value !== 'visit');
  }
  function bindSet(root) {
    root.querySelectorAll('[data-set-toggle]').forEach(function (el) {
      el.addEventListener('click', function (e) {
        e.preventDefault();
        toggleOpen(root, root.getAttribute('data-open') !== '1');
      });
    });
    var res = field(root, '.as-set-result');
    if (res) res.addEventListener('change', function () {
      if (res.value) saveSet(root, null, { requireResult: true });
      mirrorLast();
      if (window.syncResultUI) window.syncResultUI();
    });
    var saveBtn = field(root, '.as-set-save');
    if (saveBtn) saveBtn.addEventListener('click', function () { saveSet(root, null, { requireResult: false }); });
    var ptype = field(root, '.as-set-ptype');
    if (ptype) ptype.addEventListener('change', function () { syncVisitHint(root); });
    ['.as-set-start', '.as-set-dur', '.as-set-leave-same'].forEach(function (cls) {
      var el = field(root, cls);
      if (el) el.addEventListener('change', function () { syncTimes(root); });
    });
    syncTimes(root);
    syncVisitHint(root);
  }
  function mirrorLast() {
    var last = lastSet();
    if (!last) return;
    function copy(srcSel, destId) {
      var s = last.querySelector(srcSel);
      var d = document.getElementById(destId);
      if (s && d) d.value = s.value;
    }
    var place = last.querySelector('.as-set-place:checked');
    var wp = document.getElementById('as-work-place-mirror');
    if (place && wp) wp.value = place.value;
    if (place) {
      document.querySelectorAll('input[name="work_place"]').forEach(function (el) {
        el.checked = el.value === place.value;
      });
    }
    copy('.as-set-ptype', 'as-process-type');
    var work = last.querySelector('.as-set-work-date');
    copyWorkDateToVisit(work && work.value, document.getElementById('as-visit-date'));
    copy('.as-set-cause', 'as-cause-detail');
    var ta = last.querySelector('.as-set-text');
    var dest = document.getElementById('as-action-taken');
    if (ta && dest) dest.value = ta.value;
    var rs = last.querySelector('.as-set-result');
    var rc = document.getElementById('as-result-code');
    if (rs && rc) rc.value = rs.value;
  }
  function cloneSet() {
    var n = box.querySelectorAll('.as-action-set').length;
    if (n >= maxSets) { toast('조치는 20개까지입니다'); return; }
    var src = lastSet();
    if (!src) return;
    var node = src.cloneNode(true);
    var seq = n + 1;
    node.setAttribute('data-action-set', String(seq));
    node.setAttribute('data-set-id', '');
    node.querySelectorAll('input,select,textarea').forEach(function (el) {
      if (!el.name) return;
      el.name = el.name.replace(/s\d+_/, prefix(seq));
      if (el.classList.contains('as-set-text') || el.classList.contains('as-set-cause') || el.classList.contains('as-set-result')) el.value = '';
      if (el.classList.contains('as-set-save') === false && el.type === 'hidden' && el.name.indexOf('set_id') >= 0) el.value = '';
    });
    var title = node.querySelector('.as-set-head b');
    if (title) title.textContent = '조치 ' + seq;
    box.appendChild(node);
    bindSet(node);
    toggleOpen(src, false);
    toggleOpen(node, true);
    node.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    toast('조치 ' + seq + '을(를) 추가했습니다');
  }
  if (addBtn) addBtn.addEventListener('click', function () {
    var last = lastSet();
    var res = last && field(last, '.as-set-result');
    if (!res || !res.value) {
      if (addErr) addErr.classList.remove('hidden');
      if (last) { toggleOpen(last, true); showSetErr(last, '조치 ' + setSeq(last) + '의 결과를 먼저 선택해 주세요. 결과를 고르면 이 조치가 저장되고 다음 조치를 추가할 수 있습니다.'); }
      toast('조치 ' + (last ? setSeq(last) : 1) + '의 결과를 먼저 선택하세요');
      return;
    }
    if (addErr) addErr.classList.add('hidden');
    cloneSet();
  });
  box.querySelectorAll('.as-action-set').forEach(bindSet);
  form.addEventListener('submit', function () { mirrorLast(); });
  window.addEventListener('beforeunload', function (e) {
    var dirty = false;
    box.querySelectorAll('.as-action-set').forEach(function (root) {
      var t = field(root, '.as-set-text');
      var r = field(root, '.as-set-result');
      if (t && t.value && r && !r.value) dirty = true;
    });
    if (dirty) { e.preventDefault(); e.returnValue = ''; }
  });
  if (!slots.length && !durs.length) return;
})(typeof window !== 'undefined' ? window : globalThis);
