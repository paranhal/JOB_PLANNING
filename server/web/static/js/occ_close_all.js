// 95-200: 「업무 모두 종료」 인라인 모달. 건수는 열 때 preview JSON 으로 불러온다(표 전체에 미리 계산해 넣지 않는다).
(function () {
  var opener = null;
  var LABELS = { scheduled: "예정", in_progress: "진행중", deferred: "연기", hold: "보류", overdue: "미완료" };
  var ORDER = ["scheduled", "in_progress", "deferred", "hold", "overdue"];

  function modal() { return document.getElementById("occ-close-modal"); }
  function q(sel) { var m = modal(); return m ? m.querySelector(sel) : null; }

  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'a[href], button:not([disabled]), textarea, input:not([disabled]):not([type="hidden"]), select, [tabindex]:not([tabindex="-1"])'
    )).filter(function (n) { return n.offsetParent !== null || n === document.activeElement; });
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

  // 최종 결과 칸: require_result 이고 비어 있을 때만, 그리고 상위 업무를 완료하는 경우(이전 미완료를 남기지 않을 때)만 필요하다.
  function applyFinalBox() {
    var box = q("[data-final-box]");
    if (!box) return;
    var need = box.getAttribute("data-need-final") === "1";
    var earlier = parseInt((q("[data-earlier]") || {}).textContent || "0", 10) || 0;
    var cb = q("[data-include-earlier]");
    var willComplete = !(earlier > 0 && cb && !cb.checked);
    box.hidden = !(need && willComplete);
  }

  function syncSubmit() {
    applyFinalBox();
    var btn = q("[data-occ-close-submit]");
    var reason = q("[data-close-reason]");
    var finalBox = q("[data-final-box]");
    var finalIn = q("[data-final-result]");
    var ok = !!(reason && reason.value.trim());
    if (finalBox && !finalBox.hidden && !(finalIn && finalIn.value.trim())) ok = false;
    if (btn) btn.disabled = !ok;
  }

  function syncEarlierNote() {
    var earlier = parseInt((q("[data-earlier]") || {}).textContent || "0", 10) || 0;
    var cb = q("[data-include-earlier]");
    var note = q("[data-earlier-note]");
    if (note) note.hidden = !(earlier > 0 && cb && !cb.checked);
  }

  function showError(msg) {
    var e = q("[data-occ-close-err]");
    if (e) { e.textContent = msg; e.hidden = false; }
    var bar = q("[data-occ-close-errbar]");
    if (bar) bar.hidden = false;
    var f = q("[data-occ-close-form]");
    if (f) f.hidden = true;
    var l = q("[data-occ-close-loading]");
    if (l) l.hidden = true;
  }

  function fill(pv) {
    var m = modal();
    m.querySelector("[data-occ-title]").textContent = pv.occTitle || "";
    m.querySelector("[data-occ-date]").textContent = pv.occDate ? "(" + pv.occDate + ")" : "";
    m.querySelector("[data-later]").textContent = String(pv.later || 0);
    var by = pv.laterByStatus || {};
    var parts = [];
    ORDER.forEach(function (k) { if (by[k]) parts.push(LABELS[k] + " " + by[k]); });
    m.querySelector("[data-later-detail]").textContent = parts.length ? "(" + parts.join(" · ") + ")" : "";
    var earlierLi = m.querySelector("[data-sum-earlier]");
    m.querySelector("[data-earlier]").textContent = String(pv.earlier || 0);
    earlierLi.hidden = !(pv.earlier > 0);
    var cb = m.querySelector("[data-include-earlier]");
    if (cb) cb.checked = true; // 기본값: 함께 제외
    var fb = m.querySelector("[data-final-box]");
    fb.setAttribute("data-need-final", pv.needFinalResult ? "1" : "0");
    m.querySelector("[data-close-reason]").value = "";
    m.querySelector("[data-final-result]").value = "";
    m.querySelector("[data-earlier-note]").hidden = true;
    m.querySelector("[data-occ-close-loading]").hidden = true;
    m.querySelector("[data-occ-close-form]").hidden = false;
    syncSubmit();
    m.querySelector("[data-close-reason]").focus();
  }

  function open(btn) {
    var m = modal();
    if (!m) return;
    var parent = btn.getAttribute("data-parent"), occ = btn.getAttribute("data-occ");
    var base = "/workboard/tasks/" + encodeURIComponent(parent) + "/occurrences/" + encodeURIComponent(occ) + "/close-all";
    opener = btn;
    q("[data-occ-close-form]").action = base;
    q("[data-occ-close-form]").hidden = true;
    q("[data-occ-close-err]").hidden = true;
    q("[data-occ-close-errbar]").hidden = true;
    q("[data-occ-close-loading]").hidden = false;
    var sub = q("[data-occ-close-submit]");
    if (sub) { sub.disabled = true; sub.removeAttribute("aria-disabled"); }
    m.hidden = false;
    document.body.classList.add("overflow-hidden");
    var cancel = q("[data-occ-close-cancel]");
    if (cancel) cancel.focus();
    fetch(base + "/preview", { credentials: "same-origin", headers: { Accept: "application/json" } })
      .then(function (r) {
        if (r.status === 409) { showError("이미 종료되었거나 보관된 업무입니다."); return null; }
        if (!r.ok) { showError("건수를 불러오지 못했습니다. 새로고침 후 다시 시도하세요."); return null; }
        return r.json();
      })
      .then(function (pv) { if (pv) fill(pv); })
      .catch(function () { showError("건수를 불러오지 못했습니다. 새로고침 후 다시 시도하세요."); });
  }

  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-occ-close-all]");
    if (b) { e.preventDefault(); open(b); return; }
    if (e.target.closest("[data-occ-close-cancel]")) { e.preventDefault(); closeModal(); return; }
    var m = modal();
    if (m && !m.hidden && e.target === m) { e.preventDefault(); closeModal(); }
  });

  document.addEventListener("input", function (e) {
    if (e.target.matches("[data-close-reason], [data-final-result]")) syncSubmit();
  });
  document.addEventListener("change", function (e) {
    if (e.target.matches("[data-include-earlier]")) { syncEarlierNote(); syncSubmit(); }
  });

  document.addEventListener("submit", function (e) {
    if (!e.target.matches("[data-occ-close-form]")) return;
    var btn = q("[data-occ-close-submit]");
    // 서버 검증이 기준이다. 더블 클릭만 막는다.
    if (btn && btn.getAttribute("aria-disabled") === "true") { e.preventDefault(); return; }
    if (btn) btn.setAttribute("aria-disabled", "true");
  });

  document.addEventListener("keydown", function (e) {
    var m = modal();
    if (!m || m.hidden) return;
    if (e.key === "Escape") { e.preventDefault(); closeModal(); return; }
    if (e.key !== "Tab") return;
    var dlg = m.querySelector('[role="dialog"]');
    var nodes = focusables(dlg || m);
    if (!nodes.length) return;
    var first = nodes[0], last = nodes[nodes.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });
})();
