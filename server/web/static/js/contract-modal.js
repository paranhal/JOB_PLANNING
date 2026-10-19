(function () {
  var opener = null;
  var openerId = "";

  function host() {
    var el = document.getElementById("contract-modal-host");
    if (!el) {
      el = document.createElement("div");
      el.id = "contract-modal-host";
      document.body.appendChild(el);
    }
    return el;
  }

  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
    )).filter(function (n) { return n.offsetParent !== null || n === document.activeElement; });
  }

  function closeModal() {
    var h = host();
    h.innerHTML = "";
    document.body.classList.remove("overflow-hidden");
    var back = opener;
    if (!back && openerId) {
      back = document.querySelector('[data-contract-modal][data-id="' + openerId + '"]');
    }
    opener = null;
    openerId = "";
    if (back && back.focus) back.focus();
  }

  function trap(e) {
    if (e.key === "Escape") {
      e.preventDefault();
      closeModal();
      return;
    }
    if (e.key !== "Tab") return;
    var dlg = host().querySelector('[role="dialog"]');
    if (!dlg) return;
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

  function openFrom(link) {
    opener = link;
    openerId = link.getAttribute("data-id") || "";
    var href = link.getAttribute("href");
    if (!href) return;
    var url = href + (href.indexOf("?") >= 0 ? "&" : "?") + "modal=1";
    fetch(url, { headers: { "HX-Request": "true" } }).then(function (res) {
      return res.text();
    }).then(function (html) {
      var h = host();
      h.innerHTML = html;
      document.body.classList.add("overflow-hidden");
      var closeBtn = h.querySelector("[data-close-contract-modal]");
      if (closeBtn) closeBtn.focus();
    }).catch(function () {
      window.location.href = href;
    });
  }

  document.addEventListener("click", function (e) {
    var close = e.target.closest("[data-close-contract-modal]");
    if (close) {
      e.preventDefault();
      closeModal();
      return;
    }
    var root = host().querySelector("[data-contract-dialog-root]");
    if (root && e.target === root) {
      e.preventDefault();
      closeModal();
      return;
    }
    var link = e.target.closest("[data-contract-modal]");
    if (!link) return;
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    openFrom(link);
  });

  document.addEventListener("keydown", function (e) {
    if (!host().querySelector('[role="dialog"]')) return;
    trap(e);
  });
})();
