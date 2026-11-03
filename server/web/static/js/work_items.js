(function () {
  function el(id) { return document.getElementById(id); }
  function hide(n, on) {
    if (!n) return;
    n.hidden = !!on;
    n.classList.toggle('hidden', !!on);
  }
  function rootOf(node) {
    return node && node.closest('[data-work-items]');
  }
  function csrfHeaders() {
    return { 'Accept': 'application/json', 'X-Requested-With': 'fetch', 'Content-Type': 'application/x-www-form-urlencoded' };
  }

  function ensureModals() {
    if (el('wi-asset-modal')) return;
    var html = ''
      + '<div id="wi-asset-modal" hidden class="hidden modal-backdrop z-[90] flex items-center justify-center p-4" role="dialog">'
      + '  <div class="modal-panel w-full max-w-lg shadow-pop space-y-3">'
      + '    <h2 class="text-ui font-bold text-ink">자산 등록</h2>'
      + '    <form id="wi-asset-form" method="post" class="space-y-2">'
      + '      <input type="hidden" name="item_id" id="wi-asset-item">'
      + '      <label class="label">장비명</label><input class="input" name="product_name" id="wi-asset-product">'
      + '      <label class="label">모델</label><input class="input" name="model_name" id="wi-asset-model">'
      + '      <label class="label">설치일</label><input type="date" class="input" name="install_date" id="wi-asset-date">'
      + '      <label class="label">관리유형</label>'
      + '      <select class="input" name="management_type" id="wi-asset-mgmt" required>'
      + '        <option value="">선택</option>'
      + '        <option value="fault">장애대응</option>'
      + '        <option value="periodic">정기점검</option>'
      + '        <option value="third_party">타사장비</option>'
      + '      </select>'
      + '      <div id="wi-asset-rows" class="space-y-2"></div>'
      + '      <p id="wi-asset-err" class="hidden text-ui-sm text-danger"></p>'
      + '      <div class="flex justify-end gap-2">'
      + '        <button type="button" class="btn btn-ghost" id="wi-asset-cancel">취소</button>'
      + '        <button type="submit" class="btn btn-primary">확인</button>'
      + '      </div></form></div></div>'
      + '<div id="wi-pick-modal" hidden class="hidden modal-backdrop z-[90] flex items-center justify-center p-4" role="dialog">'
      + '  <div class="modal-panel w-full max-w-lg shadow-pop space-y-3">'
      + '    <h2 class="text-ui font-bold text-ink">대상 자산 선택</h2>'
      + '    <p id="wi-pick-hidden" class="text-ui-sm text-ink-muted"></p>'
      + '    <label class="label">장비 종류</label>'
      + '    <select class="input" id="wi-pick-cat"></select>'
      + '    <label class="inline-flex items-center gap-2 text-ui-sm"><input type="checkbox" id="wi-pick-all"> 전체 선택</label>'
      + '    <div id="wi-pick-list" class="max-h-64 overflow-y-auto space-y-1"></div>'
      + '    <p id="wi-pick-count" class="text-ui-sm text-ink-muted"></p>'
      + '    <p id="wi-pick-err" class="hidden text-ui-sm text-danger"></p>'
      + '    <div class="flex justify-end gap-2">'
      + '      <button type="button" class="btn btn-ghost" id="wi-pick-clear">선택 해제</button>'
      + '      <button type="button" class="btn btn-ghost" id="wi-pick-cancel">취소</button>'
      + '      <button type="button" class="btn btn-primary" id="wi-pick-ok">적용</button>'
      + '    </div></div></div>'
      + '<div id="wi-as-modal" hidden class="hidden modal-backdrop z-[90] flex items-center justify-center p-4" role="dialog">'
      + '  <div class="modal-panel w-full max-w-lg shadow-pop space-y-3">'
      + '    <h2 class="text-ui font-bold text-ink">AS 접수 연결</h2>'
      + '    <form id="wi-as-form" method="post" class="space-y-2">'
      + '      <input type="hidden" name="item_id" id="wi-as-item">'
      + '      <label class="label">증상</label><textarea class="input min-h-[5rem]" name="symptom" id="wi-as-symptom"></textarea>'
      + '      <label class="label">기존 접수</label><select class="input" name="as_id" id="wi-as-existing"><option value="">새 접수 만들기</option></select>'
      + '      <p id="wi-as-err" class="hidden text-ui-sm text-danger"></p>'
      + '      <div class="flex justify-end gap-2">'
      + '        <button type="button" class="btn btn-ghost" id="wi-as-cancel">취소</button>'
      + '        <button type="submit" class="btn btn-primary">확인</button>'
      + '      </div></form></div></div>'
      + '<div id="wi-missing-modal" hidden class="hidden modal-backdrop z-[90] flex items-center justify-center p-4" role="dialog">'
      + '  <div class="modal-panel w-full max-w-md shadow-pop space-y-3">'
      + '    <p class="text-ui font-semibold text-ink" id="wi-missing-msg">설치 완료인데 자산 미등록 N건</p>'
      + '    <div class="flex justify-end gap-2">'
      + '      <button type="button" class="btn btn-secondary" id="wi-missing-now">지금 등록</button>'
      + '      <button type="button" class="btn btn-ghost" id="wi-missing-later">나중에</button>'
      + '    </div></div></div>';
    document.body.insertAdjacentHTML('beforeend', html);
  }

  var pickState = { assets: [], filtered: [], taskId: '', itemId: '', role: 'service' };

  window.workItemsOpenAsset = function (btn, row) {
    ensureModals();
    var root = rootOf(btn);
    var form = el('wi-asset-form');
    form.action = '/workboard/tasks/' + encodeURIComponent(root.getAttribute('data-task-id')) + '/item-assets';
    el('wi-asset-item').value = row.ItemID || '';
    el('wi-asset-product').value = row.ProductName || row.Title || '';
    el('wi-asset-model').value = row.ModelName || '';
    el('wi-asset-date').value = root.getAttribute('data-work-date') || '';
    el('wi-asset-mgmt').value = '';
    var n = parseInt(row.Qty, 10) || 1;
    var box = el('wi-asset-rows');
    box.innerHTML = '';
    for (var i = 0; i < n; i++) {
      box.insertAdjacentHTML('beforeend',
        '<div class="grid grid-cols-1 sm:grid-cols-3 gap-2">'
        + '<input class="input" name="asset_serial" placeholder="일련번호(선택)">'
        + '<input class="input" name="asset_location" placeholder="설치 위치">'
        + '<input class="input" name="asset_note" placeholder="비고"></div>');
    }
    hide(el('wi-asset-err'), true);
    hide(el('wi-asset-modal'), false);
  };

  window.workItemsOpenPick = function (btn, row) {
    ensureModals();
    var root = rootOf(btn);
    pickState.taskId = root.getAttribute('data-task-id');
    pickState.itemId = row.ItemID || '';
    pickState.role = row.Kind === 'install' ? 'install' : (row.Kind === 'as' ? 'service' : 'ref');
    fetch('/workboard/tasks/' + encodeURIComponent(pickState.taskId) + '/item-assets', { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        pickState.assets = data.assets || [];
        var sel = el('wi-pick-cat');
        sel.innerHTML = '';
        var total = data.total || pickState.assets.length;
        sel.appendChild(new Option('전체 보기 (' + total + '건)', ''));
        var cats = data.categories || {};
        Object.keys(cats).forEach(function (k) {
          sel.appendChild(new Option(k + ' (' + cats[k] + '건)', k));
        });
        el('wi-pick-hidden').textContent = data.hidden ? ('다른 기관 자산 ' + data.hidden + '건은 숨겼습니다') : '';
        renderPick();
        hide(el('wi-pick-modal'), false);
      });
  };

  function renderPick() {
    var cat = el('wi-pick-cat').value;
    var list = el('wi-pick-list');
    list.innerHTML = '';
    pickState.filtered = pickState.assets.filter(function (a) { return !cat || a.category === cat || a.cat_label === cat; });
    pickState.filtered.forEach(function (a) {
      var id = 'wi-a-' + a.asset_id;
      list.insertAdjacentHTML('beforeend',
        '<label class="flex items-center gap-2 text-ui-sm"><input type="checkbox" class="wi-pick-cb" value="' + a.asset_id + '" id="' + id + '">'
        + '<span>' + (a.serial ? a.serial + ' · ' : '') + (a.name || '') + ' · ' + (a.category || '') + ' · ' + (a.location || '') + '</span></label>');
    });
    updatePickCount();
  }

  function updatePickCount() {
    var n = document.querySelectorAll('.wi-pick-cb:checked').length;
    el('wi-pick-count').textContent = '선택 ' + n + '건';
  }

  window.workItemsOpenAS = function (btn, row) {
    ensureModals();
    var root = rootOf(btn);
    var tid = root.getAttribute('data-task-id');
    el('wi-as-form').action = '/workboard/tasks/' + encodeURIComponent(tid) + '/item-as';
    el('wi-as-item').value = row.ItemID || '';
    el('wi-as-symptom').value = row.Title || row.ProductName || '';
    fetch('/workboard/tasks/' + encodeURIComponent(tid) + '/open-as', { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        var sel = el('wi-as-existing');
        sel.innerHTML = '<option value="">새 접수 만들기</option>';
        (data.items || []).forEach(function (it) {
          sel.appendChild(new Option((it.as_number || it.as_id) + ' ' + (it.symptom || ''), it.as_id));
        });
        hide(el('wi-as-modal'), false);
      });
  };

  document.addEventListener('change', function (e) {
    if (e.target.id === 'wi-pick-cat') renderPick();
    if (e.target.id === 'wi-pick-all') {
      document.querySelectorAll('.wi-pick-cb').forEach(function (cb) { cb.checked = e.target.checked; });
      updatePickCount();
    }
    if (e.target.classList.contains('wi-pick-cb')) updatePickCount();
  });
  document.addEventListener('click', function (e) {
    if (e.target.id === 'wi-asset-cancel') hide(el('wi-asset-modal'), true);
    if (e.target.id === 'wi-pick-cancel') hide(el('wi-pick-modal'), true);
    if (e.target.id === 'wi-as-cancel') hide(el('wi-as-modal'), true);
    if (e.target.id === 'wi-pick-clear') {
      document.querySelectorAll('.wi-pick-cb').forEach(function (cb) { cb.checked = false; });
      updatePickCount();
    }
    if (e.target.id === 'wi-pick-ok') {
      var ids = [];
      document.querySelectorAll('.wi-pick-cb:checked').forEach(function (cb) { ids.push(cb.value); });
      var body = new URLSearchParams();
      body.set('item_id', pickState.itemId);
      body.set('role', pickState.role);
      body.set('kind', pickState.role === 'service' ? 'as' : '');
      ids.forEach(function (id) { body.append('asset_id', id); });
      fetch('/workboard/tasks/' + encodeURIComponent(pickState.taskId) + '/item-assets/link', {
        method: 'POST', headers: csrfHeaders(), body: body.toString()
      }).then(function (r) {
        if (!r.ok) return r.json().then(function (j) { throw new Error(j.message || '실패'); });
        hide(el('wi-pick-modal'), true);
        location.reload();
      }).catch(function (err) {
        el('wi-pick-err').textContent = err.message;
        hide(el('wi-pick-err'), false);
      });
    }
    if (e.target.id === 'wi-missing-later') hide(el('wi-missing-modal'), true);
    if (e.target.id === 'wi-missing-now') {
      hide(el('wi-missing-modal'), true);
      var b = document.querySelector('[data-item-asset-register]');
      if (b) b.click();
    }
  });
  document.addEventListener('submit', function (e) {
    if (e.target && e.target.id === 'wi-asset-form') {
      if (!el('wi-asset-mgmt').value) {
        e.preventDefault();
        el('wi-asset-err').textContent = '관리유형을 선택하세요';
        hide(el('wi-asset-err'), false);
        el('wi-asset-mgmt').focus();
      }
    }
    if (e.target && e.target.id === 'wi-as-form') {
      if (el('wi-as-existing').value) {
        var inp = document.createElement('input');
        inp.type = 'hidden'; inp.name = 'mode'; inp.value = 'existing';
        e.target.appendChild(inp);
      } else {
        var inp2 = document.createElement('input');
        inp2.type = 'hidden'; inp2.name = 'mode'; inp2.value = 'new';
        e.target.appendChild(inp2);
      }
    }
    var form = e.target;
    if (!form || !form.querySelector('[data-work-items]')) return;
    if (form.dataset.wiMissingOk === '1') return;
    var missing = 0;
    form.querySelectorAll('[data-work-items] [name="item_kind"]').forEach(function (sel, i) {
      if (sel.value !== 'install') return;
      var st = form.querySelectorAll('[name="item_status"]')[i];
      var badge = sel.closest('div').parentElement && sel.closest('[class*="grid"]');
      if (st && st.value === 'done' && badge && badge.innerText.indexOf('자산 미등록') >= 0) missing++;
    });
    if (missing > 0) {
      e.preventDefault();
      ensureModals();
      el('wi-missing-msg').textContent = '설치 완료인데 자산 미등록 ' + missing + '건';
      hide(el('wi-missing-modal'), false);
      el('wi-missing-later').onclick = function () {
        form.dataset.wiMissingOk = '1';
        hide(el('wi-missing-modal'), true);
        if (form.requestSubmit) form.requestSubmit(); else form.submit();
      };
    }
  });
})();
