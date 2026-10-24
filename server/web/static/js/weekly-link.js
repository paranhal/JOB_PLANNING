(function () {
  var opener = null;

  function modal() {
    return document.getElementById("weekly-link-modal");
  }

  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'a[href], button:not([disabled]), textarea, input:not([disabled]), select, [tabindex]:not([tabindex="-1"])'
    )).filter(function (n) {
      return n.offsetParent !== null || n === document.activeElement;
    });
  }

  function closeModal() {
    var m = modal();
    if (!m) return;
    m.hidden = true;
    document.body.classList.remove("overflow-hidden");
    var back = opener;
    opener = null;
    if (back && back.focus) back.focus();
  }

  function trap(e) {
    var m = modal();
    if (!m || m.hidden) return;
    if (e.key === "Escape") {
      e.preventDefault();
      closeModal();
      return;
    }
    if (e.key !== "Tab") return;
    var dlg = m.querySelector('[role="dialog"]');
    var nodes = focusables(dlg || m);
    if (!nodes.length) return;
    var first = nodes[0];
    var last = nodes[nodes.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  function selectedProject() {
    var m = modal();
    var sel = m && m.querySelector('input[name="project_id"]:checked');
    return sel;
  }

  function syncSubmit() {
    var m = modal();
    if (!m) return;
    var btn = m.querySelector("[data-weekly-link-submit]");
    var foreignBox = m.querySelector("[data-confirm-foreign]");
    var proj = selectedProject();
    var foreign = proj && proj.getAttribute("data-foreign") === "1";
    if (foreignBox) {
      foreignBox.closest("label").hidden = !foreign;
    }
    var ok = !!proj;
    if (foreign && !(foreignBox && foreignBox.checked)) ok = false;
    var onlyMnt = (m.getAttribute("data-only-mnt") === "1");
    if (onlyMnt) ok = false;
    if (btn) btn.disabled = !ok;
  }

  function fillItems(items) {
    var host = modal().querySelector("[data-weekly-link-items]");
    host.innerHTML = "";
    items.forEach(function (it) {
      var inp = document.createElement("input");
      inp.type = "hidden";
      inp.name = "item";
      inp.value = it.kind + ":" + it.id;
      host.appendChild(inp);
    });
  }

  function markCurrentAssignee(uid, name) {
    var m = modal();
    m.querySelectorAll("[data-assignee-row]").forEach(function (row) {
      var hit = (uid && row.getAttribute("data-uid") === uid) ||
        (name && row.getAttribute("data-name") === name);
      row.classList.toggle("bg-canvas", !!hit);
    });
  }

  function openWith(items, btn) {
    var m = modal();
    if (!m || !items.length) return;
    opener = btn;
    fillItems(items);
    var title = items.length === 1 ? ("사업 연결 · " + (items[0].title || items[0].id)) : ("사업 연결 · " + items.length + "건");
    m.querySelector("[data-weekly-link-title]").textContent = title;
    var sum = m.querySelector("[data-weekly-link-summary]");
    sum.textContent = "";
    items.forEach(function (it) {
      var li = document.createElement("li");
      var meta = document.createElement("span");
      meta.className = "text-ui-xs text-ink-muted";
      meta.textContent = (it.kindLabel || it.kind) + " · " + (it.bucket || "");
      li.appendChild(meta);
      li.appendChild(document.createTextNode(" " + (it.title || it.id)));
      if (it.date) {
        var d = document.createElement("span");
        d.className = "text-ui-xs text-ink-subtle";
        d.textContent = " " + it.date;
        li.appendChild(d);
      }
      li.appendChild(document.createTextNode(" · 사업 " + (it.project || "없음") + " · 담당 " + (it.assignee || "없음")));
      sum.appendChild(li);
    });
    var foreignBox = m.querySelector("[data-confirm-foreign]");
    if (foreignBox) foreignBox.checked = false;
    var onlyMnt = items.every(function (it) { return it.kind === "maintenance_visit"; });
    m.setAttribute("data-only-mnt", onlyMnt ? "1" : "0");
    var mntHint = m.querySelector("[data-mnt-hint]");
    if (mntHint) mntHint.hidden = !onlyMnt;
    var projBox = m.querySelector("[data-project-box]");
    if (projBox) projBox.hidden = onlyMnt;
    var keep = m.querySelector("[data-assignee-keep]");
    if (keep) keep.checked = true;
    m.querySelectorAll("[data-assignee-radio]").forEach(function (r) {
      r.checked = false;
      r.disabled = true;
    });
    markCurrentAssignee(items[0] && items[0].uid, items[0] && items[0].assignee);
    var curUID = items[0] && items[0].uid;
    if (curUID) {
      m.querySelectorAll("[data-assignee-radio]").forEach(function (r) {
        if (r.value === curUID) r.checked = true;
      });
    }
    var first = selectedProject();
    if (first) first.checked = false;
    m.querySelectorAll('input[name="project_id"]').forEach(function (r) { r.checked = false; });
    m.hidden = false;
    document.body.classList.add("overflow-hidden");
    var closeBtn = m.querySelector("[data-weekly-link-close]");
    if (closeBtn) closeBtn.focus();
    syncSubmit();
  }

  function itemFromEl(el) {
    return {
      kind: el.getAttribute("data-kind") || "work_task",
      id: el.getAttribute("data-id") || "",
      title: el.getAttribute("data-title") || "",
      kindLabel: el.getAttribute("data-kind-label") || "",
      bucket: el.getAttribute("data-bucket") || "",
      date: el.getAttribute("data-date") || "",
      project: el.getAttribute("data-project") || "없음",
      assignee: el.getAttribute("data-assignee") || "",
      uid: el.getAttribute("data-uid") || ""
    };
  }

  document.addEventListener("click", function (e) {
    if (e.target.closest("[data-weekly-link-close]")) {
      e.preventDefault();
      closeModal();
      return;
    }
    var m = modal();
    if (m && !m.hidden && e.target === m) {
      e.preventDefault();
      closeModal();
      return;
    }
    var one = e.target.closest("[data-weekly-link]");
    if (one) {
      e.preventDefault();
      openWith([itemFromEl(one)], one);
      return;
    }
    var bulk = e.target.closest("[data-weekly-link-bulk]");
    if (bulk) {
      e.preventDefault();
      var root = bulk.closest("[data-weekly-task-link]");
      var picked = [];
      (root ? root : document).querySelectorAll("[data-weekly-item]:checked").forEach(function (cb) {
        var row = cb.closest("[data-weekly-row]");
        if (row) picked.push(itemFromEl(row.querySelector("[data-weekly-link]") || row));
      });
      if (!picked.length) return;
      openWith(picked, bulk);
    }
  });

  document.addEventListener("change", function (e) {
    if (e.target.matches("[data-weekly-item]")) {
      var root = e.target.closest("[data-weekly-task-link]");
      var n = root ? root.querySelectorAll("[data-weekly-item]:checked").length : 0;
      var bulk = root && root.querySelector("[data-weekly-link-bulk]");
      if (bulk) bulk.textContent = n ? ("선택한 " + n + "건 사업 연결") : "선택한 건 사업 연결";
      return;
    }
    if (!modal() || modal().hidden) return;
    if (e.target.matches("[data-assignee-keep]")) {
      var on = e.target.checked;
      modal().querySelectorAll("[data-assignee-radio]").forEach(function (r) {
        r.disabled = on;
        if (on) r.checked = false;
      });
    }
    if (e.target.matches('input[name="project_id"]') || e.target.matches("[data-confirm-foreign]")) {
      syncSubmit();
    }
  });

  document.addEventListener("input", function (e) {
    if (e.target.matches("[data-project-filter]")) {
      var q = (e.target.value || "").toLowerCase();
      modal().querySelectorAll("[data-project-row]").forEach(function (row) {
        var t = (row.getAttribute("data-name") || "").toLowerCase();
        row.hidden = q && t.indexOf(q) < 0;
      });
    }
    if (e.target.matches("[data-assignee-filter]")) {
      var q2 = (e.target.value || "").toLowerCase();
      modal().querySelectorAll("[data-assignee-row]").forEach(function (row) {
        var t = (row.getAttribute("data-name") || "").toLowerCase();
        row.hidden = q2 && t.indexOf(q2) < 0;
      });
    }
  });

  document.addEventListener("keydown", trap);
})();
