(function () {
  function groups() {
    if (window.WB_WORK_GROUPS && window.WB_WORK_GROUPS.length) return window.WB_WORK_GROUPS;
    var el = document.getElementById("wb-work-groups");
    if (!el || !el.textContent) return [];
    try { return JSON.parse(el.textContent); } catch (e) { return []; }
  }

  function findGroup(id) {
    var list = groups();
    for (var i = 0; i < list.length; i++) {
      if (list[i].id === id) return list[i];
    }
    return null;
  }

  function alpineOf(el) {
    var n = el;
    while (n) {
      if (n._x_dataStack && n._x_dataStack[0]) return n._x_dataStack[0];
      n = n.parentElement;
    }
    return null;
  }

  function applySelect(sel) {
    var form = sel.form || sel.closest("form");
    if (!form) return;
    var ax = alpineOf(sel) || alpineOf(form);
    var g = findGroup(sel.value);
    if (!g) {
      if (ax && ax._wgAuto) {
        if (!ax._wgTouchedAssignee) ax.preAssignee = ax._wgPrevAssignee || "";
        if (!ax._wgTouchedMembers) ax.memberRows = ax._wgPrevMembers || [];
        ax._wgAuto = false;
      }
      return;
    }
    if (ax && !ax._wgAuto) {
      ax._wgPrevAssignee = ax.preAssignee || "";
      ax._wgPrevMembers = (ax.memberRows || []).slice();
      ax._wgAuto = true;
      ax._wgTouchedAssignee = false;
      ax._wgTouchedMembers = false;
    }
    var leader = g.leader_name || "";
    var rows = [];
    (g.members || []).forEach(function (m) {
      if (!m.name || m.name === leader) return;
      rows.push({ name: m.name, mins: "" });
    });
    if (ax) {
      ax.preAssignee = leader;
      ax.memberRows = rows;
    } else {
      var asg = form.querySelector("select[name=assignee]");
      if (asg) asg.value = leader;
    }
  }

  document.addEventListener("change", function (e) {
    var t = e.target;
    if (!t || t.name !== "work_group_id") return;
    applySelect(t);
  });

  document.addEventListener("change", function (e) {
    var t = e.target;
    if (!t || t.name !== "assignee") return;
    var ax = alpineOf(t);
    if (ax && ax._wgAuto) ax._wgTouchedAssignee = true;
  });
})();
