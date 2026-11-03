(function () {
  function boot() {
  function el(id) { return document.getElementById(id); }
  var board = document.querySelector('[data-kanban-board]');
  if (!board) return;
  var action = board.getAttribute('data-move') || '/workboard/board/kanban-move';

  function post(fields) {
    var form = document.createElement('form');
    form.method = 'post';
    form.action = action;
    Object.keys(fields).forEach(function (k) {
      var i = document.createElement('input');
      i.type = 'hidden';
      i.name = k;
      i.value = fields[k];
      form.appendChild(i);
    });
    var redir = document.createElement('input');
    redir.type = 'hidden';
    redir.name = 'redirect';
    redir.value = location.pathname + location.search;
    form.appendChild(redir);
    document.body.appendChild(form);
    form.submit();
  }

  function forbidden(from, to) {
    var today = from === 'today_plan' || from === 'today_progress' || from === 'today_partial' || from === 'today_done';
    return today && (to === 'delay' || to === 'off');
  }

  board.addEventListener('dragstart', function (e) {
    var card = e.target.closest('[data-kanban-card]');
    if (!card) return;
    var col = card.closest('[data-kanban-col]');
    e.dataTransfer.setData('text/plain', JSON.stringify({
      key: card.getAttribute('data-item-key'),
      from: col ? col.getAttribute('data-kanban-col') : '',
      as: card.getAttribute('data-as'),
      mnt: card.getAttribute('data-mnt'),
      remain: card.getAttribute('data-remain')
    }));
  });

  board.querySelectorAll('[data-kanban-drop]').forEach(function (drop) {
    drop.addEventListener('dragover', function (e) {
      e.preventDefault();
      var to = drop.getAttribute('data-kanban-drop');
      try {
        var p = JSON.parse(e.dataTransfer.getData('text/plain') || '{}');
        if (forbidden(p.from, to)) e.dataTransfer.dropEffect = 'none';
      } catch (err) {}
    });
    drop.addEventListener('drop', function (e) {
      e.preventDefault();
      var to = drop.getAttribute('data-kanban-drop');
      var p = {};
      try { p = JSON.parse(e.dataTransfer.getData('text/plain') || '{}'); } catch (err) { return; }
      if (!p.key || p.from === to) return;
      if (forbidden(p.from, to)) { alert('오늘의 업무에서 지연 업무·진행중(일정 외)으로 옮길 수 없습니다'); return; }
      if ((p.from === 'delay' || p.from === 'off') && to === 'today_plan') {
        var btn = board.querySelector('[data-item-key="' + p.key + '"]');
        if (btn) btn.setAttribute('data-carry-open', '1');
        if (btn) btn.click();
        return;
      }
      if (to === 'today_done' && (p.as === '1' || p.mnt === '1')) {
        alert('조치 화면에서 완료하세요');
        return;
      }
      var fields = { item_key: p.key, from: p.from, to: to };
      if (to === 'today_done' || to === 'today_partial') {
        var note = prompt('완료 내용');
        if (!note) return;
        fields.complete_note = note;
      }
      if ((p.from === 'today_done' || p.from === 'today_partial') && to === 'today_progress') {
        if (!confirm('진행중으로 되돌릴까요? 후속 작업은 유지됩니다')) return;
        fields.confirm_reopen = '1';
      }
      if (to === 'today_partial' && Number(p.remain || 0) < 1) {
        alert('모든 항목이 끝났습니다. 완료로 옮겨 주세요');
        return;
      }
      post(fields);
    });
  });

  board.addEventListener('click', function (e) {
    var menu = e.target.closest('[data-kanban-menu]');
    if (!menu) return;
    var card = menu.closest('[data-kanban-card]');
    var col = card && card.closest('[data-kanban-col]');
    var from = col ? col.getAttribute('data-kanban-col') : '';
    var to = prompt('이동할 열: delay, off, today_plan, today_progress, today_partial, today_done');
    if (!to) return;
    post({ item_key: card.getAttribute('data-item-key'), from: from, to: to });
  });
  board.setAttribute('data-kanban-bound', '1');
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
