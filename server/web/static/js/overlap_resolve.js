(function () {
  function listEl() { return document.getElementById('overlap-conflict-list'); }
  function mask() { return document.getElementById('overlap-resolve'); }
  function errEl() { return document.getElementById('overlap-resolve-err'); }
  function busyEl() { return document.getElementById('overlap-resolve-busy'); }
  var pendingForm = null;
  var pendingConflicts = [];
  var lastFocus = null;
  var busy = false;

  function placeLabel(c) {
    if (c.work_place_label) return c.work_place_label;
    if (c.work_place === 'field') return '외근';
    if (c.work_place === 'office') return '내근';
    return '';
  }

  function isOpen() {
    var m = mask();
    return !!(m && !m.hidden && !m.classList.contains('hidden'));
  }

  function focusables() {
    var m = mask();
    if (!m) return [];
    return Array.prototype.slice.call(m.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'))
      .filter(function (el) { return !el.disabled && el.getAttribute('aria-hidden') !== 'true'; });
  }

  function actionButtons() {
    var m = mask();
    if (!m) return [];
    return Array.prototype.slice.call(m.querySelectorAll('[data-overlap-action]'));
  }

  function setBusy(on) {
    busy = !!on;
    var b = busyEl();
    if (b) b.hidden = !busy;
    actionButtons().forEach(function (btn) {
      btn.disabled = busy;
      if (busy) {
        if (!btn.dataset.overlapLabel) btn.dataset.overlapLabel = btn.textContent;
        if (btn.getAttribute('data-overlap-action') !== 'cancel') btn.textContent = '처리 중…';
      } else if (btn.dataset.overlapLabel) {
        btn.textContent = btn.dataset.overlapLabel;
      }
    });
  }

  function showModalError(msg) {
    var el = errEl();
    if (el) {
      el.hidden = !msg;
      el.textContent = msg || '';
      if (msg && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'nearest' });
      return;
    }
    flash(msg || '저장하지 못했습니다');
  }

  function clearModalError() {
    var el = errEl();
    if (el) { el.hidden = true; el.textContent = ''; }
  }

  function flattenConflicts(payload) {
    if (payload && payload.conflicts && payload.conflicts.length) return payload.conflicts;
    var days = (payload && payload.dates) || [];
    var out = [];
    days.forEach(function (d) {
      (d.conflicts || []).forEach(function (c) {
        if (d.date && !c.date) c.date = d.date;
        out.push(c);
      });
    });
    return out;
  }

  function renderConflicts(conflicts) {
    var ul = listEl();
    if (!ul) return;
    ul.innerHTML = '';
    var groups = {};
    var order = [];
    (conflicts || []).forEach(function (c) {
      var d = c.date || '';
      if (!groups[d]) { groups[d] = []; order.push(d); }
      groups[d].push(c);
    });
    var many = (conflicts || []).length > 20;
    order.forEach(function (d, gi) {
      var wrap = document.createElement('li');
      wrap.className = 'rounded-control border border-line px-3 py-2 text-ui text-ink space-y-1';
      if (many && gi > 2) wrap.hidden = true;
      var head = document.createElement('p');
      head.className = 'font-semibold';
      head.textContent = d || '겹친 일정';
      wrap.appendChild(head);
      groups[d].forEach(function (c) {
        var p = document.createElement('p');
        var place = placeLabel(c);
        p.textContent = (c.title || c.task_id) +
          (c.assignee ? ' · ' + c.assignee : '') +
          (c.start ? ' · ' + c.start + '~' + (c.end || '') : '') +
          (place ? ' · ' + place : '');
        wrap.appendChild(p);
        if (c.split_block_label || (!c.can_split && c.split_block_reason)) {
          var why = document.createElement('p');
          why.className = 'text-ui-sm text-danger';
          why.textContent = c.split_block_label || '';
          wrap.appendChild(why);
        }
        var lab = document.createElement('label');
        lab.className = 'text-ui-sm text-ink-muted block';
        lab.innerHTML = '이 날짜 <select data-day-action="' + (c.date || '') + '">' +
          '<option value="split">끼워넣기</option>' +
          '<option value="skip">건너뛰기</option>' +
          '<option value="edit">기존 계획 수정</option></select>';
        wrap.appendChild(lab);
      });
      ul.appendChild(wrap);
    });
    if (many) {
      var more = document.createElement('li');
      more.className = 'text-ui-sm text-info';
      more.textContent = '겹침 ' + conflicts.length + '건 — 더 보기';
      more.setAttribute('role', 'button');
      more.addEventListener('click', function () {
        Array.prototype.forEach.call(ul.querySelectorAll('li[hidden]'), function (el) { el.hidden = false; });
        more.hidden = true;
      });
      ul.appendChild(more);
    }
  }

  function hideOverlay(clearPending) {
    var m = mask();
    if (m) { m.hidden = true; m.classList.add('hidden'); }
    setBusy(false);
    clearModalError();
    if (clearPending) pendingForm = null;
    var back = lastFocus;
    lastFocus = null;
    if (back && back.focus) {
      try { back.focus(); } catch (e) {}
    }
  }

  function flash(msg) {
    msg = msg || '저장하지 못했습니다';
    if (window.showFlashError) window.showFlashError(msg);
    else if (typeof alert === 'function') alert(msg);
  }

  window.closeOverlapResolve = function (clearPending) {
    hideOverlay(clearPending !== false);
  };

  function travelInput() { return document.getElementById('overlap-travel-min'); }

  function currentTravel() {
    var el = travelInput();
    var n = el ? parseInt(el.value, 10) : 0;
    if (isNaN(n) || n < 0) n = 0;
    if (n > 180) n = 180;
    return n;
  }

  function renderPreview(conflicts) {
    var box = document.getElementById('overlap-split-preview');
    if (!box) return;
    box.innerHTML = '';
    (conflicts || []).forEach(function (c) {
      (c.preview || []).forEach(function (line) {
        var p = document.createElement('p');
        p.textContent = line;
        box.appendChild(p);
      });
    });
    var splitBtn = document.querySelector('[data-overlap-action="split"]');
    if (splitBtn) {
      var any = (conflicts || []).some(function (c) { return c.can_split; });
      splitBtn.disabled = !any;
      splitBtn.removeAttribute('title');
    }
  }

  window.showOverlapResolve = function (form, payload) {
    pendingForm = form;
    pendingConflicts = flattenConflicts(payload);
    var msg = document.getElementById('overlap-resolve-msg');
    if (msg) msg.textContent = (payload && payload.message) || '같은 담당자의 시간이 겹칩니다';
    clearModalError();
    setBusy(false);
    var tr = travelInput();
    if (tr && payload && payload.travel_min != null) tr.value = String(payload.travel_min);
    else if (tr && !tr.value) tr.value = '0';
    renderConflicts(pendingConflicts);
    renderPreview(pendingConflicts);
    var m = mask();
    if (m) {
      document.body.appendChild(m);
      m.hidden = false;
      m.classList.remove('hidden');
      lastFocus = document.activeElement;
      var first = m.querySelector('[data-overlap-action]');
      if (first && first.focus) first.focus();
    }
  };

  function postAction(action) {
    if (busy) return;
    if (action === 'cancel') { hideOverlay(false); return; }
    if (!pendingForm) { hideOverlay(true); flash('저장할 폼을 찾지 못했습니다'); return; }
    var fd = new FormData(pendingForm);
    fd.set('overlap_action', action);
    fd.set('split_travel_min', String(currentTravel()));
    fd.delete('conflict_task_id');
    pendingConflicts.forEach(function (c) {
      if (c && c.task_id) fd.append('conflict_task_id', c.task_id);
    });
    var skipSel = document.querySelectorAll('[data-day-action]');
    skipSel.forEach(function (sel) {
      if (sel.value === 'skip' && sel.getAttribute('data-day-action')) {
        fd.append('overlap_skip_date', sel.getAttribute('data-day-action'));
      }
    });
    if (action === 'skip') {
      pendingConflicts.forEach(function (c) {
        if (c && c.date) fd.append('overlap_skip_date', c.date);
      });
    }
    if (pendingConflicts[0] && pendingConflicts[0].task_id) {
      fd.set('overlap_edit_task_id', pendingConflicts[0].task_id);
    }
    var formBtns = pendingForm.querySelectorAll('button[type="submit"], input[type="submit"]');
    formBtns.forEach(function (b) { b.disabled = true; });
    setBusy(true);
    clearModalError();
    fetch(pendingForm.action, {
      method: 'POST',
      body: fd,
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' }
    }).then(function (r) {
      if (r.redirected && r.url) { window.location.assign(r.url); return true; }
      if (r.status === 409) {
        return r.json().then(function (j) { window.showOverlapResolve(pendingForm, j); return true; })
          .catch(function () { showModalError('시간이 겹칩니다'); return false; });
      }
      if (r.ok) {
        return r.json().then(function (j) {
          if (j && j.cancelled) { hideOverlay(false); flash('취소했습니다'); return true; }
          if (j && j.redirect) { window.location.assign(j.redirect); return true; }
          var dest = fd.get('redirect') || fd.get('back') || window.location.href;
          window.location.assign(dest);
          return true;
        }).catch(function () {
          var dest = fd.get('redirect') || fd.get('back') || window.location.href;
          window.location.assign(dest);
          return true;
        });
      }
      return r.json().then(function (j) {
        showModalError((j && j.message) ? j.message : '저장하지 못했습니다');
        return false;
      }).catch(function () {
        showModalError('저장하지 못했습니다');
        return false;
      });
    }).catch(function () {
      showModalError('저장하지 못했습니다. 네트워크를 확인해 주세요');
    }).finally(function () {
      formBtns.forEach(function (b) { b.disabled = false; });
      if (isOpen()) setBusy(false);
    });
  }

  document.addEventListener('click', function (e) {
    var chip = e.target && e.target.closest ? e.target.closest('[data-travel]') : null;
    if (chip && isOpen()) {
      e.preventDefault();
      var tr = travelInput();
      if (tr) tr.value = chip.getAttribute('data-travel') || '0';
      return;
    }
    var bulk = e.target && e.target.closest ? e.target.closest('[data-overlap-bulk]') : null;
    if (bulk && isOpen()) {
      e.preventDefault();
      var kind = bulk.getAttribute('data-overlap-bulk');
      document.querySelectorAll('[data-day-action]').forEach(function (sel) { sel.value = kind; });
      postAction(kind);
      return;
    }
    var b = e.target && e.target.closest ? e.target.closest('[data-overlap-action]') : null;
    if (!b) return;
    e.preventDefault();
    postAction(b.getAttribute('data-overlap-action'));
  });

  document.addEventListener('keydown', function (e) {
    if (!isOpen()) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      hideOverlay(false);
      return;
    }
    if (e.key !== 'Tab') return;
    var els = focusables();
    if (!els.length) return;
    var first = els[0];
    var last = els[els.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }, true);

  document.addEventListener('submit', function (e) {
    var form = e.target;
    if (!form || !form.classList || !form.classList.contains('js-keep-modal')) return;
    if (form.dataset.keepSkip === '1') return;
    if (typeof form.checkValidity === 'function' && !form.checkValidity()) {
      e.preventDefault();
      e.stopImmediatePropagation();
      if (form.reportValidity) form.reportValidity();
      var bad = form.querySelector(':invalid');
      if (bad && typeof bad.scrollIntoView === 'function') bad.scrollIntoView({ block: 'center' });
      flash('필수 항목을 확인해 주세요');
    }
  }, true);

  window.handleKeepModalResponse = function (r, form, fd) {
    if (!r) { flash('저장하지 못했습니다'); return Promise.resolve(false); }
    if (r.redirected && r.url) { window.location.assign(r.url); return Promise.resolve(true); }
    if (r.status === 409) {
      return r.json().then(function (j) {
        if (j && (j.conflicts || j.error === 'overlap')) {
          window.showOverlapResolve(form, j);
          return true;
        }
        flash((j && j.message) ? j.message : '시간이 겹칩니다');
        return false;
      }).catch(function () {
        flash('시간이 겹칩니다');
        return false;
      });
    }
    if (r.ok) {
      return r.json().then(function (j) {
        if (j && j.cancelled) { flash('취소했습니다'); return true; }
        if (j && j.redirect) { window.location.assign(j.redirect); return true; }
        var dest = (fd && fd.get) ? (fd.get('redirect') || fd.get('back')) : null;
        window.location.assign(dest || window.location.href);
        return true;
      }).catch(function () {
        var dest = (fd && fd.get) ? (fd.get('redirect') || fd.get('back')) : null;
        window.location.assign(dest || window.location.href);
        return true;
      });
    }
    return r.json().then(function (j) {
      flash((j && j.message) ? j.message : '저장하지 못했습니다');
      return false;
    }).catch(function () {
      flash('저장하지 못했습니다');
      return false;
    });
  };

  function focusDueDateOrder() {
    try {
      var q = new URLSearchParams(location.search).get('err');
      if (q !== 'date_order') return;
      var el = document.querySelector('[data-wb-period-dates] [name=due_date]') || document.querySelector('[name=due_date]');
      if (!el) return;
      var hint = el.parentElement && el.parentElement.querySelector('.due-date-err');
      if (hint) hint.hidden = false;
      el.focus();
      if (el.scrollIntoView) el.scrollIntoView({ block: 'center' });
    } catch (e) {}
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', focusDueDateOrder);
  else focusDueDateOrder();
})();
