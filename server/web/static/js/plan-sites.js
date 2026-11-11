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
