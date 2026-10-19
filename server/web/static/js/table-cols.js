(function () {
  var DEFAULTS = {
    projects: [92, 300, 120, 120, 64, 120, 200, 64, 84, 100, 130],
    contracts: [92, 300, 160, 120, 120, 120, 100, 88, 160, 130]
  };
  var MINS = {
    projects: [64, 140, 90, 80, 48, 80, 120, 44, 44, 70, 60],
    contracts: [64, 140, 90, 80, 80, 80, 80, 64, 80, 60]
  };

  function readStore(key) {
    try {
      var raw = localStorage.getItem("table_cols:" + key);
      if (!raw) return null;
      var arr = JSON.parse(raw);
      if (!Array.isArray(arr)) return null;
      return arr;
    } catch (e) {
      return null;
    }
  }

  function writeStore(key, arr) {
    try {
      localStorage.setItem("table_cols:" + key, JSON.stringify(arr));
    } catch (e) {}
  }

  function applyWidths(table, widths) {
    var cols = table.querySelectorAll("colgroup col");
    var sum = 0;
    for (var i = 0; i < cols.length; i++) {
      var w = widths[i];
      cols[i].style.width = w + "px";
      sum += w;
    }
    table.style.width = sum + "px";
  }

  function widthsFor(table) {
    var key = table.getAttribute("data-table-cols") || "projects";
    var def = (DEFAULTS[key] || DEFAULTS.projects).slice();
    var stored = readStore(key);
    if (!stored || stored.length !== def.length) return def;
    var mins = MINS[key] || MINS.projects;
    return stored.map(function (v, i) {
      var n = Number(v);
      if (!isFinite(n)) return def[i];
      return Math.max(mins[i] || 40, n);
    });
  }

  function bindTable(table) {
    if (table.getAttribute("data-cols-bound") === "1") {
      applyWidths(table, widthsFor(table));
      return;
    }
    table.setAttribute("data-cols-bound", "1");
    var key = table.getAttribute("data-table-cols") || "projects";
    var mins = MINS[key] || MINS.projects;
    var def = (DEFAULTS[key] || DEFAULTS.projects).slice();
    applyWidths(table, widthsFor(table));

    function current() {
      var cols = table.querySelectorAll("colgroup col");
      var out = [];
      for (var i = 0; i < cols.length; i++) {
        out.push(parseInt(cols[i].style.width, 10) || def[i]);
      }
      return out;
    }

    function setCol(i, px) {
      var min = mins[i] || 40;
      var w = Math.max(min, px);
      var arr = current();
      arr[i] = w;
      applyWidths(table, arr);
      writeStore(key, arr);
    }

    table.querySelectorAll(".col-resize-handle").forEach(function (handle) {
      handle.addEventListener("click", function (e) {
        e.preventDefault();
        e.stopPropagation();
      });
      handle.addEventListener("dblclick", function (e) {
        e.preventDefault();
        e.stopPropagation();
        var i = Number(handle.getAttribute("data-col"));
        setCol(i, def[i]);
      });
      handle.addEventListener("keydown", function (e) {
        var i = Number(handle.getAttribute("data-col"));
        if (e.key === "ArrowLeft") {
          e.preventDefault();
          setCol(i, current()[i] - 16);
        } else if (e.key === "ArrowRight") {
          e.preventDefault();
          setCol(i, current()[i] + 16);
        }
      });
      handle.addEventListener("pointerdown", function (e) {
        e.preventDefault();
        e.stopPropagation();
        handle.setPointerCapture(e.pointerId);
        var i = Number(handle.getAttribute("data-col"));
        var startX = e.clientX;
        var startW = current()[i];
        function move(ev) {
          ev.preventDefault();
          setCol(i, startW + (ev.clientX - startX));
        }
        function up() {
          handle.releasePointerCapture(e.pointerId);
          handle.removeEventListener("pointermove", move);
          handle.removeEventListener("pointerup", up);
        }
        handle.addEventListener("pointermove", move);
        handle.addEventListener("pointerup", up);
      });
    });
  }

  function applyAll() {
    document.querySelectorAll("table[data-table-cols]").forEach(bindTable);
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-table-cols-reset]");
    if (!btn) return;
    var key = btn.getAttribute("data-table-cols-reset");
    try { localStorage.removeItem("table_cols:" + key); } catch (err) {}
    document.querySelectorAll('table[data-table-cols="' + key + '"]').forEach(function (table) {
      table.removeAttribute("data-cols-bound");
      bindTable(table);
    });
  });

  document.addEventListener("DOMContentLoaded", applyAll);
  document.body.addEventListener("htmx:afterSwap", applyAll);
  applyAll();
})();
