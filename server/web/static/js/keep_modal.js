(function (root) {
  if (root.__wbKeepModalBound) return;

  function tryLockSubmit(form) {
    if (!form) return false;
    if (form.dataset.submitting === '1') return false;
    form.dataset.submitting = '1';
    var btns = form.querySelectorAll('button[type="submit"], input[type="submit"]');
    btns.forEach(function (b) { b.disabled = true; });
    return true;
  }

  function unlockSubmit(form) {
    if (!form) return;
    form.dataset.submitting = '';
    var btns = form.querySelectorAll('button[type="submit"], input[type="submit"]');
    btns.forEach(function (b) { b.disabled = false; });
  }

  root.__wbKeepModalBound = true;
  root.tryLockKeepModalSubmit = tryLockSubmit;
  root.unlockKeepModalSubmit = unlockSubmit;

  if (typeof document === 'undefined' || !document.addEventListener) {
    if (typeof module !== 'undefined') {
      module.exports = { tryLockSubmit: tryLockSubmit, unlockSubmit: unlockSubmit };
    }
    return;
  }

  document.addEventListener('submit', function (e) {
    var form = e.target;
    if (!form || !form.classList || !form.classList.contains('js-keep-modal')) return;
    if (form.dataset.keepSkip === '1') return;
    if (!tryLockSubmit(form)) {
      e.preventDefault();
      e.stopImmediatePropagation();
      return;
    }
    e.preventDefault();
    var fd = new FormData(form);
    if (typeof fetch !== 'function') {
      unlockSubmit(form);
      form.submit();
      return;
    }
    fetch(form.action, {
      method: 'POST',
      body: fd,
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' }
    }).then(function (r) {
      if (window.handleKeepModalResponse) return window.handleKeepModalResponse(r, form, fd);
      if (r.redirected && r.url) { window.location.assign(r.url); return true; }
      if (r.ok) {
        var dest = fd.get('redirect') || fd.get('back') || window.location.href;
        window.location.assign(dest);
        return true;
      }
      return r.json().then(function (j) {
        var msg = (j && j.message) ? j.message : '저장하지 못했습니다';
        if (window.showFlashError) window.showFlashError(msg);
        return false;
      }).catch(function () {
        if (window.showFlashError) window.showFlashError('저장하지 못했습니다');
        return false;
      });
    }).catch(function () {
      if (window.showFlashError) window.showFlashError('저장하지 못했습니다. 네트워크를 확인해 주세요');
      return false;
    }).then(function (kept) {
      if (kept) return;
      unlockSubmit(form);
    });
  });

  if (typeof module !== 'undefined') {
    module.exports = { tryLockSubmit: tryLockSubmit, unlockSubmit: unlockSubmit };
  }
})(typeof window !== 'undefined' ? window : globalThis);
