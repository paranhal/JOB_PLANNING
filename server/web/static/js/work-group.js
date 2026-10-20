(function () {
  var opener = null;

  function host() {
    return document.getElementById("work-group-host");
  }

  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
    )).filter(function (n) { return n.offsetParent !== null || n === document.activeElement; });
  }

  function closeModal() {
    var h = host();
    if (h) h.innerHTML = "";
    document.body.classList.remove("overflow-hidden");
    var back = opener;
    opener = null;
    if (back && back.focus) back.focus();
  }

  function trap(e) {
    var h = host();
    if (!h || !h.querySelector('[role="dialog"]')) return;
    if (e.key === "Escape") {
      e.preventDefault();
      closeModal();
      return;
    }
    if (e.key !== "Tab") return;
    var dlg = h.querySelector('[role="dialog"]');
    var nodes = focusables(dlg);
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

  function openFrom(btn) {
    opener = btn;
    var href = btn.getAttribute("data-href");
    if (!href) return;
    fetch(href, { headers: { "HX-Request": "true", "Accept": "text/html" } }).then(function (res) {
      if (res.status === 403 || res.status === 404) throw new Error("denied");
      return res.text();
    }).then(function (html) {
      var h = host();
      if (!h) return;
      h.innerHTML = html;
      document.body.classList.add("overflow-hidden");
      var closeBtn = h.querySelector("[data-close-work-group]");
      if (closeBtn) closeBtn.focus();
    }).catch(function () {
      alert("열 수 없습니다");
    });
  }

  document.addEventListener("click", function (e) {
    var close = e.target.closest("[data-close-work-group]");
    if (close) {
      e.preventDefault();
      closeModal();
      return;
    }
    var h = host();
    var root = h && h.querySelector("[data-work-group-dialog-root]");
    if (root && e.target === root) {
      e.preventDefault();
      closeModal();
      return;
    }
    var btn = e.target.closest("[data-work-group-edit]");
    if (!btn) return;
    e.preventDefault();
    openFrom(btn);
  });

  document.addEventListener("keydown", trap);
})();
