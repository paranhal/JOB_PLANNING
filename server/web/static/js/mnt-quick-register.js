window.bindMntQuickRegister = function () {
  var btn = document.getElementById('mnt-quick-save');
  var form = document.getElementById('mnt-quick-form');
  if (!btn || !form || btn.dataset.bound === '1') return;
  btn.dataset.bound = '1';
  var monthEl = document.getElementById('mnt-quick-month');
  if (monthEl && !monthEl.value) {
    monthEl.value = String(new Date().getMonth() + 1);
  }
  var errEl = document.getElementById('mnt-quick-err');
  var hintEl = document.getElementById('mnt-quick-hint');
  function csrf() {
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') : '';
  }
  function showErr(msg) {
    if (!errEl) return;
    errEl.hidden = !msg;
    errEl.textContent = msg || '';
  }
  function customerId() {
    var hid = form.querySelector('input[name="mnt_customer_id"]');
    return hid ? hid.value : '';
  }
  btn.addEventListener('click', function () {
    showErr('');
    var cid = customerId();
    if (!cid) {
      showErr('사이트를 선택하세요');
      return;
    }
    var body = {
      customer_id: cid,
      month: parseInt(monthEl && monthEl.value, 10) || 0,
      visit_date: (document.getElementById('mnt-quick-date') || {}).value || '',
      assignee: (document.getElementById('mnt-quick-assignee') || {}).value || ''
    };
    fetch('/workboard/mnt-quick-register', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrf()
      },
      body: JSON.stringify(body)
    }).then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }); }).then(function (res) {
      if (!res.ok || (res.j && res.j.error)) {
        showErr((res.j && res.j.error) || '등록에 실패했습니다');
        return;
      }
      if (hintEl && res.j.hint) {
        hintEl.hidden = false;
        hintEl.textContent = res.j.hint;
      }
      var sel = document.getElementById('mnt-register-source');
      if (sel && res.j.visit_id) {
        var opt = document.createElement('option');
        opt.value = res.j.visit_id;
        opt.selected = true;
        opt.textContent = (res.j.title || '') + ' · ' + (res.j.source_number || '');
        sel.appendChild(opt);
        sel.value = res.j.visit_id;
        sel.dispatchEvent(new Event('change'));
      }
    }).catch(function () { showErr('등록에 실패했습니다'); });
  });
};

document.addEventListener('DOMContentLoaded', function () { window.bindMntQuickRegister(); });
document.addEventListener('htmx:afterSwap', function () { window.bindMntQuickRegister(); });
