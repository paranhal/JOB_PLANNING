(function (root) {
  var dragging = false;
  var skipClick = false;
  var dragVisit = null;
  var fromDate = "";
  var fromCell = null;
  var chipEl = null;
  var toastTimer = null;

  function csrfHeaders() {
    var tok = "";
    try {
      var m = document.querySelector('meta[name="csrf-token"]');
      if (m) tok = m.getAttribute("content") || "";
    } catch (e) {}
    if (!tok) {
      try {
        var m = document.cookie.match(/(?:^|; )jp_csrf=([^;]*)/);
        if (m) tok = decodeURIComponent(m[1]);
      } catch (e2) {}
    }
    return {
      Accept: "application/json",
      "X-Requested-With": "fetch",
      "Content-Type": "application/x-www-form-urlencoded",
      "X-CSRF-Token": tok
    };
  }

  function isMobileWidth() {
    try {
      return !!(root.matchMedia && root.matchMedia("(max-width: 1023px)").matches);
    } catch (e) {
      return false;
    }
  }

  function canStartDrag(chip) {
    if (!chip) return false;
    if (chip.getAttribute("data-locked") === "1") return false;
    if (chip.getAttribute("draggable") !== "true") return false;
    if (isMobileWidth()) return false;
    return true;
  }

  function sameDate(a, b) {
    return String(a || "") === String(b || "");
  }

  function shouldPost(from, to) {
    return !sameDate(from, to);
  }

  function cellOfDate(date) {
    if (!date) return null;
    return document.querySelector('#mnt-cal-grid .mnt-cal-cell[data-date="' + date + '"]');
  }

  function chipHost(cell) {
    if (!cell) return null;
    return cell.querySelector(".mnt-cal-chips") || cell;
  }

  function applyOptimistic(chip, destCell) {
    if (!chip || !destCell) return;
    var host = chipHost(destCell);
    if (host) host.insertBefore(chip, host.firstChild);
    chip.setAttribute("data-date", destCell.getAttribute("data-date") || "");
  }

  function revertChip(chip, srcCell) {
    if (!chip || !srcCell) return;
    var host = chipHost(srcCell);
    if (host) host.appendChild(chip);
    chip.setAttribute("data-date", srcCell.getAttribute("data-date") || "");
  }

  function showErr(msg) {
    var rootEl = document.getElementById("mnt_calendar");
    if (!rootEl) return;
    var el = document.getElementById("mnt-cal-dnd-err");
    if (!el) {
      el = document.createElement("p");
      el.id = "mnt-cal-dnd-err";
      el.className = "mnt-cal-dnd-err px-3";
      rootEl.appendChild(el);
    }
    el.hidden = false;
    el.textContent = msg || "옮기지 못했습니다";
  }

  function clearErr() {
    var el = document.getElementById("mnt-cal-dnd-err");
    if (el) {
      el.textContent = "";
      el.hidden = true;
    }
  }

  function alpineState() {
    var el = document.getElementById("mnt_calendar");
    var wrap = el && el.closest("[x-data]");
    if (wrap && wrap._x_dataStack && wrap._x_dataStack[0]) return wrap._x_dataStack[0];
    return null;
  }

  function moveAlpineCard(visitId, from, to) {
    var st = alpineState();
    if (!st || !st.dayVisits) return;
    var src = st.dayVisits[from] || [];
    var idx = -1;
    var card = null;
    for (var i = 0; i < src.length; i++) {
      if (src[i] && src[i].visit_id === visitId) {
        idx = i;
        card = src[i];
        break;
      }
    }
    if (idx < 0 || !card) return;
    src.splice(idx, 1);
    st.dayVisits[from] = src;
    card.date = to;
    st.dayVisits[to] = (st.dayVisits[to] || []).concat([card]);
  }

  function formatKo(date) {
    var p = String(date || "").split("-");
    if (p.length < 3) return date;
    return Number(p[1]) + "월 " + Number(p[2]) + "일";
  }

  function showToast(from, to, visitId) {
    var host = document.getElementById("mnt-cal-toast-host");
    if (!host) {
      host = document.createElement("div");
      host.id = "mnt-cal-toast-host";
      host.className = "fixed bottom-4 left-1/2 z-40 -translate-x-1/2";
      document.body.appendChild(host);
    }
    host.innerHTML = "";
    var box = document.createElement("div");
    box.className = "mnt-cal-toast";
    box.setAttribute("role", "status");
    var msg = document.createElement("span");
    msg.textContent = formatKo(to) + "로 옮겼습니다 · ";
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "text-primary underline";
    btn.textContent = "되돌리기";
    btn.addEventListener("click", function () {
      var chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
      var dest = cellOfDate(from);
      if (chip && dest) applyOptimistic(chip, dest);
      moveAlpineCard(visitId, to, from);
      postMove(visitId, from, to, true);
      host.innerHTML = "";
    });
    box.appendChild(msg);
    box.appendChild(btn);
    host.appendChild(box);
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      host.innerHTML = "";
    }, 8000);
  }

  function postMove(visitId, toDate, fromDateNow, isUndo) {
    var body = "date=" + encodeURIComponent(toDate);
    return fetch("/maintenance/visits/" + encodeURIComponent(visitId) + "/move", {
      method: "POST",
      headers: csrfHeaders(),
      credentials: "same-origin",
      body: body
    }).then(function (res) {
      return res.json().then(function (j) {
        return { ok: res.ok && j && j.ok, json: j, status: res.status };
      }).catch(function () {
        return { ok: false, json: { error: "응답을 읽지 못했습니다" }, status: res.status };
      });
    }).then(function (r) {
      if (!r.ok) {
        var chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
        var src = cellOfDate(fromDateNow);
        if (chip && src) revertChip(chip, src);
        moveAlpineCard(visitId, toDate, fromDateNow);
        showErr((r.json && (r.json.error || r.json.code)) || "옮기지 못했습니다");
        return r;
      }
      clearErr();
      var chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
      if (chip) chip.setAttribute("data-date", toDate);
      if (!isUndo) {
        var warns = (r.json && r.json.warnings) || [];
        if (warns.length) showErr(warns.join(" · "));
        showToast(fromDateNow, toDate, visitId);
      }
      return r;
    }).catch(function () {
      var chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
      var src = cellOfDate(fromDateNow);
      if (chip && src) revertChip(chip, src);
      showErr("옮기지 못했습니다");
      return { ok: false };
    });
  }

  function onDragStart(e) {
    var chip = e.target && e.target.closest ? e.target.closest("[data-visit-id]") : null;
    if (!canStartDrag(chip)) {
      e.preventDefault();
      return;
    }
    dragging = true;
    skipClick = false;
    dragVisit = chip.getAttribute("data-visit-id");
    fromDate = chip.getAttribute("data-date") || "";
    fromCell = chip.closest(".mnt-cal-cell");
    chipEl = chip;
    try {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragVisit);
    } catch (err) {}
  }

  function onDragEnd() {
    dragging = false;
    skipClick = true;
    setTimeout(function () { skipClick = false; }, 0);
    document.querySelectorAll(".mnt-cal-drop").forEach(function (n) {
      n.classList.remove("mnt-cal-drop");
    });
  }

  function cellFromEvent(e) {
    var t = e.target;
    if (!t || !t.closest) return null;
    return t.closest("#mnt-cal-grid .mnt-cal-cell[data-date]");
  }

  function onDragOver(e) {
    if (!dragging) return;
    var cell = cellFromEvent(e);
    document.querySelectorAll(".mnt-cal-drop").forEach(function (n) {
      if (n !== cell) n.classList.remove("mnt-cal-drop");
    });
    if (!cell) {
      e.dataTransfer && (e.dataTransfer.dropEffect = "none");
      return;
    }
    e.preventDefault();
    cell.classList.add("mnt-cal-drop");
    try { e.dataTransfer.dropEffect = "move"; } catch (err) {}
  }

  function commitDrop(visitId, to, from) {
    if (!shouldPost(from, to)) return Promise.resolve({ skipped: true });
    var chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
    var cell = cellOfDate(to);
    if (!chip || !cell) return Promise.resolve({ ok: false });
    if (chip.getAttribute("data-locked") === "1" || chip.getAttribute("draggable") !== "true") {
      return Promise.resolve({ skipped: true, locked: true });
    }
    applyOptimistic(chip, cell);
    moveAlpineCard(visitId, from, to);
    return postMove(visitId, to, from, false);
  }

  function onDrop(e) {
    var cell = cellFromEvent(e);
    if (!cell || !dragVisit) return;
    e.preventDefault();
    var to = cell.getAttribute("data-date") || "";
    var from = fromDate;
    var id = dragVisit;
    commitDrop(id, to, from);
  }

  function onClickCapture(e) {
    if (!skipClick) return;
    var chip = e.target && e.target.closest && e.target.closest("[data-visit-id]");
    if (chip) {
      e.preventDefault();
      e.stopPropagation();
    }
  }

  function bind() {
    var rootEl = document.getElementById("mnt_calendar");
    if (!rootEl || rootEl.getAttribute("data-mnt-dnd") === "1") return;
    rootEl.setAttribute("data-mnt-dnd", "1");
    if (isMobileWidth()) {
      rootEl.querySelectorAll("[draggable='true']").forEach(function (n) {
        n.removeAttribute("draggable");
      });
      return;
    }
    rootEl.addEventListener("dragstart", onDragStart);
    rootEl.addEventListener("dragend", onDragEnd);
    rootEl.addEventListener("dragover", onDragOver);
    rootEl.addEventListener("drop", onDrop);
    rootEl.addEventListener("click", onClickCapture, true);
  }

  if (typeof document !== "undefined") {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", bind);
    } else {
      bind();
    }
  }

  var api = {
    canStartDrag: canStartDrag,
    sameDate: sameDate,
    shouldPost: shouldPost,
    commitDrop: commitDrop,
    applyOptimistic: applyOptimistic,
    revertChip: revertChip,
    isMobileWidth: isMobileWidth,
    bind: bind,
    csrfHeaders: csrfHeaders
  };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  root.MntCalendarDnd = api;
})(typeof window !== "undefined" ? window : globalThis);
