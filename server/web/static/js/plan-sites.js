(function (w) {
  function closest(el, sel) {
    return el && el.closest ? el.closest(sel) : null;
  }
  function pickedBox(root) {
    return root.querySelector('[data-plan-picked]');
  }
  function remember(root, id, name, taken) {
    if (!id || taken) return;
    var box = pickedBox(root);
    if (!box || box.querySelector('[data-id="' + id + '"]')) return;
    var chip = document.createElement('label');
    chip.className = 'badge';
    chip.setAttribute('data-id', id);
    chip.textContent = name || id;
    var hid = document.createElement('input');
    hid.type = 'hidden';
    hid.name = 'site_id';
    hid.value = id;
    chip.appendChild(hid);
    box.appendChild(chip);
  }
  function renderHits(root, sites) {
    var box = root.querySelector('[data-plan-results]');
    if (!box) return;
    box.textContent = '';
    (sites || []).forEach(function (s) {
      var row = document.createElement('label');
      row.className = 'flex items-center gap-2 text-sm';
      var cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.disabled = !!s.Taken;
      cb.addEventListener('change', function () {
        if (cb.checked) remember(root, s.ID, s.Name, false);
      });
      row.appendChild(cb);
      var text = (s.Name || '') + (s.Region ? ' · ' + s.Region : '');
      if (s.Taken && s.PlanName) text += ' · ' + s.PlanName + ' 계획';
      row.appendChild(document.createTextNode(text));
      box.appendChild(row);
    });
  }
  function search(root, offset) {
    var q = (root.querySelector('[data-plan-q]') || {}).value || '';
    var region = (root.querySelector('[data-plan-region]') || {}).value || '';
    var project = root.getAttribute('data-project') || '';
    var pkg = '';
    var form = closest(root, 'form');
    if (form) {
      var hid = form.querySelector('[name=package_id]');
      if (hid) pkg = hid.value || '';
    }
    var url = '/projects/' + encodeURIComponent(project) + '/plans/sites?package_id=' + encodeURIComponent(pkg) +
      '&q=' + encodeURIComponent(q) + '&region=' + encodeURIComponent(region) + '&offset=' + (offset || 0);
    fetch(url, { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (j) {
      renderHits(root, j.sites || []);
      root._offset = j.offset || 0;
      root._total = j.total || 0;
    }).catch(function () {});
  }
  document.addEventListener('click', function (ev) {
    var add = closest(ev.target, '[data-add-plan]');
    if (add) {
      var form = document.querySelector('[data-plan-form]');
      if (!form) return;
      form.classList.remove('hidden');
      var hid = form.querySelector('[name=package_id]');
      if (hid) hid.value = add.getAttribute('data-add-plan') || '';
      var name = form.querySelector('[name=name]');
      if (name) name.focus();
      return;
    }
    var btn = closest(ev.target, '[data-plan-search]');
    if (btn) {
      var root = closest(btn, '[data-plan-sites]');
      if (root) search(root, 0);
      return;
    }
    var all = closest(ev.target, '[data-plan-all]');
    if (all) {
      var root2 = closest(all, '[data-plan-sites]');
      if (!root2) return;
      root2.querySelectorAll('[data-plan-results] input[type=checkbox]').forEach(function (cb) {
        if (!cb.disabled) {
          cb.checked = true;
          cb.dispatchEvent(new Event('change'));
        }
      });
    }
  });
  document.addEventListener('keydown', function (ev) {
    if (ev.key !== 'Enter') return;
    var q = closest(ev.target, '[data-plan-q]');
    if (!q) return;
    ev.preventDefault();
    var root = closest(q, '[data-plan-sites]');
    if (root) search(root, 0);
  });

  function loadToday(form, projectId) {
    var slot = form && form.querySelector('[data-plan-slot]');
    if (!slot) return;
    var pkg = (form.querySelector('[name=package_id]') || {}).value || '';
    var plan = (form.querySelector('[name=plan_id]') || {}).value || '';
    var box = slot.querySelector('[data-today-sites]');
    if (!box) {
      box = document.createElement('div');
      box.setAttribute('data-today-sites', '1');
      box.className = 'mt-2 space-y-1';
      slot.appendChild(box);
    }
    var key = pkg + '|' + plan;
    if (form._planKey !== key) {
      form._planKey = key;
      form._todayPicked = {};
    }
    if (!pkg) {
      box.textContent = '';
      return;
    }
    var qEl = box.querySelector('[data-today-q]');
    var rEl = box.querySelector('[data-today-region]');
    var q = qEl ? qEl.value : '';
    var region = rEl ? rEl.value : '';
    var picked = form._todayPicked || {};
    var add = Object.keys(picked).length;
    var day = (form.querySelector('[name=work_date]') || {}).value || '';
    var url = '/projects/' + encodeURIComponent(projectId) + '/open-sites?package_id=' + encodeURIComponent(pkg) +
      '&plan_id=' + encodeURIComponent(plan) + '&add=' + add + '&q=' + encodeURIComponent(q) +
      '&region=' + encodeURIComponent(region) + '&date=' + encodeURIComponent(day);
    fetch(url, { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (j) { if (j) paintToday(form, box, j, projectId, q, region); });
  }
  function syncToday(form, box) {
    var hold = box.querySelector('[data-today-picked]');
    if (!hold) return;
    hold.innerHTML = '';
    Object.keys(form._todayPicked || {}).forEach(function (id) {
      var h = document.createElement('input');
      h.type = 'hidden';
      h.name = 'site_id';
      h.value = id;
      hold.appendChild(h);
    });
  }
  function paintToday(form, box, j, projectId, q, region) {
    var keepQ = box.querySelector('[data-today-q]');
    var keepR = box.querySelector('[data-today-region]');
    if (!keepQ) {
      box.innerHTML = '';
      var title = document.createElement('p');
      title.className = 'text-sm font-medium text-ink';
      title.textContent = '오늘 처리할 사이트';
      var qIn = document.createElement('input');
      qIn.setAttribute('data-today-q', '1');
      qIn.className = 'input text-sm w-full';
      qIn.placeholder = '학교명 · 초성 검색 가능';
      qIn.addEventListener('keydown', function (ev) { if (ev.key === 'Enter') ev.preventDefault(); });
      qIn.addEventListener('input', function () { loadToday(form, projectId); });
      var rIn = document.createElement('input');
      rIn.setAttribute('data-today-region', '1');
      rIn.className = 'input text-sm w-full';
      rIn.placeholder = '지역';
      rIn.addEventListener('keydown', function (ev) { if (ev.key === 'Enter') ev.preventDefault(); });
      rIn.addEventListener('input', function () { loadToday(form, projectId); });
      var prev = document.createElement('p');
      prev.setAttribute('data-today-preview', '1');
      prev.className = 'text-xs text-ink-muted';
      var list = document.createElement('div');
      list.setAttribute('data-today-list', '1');
      list.className = 'space-y-1';
      var hold = document.createElement('div');
      hold.setAttribute('data-today-picked', '1');
      box.appendChild(title);
      box.appendChild(qIn);
      box.appendChild(rIn);
      box.appendChild(prev);
      box.appendChild(list);
      box.appendChild(hold);
      keepQ = qIn;
      keepR = rIn;
    }
    if (document.activeElement !== keepQ) keepQ.value = q || '';
    if (document.activeElement !== keepR) keepR.value = region || '';
    var prevEl = box.querySelector('[data-today-preview]');
    if (prevEl) prevEl.textContent = j.preview || '';
    var listEl = box.querySelector('[data-today-list]');
    listEl.innerHTML = '';
    var sites = j.sites || [];
    if (!sites.length) {
      var empty = document.createElement('p');
      empty.className = 'text-xs text-ink-muted';
      empty.textContent = '미완료 사이트가 없습니다';
      listEl.appendChild(empty);
    }
    sites.forEach(function (s) {
      var row = document.createElement('label');
      row.className = 'flex items-center gap-2 text-sm';
      var cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.checked = !!(form._todayPicked || {})[s.CustomerID || s.customer_id];
      var cid = s.CustomerID || s.customer_id;
      cb.addEventListener('change', function () {
        form._todayPicked = form._todayPicked || {};
        if (cb.checked) {
          if (Object.keys(form._todayPicked).length >= 100) { cb.checked = false; return; }
          form._todayPicked[cid] = s.Name || s.name || cid;
        } else {
          delete form._todayPicked[cid];
        }
        syncToday(form, box);
        loadToday(form, projectId);
      });
      var text = (s.Name || s.name || '') + ((s.Region || s.region) ? ' · ' + (s.Region || s.region) : '');
      var cont = s.Continue || s.continue;
      if (cont) text += ' · 이어서' + ((s.LastDate || s.last_date) ? ' ' + (s.LastDate || s.last_date) : '') + ((s.LastUser || s.last_user) ? ' ' + (s.LastUser || s.last_user) : '');
      var assigned = s.Assigned || s.assigned;
      if (assigned) text += ' · ' + assigned + ' 배정됨';
      row.appendChild(cb);
      row.appendChild(document.createTextNode(text));
      listEl.appendChild(row);
    });
    syncToday(form, box);
  }
  function mountPanel(form, projectId) {
    if (!form || !form.hasAttribute('data-plan-panel')) return;
    var slot = form.querySelector('[data-plan-slot]');
    if (!slot) {
      slot = document.createElement('div');
      slot.setAttribute('data-plan-slot', '1');
      var title = form.querySelector('[name=title]');
      if (title && title.parentNode) title.parentNode.insertAdjacentElement('afterend', slot);
      else form.insertBefore(slot, form.firstChild);
    }
    if (!projectId) {
      slot.textContent = '';
      return;
    }
    fetch('/projects/' + encodeURIComponent(projectId) + '/structure/options', { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : { has: false }; })
      .then(function (j) {
        slot.textContent = '';
        if (!j || !j.has) return;
        var wrap = document.createElement('div');
        wrap.className = 'space-y-1';
        wrap.setAttribute('data-structure-panel', '1');
        var lab = document.createElement('label');
        lab.className = 'block text-sm font-medium text-ink';
        lab.textContent = '수행 단계';
        var sel = document.createElement('select');
        sel.className = 'input text-sm w-full';
        sel.name = 'plan_pick';
        var empty = document.createElement('option');
        empty.value = '';
        empty.textContent = '단계 없이 등록(미분류로 집계)';
        sel.appendChild(empty);
        var showDone = document.createElement('label');
        showDone.className = 'text-xs text-ink-muted flex items-center gap-1';
        var cb = document.createElement('input');
        cb.type = 'checkbox';
        showDone.appendChild(cb);
        showDone.appendChild(document.createTextNode('완료된 계획 보기'));
        function fill() {
          var cur = sel.value;
          while (sel.options.length > 1) sel.remove(1);
          (j.options || []).forEach(function (o) {
            if (o.Hold) return;
            if (o.Done && !cb.checked) return;
            var op = document.createElement('option');
            op.value = (o.PackageID || '') + '|' + (o.PlanID || '');
            op.textContent = o.Label || '';
            op.setAttribute('data-label', o.Label || '');
            sel.appendChild(op);
          });
          sel.value = cur;
        }
        cb.addEventListener('change', fill);
        fill();
        var pkg = document.createElement('input');
        pkg.type = 'hidden';
        pkg.name = 'package_id';
        var plan = document.createElement('input');
        plan.type = 'hidden';
        plan.name = 'plan_id';
        sel.addEventListener('change', function () {
          var parts = (sel.value || '').split('|');
          pkg.value = parts[0] || '';
          plan.value = parts[1] || '';
          loadToday(form, projectId);
          var title = form.querySelector('[name=title]');
          if (!title || title.getAttribute('data-title-touched') === '1') return;
          var opt = sel.options[sel.selectedIndex];
          var label = opt ? opt.getAttribute('data-label') || '' : '';
          if (label) {
            var bits = label.split(' › ');
            title.value = bits.length >= 2 ? (bits[bits.length - 1] + ' ' + bits[bits.length - 2]) : label;
          }
        });
        var title = form.querySelector('[name=title]');
        if (title && !title._planHook) {
          title._planHook = true;
          title.addEventListener('input', function () { title.setAttribute('data-title-touched', '1'); });
        }
        wrap.appendChild(lab);
        wrap.appendChild(sel);
        wrap.appendChild(showDone);
        wrap.appendChild(pkg);
        wrap.appendChild(plan);
        slot.appendChild(wrap);
      }).catch(function () {});
  }
  document.addEventListener('project-picked', function (ev) {
    var form = closest(ev.target, 'form');
    var id = ev.detail && ev.detail.id;
    mountPanel(form, id || '');
  });
  document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('form[data-plan-panel]').forEach(function (form) {
      var hid = form.querySelector('[name=project_id]');
      if (hid && hid.value) mountPanel(form, hid.value);
    });
  });
  w.mountPlanPanel = mountPanel;
})(window);
