(function (root) {
  var KEY = "mnt.cal.panelW";
  var DEFAULT_W = 285;
  var MIN_W = 240;
  var MAX_W = 560;
  var CAL_MIN = 560;
  var STEP = 16;
  var STEP_SHIFT = 64;

  function readStore() {
    try {
      var raw = localStorage.getItem(KEY);
      if (raw == null || raw === "") return null;
      var n = Number(raw);
      if (!isFinite(n)) return null;
      return n;
    } catch (e) {
      return null;
    }
  }

  function writeStore(w) {
    try {
      localStorage.setItem(KEY, String(w));
    } catch (e) {}
  }

  function clamp(w, total, splitW) {
    splitW = splitW || 8;
    var max = Math.min(MAX_W, total * 0.5);
    var byCal = total - CAL_MIN - splitW;
    if (isFinite(byCal)) max = Math.min(max, byCal);
    if (!isFinite(max) || max < MIN_W) max = MIN_W;
    var n = Number(w);
    if (!isFinite(n)) n = DEFAULT_W;
    if (n < MIN_W) return MIN_W;
    if (n > max) return max;
    return n;
  }

  function isLg() {
    try {
      return !!(root.matchMedia && root.matchMedia("(min-width: 1024px)").matches);
    } catch (e) {
      return true;
    }
  }

  function applyWidth(panel, splitter, w) {
    if (!panel) return;
    panel.style.width = w + "px";
    panel.style.flex = "0 0 " + w + "px";
    if (splitter) {
      splitter.setAttribute("aria-valuenow", String(Math.round(w)));
    }
  }

  function currentTotal(rootEl, splitter) {
    var rect = rootEl.getBoundingClientRect();
    var sw = splitter ? splitter.getBoundingClientRect().width : 8;
    return { total: rect.width, split: sw };
  }

  function bind() {
    var rootEl = document.getElementById("mnt_calendar");
    var panel = document.getElementById("mnt-day-panel");
    var splitter = document.getElementById("mnt-cal-splitter");
    if (!rootEl || !panel || !splitter || splitter.getAttribute("data-mnt-resize") === "1") return;
    splitter.setAttribute("data-mnt-resize", "1");

    function layout() {
      if (!isLg()) {
        panel.style.width = "";
        panel.style.flex = "";
        return;
      }
      var t = currentTotal(rootEl, splitter);
      var w = clamp(readStore() != null ? readStore() : DEFAULT_W, t.total, t.split);
      applyWidth(panel, splitter, w);
    }

    layout();
    try {
      root.addEventListener("resize", layout);
    } catch (e) {}

    var dragging = false;
    var startX = 0;
    var startW = 0;

    function setIframePointer(off) {
      try {
        document.querySelectorAll("iframe").forEach(function (f) {
          if (off) {
            if (!f.hasAttribute("data-mnt-pe")) f.setAttribute("data-mnt-pe", f.style.pointerEvents || "");
            f.style.pointerEvents = "none";
          } else {
            f.style.pointerEvents = f.getAttribute("data-mnt-pe") || "";
            f.removeAttribute("data-mnt-pe");
          }
        });
      } catch (e) {}
    }

    function onMove(ev) {
      if (!dragging) return;
      ev.preventDefault();
      var t = currentTotal(rootEl, splitter);
      var dx = startX - ev.clientX;
      var w = clamp(startW + dx, t.total, t.split);
      applyWidth(panel, splitter, w);
    }

    function onUp(ev) {
      if (!dragging) return;
      dragging = false;
      try {
        if (splitter.releasePointerCapture && ev && ev.pointerId != null) {
          splitter.releasePointerCapture(ev.pointerId);
        }
      } catch (e) {}
      document.body.classList.remove("select-none");
      setIframePointer(false);
      var t = currentTotal(rootEl, splitter);
      var w = panel.getBoundingClientRect().width;
      w = clamp(w, t.total, t.split);
      applyWidth(panel, splitter, w);
      writeStore(w);
    }

    splitter.addEventListener("pointerdown", function (ev) {
      if (!isLg() || ev.button) return;
      dragging = true;
      startX = ev.clientX;
      startW = panel.getBoundingClientRect().width;
      document.body.classList.add("select-none");
      setIframePointer(true);
      try {
        if (splitter.setPointerCapture) splitter.setPointerCapture(ev.pointerId);
      } catch (e) {}
      ev.preventDefault();
    });
    splitter.addEventListener("pointermove", onMove);
    splitter.addEventListener("pointerup", onUp);
    splitter.addEventListener("pointercancel", onUp);
    splitter.addEventListener("dblclick", function () {
      if (!isLg()) return;
      var t = currentTotal(rootEl, splitter);
      var w = clamp(DEFAULT_W, t.total, t.split);
      applyWidth(panel, splitter, w);
      writeStore(w);
    });
    splitter.addEventListener("keydown", function (ev) {
      if (!isLg()) return;
      var dir = 0;
      if (ev.key === "ArrowRight") dir = 1;
      if (ev.key === "ArrowLeft") dir = -1;
      if (!dir) return;
      ev.preventDefault();
      var step = ev.shiftKey ? STEP_SHIFT : STEP;
      var t = currentTotal(rootEl, splitter);
      var cur = panel.getBoundingClientRect().width;
      var w = clamp(cur + dir * step, t.total, t.split);
      applyWidth(panel, splitter, w);
      writeStore(w);
    });
  }

  if (typeof document !== "undefined") {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", bind);
    } else {
      bind();
    }
  }

  var api = {
    KEY: KEY,
    DEFAULT_W: DEFAULT_W,
    MIN_W: MIN_W,
    MAX_W: MAX_W,
    CAL_MIN: CAL_MIN,
    STEP: STEP,
    STEP_SHIFT: STEP_SHIFT,
    readStore: readStore,
    writeStore: writeStore,
    clamp: clamp,
    bind: bind
  };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  root.MntCalendarResize = api;
})(typeof window !== "undefined" ? window : globalThis);
