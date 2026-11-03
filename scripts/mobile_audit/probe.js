(function () {
  function vis(el) {
    var st = window.getComputedStyle(el);
    if (st.display === "none" || st.visibility === "hidden" || st.opacity === "0") return false;
    var r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  }
  function overflowX(el) {
    var st = window.getComputedStyle(el);
    return st.overflowX === "auto" || st.overflowX === "scroll";
  }
  function inScrollParent(el) {
    var p = el.parentElement;
    while (p && p !== document.body) {
      if (overflowX(p) || (p.classList && (p.classList.contains("table-scroll") || p.classList.contains("overflow-x-auto")))) {
        return true;
      }
      p = p.parentElement;
    }
    return false;
  }
  var vw = window.innerWidth;
  var vh = window.innerHeight;
  var scrollW = Math.max(document.documentElement.scrollWidth, document.body ? document.body.scrollWidth : 0);
  var tables = [];
  document.querySelectorAll("table").forEach(function (t) {
    if (!vis(t)) return;
    var r = t.getBoundingClientRect();
    tables.push({
      w: Math.round(r.width),
      scrollParent: inScrollParent(t),
      clipped: r.width > vw + 2 && !inScrollParent(t)
    });
  });
  var targets = [];
  var sel = "a, button, input, select, textarea, [role='button'], [role='link']";
  document.querySelectorAll(sel).forEach(function (el) {
    if (!vis(el)) return;
    if (el.closest && el.closest(".col-resize-handle")) return;
    var r = el.getBoundingClientRect();
    var w = Math.round(r.width);
    var h = Math.round(r.height);
    targets.push({
      tag: (el.tagName || "").toLowerCase(),
      w: w,
      h: h,
      short: Math.min(w, h)
    });
  });
  var fixed = [];
  document.querySelectorAll("*").forEach(function (el) {
    var st = window.getComputedStyle(el);
    if (st.position !== "fixed" && st.position !== "sticky") return;
    if (!vis(el)) return;
    var r = el.getBoundingClientRect();
    if (r.width < 8 || r.height < 8) return;
    fixed.push({
      pos: st.position,
      t: Math.round(r.top),
      l: Math.round(r.left),
      b: Math.round(r.bottom),
      r: Math.round(r.right)
    });
  });
  return {
    innerWidth: vw,
    innerHeight: vh,
    scrollWidth: scrollW,
    tables: tables,
    targets: targets,
    fixed: fixed
  };
})();
