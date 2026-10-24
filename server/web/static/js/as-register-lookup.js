window.bindASRegisterLookup = function (opts) {
  opts = opts || {};
  var popup = !!opts.missingDialog;
  var q = document.getElementById('as-register-q');
  var box = document.getElementById('as-lookup-status');
  if (!q || !box || q.dataset.lookupBound === '1') return;
  q.dataset.lookupBound = '1';
  var timer = null, last = null, dismissed = '', awaitingReturn = false;
  var dlg = null;

  function txt(s) { return document.createTextNode(s || ''); }
  function el(tag, cls, children) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    (children || []).forEach(function (c) { n.appendChild(c); });
    return n;
  }
  function hide() { box.hidden = true; box.replaceChildren(); last = null; }
  function show(node) { box.hidden = false; box.replaceChildren(node); }

  function ellipsis(s) {
    var chars = Array.from(s || '');
    if (chars.length <= 24) return chars.join('');
    return chars.slice(0, 24).join('') + '…';
  }

  function closeMissingDialog() {
    window.__asLookupMissingOpen = false;
    if (!dlg) return;
    dlg.remove();
    dlg = null;
  }

  function showMissingDialog(data, query) {
    if (dismissed === query) return;
    closeMissingDialog();
    window.__asLookupMissingOpen = true;
    dlg = el('div', 'modal-backdrop z-[60] flex items-center justify-center p-4', []);
    dlg.setAttribute('role', 'alertdialog');
    dlg.setAttribute('aria-modal', 'true');
    dlg.setAttribute('aria-labelledby', 'as-miss-title');
    dlg.setAttribute('aria-describedby', 'as-miss-body');
    var panel = el('div', 'modal-panel w-full max-w-md shadow-pop', []);
    var title = el('h2', 'text-ui font-bold text-ink', [txt('접수된 내용이 없습니다')]);
    title.id = 'as-miss-title';
    var body = el('p', 'mt-2 text-ui text-ink', []);
    body.id = 'as-miss-body';
    body.appendChild(txt("'"));
    body.appendChild(txt(ellipsis(query)));
    body.appendChild(txt("'에 해당하는 AS 접수가 없습니다. 새로 접수하시겠습니까?"));
    panel.appendChild(title);
    panel.appendChild(body);
    var btns = el('div', 'mt-4 flex justify-end gap-2', []);
    var addBtn = null;
    if (data.can_create) {
      addBtn = el('a', 'inline-flex h-10 items-center rounded-[10px] bg-success px-4 font-semibold text-primary-fg', [txt('접수 추가')]);
      addBtn.href = data.new_href || '/as/new';
      addBtn.target = '_blank';
      addBtn.rel = 'noopener';
      addBtn.setAttribute('aria-label', 'AS 접수 추가');
      addBtn.addEventListener('click', function () {
        awaitingReturn = true;
        dismissed = '';
        closeMissingDialog();
      });
      btns.appendChild(addBtn);
    } else {
      panel.appendChild(el('p', 'mt-2 text-ui text-ink-muted', [txt('AS 접수 권한이 없습니다. 담당자에게 접수를 요청하세요.')]));
    }
    var closeBtn = el('button', 'btn btn-secondary', [txt('닫기')]);
    closeBtn.type = 'button';
    closeBtn.setAttribute('aria-label', '안내 닫기');
    closeBtn.addEventListener('click', function () {
      dismissed = query;
      closeMissingDialog();
      q.focus();
      if (q.select) q.select();
    });
    btns.appendChild(closeBtn);
    panel.appendChild(btns);
    dlg.appendChild(panel);
    dlg.addEventListener('click', function (e) {
      if (e.target === dlg) closeBtn.click();
    });
    document.body.appendChild(dlg);
    var focusEl = addBtn || closeBtn;
    if (focusEl && focusEl.focus) focusEl.focus();
  }

  function renderInline(data) {
    var wrap = el('div', 'space-y-2', []);
    if (data.kind === 'short' || (data.kind === 'missing' && !popup)) {
      wrap.appendChild(el('p', '', [txt(data.hint || 'AS가 존재하지 않습니다.')]));
    }
    if (data.kind === 'placed' || data.kind === 'closed') {
      (data.items || []).forEach(function (it) {
        var row = el('div', '', []);
        if (it.kind === 'placed') {
          row.appendChild(el('p', '', [txt('이미 일일업무로 등록된 AS입니다')]));
          var line = (it.as_number || '') + ' · ' + (it.assignee || '') + (it.when ? ' · ' + it.when : '');
          row.appendChild(el('p', 'text-ink-muted', [txt(line)]));
          if (it.task_href) {
            var a = el('a', 'text-primary underline', [txt('해당 업무 보기')]);
            a.href = it.task_href;
            row.appendChild(a);
          }
        } else if (it.kind === 'closed') {
          row.appendChild(el('p', '', [txt('완료된 AS입니다(' + (it.status || '') + ')')]));
          var asA = el('a', 'text-primary underline', [txt((it.as_number || '') + ' 상세')]);
          asA.href = it.as_href || ('/as/' + (it.as_id || ''));
          row.appendChild(asA);
        }
        wrap.appendChild(row);
      });
    }
    if (data.kind === 'missing' && !popup) {
      var btns = el('div', 'flex gap-2', []);
      if (data.can_create) {
        var add = el('button', 'btn btn-primary', [txt('추가')]);
        add.type = 'button';
        add.setAttribute('aria-label', 'AS 접수로 이동');
        add.addEventListener('click', function () {
          var form = q.closest('form');
          var dirty = false;
          if (form) {
            Array.prototype.forEach.call(form.querySelectorAll('input,select,textarea'), function (inp) {
              if (inp === q || inp.type === 'hidden' || inp.disabled) return;
              if ((inp.value || '') !== '' && inp.value !== q.value) dirty = true;
            });
          }
          if (dirty && !confirm('입력 중인 내용이 사라집니다')) return;
          window.location.href = data.new_href || '/as/new';
        });
        btns.appendChild(add);
      }
      var ok = el('button', 'btn btn-secondary', [txt('확인')]);
      ok.type = 'button';
      ok.setAttribute('aria-label', '안내 닫고 검색 칸으로');
      ok.addEventListener('click', function () {
        q.value = '';
        q.dispatchEvent(new Event('input', { bubbles: true }));
        hide();
        q.focus();
      });
      btns.appendChild(ok);
      wrap.appendChild(btns);
    }
    if (!wrap.childNodes.length) {
      hide();
      return;
    }
    show(wrap);
  }

  function lookup() {
    var v = (q.value || '').trim();
    if (!v) { hide(); closeMissingDialog(); return; }
    var n = Array.from(v).length;
    if (n < 2) {
      closeMissingDialog();
      show(el('p', '', [txt('두 글자 이상 입력하세요')]));
      return;
    }
    fetch('/workboard/as-lookup?q=' + encodeURIComponent(v), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        last = data;
        if (!data || data.kind === 'ok') {
          hide();
          closeMissingDialog();
          return;
        }
        if (popup && data.kind === 'missing') {
          hide();
          showMissingDialog(data, v);
          return;
        }
        if (data.kind === 'placed' || data.kind === 'closed' || data.kind === 'short' || data.kind === 'missing') {
          closeMissingDialog();
          renderInline(data);
        }
      });
  }

  q.addEventListener('input', function () {
    clearTimeout(timer);
    if ((q.value || '').trim() !== dismissed) dismissed = '';
    timer = setTimeout(lookup, popup ? 700 : 250);
  });
  if (popup) {
    q.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter') return;
      e.preventDefault();
      clearTimeout(timer);
      lookup();
    });
  }
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape' || !dlg) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    var v = (q.value || '').trim();
    dismissed = v;
    closeMissingDialog();
    q.focus();
    if (q.select) q.select();
  }, true);
  window.addEventListener('focus', function () {
    if (!awaitingReturn) return;
    awaitingReturn = false;
    lookup();
  });
};
window.bindASRegisterLookup();
