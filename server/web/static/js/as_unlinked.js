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
    ['q', 'status', 'who', 'from', 'to', 'product'].forEach(function (k) {
      var v = (fd.get(k) || '').toString().trim();
      if (v) u.searchParams.set('ul_' + k, v);
    });
    window.history.pushState({ asUnlinked: 1 }, '', u.pathname + u.search);
  });

  // 뒤로 가기: 주소의 ul_* 대로 영역을 다시 불러온다.
  window.addEventListener('popstate', function () { if (root()) reload(); });
})();
