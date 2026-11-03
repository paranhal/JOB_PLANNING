(function () {
  function boot() {
  var root = document.getElementById('board-root');
  if (!root) return;
  if (root.getAttribute('data-board-sched-bound') === '1') return;
  var form = document.getElementById('board-move-form');
  if (!form) {
    root.setAttribute('data-board-sched-bound', '0');
    return;
  }
  var leaveNames = [];
  try {
    var raw = root.getAttribute('data-leave-names') || '[]';
    leaveNames = JSON.parse(raw);
    if (!Array.isArray(leaveNames)) leaveNames = [];
  } catch (e) {
    leaveNames = [];
  }
  var picked = null;
  var dragBar = null;
  var resizing = null;
  var pendingMove = null;
  var moving = null;
  var ignoreClickUntil = 0;
  var PX_HOUR = 64;
  var DAY_START = 7 * 60;
  var DAY_END = 20 * 60;

  function hhmm(min) {
    min = Math.max(DAY_START, Math.min(DAY_END, min));
    var h = Math.floor(min / 60);
    var m = min % 60;
    return String(h).padStart(2, '0') + ':' + String(m).padStart(2, '0');
  }
  function parseMin(t) {
    if (!t) return 9 * 60;
    var p = String(t).split(':');
    return (parseInt(p[0], 10) || 0) * 60 + (parseInt(p[1], 10) || 0);
  }
  function addMin(t, add) {
    return hhmm(parseMin(t) + (add || 0));
  }
  function snapY(zone, clientY) {
    var r = zone.getBoundingClientRect();
    var m = Math.round(((clientY - r.top) / PX_HOUR * 60 + DAY_START) / 15) * 15;
    return Math.max(DAY_START, Math.min(DAY_END, m));
  }
  function clampEnd(start, end) {
    var sm = parseMin(start);
    var em = parseMin(end);
    if (em > DAY_END) em = DAY_END;
    if (em < sm + 15) em = sm + 15;
    if (em > DAY_END) em = DAY_END;
    return hhmm(em);
  }
  function durationOf(start, end) {
    var n = parseMin(end) - parseMin(start);
    if (n < 15) n = 15;
    return n;
  }
  function onLeave(name) {
    return !!name && leaveNames.indexOf(name) >= 0;
  }
  function setPicked(el) {
    if (picked) picked.classList.remove('board-bar-picked');
    picked = el;
    if (picked) picked.classList.add('board-bar-picked');
  }
  function fillForm(bar, date, start, end, assignee, fromCol, dur) {
    document.getElementById('board-m-kind').value = bar.getAttribute('data-kind') || 'task';
    document.getElementById('board-m-ref').value = bar.getAttribute('data-ref') || '';
    document.getElementById('board-m-title').value = bar.getAttribute('data-title') || '';
    document.getElementById('board-m-assignee').value = assignee != null ? assignee : (bar.getAttribute('data-assignee') || '');
    document.getElementById('board-m-from-col').value = fromCol ? '1' : '';
    document.getElementById('board-m-dur').value = String(dur);
    document.getElementById('board-m-date').value = date;
    document.getElementById('board-m-start').value = start || '';
    document.getElementById('board-m-end').value = end || '';
  }
  function submitMove(bar, date, start, end, assignee, fromCol) {
    if (!bar || bar.getAttribute('data-can-move') !== '1') return;
    var dur = parseInt(bar.getAttribute('data-duration'), 10) || 30;
    if (dur < 15) dur = 15;
    if (start && !end) end = addMin(start, dur);
    if (start && end) {
      end = clampEnd(start, end);
      dur = durationOf(start, end);
    }
    if (assignee && onLeave(assignee)) {
      if (!confirm(assignee + '은 이 날 연차입니다. 그래도 배정할까요?')) return;
    }
    fillForm(bar, date, start, end, assignee, fromCol, dur);
    form.submit();
  }
  function submitResize(bar, start, end) {
    if (!bar || bar.getAttribute('data-can-resize') !== '1') return;
    start = start || bar.getAttribute('data-start') || '09:00';
    end = clampEnd(start, end);
    var dur = durationOf(start, end);
    fillForm(bar, bar.getAttribute('data-date'), start, end, bar.getAttribute('data-assignee'), false, dur);
    ignoreClickUntil = Date.now() + 400;
    form.submit();
  }
  function barFrom(el) {
    if (!el || !el.closest) return null;
    return el.closest('[data-board-bar], .board-day-ev, .board-week-ev');
  }
  function zoneFrom(el) {
    return el && el.closest ? el.closest('[data-drop]') : null;
  }
  function dayzoneAt(x, y, ignore) {
    var prev = '';
    if (ignore) {
      prev = ignore.style.pointerEvents;
      ignore.style.pointerEvents = 'none';
    }
    var hit = document.elementFromPoint(x, y);
    if (ignore) ignore.style.pointerEvents = prev;
    return hit && hit.closest ? hit.closest('[data-dayzone]') : null;
  }

  root.addEventListener('click', function (e) {
    if (Date.now() < ignoreClickUntil) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    var un = e.target.closest('[data-unlock-open],[data-unlock-close]');
    if (un) {
      var box = document.getElementById('board-unlock');
      if (!box) return;
      if (un.hasAttribute('data-unlock-open')) { box.hidden = false; box.classList.add('on'); }
      else { box.hidden = true; box.classList.remove('on'); }
      return;
    }
    if (e.target.closest('[data-bar-act],[data-bar-open],[data-resize]')) return;
    var bar = barFrom(e.target);
    if (bar) {
      if (bar.getAttribute('data-done') === '1') {
        var open = bar.getAttribute('data-href');
        if (open) window.location.href = open;
        return;
      }
      if (bar.getAttribute('data-support') === '1' || bar.getAttribute('data-can-move') !== '1') return;
      if (picked === bar) { setPicked(null); return; }
      setPicked(bar);
      return;
    }
    var z = zoneFrom(e.target);
    if (z && picked) {
      var date = z.getAttribute('data-drop');
      var start = picked.getAttribute('data-start') || '09:00';
      var end = picked.getAttribute('data-end') || '';
      var colAs = z.getAttribute('data-assignee');
      if (z.getAttribute('data-dayzone')) {
        start = hhmm(snapY(z, e.clientY));
        end = '';
      } else if (z.getAttribute('data-monthzone')) {
        start = '';
        end = '';
      }
      submitMove(picked, date, start, end, colAs !== null ? colAs : picked.getAttribute('data-assignee'), colAs !== null);
      setPicked(null);
    }
  });

  root.addEventListener('dragstart', function (e) {
    if (resizing || moving || pendingMove || Date.now() < ignoreClickUntil) {
      e.preventDefault();
      return;
    }
    if (e.target.closest('[data-resize],[data-bar-act]')) {
      e.preventDefault();
      return;
    }
    var bar = barFrom(e.target);
    if (!bar || bar.getAttribute('data-can-move') !== '1') return;
    dragBar = bar;
    bar.classList.add('drag');
    e.dataTransfer.setData('text/plain', bar.getAttribute('data-ref') || '');
    e.dataTransfer.effectAllowed = 'move';
  });
  root.addEventListener('dragend', function () {
    dragBar = null;
    root.querySelectorAll('.drag,.drop').forEach(function (x) { x.classList.remove('drag', 'drop'); });
  });
  root.addEventListener('dragover', function (e) {
    var z = zoneFrom(e.target);
    if (!z || !dragBar) return;
    e.preventDefault();
    root.querySelectorAll('.drop').forEach(function (x) { if (x !== z) x.classList.remove('drop'); });
    z.classList.add('drop');
  });
  root.addEventListener('drop', function (e) {
    var z = zoneFrom(e.target);
    if (!z || !dragBar) return;
    e.preventDefault();
    var date = z.getAttribute('data-drop');
    var start = dragBar.getAttribute('data-start') || '09:00';
    var end = dragBar.getAttribute('data-end') || '';
    var colAs = z.getAttribute('data-assignee');
    if (z.getAttribute('data-dayzone')) {
      start = hhmm(snapY(z, e.clientY));
      end = '';
    } else if (z.getAttribute('data-monthzone')) {
      start = '';
      end = '';
    }
    submitMove(dragBar, date, start, end, colAs !== null ? colAs : dragBar.getAttribute('data-assignee'), colAs !== null);
    dragBar = null;
  });

  root.addEventListener('pointerdown', function (e) {
    var h = e.target.closest('[data-resize]');
    if (h) {
      var bar = barFrom(h);
      if (!bar || bar.getAttribute('data-can-resize') !== '1') return;
      e.preventDefault();
      e.stopPropagation();
      if (h.setPointerCapture) {
        try { h.setPointerCapture(e.pointerId); } catch (err) {}
      }
      resizing = {
        bar: bar,
        start: bar.getAttribute('data-start') || '09:00',
        origH: bar.style.height,
        pointerId: e.pointerId
      };
      bar.classList.add('board-bar-picked');
      ignoreClickUntil = Date.now() + 800;
      return;
    }
    if (e.target.closest('[data-bar-act],[data-bar-open]')) return;
    var moveBar = barFrom(e.target);
    if (!moveBar || moveBar.getAttribute('data-can-move') !== '1') return;
    var zone = moveBar.closest('[data-dayzone]');
    if (!zone) return;
    pendingMove = {
      bar: moveBar,
      zone: zone,
      x: e.clientX,
      y: e.clientY,
      origTop: parseInt(moveBar.style.top, 10) || 0,
      pointerId: e.pointerId
    };
  });
  document.addEventListener('pointermove', function (e) {
    if (pendingMove && !moving && !resizing) {
      var dx = e.clientX - pendingMove.x;
      var dy = e.clientY - pendingMove.y;
      if (dx * dx + dy * dy < 36) return;
      moving = pendingMove;
      pendingMove = null;
      moving.bar.classList.add('board-bar-picked', 'drag');
      ignoreClickUntil = Date.now() + 800;
      if (moving.bar.setPointerCapture) {
        try { moving.bar.setPointerCapture(e.pointerId); } catch (err) {}
      }
    }
    if (moving) {
      e.preventDefault();
      var over = dayzoneAt(e.clientX, e.clientY, moving.bar);
      if (over) moving.zone = over;
      var startMin = snapY(moving.zone, e.clientY);
      moving.bar.style.top = ((startMin - DAY_START) * PX_HOUR / 60 + 2) + 'px';
      moving.start = hhmm(startMin);
      return;
    }
    if (!resizing) return;
    e.preventDefault();
    var zone = resizing.bar.closest('[data-dayzone]') || resizing.bar.closest('[data-day-body]');
    if (!zone) return;
    var endMin = snapY(zone, e.clientY);
    var startMin = parseMin(resizing.start);
    if (endMin < startMin + 15) endMin = startMin + 15;
    if (endMin > DAY_END) endMin = DAY_END;
    var dur = endMin - startMin;
    var minH = dur > 30 ? 96 : 64;
    resizing.bar.style.height = Math.max(minH, dur * PX_HOUR / 60 - 4) + 'px';
    resizing.end = hhmm(endMin);
  }, { passive: false });
  document.addEventListener('pointerup', function () {
    if (pendingMove) {
      pendingMove = null;
    }
    if (moving) {
      var m = moving;
      moving = null;
      ignoreClickUntil = Date.now() + 400;
      m.bar.classList.remove('board-bar-picked', 'drag');
      if (m.start) {
        var colAs = m.zone ? m.zone.getAttribute('data-assignee') : null;
        submitMove(m.bar, m.bar.getAttribute('data-date'), m.start, '',
          colAs !== null ? colAs : m.bar.getAttribute('data-assignee'), colAs !== null);
      } else if (m.origTop != null) {
        m.bar.style.top = m.origTop + 'px';
      }
      return;
    }
    if (!resizing) return;
    var r = resizing;
    resizing = null;
    ignoreClickUntil = Date.now() + 400;
    r.bar.classList.remove('board-bar-picked');
    if (r.end) submitResize(r.bar, r.start, r.end);
    else if (r.origH != null) r.bar.style.height = r.origH;
  });
  document.addEventListener('pointercancel', function () {
    if (pendingMove) pendingMove = null;
    if (moving) {
      if (moving.origTop != null) moving.bar.style.top = moving.origTop + 'px';
      moving.bar.classList.remove('board-bar-picked', 'drag');
      moving = null;
      ignoreClickUntil = Date.now() + 400;
    }
    if (!resizing) return;
    if (resizing.origH != null) resizing.bar.style.height = resizing.origH;
    resizing.bar.classList.remove('board-bar-picked');
    resizing = null;
    ignoreClickUntil = Date.now() + 400;
  });
  root.setAttribute('data-board-sched-bound', '1');
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
