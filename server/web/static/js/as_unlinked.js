// 95-202: /as 하단 「자산 연결이 없는 작업」 지연 로딩 보조.
//  - 첫 요청에 주소의 ul_* 와 범위(mine·assignee·assignee_name·scope) 파라미터를 실어 보내 새로고침·뒤로 가기에서도 같은 탭·필터로 열린다.
//  - 필터 폼 제출 때 /as 주소에 ul_* 를 남긴다(주소로 상태 유지).
//  - 조회가 실패하면(HTTP 오류·네트워크) 영역 안에 「목록을 불러오지 못했습니다 · 다시 시도」를 보인다. 나머지 화면은 그대로 둔다.
(function () {
  var SCOPE = ['mine', 'assignee', 'assignee_name', 'scope'];

  function root() { return document.getElementById('as-unlinked'); }

  function initialQuery() {
    var src = new URLSearchParams(window.location.search), out = new URLSearchParams();
    src.forEach(function (v, k) { if (k.indexOf('ul_') === 0 || SCOPE.indexOf(k) >= 0) out.append(k, v); });
    return out.toString();
  }

  document.addEventListener('htmx:configRequest', function (e) {
    var el = e.detail.elt;
    if (el && el.id === 'as-unlinked' && /\/as\/unlinked\/counts/.test(e.detail.path)) {
      var q = initialQuery();
      if (q) e.detail.path += (e.detail.path.indexOf('?') >= 0 ? '&' : '?') + q;
    }
  });

  function errorBox(retry) {
    var d = document.createElement('div');
    d.setAttribute('data-unlinked-error', '1');
    d.setAttribute('role', 'alert');
    d.className = 'rounded-card border border-danger bg-danger-soft px-4 py-3 text-ui-sm text-danger';
    var p = document.createElement('p'); p.textContent = '목록을 불러오지 못했습니다.'; d.appendChild(p);
    var b = document.createElement('button'); b.type = 'button'; b.className = 'btn-secondary btn-sm mt-2'; b.textContent = '다시 시도';
    b.addEventListener('click', function () { retry(); });
    d.appendChild(b);
    return d;
  }

  function reload() {
    var r = root(); if (!r || !window.htmx) return;
    var q = initialQuery();
    window.htmx.ajax('GET', '/as/unlinked/counts' + (q ? '?' + q : ''), { target: '#as-unlinked-inner', swap: 'innerHTML' });
  }

  function onFail(e) {
    var r = root();
    if (!r || !e.detail || !e.detail.elt || !r.contains(e.detail.elt)) return;
    var target = e.detail.target || document.getElementById('as-unlinked-inner');
    if (!target) return;
    var elt = e.detail.elt, path = (e.detail.pathInfo && e.detail.pathInfo.requestPath) || '';
    target.innerHTML = '';
    target.appendChild(errorBox(function () {
      if (window.htmx && path) window.htmx.ajax('GET', path, { target: '#' + target.id, swap: 'innerHTML' });
      else reload();
    }));
  }
  document.addEventListener('htmx:responseError', onFail);
  document.addEventListener('htmx:sendError', onFail);

  // 필터 제출: /as 주소에 ul_* 를 남긴다(새로고침·뒤로 가기에서 필터 유지).
  document.addEventListener('submit', function (e) {
    var f = e.target;
    if (!f.matches || !f.matches('[data-unlinked-filter]')) return;
    var u = new URL(window.location.href), fd = new FormData(f);
    Array.from(u.searchParams.keys()).forEach(function (k) { if (k.indexOf('ul_') === 0) u.searchParams.delete(k); });
    u.searchParams.set('ul_tab', f.getAttribute('data-tab') || 'as');
    ['q', 'status', 'who', 'from', 'to', 'product', 'sort'].forEach(function (k) {
      var v = (fd.get(k) || '').toString().trim();
      if (k === 'sort' && (v === 'recv_desc' || v === 'visit_desc')) v = ''; // 기본 정렬은 주소에서 뺀다
      if (v) u.searchParams.set('ul_' + k, v);
    });
    window.history.pushState({ asUnlinked: 1 }, '', u.pathname + u.search);
  });

  // 정렬 선택을 바꾸면 바로 조회한다(기본 정렬 값은 주소에서 뺀다).
  document.addEventListener('change', function (e) {
    var sel = e.target;
    if (!sel.matches || !sel.matches('[data-unlinked-sort-select]')) return;
    var f = sel.closest('form');
    if (f && f.requestSubmit) f.requestSubmit();
  });

  // 뒤로 가기: 주소의 ul_* 대로 영역을 다시 불러온다.
  window.addEventListener('popstate', function () { if (root()) reload(); });
})();

// 95-204: 일괄 연결 — 체크한 건만 모아 확인 창(앞 5건 미리보기)을 거쳐 연결한다. 대상 판정·권한은 서버가 요청 시점에 다시 한다.
(function () {
  var PREVIEW = 5;
  function body() { return document.getElementById('as-unlinked-body'); }
  function checks() { return Array.prototype.slice.call(document.querySelectorAll('#as-unlinked-body [data-unlinked-check]')); }
  function selectedValues() {
    var seen = {}, out = [];
    checks().forEach(function (c) { if (c.checked && !seen[c.value]) { seen[c.value] = c; out.push(c); } });
    return out;
  }
  function sync() {
    var all = checks(), sel = selectedValues();
    var n = document.querySelector('#as-unlinked-body [data-unlinked-selected]');
    if (n) n.textContent = String(sel.length);
    var open = document.querySelector('#as-unlinked-body [data-unlinked-open]');
    if (open) open.disabled = sel.length === 0;
    var ca = document.querySelector('#as-unlinked-body [data-unlinked-checkall]');
    if (ca) { ca.checked = all.length > 0 && all.every(function (c) { return c.checked; }); ca.indeterminate = !ca.checked && sel.length > 0; }
  }
  function modal() { return document.querySelector('#as-unlinked-body [data-unlinked-modal]'); }
  function showModal(on) {
    var m = modal(); if (!m) return;
    m.classList.toggle('hidden', !on); m.classList.toggle('flex', on);
    if (on) { var b = m.querySelector('[data-unlinked-confirm]'); if (b) { b.disabled = false; b.focus(); } }
    else { var o = document.querySelector('#as-unlinked-body [data-unlinked-open]'); if (o) o.focus(); }
  }
  document.addEventListener('change', function (e) {
    var t = e.target; if (!t.matches) return;
    if (t.matches('[data-unlinked-checkall]')) { checks().forEach(function (c) { c.checked = t.checked; }); sync(); }
    else if (t.matches('[data-unlinked-check]')) {
      // 같은 건이 표·카드 두 벌로 있어도 한 건으로 센다. 두 벌의 체크 상태를 맞춘다.
      checks().forEach(function (c) { if (c.value === t.value) c.checked = t.checked; });
      sync();
    }
  });
  document.addEventListener('click', function (e) {
    var t = e.target; if (!t.closest) return;
    if (t.closest('[data-unlinked-open]')) {
      var sel = selectedValues(), m = modal(); if (!m || !sel.length) return;
      m.querySelector('[data-unlinked-modal-n]').textContent = String(sel.length);
      var ul = m.querySelector('[data-unlinked-modal-list]'); ul.innerHTML = '';
      sel.slice(0, PREVIEW).forEach(function (c) {
        var li = document.createElement('li'); li.textContent = c.getAttribute('data-title') + ' → ' + c.getAttribute('data-asset'); ul.appendChild(li);
      });
      m.querySelector('[data-unlinked-modal-more]').textContent = sel.length > PREVIEW ? '외 ' + (sel.length - PREVIEW) + '건' : '';
      showModal(true);
    } else if (t.closest('[data-unlinked-cancel]')) {
      showModal(false);
    } else if (t.closest('[data-unlinked-confirm]')) {
      var btn = t.closest('[data-unlinked-confirm]'); if (btn.disabled) return; btn.disabled = true; // 더블 클릭 방지(서버도 멱등)
      var f = document.getElementById('ul-bulk-form'); if (f && f.requestSubmit) f.requestSubmit();
    } else if (t.matches && t.matches('[data-unlinked-modal]')) {
      showModal(false);
    }
  });
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var m = modal(); if (m && !m.classList.contains('hidden')) showModal(false);
  });
  document.addEventListener('htmx:afterSwap', function () { sync(); });
})();
