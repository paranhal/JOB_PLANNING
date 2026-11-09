(function (root) {
  var REASON = '미배치 업무 삭제';

  function bindUnplacedDelete(box, opts) {
    opts = opts || {};
    var sel = (opts.workListSelect || (typeof workListSelect === 'function' ? workListSelect : null));
    if (!box || !sel) return null;
    var api = sel(opts.csrf || '');
    api.$root = box;
    var mask = opts.mask || (typeof document !== 'undefined' ? document.getElementById('unplaced-del-mask') : null);
    var titleEl = opts.titleEl || (mask && mask.querySelector('#unplaced-del-title'));
    var metaEl = opts.metaEl || (mask && mask.querySelector('#unplaced-del-meta'));
    var previewEl = opts.previewEl || (mask && mask.querySelector('#unplaced-del-preview'));
    var errEl = opts.errEl || (mask && mask.querySelector('#unplaced-del-err'));
    var cancelBtn = opts.cancelBtn || (mask && mask.querySelector('#unplaced-del-cancel'));
    var okBtn = opts.okBtn || (mask && mask.querySelector('#unplaced-del-ok'));
    var bulkBtn = opts.bulkBtn || box.querySelector('[data-unplaced-bulk]');
    var statusEl = opts.statusEl || box.querySelector('#unplaced-status');
    var lastFocus = null;
    var pendingIds = [];
    var mode = '';

    function setStatus(msg) {
      if (statusEl) statusEl.textContent = msg || '';
    }
    function showErr(msg) {
      if (!errEl) return;
      if (msg) {
        errEl.hidden = false;
        errEl.textContent = msg;
      } else {
        errEl.hidden = true;
        errEl.textContent = '';
      }
    }
    function syncBulk() {
      api.sync();
      if (bulkBtn) {
        bulkBtn.disabled = api.n === 0;
        bulkBtn.textContent = '선택한 ' + api.n + '건 삭제';
      }
    }
    function closeDlg() {
      if (mask) {
        mask.hidden = true;
        mask.classList.remove('on');
      }
      pendingIds = [];
      mode = '';
      showErr('');
      var btn = lastFocus;
      lastFocus = null;
      if (btn && btn.focus) btn.focus();
    }
    function openDlg() {
      if (!mask) return;
      mask.hidden = false;
      mask.classList.add('on');
      showErr('');
      if (cancelBtn && cancelBtn.focus) cancelBtn.focus();
    }
    function removeCards(ids) {
      var set = {};
      (ids || []).forEach(function (id) { set[String(id)] = true; });
      box.querySelectorAll('.board-pc').forEach(function (card) {
        var k = card.getAttribute('data-delete-key') || '';
        if (set[k]) card.remove();
      });
      if (typeof window !== 'undefined' && window.refreshUnplacedPanelCounts) {
        window.refreshUnplacedPanelCounts();
      }
      if (!box.querySelector('.board-pc')) {
        var list = box.querySelector('.board-plist');
        if (list && !list.querySelector('.board-pempty')) {
          var empty = (typeof document !== 'undefined') ? document.createElement('div') : null;
          if (empty) {
            empty.className = 'board-pempty';
            empty.textContent = '미배치 업무가 없습니다.';
            list.appendChild(empty);
          }
        }
      }
      syncBulk();
    }
    function postDelete(ids) {
      api.applyCsrf(api.readCsrf());
      var body = (ids || []).map(function (id) { return 'ids=' + encodeURIComponent(id); }).join('&');
      body += '&csrf=' + encodeURIComponent(api.csrf);
      body += '&reason=' + encodeURIComponent(REASON);
      if (typeof fetch !== 'function') {
        return Promise.resolve({ ok: false, error: '삭제하지 못했습니다' });
      }
      return fetch('/workboard/tasks/bulk-delete', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
          'X-Requested-With': 'fetch',
          'X-CSRF-Token': api.csrf
        },
        body: body
      }).then(function (r) {
        return r.json().then(function (d) {
          return { ok: r.ok, status: r.status, d: d || {} };
        }).catch(function () {
          return { ok: false, status: r.status, d: { error: '삭제 결과를 받지 못했습니다' } };
        });
      }).then(function (res) {
        var d = res.d || {};
        if (!res.ok || d.error) {
          return { ok: false, error: d.error || '삭제하지 못했습니다', status: res.status };
        }
        return { ok: true, deleted: d.deleted || [], skipped: d.skipped || [] };
      });
    }
    function fillPreview(del, skip) {
      var parts = [];
      parts.push('삭제 가능 ' + (del || []).length + '건, 불가 ' + (skip || []).length + '건');
      (skip || []).forEach(function (s) {
        parts.push((s.title || s.id || '') + ' — ' + (s.reason || ''));
      });
      if (previewEl) previewEl.textContent = parts.join('\n');
      if (okBtn) okBtn.disabled = !(del && del.length);
    }
    function afterPreview() {
      fillPreview(api.deletable, api.skipped);
      pendingIds = (api.deletable || []).map(function (r) { return r.id; });
      openDlg();
    }

    box.addEventListener('change', function () { syncBulk(); });
    if (bulkBtn) {
      bulkBtn.addEventListener('click', function () {
        lastFocus = bulkBtn;
        var ids = api.selectedIds();
        if (!ids.length) return;
        if (titleEl) titleEl.textContent = '선택한 업무 삭제';
        if (metaEl) metaEl.textContent = ids.length + '건을 확인합니다.';
        if (previewEl) previewEl.textContent = '';
        mode = 'bulk';
        api.ids = ids;
        api.lastError = '';
        api.applyCsrf(api.readCsrf());
        Promise.resolve(api.fetchPreview(ids, false)).then(function () {
          if (api.lastError) return;
          afterPreview();
        });
      });
    }
    box.addEventListener('click', function (e) {
      var del = e.target.closest('[data-unplaced-del]');
      if (del && !del.disabled) {
        e.preventDefault();
        lastFocus = del;
        var card = del.closest('.board-pc');
        pendingIds = [del.getAttribute('data-unplaced-del')];
        mode = 'single';
        if (titleEl) titleEl.textContent = '업무 삭제';
        if (metaEl) {
          metaEl.textContent = [
            (card && card.getAttribute('data-title')) || '',
            (card && card.getAttribute('data-assignee')) || '',
            (card && card.querySelector('.board-pc-c') && card.querySelector('.board-pc-c').textContent) || ''
          ].filter(Boolean).join(' · ');
        }
        if (previewEl) previewEl.textContent = '';
        if (okBtn) okBtn.disabled = false;
        openDlg();
        return;
      }
      var dup = e.target.closest('[data-unplaced-dup]');
      if (dup) {
        e.preventDefault();
        lastFocus = dup;
        var raw = dup.getAttribute('data-unplaced-dup') || '';
        var ids = raw.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
        if (!ids.length) return;
        mode = 'dup';
        if (titleEl) titleEl.textContent = '중복 업무 정리';
        if (metaEl) metaEl.textContent = '가장 오래된 1건을 남기고 ' + ids.length + '건을 삭제합니다.';
        if (previewEl) previewEl.textContent = '';
        api.ids = ids;
        api.lastError = '';
        api.applyCsrf(api.readCsrf());
        Promise.resolve(api.fetchPreview(ids, false)).then(function () {
          if (api.lastError) return;
          afterPreview();
        });
      }
    });
    if (cancelBtn) cancelBtn.addEventListener('click', closeDlg);
    if (mask) {
      mask.addEventListener('click', function (e) {
        if (e.target === mask) closeDlg();
      });
    }
    function onKey(e) {
      if (e.key !== 'Escape') return;
      if (!mask || mask.hidden) return;
      e.preventDefault();
      closeDlg();
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('keydown', onKey);
    }
    if (okBtn) {
      okBtn.addEventListener('click', function () {
        var ids = pendingIds.slice();
        if (!ids.length) return;
        okBtn.disabled = true;
        postDelete(ids).then(function (res) {
          okBtn.disabled = false;
          if (!res.ok) {
            showErr(res.error || '삭제하지 못했습니다');
            return;
          }
          var skipped = res.skipped || [];
          removeCards(res.deleted || []);
          if (skipped.length && !(res.deleted || []).length) {
            showErr(skipped[0].reason || '삭제할 수 없습니다');
            return;
          }
          var msg = (res.deleted || []).length + '건을 삭제했습니다';
          if (skipped.length) msg += ', ' + skipped.length + '건은 건너뜀';
          setStatus(msg);
          closeDlg();
        }).catch(function () {
          okBtn.disabled = false;
          showErr('삭제하지 못했습니다');
        });
      });
    }
    syncBulk();
    return {
      api: api,
      closeDlg: closeDlg,
      openDlg: openDlg,
      removeCards: removeCards,
      postDelete: postDelete,
      setStatus: setStatus,
      REASON: REASON
    };
  }

  root.bindUnplacedDelete = bindUnplacedDelete;
  root.UNPLACED_DELETE_REASON = REASON;
  if (typeof document !== 'undefined') {
    function boot() {
      var box = document.getElementById('unplanned');
      if (box) bindUnplacedDelete(box);
    }
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', boot);
    } else {
      boot();
    }
  }
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { bindUnplacedDelete: bindUnplacedDelete, REASON: REASON };
  }
})(typeof window !== 'undefined' ? window : (typeof global !== 'undefined' ? global : this));
