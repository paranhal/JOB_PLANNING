// 95-183 C-2 수행 구조 「변경 내용 저장」.
// 1단계·하부 업무·작업 계획의 수정 폼(form[data-struct-form])을 지켜보다가 값이 바뀌면 그 줄에 「수정됨」을 붙이고,
// 화면 아래 저장 막대로 바뀐 곳을 한 번에 저장한다(서버가 한 트랜잭션: 전부 저장 또는 전부 취소).
(function () {
  var bar = document.querySelector('[data-struct-bar]');
  if (!bar) return;
  var forms = [].slice.call(document.querySelectorAll('form[data-struct-form]'));
  if (!forms.length) return;
  var countEl = bar.querySelector('[data-struct-count]');
  var msgEl = bar.querySelector('[data-struct-msg]');
  var saveBtn = bar.querySelector('[data-struct-save]');
  var resetBtn = bar.querySelector('[data-struct-reset]');
  var allowLeave = false;
  var saving = false;
  var KEY = 'struct-batch-restore:' + location.pathname;

  // 연장 사유는 값이 바뀐 걸로 치지 않는다(마감일을 바꿀 때 같이 쓰는 입력이다). 저장 때는 함께 보낸다.
  function values(form, all) {
    var out = {};
    [].slice.call(form.elements).forEach(function (el) {
      if (!el.name || el.type === 'submit' || el.type === 'button') return;
      if (!all && el.name === 'extend_reason') return;
      if (el.type === 'checkbox' && !el.checked) return;
      out[el.name] = el.value;
    });
    return out;
  }
  function same(a, b) {
    var ka = Object.keys(a), kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    return ka.every(function (k) { return a[k] === b[k]; });
  }
  forms.forEach(function (f) { f._orig = values(f, false); });

  function dirtyForms() { return forms.filter(function (f) { return !same(f._orig, values(f, false)); }); }

  function badge(f, on) {
    var det = f.closest('details');
    var sum = det && det.querySelector('summary');
    var tag = f.querySelector('[data-dirty-tag]');
    if (on) {
      f.setAttribute('data-dirty', '1');
      f.style.outline = '2px solid var(--color-primary, #2563eb)';
      f.style.outlineOffset = '2px';
      if (sum && !sum.querySelector('[data-dirty-badge]')) {
        var b = document.createElement('span');
        b.setAttribute('data-dirty-badge', '1');
        b.className = 'badge badge-warning ml-1';
        b.textContent = '수정됨';
        sum.appendChild(b);
      }
      if (!tag) { /* 접힘 밖 폼은 윤곽선만 */ }
    } else {
      f.removeAttribute('data-dirty');
      f.style.outline = '';
      f.style.outlineOffset = '';
      var old = sum && sum.querySelector('[data-dirty-badge]');
      if (old) old.remove();
      var err = f.parentNode && f.parentNode.querySelector('[data-struct-err]');
      if (err) err.remove();
    }
  }

  function refresh() {
    var d = dirtyForms();
    forms.forEach(function (f) { badge(f, d.indexOf(f) >= 0); });
    if (!d.length) {
      bar.hidden = true;
      msgEl.textContent = '';
      document.body.style.paddingBottom = '';
      return;
    }
    bar.hidden = false;
    countEl.textContent = d.length + '곳 수정됨';
    // 막대가 마지막 내용을 가리지 않도록 아래 여백을 둔다.
    document.body.style.paddingBottom = bar.offsetHeight + 'px';
  }

  forms.forEach(function (f) {
    f.addEventListener('input', refresh);
    f.addEventListener('change', refresh);
    f.addEventListener('submit', function () {
      // 개별 저장: 나머지 수정이 없을 때만 이동 확인을 건너뛴다.
      var d = dirtyForms();
      if (d.length <= 1) allowLeave = true;
    });
  });

  window.addEventListener('beforeunload', function (e) {
    if (allowLeave || saving || !dirtyForms().length) return;
    e.preventDefault();
    e.returnValue = '';
  });

  function remember() {
    var open = [];
    [].slice.call(document.querySelectorAll('details[data-phase-edit][open], details[data-pkg-meta-edit][open], details[data-plan-edit][open]')).forEach(function (d) {
      var f = d.querySelector('form[data-struct-form]');
      if (f) open.push(f.getAttribute('data-struct-form') + ':' + f.getAttribute('data-id'));
    });
    try { sessionStorage.setItem(KEY, JSON.stringify({ y: window.scrollY, open: open })); } catch (e) { /* 저장소가 막혀도 저장은 된다 */ }
  }
  function restore() {
    var raw = null;
    try { raw = sessionStorage.getItem(KEY); sessionStorage.removeItem(KEY); } catch (e) { return; }
    if (!raw) return;
    var st;
    try { st = JSON.parse(raw); } catch (e) { return; }
    (st.open || []).forEach(function (k) {
      var parts = k.split(':');
      var f = document.querySelector('form[data-struct-form="' + parts[0] + '"][data-id="' + parts[1] + '"]');
      var d = f && f.closest('details');
      if (d) d.open = true;
    });
    window.scrollTo(0, st.y || 0);
    // #project-phases 앵커가 뒤늦게 위치를 옮기므로 불러온 뒤 한 번 더 맞춘다.
    window.addEventListener('load', function () { window.scrollTo(0, st.y || 0); });
  }
  restore();

  function showError(items, j) {
    msgEl.textContent = j && j.message ? j.message : '저장하지 못했습니다';
    var f = j && typeof j.index === 'number' && items[j.index] ? items[j.index].form : null;
    if (!f) return;
    var det = f.closest('details');
    if (det) det.open = true;
    var p = f.parentNode.querySelector('[data-struct-err]');
    if (!p) {
      p = document.createElement('p');
      p.setAttribute('data-struct-err', '1');
      p.className = 'text-sm text-warning';
      p.setAttribute('role', 'alert');
      f.parentNode.insertBefore(p, f.nextSibling);
    }
    p.textContent = j.message;
    if (f.scrollIntoView) f.scrollIntoView({ block: 'center' });
  }

  saveBtn.addEventListener('click', function () {
    if (saving) return;
    var d = dirtyForms();
    if (!d.length) return;
    var items = d.map(function (f) {
      return { kind: f.getAttribute('data-struct-form'), id: f.getAttribute('data-id'), fields: values(f, true), form: f };
    });
    saving = true;
    saveBtn.disabled = true;
    resetBtn.disabled = true;
    msgEl.textContent = '저장 중…';
    fetch(bar.getAttribute('data-url'), {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json', 'X-Requested-With': 'fetch' },
      body: JSON.stringify({ items: items.map(function (i) { return { kind: i.kind, id: i.id, fields: i.fields }; }) })
    }).then(function (r) {
      return r.json().catch(function () { return { ok: false, message: '저장하지 못했습니다(' + r.status + ')' }; }).then(function (j) { return { status: r.status, body: j }; });
    }).then(function (res) {
      if (res.body && res.body.ok) {
        allowLeave = true;
        remember();
        var u = location.pathname + '?ok=batch:' + res.body.n + '#project-phases';
        location.href = u;
        return;
      }
      saving = false;
      saveBtn.disabled = false;
      resetBtn.disabled = false;
      showError(items, res.body);
    }).catch(function () {
      saving = false;
      saveBtn.disabled = false;
      resetBtn.disabled = false;
      msgEl.textContent = '저장하지 못했습니다. 네트워크를 확인하고 다시 눌러 주세요(입력한 값은 그대로입니다).';
    });
  });

  resetBtn.addEventListener('click', function () {
    dirtyForms().forEach(function (f) { f.reset(); });
    refresh();
  });

  refresh();
})();
