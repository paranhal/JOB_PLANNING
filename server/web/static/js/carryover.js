(function () {
  function el(id) { return document.getElementById(id); }
  function hide(node, on) {
    if (!node) return;
    node.hidden = !!on;
    if (on) node.classList.add('hidden');
    else node.classList.remove('hidden');
  }
  function trap(mask) {
    var nodes = mask.querySelectorAll('button, [href], input, select, textarea');
    var first = nodes[0], last = nodes[nodes.length - 1];
    mask.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') closeCarry();
      if (e.key !== 'Tab' || nodes.length === 0) return;
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    });
  }

  function closeCarry() {
    hide(el('carry-modal'), true);
  }
  function closeReason() {
    hide(el('delay-reason-modal'), true);
  }

  function openCarryFrom(btn) {
    var m = el('carry-modal');
    if (!m || !btn) return;
    document.body.appendChild(m);
    var id = btn.getAttribute('data-carry-id');
    var form = el('carry-form');
    form.action = '/workboard/tasks/' + encodeURIComponent(id) + '/carry';
    el('carry-task-title').textContent = btn.getAttribute('data-carry-title') || '';
    var work = btn.getAttribute('data-carry-work') || '';
    var due = btn.getAttribute('data-carry-due') || '';
    var st = btn.getAttribute('data-carry-start') || '';
    var en = btn.getAttribute('data-carry-end') || '';
    var sched = (work || '시작 없음') + (due && due !== work ? ' ~ ' + due : '');
    if (st) sched += ' · ' + st + (en ? '–' + en : '');
    el('carry-task-sched').textContent = sched;
    var delayed = btn.getAttribute('data-carry-delayed') === 'true' || btn.getAttribute('data-carry-delayed') === '1';
    var days = btn.getAttribute('data-carry-days') || '0';
    var dEl = el('carry-task-delay');
    if (delayed) { dEl.textContent = '지연 ' + days + '일'; dEl.classList.remove('hidden'); }
    else { dEl.classList.add('hidden'); }
    var span = btn.getAttribute('data-carry-span') === 'true' || btn.getAttribute('data-carry-span') === '1';
    hide(el('carry-single-wrap'), span);
    hide(el('carry-span-wrap'), !span);
    var hol = el('carry-holiday');
    if (hol && hol.getAttribute('data-holiday') === '1') hol.classList.remove('hidden');
    el('carry-reason').required = delayed;
    el('carry-reason-err').classList.add('hidden');
    el('carry-form-err').classList.add('hidden');
    el('carry-redirect').value = location.pathname + location.search;
    el('carry-back').value = location.pathname + location.search;
    hide(m, false);
    var def = btn.getAttribute('data-carry-default');
    if (def === 'today') {
      var min = el('carry-new-date') && el('carry-new-date').getAttribute('min');
      if (min) {
        if (el('carry-new-date')) el('carry-new-date').value = min;
        if (el('carry-new-due')) el('carry-new-due').value = min;
      }
    }
    var focus = span ? el('carry-new-due') : el('carry-new-date');
    if (focus) focus.focus();
  }

  function openReasonFrom(btn) {
    var m = el('delay-reason-modal');
    if (!m || !btn) return;
    document.body.appendChild(m);
    var id = btn.getAttribute('data-carry-id');
    el('delay-reason-form').action = '/workboard/tasks/' + encodeURIComponent(id) + '/delay-reason';
    el('delay-reason-redirect').value = location.pathname + location.search;
    el('delay-reason-err').classList.add('hidden');
    hide(m, false);
    el('delay-reason-text').focus();
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-carry-open]');
    if (btn) { e.preventDefault(); openCarryFrom(btn); return; }
    var rb = e.target.closest('[data-delay-reason-open]');
    if (rb) { e.preventDefault(); openReasonFrom(rb); return; }
    if (e.target.id === 'carry-cancel') { e.preventDefault(); closeCarry(); }
    if (e.target.id === 'delay-reason-cancel') { e.preventDefault(); closeReason(); }
    if (e.target.id === 'carry-modal') closeCarry();
    if (e.target.id === 'delay-reason-modal') closeReason();
    if (e.target.id === 'carry-hol-keep') {
      e.preventDefault();
      el('carry-holiday').classList.add('hidden');
    }
    if (e.target.id === 'carry-hol-change') {
      e.preventDefault();
      var next = el('carry-holiday').getAttribute('data-next');
      if (next) {
        var a = el('carry-new-date'); if (a) a.value = next;
        var b = el('carry-new-due'); if (b) b.value = next;
      }
      el('carry-holiday').classList.add('hidden');
    }
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { closeCarry(); closeReason(); }
  });

  var form = el('carry-form');
  if (form) {
    trap(el('carry-modal'));
    form.addEventListener('submit', function (e) {
      var delayed = el('carry-reason').required;
      if (delayed && !String(el('carry-reason').value || '').trim()) {
        e.preventDefault();
        el('carry-reason-err').classList.remove('hidden');
        el('carry-reason').focus();
        if (el('carry-reason').scrollIntoView) el('carry-reason').scrollIntoView({ block: 'nearest' });
      }
    });
  }
  if (el('delay-reason-form')) {
    trap(el('delay-reason-modal'));
    el('delay-reason-form').addEventListener('submit', function (e) {
      if (!String(el('delay-reason-text').value || '').trim()) {
        e.preventDefault();
        el('delay-reason-err').classList.remove('hidden');
        el('delay-reason-text').focus();
      }
    });
  }
})();
