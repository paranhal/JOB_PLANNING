window.bindASRegisterLookup = function () {
  var q = document.getElementById('as-register-q');
  var box = document.getElementById('as-lookup-status');
  if (!q || !box || q.dataset.lookupBound === '1') return;
  q.dataset.lookupBound = '1';
  if (!q || !box) return;
  var timer = null, last = null;
  function txt(s) { return document.createTextNode(s || ''); }
  function el(tag, cls, children) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    (children || []).forEach(function (c) { n.appendChild(c); });
    return n;
  }
  function hide() { box.hidden = true; box.replaceChildren(); last = null; }
  function show(node) { box.hidden = false; box.replaceChildren(node); }
  function lookup() {
    var v = (q.value || '').trim();
    if (!v) { hide(); return; }
    var n = Array.from(v).length;
    if (n < 2) {
      show(el('p', '', [txt('두 글자 이상 입력하세요')]));
      return;
    }
    fetch('/workboard/as-lookup?q=' + encodeURIComponent(v), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(function (data) {
        last = data;
        if (!data || data.kind === 'ok') { hide(); return; }
        var wrap = el('div', 'space-y-2', []);
        if (data.kind === 'short' || data.kind === 'missing') {
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
        if (data.kind === 'missing') {
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
        show(wrap);
      });
  }
  q.addEventListener('input', function () {
    clearTimeout(timer);
    timer = setTimeout(lookup, 250);
  });
};
window.bindASRegisterLookup();
