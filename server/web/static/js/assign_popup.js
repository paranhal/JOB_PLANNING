// 95-225 배정받은 업무 팝업. 로그인한 모든 화면에서 동작한다.
// - 로그인 세션의 첫 확인에는 대기+보류 전부(서버가 세션 쿠키로 판정), 이후 60초마다 대기 중인 항목만 확인한다.
// - 항목마다 사업(또는 업무 구분)·배정자(이름·조직)·대상을 보여 주고 수락 / 거부(사유 입력 후 확정) / 보류만 있다.
// - 보류 항목에는 「계속 보류」가 있다(이 세션에서는 다시 띄우지 않고, 다음 로그인 때 다시 알린다).
(function () {
  'use strict';
  var root = document.getElementById('assign-popup');
  if (!root) return;
  var DISMISS = 'assignHeldDismissed', POLL_MS = window.ASSIGN_POLL_MS || 60000;
  var items = [];
  function csrf() { var m = document.querySelector('meta[name="csrf-token"]'); return m ? m.getAttribute('content') || '' : ''; }
  function ss(k, v) { try { if (v === undefined) return sessionStorage.getItem(k); sessionStorage.setItem(k, v); } catch (e) { return null; } return null; }
  function dismissed() { try { return JSON.parse(ss(DISMISS) || '[]'); } catch (e) { return []; } }
  function el(tag, cls, text) { var n = document.createElement(tag); if (cls) n.className = cls; if (text != null) n.textContent = text; return n; }
  function showErr(msg) { var p = root.querySelector('[data-assign-err]'); p.textContent = msg || ''; p.hidden = !msg; }
  function typeLabel(t) { return ({ sub: '하부 업무', site: '사이트', subtask: '하부 작업', as_work: 'AS 하부업무' })[t] || t; }

  function post(id, action, reason) {
    var fd = new FormData(); fd.append('action', action); if (reason != null) fd.append('reason', reason);
    return fetch('/workboard/assignments/' + encodeURIComponent(id) + '/decide', {
      method: 'POST', body: fd, credentials: 'same-origin',
      headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch', 'X-CSRF-Token': csrf() }
    }).then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }).catch(function () { return { ok: r.ok, j: {} }; }); });
  }

  function remove(id) { items = items.filter(function (a) { return a.id !== id; }); render(); }

  function render() {
    var list = root.querySelector('[data-assign-list]');
    list.textContent = '';
    var hide = dismissed();
    var shown = items.filter(function (a) { return !(a.status === 'held' && hide.indexOf(a.id) >= 0); });
    root.hidden = shown.length === 0;
    shown.forEach(function (a) {
      var li = el('li', 'rounded-lg border border-line p-3 space-y-2');
      li.setAttribute('data-assign-item', a.id);
      li.appendChild(el('p', 'text-sm font-semibold text-ink', a.project_name || a.category_name || '(사업·구분 없음)'));
      li.appendChild(el('p', 'text-ui-sm text-ink-muted', '배정: ' + a.from_name + (a.from_org_name ? ' · ' + a.from_org_name : '')));
      li.appendChild(el('p', 'text-ui-sm text-ink', typeLabel(a.target_type) + ': ' + a.target_label));
      if (a.status === 'held') li.appendChild(el('span', 'badge-neutral text-ui-xs', '보류 중'));
      var reasonBox = el('div', 'space-y-2'); reasonBox.hidden = true;
      var ta = el('textarea', 'input w-full'); ta.rows = 2; ta.placeholder = '거부 사유(필수)'; ta.setAttribute('data-assign-reason', '');
      var rrow = el('div', 'flex gap-2 justify-end');
      var rok = el('button', 'btn btn-danger btn-sm', '거부 확정'); rok.type = 'button'; rok.setAttribute('data-assign-reject-confirm', '');
      var rcancel = el('button', 'btn btn-secondary btn-sm', '되돌아가기'); rcancel.type = 'button';
      rrow.appendChild(rcancel); rrow.appendChild(rok); reasonBox.appendChild(ta); reasonBox.appendChild(rrow);
      var row = el('div', 'flex flex-wrap gap-2');
      var bAccept = el('button', 'btn btn-primary btn-sm', '수락'); bAccept.type = 'button'; bAccept.setAttribute('data-assign-accept', '');
      var bReject = el('button', 'btn btn-secondary btn-sm', '거부'); bReject.type = 'button'; bReject.setAttribute('data-assign-reject', '');
      row.appendChild(bAccept); row.appendChild(bReject);
      if (a.status === 'pending') { var bHold = el('button', 'btn btn-secondary btn-sm', '보류'); bHold.type = 'button'; bHold.setAttribute('data-assign-hold', ''); row.appendChild(bHold);
        bHold.addEventListener('click', function () { go(a, 'hold'); }); }
      if (a.status === 'held') { var bKeep = el('button', 'btn btn-ghost btn-sm', '계속 보류'); bKeep.type = 'button'; bKeep.setAttribute('data-assign-keep', ''); row.appendChild(bKeep);
        bKeep.addEventListener('click', function () { var d = dismissed(); d.push(a.id); ss(DISMISS, JSON.stringify(d)); render(); }); }
      bAccept.addEventListener('click', function () { go(a, 'accept'); });
      bReject.addEventListener('click', function () { reasonBox.hidden = false; row.hidden = true; ta.focus(); });
      rcancel.addEventListener('click', function () { reasonBox.hidden = true; row.hidden = false; showErr(''); });
      rok.addEventListener('click', function () {
        if (!ta.value.trim()) { showErr('거부 사유를 입력하세요'); ta.focus(); return; }
        go(a, 'reject', ta.value.trim());
      });
      li.appendChild(row); li.appendChild(reasonBox);
      list.appendChild(li);
    });
  }

  function go(a, action, reason) {
    showErr('');
    post(a.id, action, reason).then(function (res) {
      if (!res.ok) { showErr((res.j && res.j.error) || '처리하지 못했습니다'); if (res.j && /종료/.test(res.j.error || '')) remove(a.id); return; }
      remove(a.id);
      if (action === 'hold') { /* 보류 항목은 업무 화면 「보류」 그룹에 모인다 */ }
      if (action === 'accept' && typeof window.location !== 'undefined' && /\/workboard\//.test(window.location.pathname)) window.location.reload();
    });
  }

  function merge(list) {
    list.forEach(function (n) {
      var i = items.findIndex(function (a) { return a.id === n.id; });
      if (i >= 0) items[i] = n; else items.push(n);
    });
    render();
  }

  function check() {
    return fetch('/workboard/assignments/popup', { credentials: 'same-origin', headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' } })
      .then(function (r) { var ct = r.headers.get('content-type') || ''; return r.ok && ct.indexOf('application/json') >= 0 ? r.json() : null; })
      .then(function (j) { if (!j) return; if (j.first) { try { sessionStorage.removeItem(DISMISS); } catch (e) { /* 무시 */ } items = []; } merge(j.items || []); })
      .catch(function () { /* 알림 확인 실패는 화면을 막지 않는다 */ });
  }

  check();
  setInterval(check, POLL_MS);
})();
