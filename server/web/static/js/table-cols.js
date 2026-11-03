(function (root) {
  var DEFAULTS = {
    projects: [92, 300, 120, 120, 64, 120, 200, 64, 84, 100, 130],
    contracts: [92, 300, 160, 120, 120, 120, 100, 88, 160, 130],
    product_groups: [36, 110, 110, 160, 160, 130, 120],
    workboard_list: [32, 280, 200, 140, 120, 88]
  };
  var MINS = {
    projects: [64, 140, 90, 80, 48, 80, 120, 44, 44, 70, 60],
    contracts: [64, 140, 90, 80, 80, 80, 80, 64, 80, 60],
    product_groups: [28, 80, 80, 100, 100, 90, 80],
    workboard_list: [28, 120, 90, 80, 70, 64]
  };
  var FALLBACK = 120;
  var MIN_FALLBACK = 40;

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

  function defaultsFor(key, n) {
    var def = DEFAULTS[key];
    if (def && def.length === n) return def.slice();
    var out = [];
    for (var i = 0; i < n; i++) out.push(FALLBACK);
    return out;
  }

  function minsFor(key, n) {
    var m = MINS[key];
    if (m && m.length === n) return m.slice();
    var out = [];
    for (var i = 0; i < n; i++) out.push(MIN_FALLBACK);
    return out;
  }

  function mergeWidths(key, n, stored) {
    var def = defaultsFor(key, n);
    var mins = minsFor(key, n);
    if (!stored || stored.length !== n) return def;
    return stored.map(function (v, i) {
      var num = Number(v);
      if (!isFinite(num)) return def[i];
      return Math.max(mins[i] || MIN_FALLBACK, num);
    });
  }

  function sumPx(widths) {
    var sum = 0;
    for (var i = 0; i < widths.length; i++) sum += widths[i];
    return sum;
  }

  function tableKey(table, loc) {
    var k = table.getAttribute && table.getAttribute("data-table-cols");
    if (k) return k;
    var path = "/";
    if (loc && loc.pathname) path = loc.pathname;
    else if (typeof location !== "undefined" && location.pathname) path = location.pathname;
    path = String(path).replace(/\/+$/, "") || "/";
    var idx = table.getAttribute && table.getAttribute("data-table-cols-idx");
    return "auto:" + path + "#" + (idx || "0");
  }

  function skipNode(el) {
    if (!el) return false;
    if (el.getAttribute && (el.getAttribute("data-table-cols-skip") === "" || el.getAttribute("data-table-cols-skip") === "1" || el.hasAttribute && el.hasAttribute("data-table-cols-skip"))) {
      return true;
    }
    var p = el.parentElement || el.parentNode;
    while (p && p !== el && p.getAttribute) {
      if (p.hasAttribute && p.hasAttribute("data-table-cols-skip")) return true;
      p = p.parentElement || p.parentNode;
    }
    return false;
  }

  function isListTable(table) {
    if (!table || table.tagName && table.tagName.toLowerCase() !== "table") return false;
    if (skipNode(table)) return false;
    var ths = headerCells(table);
    return ths.length >= 2;
  }

  function headerCells(table) {
    var thead = table.tHead || (table.querySelector && table.querySelector("thead"));
    if (!thead) return [];
    var row = thead.rows && thead.rows[0];
    if (row && row.cells) return Array.prototype.slice.call(row.cells);
    var q = thead.querySelectorAll && thead.querySelectorAll("tr:first-child th, tr:first-child td");
    return q ? Array.prototype.slice.call(q) : [];
  }

  function colEls(table) {
    var g = table.querySelector && table.querySelector("colgroup");
    if (!g) return [];
    return Array.prototype.slice.call(g.querySelectorAll("col"));
  }

  function ensureColgroup(table, n) {
    var cols = colEls(table);
    if (cols.length === n) return cols;
    var g = table.querySelector && table.querySelector("colgroup");
    if (g && g.parentNode) g.parentNode.removeChild(g);
    g = table.ownerDocument ? table.ownerDocument.createElement("colgroup") : null;
    if (!g) return [];
    for (var i = 0; i < n; i++) {
      var c = table.ownerDocument.createElement("col");
      c.setAttribute("data-col", String(i));
      g.appendChild(c);
    }
    table.insertBefore(g, table.firstChild);
    return colEls(table);
  }

  function applyWidths(table, widths) {
    var cols = colEls(table);
    var n = Math.min(cols.length, widths.length);
    for (var i = 0; i < n; i++) {
      cols[i].style.width = widths[i] + "px";
    }
    table.style.width = sumPx(widths.slice(0, n)) + "px";
    table.style.maxWidth = "none";
    if (table.classList) {
      table.classList.add("table-cols");
      table.classList.remove("w-full");
    }
  }

  function currentWidths(table, def) {
    var cols = colEls(table);
    var out = [];
    for (var i = 0; i < cols.length; i++) {
      out.push(parseInt(cols[i].style.width, 10) || def[i] || FALLBACK);
    }
    return out;
  }

  function ensureHandles(table) {
    var ths = headerCells(table);
    for (var i = 0; i < ths.length; i++) {
      var th = ths[i];
      if (th.classList) th.classList.add("relative");
      var existing = th.querySelector && th.querySelector(".col-resize-handle");
      if (existing) {
        existing.setAttribute("data-col", String(i));
        if (existing.classList) existing.classList.add("col-resize-handle-wide");
        continue;
      }
      var doc = th.ownerDocument || (typeof document !== "undefined" ? document : null);
      if (!doc) continue;
      var handle = doc.createElement("span");
      handle.className = "col-resize-handle col-resize-handle-wide";
      handle.setAttribute("role", "separator");
      handle.setAttribute("tabindex", "0");
      handle.setAttribute("data-col", String(i));
      handle.setAttribute("aria-label", "열 너비 조절");
      th.appendChild(handle);
    }
  }

  function ensureScroll(table) {
    var p = table.parentElement;
    if (!p) return;
    if (p.classList && p.classList.contains("table-scroll")) return;
    var st = (typeof window !== "undefined" && window.getComputedStyle) ? window.getComputedStyle(p) : null;
    if (st && (st.overflowX === "auto" || st.overflowX === "scroll")) return;
    if (p.classList) p.classList.add("table-scroll");
  }

  function bindResize(table, key, mins, def) {
    function setCol(i, px) {
      var min = mins[i] || MIN_FALLBACK;
      var w = Math.max(min, px);
      var arr = currentWidths(table, def);
      arr[i] = w;
      applyWidths(table, arr);
      writeStore(key, arr);
    }
    var handles = table.querySelectorAll ? table.querySelectorAll(".col-resize-handle") : [];
    Array.prototype.forEach.call(handles, function (handle) {
      if (handle.getAttribute("data-cols-hbound") === "1") return;
      handle.setAttribute("data-cols-hbound", "1");
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
          setCol(i, currentWidths(table, def)[i] - 16);
        } else if (e.key === "ArrowRight") {
          e.preventDefault();
          setCol(i, currentWidths(table, def)[i] + 16);
        }
      });
      handle.addEventListener("pointerdown", function (e) {
        e.preventDefault();
        e.stopPropagation();
        if (handle.setPointerCapture) handle.setPointerCapture(e.pointerId);
        var i = Number(handle.getAttribute("data-col"));
        var startX = e.clientX;
        var startW = currentWidths(table, def)[i];
        function move(ev) {
          ev.preventDefault();
          setCol(i, startW + (ev.clientX - startX));
        }
        function up() {
          if (handle.releasePointerCapture) handle.releasePointerCapture(e.pointerId);
          handle.removeEventListener("pointermove", move);
          handle.removeEventListener("pointerup", up);
        }
        handle.addEventListener("pointermove", move);
        handle.addEventListener("pointerup", up);
      });
    });
  }

  function bindTable(table) {
    if (!isListTable(table)) return;
    var n = headerCells(table).length;
    if (n < 2) return;
    var key = tableKey(table);
    var def = defaultsFor(key, n);
    var mins = minsFor(key, n);
    if (table.getAttribute("data-cols-bound") === "1") {
      applyWidths(table, mergeWidths(key, n, readStore(key)));
      return;
    }
    table.setAttribute("data-cols-bound", "1");
    if (!table.getAttribute("data-table-cols")) table.setAttribute("data-table-cols", key);
    ensureColgroup(table, n);
    ensureHandles(table);
    ensureScroll(table);
    applyWidths(table, mergeWidths(key, n, readStore(key)));
    bindResize(table, key, mins, def);
  }

  function applyGridWidths(root, widths) {
    var parts = [];
    for (var i = 0; i < widths.length; i++) parts.push(widths[i] + "px");
    var val = parts.join(" ");
    root.style.setProperty("--wl-cols", val);
    var nodes = root.querySelectorAll ? root.querySelectorAll(".wl-head, .wl-row") : [];
    Array.prototype.forEach.call(nodes, function (el) {
      el.style.gridTemplateColumns = val;
    });
  }

  function bindGrid(root) {
    if (!root || skipNode(root)) return;
    var key = root.getAttribute("data-grid-cols") || "workboard_list";
    var head = root.querySelector && root.querySelector(".wl-head");
    if (!head) return;
    var cells = [];
    for (var i = 0; i < head.children.length; i++) cells.push(head.children[i]);
    var n = cells.length;
    if (n < 2) return;
    var def = defaultsFor(key, n);
    var mins = minsFor(key, n);
    var widths = mergeWidths(key, n, readStore(key));
    applyGridWidths(root, widths);
    if (root.getAttribute("data-cols-bound") === "1") return;
    root.setAttribute("data-cols-bound", "1");
    for (var c = 0; c < cells.length; c++) {
      var cell = cells[c];
      if (cell.classList) cell.classList.add("relative");
      var h = cell.querySelector && cell.querySelector(".col-resize-handle");
      if (!h) {
        var doc = cell.ownerDocument || (typeof document !== "undefined" ? document : null);
        if (!doc) continue;
        h = doc.createElement("span");
        h.className = "col-resize-handle col-resize-handle-wide";
        h.setAttribute("role", "separator");
        h.setAttribute("tabindex", "0");
        h.setAttribute("aria-label", "열 너비 조절");
        cell.appendChild(h);
      }
      h.setAttribute("data-col", String(c));
    }
    function setCol(i, px) {
      var arr = widths.slice();
      arr[i] = Math.max(mins[i] || MIN_FALLBACK, px);
      widths = arr;
      applyGridWidths(root, arr);
      writeStore(key, arr);
    }
    var handles = head.querySelectorAll(".col-resize-handle");
    Array.prototype.forEach.call(handles, function (handle) {
      if (handle.getAttribute("data-cols-hbound") === "1") return;
      handle.setAttribute("data-cols-hbound", "1");
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
      handle.addEventListener("pointerdown", function (e) {
        e.preventDefault();
        e.stopPropagation();
        if (handle.setPointerCapture) handle.setPointerCapture(e.pointerId);
        var i = Number(handle.getAttribute("data-col"));
        var startX = e.clientX;
        var startW = widths[i];
        function move(ev) {
          ev.preventDefault();
          setCol(i, startW + (ev.clientX - startX));
        }
        function up() {
          if (handle.releasePointerCapture) handle.releasePointerCapture(e.pointerId);
          handle.removeEventListener("pointermove", move);
          handle.removeEventListener("pointerup", up);
        }
        handle.addEventListener("pointermove", move);
        handle.addEventListener("pointerup", up);
      });
    });
  }

  function indexTables(rootEl) {
    var doc = rootEl || (typeof document !== "undefined" ? document : null);
    if (!doc || !doc.querySelectorAll) return;
    var tables = doc.querySelectorAll("table");
    var n = 0;
    Array.prototype.forEach.call(tables, function (table) {
      if (!isListTable(table)) return;
      if (!table.getAttribute("data-table-cols") && !table.getAttribute("data-table-cols-idx")) {
        table.setAttribute("data-table-cols-idx", String(n));
      }
      n += 1;
      bindTable(table);
    });
    var grids = doc.querySelectorAll("[data-grid-cols]");
    Array.prototype.forEach.call(grids, function (el) {
      if (el.querySelector && el.querySelector(".wl-head")) bindGrid(el);
    });
  }

  function applyAll() {
    indexTables(typeof document !== "undefined" ? document : null);
  }

  function onResetClick(e) {
    var btn = e.target && e.target.closest && e.target.closest("[data-table-cols-reset]");
    if (!btn) return;
    var key = btn.getAttribute("data-table-cols-reset");
    try { localStorage.removeItem("table_cols:" + key); } catch (err) {}
    var doc = typeof document !== "undefined" ? document : null;
    if (!doc) return;
    var tables = doc.querySelectorAll('table[data-table-cols="' + key + '"]');
    Array.prototype.forEach.call(tables, function (table) {
      table.removeAttribute("data-cols-bound");
      bindTable(table);
    });
    var grids = doc.querySelectorAll('[data-grid-cols="' + key + '"]');
    Array.prototype.forEach.call(grids, function (g) {
      g.removeAttribute("data-cols-bound");
      bindGrid(g);
    });
  }

  if (typeof document !== "undefined" && document.addEventListener) {
    document.addEventListener("click", onResetClick);
    document.addEventListener("DOMContentLoaded", applyAll);
    if (document.body && document.body.addEventListener) {
      document.body.addEventListener("htmx:afterSwap", applyAll);
    }
    if (document.readyState !== "loading") applyAll();
  }

  var api = {
    DEFAULTS: DEFAULTS,
    MINS: MINS,
    readStore: readStore,
    writeStore: writeStore,
    mergeWidths: mergeWidths,
    sumPx: sumPx,
    tableKey: tableKey,
    isListTable: isListTable,
    applyWidths: applyWidths,
    bindTable: bindTable,
    bindGrid: bindGrid,
    applyAll: applyAll
  };
  root.TableCols = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof window !== "undefined" ? window : (typeof global !== "undefined" ? global : this));
