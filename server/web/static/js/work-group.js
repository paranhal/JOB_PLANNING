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
    if (document.querySelector("table") && location.pathname.indexOf("/work-groups") === 0) {
      location.reload();
    }
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

  function bindFormExtras(root) {
    if (!root) return;
    var q = root.querySelector("[data-wg-filter]");
    if (q) {
      q.addEventListener("input", function () {
        var v = (q.value || "").toLowerCase();
        root.querySelectorAll("[data-wg-cand]").forEach(function (el) {
          var n = (el.getAttribute("data-name") || "").toLowerCase();
          el.style.display = !v || n.indexOf(v) >= 0 ? "" : "none";
        });
      });
    }
    var lead = root.querySelector("[data-wg-leader]");
    if (lead) {
      function syncLeader() {
        var keep = lead.value;
        lead.innerHTML = '<option value="">없음</option>';
        root.querySelectorAll("[data-wg-member]:checked").forEach(function (cb) {
          var o = document.createElement("option");
          o.value = cb.value;
          o.textContent = cb.getAttribute("data-label") || cb.value;
          lead.appendChild(o);
        });
        if (keep) lead.value = keep;
      }
      root.addEventListener("change", function (e) {
        if (e.target && e.target.getAttribute("data-wg-member") != null) syncLeader();
      });
      syncLeader();
    }
  }

  function putHTML(html) {
    var h = host();
    if (!h) return;
    h.innerHTML = html;
    document.body.classList.add("overflow-hidden");
    bindFormExtras(h);
    var st = h.querySelector("[data-wg-status]");
    if (st && st.focus) st.focus();
    else {
      var closeBtn = h.querySelector("[data-close-work-group]");
      if (closeBtn) closeBtn.focus();
    }
  }

  function openFrom(btn) {
    opener = btn;
    var href = btn.getAttribute("data-href");
    if (!href) return;
    fetch(href, { headers: { "HX-Request": "true", "Accept": "text/html" } }).then(function (res) {
      if (res.status === 403 || res.status === 404) throw new Error("denied");
      return res.text();
    }).then(putHTML).catch(function () {
      alert("열 수 없습니다");
    });
  }

  function submitFetch(form) {
    var action = form.getAttribute("action") || location.href;
    var body = new FormData(form);
    fetch(action, {
      method: (form.getAttribute("method") || "POST").toUpperCase(),
      body: body,
      headers: { "X-Requested-With": "fetch", "Accept": "text/html" },
      redirect: "follow"
    }).then(function (res) {
      if (res.status === 403 || res.status === 404) throw new Error("denied");
      var loc = res.headers.get("Location") || "";
      if (res.redirected || (res.status >= 300 && res.status < 400 && loc)) {
        window.location.href = loc || res.url;
        return "";
      }
      return res.text();
    }).then(function (html) {
      if (html) putHTML(html);
    }).catch(function () {
      form.removeAttribute("data-wg-busy");
      form.submit();
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

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form || !form.closest || !form.closest("#work-group-host")) return;
    if (!form.matches("[data-wg-form]")) return;
    e.preventDefault();
    submitFetch(form);
  });

  document.addEventListener("keydown", trap);
})();
