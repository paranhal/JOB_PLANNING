(function () {
  var opener = null;
  var fallback = null;

  function host() {
    return document.getElementById("sales-next-host");
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
    var rowLink = fallback;
    opener = null;
    fallback = null;
    if (back && document.contains(back) && back.focus) {
      back.focus();
      return;
    }
    if (rowLink && document.contains(rowLink) && rowLink.focus) rowLink.focus();
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

  function showErrors(root, payload) {
    var general = root.querySelector("[data-sales-dialog-err]");
    root.querySelectorAll("[data-field-error]").forEach(function (n) {
      n.hidden = true;
      n.textContent = "";
    });
    var msg = (payload && payload.error) || "처리할 수 없습니다";
    var field = payload && payload.field;
    if (field) {
      var slot = root.querySelector('[data-field-error="' + field + '"]');
      if (slot) {
        slot.textContent = msg;
        slot.hidden = false;
        var inp = root.querySelector('[data-field="' + field + '"]');
        if (inp && inp.focus) inp.focus();
        return;
      }
    }
    if (general) {
      general.textContent = msg;
      general.hidden = false;
    }
  }

  function openFrom(btn) {
    opener = btn;
    var row = btn.closest("tr");
    fallback = row ? row.querySelector("a[data-sales-row-link]") : null;
    var href = btn.getAttribute("data-href");
    if (!href) return;
    fetch(href, { headers: { "HX-Request": "true", "Accept": "text/html" } }).then(function (res) {
      if (res.status === 403) throw new Error("forbidden");
      if (!res.ok) throw new Error("fail");
      return res.text();
    }).then(function (html) {
      var h = host();
      if (!h) return;
      h.innerHTML = html;
      document.body.classList.add("overflow-hidden");
      var closeBtn = h.querySelector("[data-close-sales-dialog]");
      if (closeBtn) closeBtn.focus();
    }).catch(function () {
      alert("처리할 수 없습니다");
    });
  }

  document.addEventListener("click", function (e) {
    var close = e.target.closest("[data-close-sales-dialog]");
    if (close) {
      e.preventDefault();
      closeModal();
      return;
    }
    var h = host();
    var root = h && h.querySelector("[data-sales-dialog-root]");
    if (root && e.target === root) {
      e.preventDefault();
      closeModal();
      return;
    }
    var btn = e.target.closest("[data-sales-next]");
    if (!btn) return;
    e.preventDefault();
    openFrom(btn);
  });

  document.addEventListener("keydown", trap);

  document.addEventListener("submit", function (e) {
    var form = e.target.closest("[data-sales-next-form]");
    if (!form) return;
    e.preventDefault();
    var root = form.closest("[data-sales-dialog-root]") || form;
    fetch(form.action, {
      method: "POST",
      body: new FormData(form),
      headers: { "Accept": "application/json", "X-Requested-With": "XMLHttpRequest" }
    }).then(function (res) {
      return res.json().then(function (body) {
        return { res: res, body: body };
      }).catch(function () {
        return { res: res, body: { error: "처리할 수 없습니다" } };
      });
    }).then(function (pack) {
      if (pack.res.status === 403) {
        showErrors(root, { error: "권한이 없습니다" });
        return;
      }
      if (!pack.res.ok || (pack.body && pack.body.ok === false)) {
        showErrors(root, pack.body || {});
        return;
      }
      if (pack.body && pack.body.next) {
        window.location.href = pack.body.next;
        return;
      }
      window.location.reload();
    }).catch(function () {
      showErrors(root, { error: "처리할 수 없습니다" });
    });
  });
})();
