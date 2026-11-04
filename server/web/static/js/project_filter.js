function pfDebounce(fn, ms) {
  var t;
  return function () {
    var self = this, args = arguments;
    clearTimeout(t);
    t = setTimeout(function () { fn.apply(self, args); }, ms);
  };
}

function pfOwnMatches(names, q) {
  q = String(q || '').trim();
  if (!q) return names.slice();
  return names.filter(function (n) { return String(n).indexOf(q) >= 0; });
}

function pfShouldHintOther(ownHits, otherHits, q, moreOpen) {
  return !!(String(q || '').trim() && ownHits === 0 && otherHits > 0 && !moreOpen);
}

function pfEsc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function initProjectFilter(root) {
  if (!root || root._pfBound) return;
  root._pfBound = true;
  var panel = root.querySelector('[data-pf-panel]');
  var btn = root.querySelector('[data-pf-open]');
  var hidden = root.querySelector('input[name="project"]');
  var qEl = root.querySelector('[data-pf-q]');
  var moreBtn = root.querySelector('[data-pf-more]');
  var otherBox = root.querySelector('[data-pf-other]');
  var hintEl = root.querySelector('[data-pf-hint]');
  var form = root.closest('form');
  var allOrgs = root.getAttribute('data-all-orgs') === '1';
  var otherURL = root.getAttribute('data-other-url') || '/workboard/projects/other';
  var otherTotal = parseInt(root.getAttribute('data-other-count') || '0', 10) || 0;
  var state = { open: false, more: false, groups: null, loading: false, err: '', openGrp: {}, hi: -1 };

  function options() {
    if (!panel) return [];
    return Array.prototype.slice.call(panel.querySelectorAll('[role="option"]'));
  }

  function setOpen(on) {
    state.open = !!on;
    if (panel) panel.hidden = !state.open;
    if (btn) btn.setAttribute('aria-expanded', state.open ? 'true' : 'false');
    if (state.open) {
      if (allOrgs && !state.groups && !state.loading) loadOther();
      if (qEl) qEl.focus();
    } else {
      state.more = false;
      if (otherBox) otherBox.hidden = true;
      syncMoreLabel();
      if (btn) btn.focus();
    }
  }

  function submitSel(id) {
    if (hidden) hidden.value = id || '';
    setOpen(false);
    if (!form) return;
    var u = new URL(form.getAttribute('action') || window.location.pathname, window.location.href);
    var fd = new FormData(form);
    fd.set('project', id || '');
    var qs = new URLSearchParams(fd).toString();
    window.location.assign(u.pathname + (qs ? ('?' + qs) : ''));
  }

  function ownNames() {
    var out = [];
    root.querySelectorAll('[data-pf-own-list] [data-pf-name]').forEach(function (el) {
      out.push(el.getAttribute('data-pf-name') || '');
    });
    return out;
  }

  function filterOwn() {
    var q = qEl ? qEl.value.trim() : '';
    var n = 0;
    root.querySelectorAll('[data-pf-own-list] [data-pf-name]').forEach(function (el) {
      var it = el.closest('.pf-it') || el;
      var hit = !q || (el.getAttribute('data-pf-name') || '').indexOf(q) >= 0;
      it.hidden = !hit;
      if (hit) n += 1;
    });
    var allRow = root.querySelector('[data-pf-all]');
    if (allRow) allRow.hidden = !!q;
    var empty = root.querySelector('[data-pf-own-empty]');
    if (!empty && root.querySelector('[data-pf-own-list]')) {
      empty = document.createElement('p');
      empty.className = 'pf-empty';
      empty.setAttribute('data-pf-own-empty', '1');
      empty.hidden = true;
      root.querySelector('[data-pf-own-list]').appendChild(empty);
    }
    if (empty) {
      empty.hidden = !q || n > 0;
      empty.textContent = '내 조직 사업 중 「' + q + '」 검색 결과가 없습니다.';
    }
    return n;
  }

  function syncMoreLabel(filteredN) {
    if (!moreBtn) return;
    var n = (qEl && qEl.value.trim() && filteredN != null) ? filteredN : otherTotal;
    moreBtn.textContent = state.more ? '▲ 타 부서 사업 접기' : ('▼ 타 부서 사업 보기 (' + n + '건)');
  }

  function renderOther(groups, filteredTotal) {
    if (!otherBox) return;
    otherBox.hidden = !state.more;
    if (!state.more) return;
    if (state.loading) {
      otherBox.innerHTML = '<div class="pf-obox"><p class="pf-empty">불러오는 중…</p></div>';
      return;
    }
    if (state.err) {
      otherBox.innerHTML = '<div class="pf-obox"><p class="pf-empty">' + pfEsc(state.err) +
        '</p><div class="pf-foot"><button type="button" class="pf-more" data-pf-retry>다시 시도</button></div></div>';
      var rt = otherBox.querySelector('[data-pf-retry]');
      if (rt) rt.onclick = function () { loadOther(); };
      return;
    }
    var q = qEl ? qEl.value.trim() : '';
    var html = '<div class="pf-obox"><div class="pf-oh"><span>타 부서 사업 · ' + otherTotal +
      '건 (조회·필터용)</span><span class="pf-ox" data-pf-close-other>닫기</span></div>';
    if (!groups || !groups.length) {
      html += '<p class="pf-empty">일치하는 타 부서 사업이 없습니다.</p></div>';
      otherBox.innerHTML = html;
      bindOther();
      return;
    }
    groups.forEach(function (g) {
      var key = g.org_id || g.org_name || '';
      var open = !!state.openGrp[key] || !!q;
      html += '<div class="grp border-t border-line"><div class="pf-gh" data-g="' + pfEsc(key) + '"><span>' +
        pfEsc(g.org_name || '조직 미지정') + '</span><small class="text-ink-muted">' + (g.items || []).length +
        '건 ' + (open ? '▾' : '▸') + '</small></div>';
      if (open) {
        html += '<div class="pf-gl">';
        (g.items || []).forEach(function (it) {
          html += '<button type="button" class="pf-it" role="option" data-project-id="' + pfEsc(it.project_id) +
            '" data-org-name="' + pfEsc(g.org_name || '') + '"><span class="pf-chk"></span><span class="pf-org">' +
            pfEsc(g.org_name || '') + '</span><span class="pf-nm">' + pfEsc(it.name) + '</span></button>';
        });
        html += '</div>';
      }
      html += '</div>';
    });
    html += '</div>';
    otherBox.innerHTML = html;
    bindOther();
    void filteredTotal;
  }

  function bindOther() {
    if (!otherBox) return;
    var x = otherBox.querySelector('[data-pf-close-other]');
    if (x) x.onclick = function () { state.more = false; renderOther(state.groups); syncMoreLabel(); };
    otherBox.querySelectorAll('.pf-gh').forEach(function (g) {
      g.onclick = function () {
        var k = g.getAttribute('data-g');
        state.openGrp[k] = !state.openGrp[k];
        renderOther(state.groups);
      };
    });
    otherBox.querySelectorAll('.pf-it[data-project-id]').forEach(function (it) {
      it.onclick = function () { submitSel(it.getAttribute('data-project-id')); };
    });
  }

  function renderAllOrgs(groups) {
    var own = root.querySelector('[data-pf-own]');
    if (!own) return;
    var q = qEl ? qEl.value.trim() : '';
    var html = '<div class="pf-sech"><span>전체 조직 사업</span><small>모든 조직을 조직별로 묶어 보여 줍니다</small></div><div class="pf-sec">';
    if (state.loading) {
      own.innerHTML = '<p class="pf-empty">불러오는 중…</p>';
      return;
    }
    if (state.err) {
      own.innerHTML = '<p class="pf-empty">' + pfEsc(state.err) + '</p><div class="pf-foot"><button type="button" class="pf-more" data-pf-retry>다시 시도</button></div>';
      var rt = own.querySelector('[data-pf-retry]');
      if (rt) rt.onclick = function () { loadOther(); };
      return;
    }
    (groups || []).forEach(function (g) {
      var key = g.org_id || g.org_name || '';
      var open = state.openGrp[key] !== false;
      if (q) open = true;
      html += '<div class="border-t border-line"><div class="pf-gh" data-g="' + pfEsc(key) + '"><span>' +
        pfEsc(g.org_name || '조직 미지정') + '</span><small class="text-ink-muted">' + (g.items || []).length +
        '건 ' + (open ? '▾' : '▸') + '</small></div>';
      if (open) {
        html += '<div class="pf-gl">';
        (g.items || []).forEach(function (it) {
          html += '<button type="button" class="pf-it" role="option" data-project-id="' + pfEsc(it.project_id) +
            '"><span class="pf-chk"></span><span class="pf-org">' + pfEsc(g.org_name || '') +
            '</span><span class="pf-nm">' + pfEsc(it.name) + '</span></button>';
        });
        html += '</div>';
      }
      html += '</div>';
    });
    html += '</div>';
    own.innerHTML = html;
    own.querySelectorAll('.pf-gh').forEach(function (g) {
      g.onclick = function () {
        var k = g.getAttribute('data-g');
        state.openGrp[k] = state.openGrp[k] === false ? true : false;
        renderAllOrgs(state.groups);
      };
    });
    own.querySelectorAll('.pf-it[data-project-id]').forEach(function (it) {
      it.onclick = function () { submitSel(it.getAttribute('data-project-id')); };
    });
  }

  function loadOther() {
    state.loading = true;
    state.err = '';
    if (allOrgs) renderAllOrgs(state.groups);
    else if (state.more) renderOther(state.groups);
    var q = qEl ? encodeURIComponent(qEl.value.trim()) : '';
    var url = otherURL + (q ? ('?q=' + q) : '');
    fetch(url, { credentials: 'same-origin', headers: { 'Accept': 'application/json' } })
      .then(function (res) {
        if (!res.ok) throw new Error('목록을 불러오지 못했습니다.');
        return res.json();
      })
      .then(function (data) {
        state.loading = false;
        state.groups = (data && data.groups) || [];
        var tot = data && typeof data.total === 'number' ? data.total : 0;
        if (allOrgs) renderAllOrgs(state.groups);
        else {
          renderOther(state.groups, tot);
          applyHint(tot);
          syncMoreLabel(tot);
        }
      })
      .catch(function () {
        state.loading = false;
        state.err = '목록을 불러오지 못했습니다.';
        if (allOrgs) renderAllOrgs(null);
        else renderOther(null);
      });
  }

  function applyHint(otherHits) {
    if (!hintEl) return;
    var q = qEl ? qEl.value.trim() : '';
    var ownN = filterOwn();
    var show = pfShouldHintOther(ownN, otherHits, q, state.more);
    hintEl.hidden = !show;
    if (show) hintEl.textContent = '타 부서 사업 ' + otherHits + '건이 일치합니다. 위 버튼으로 확인하세요.';
  }

  function onSearch() {
    var ownN = filterOwn();
    if (allOrgs) {
      if (state.groups) renderAllOrgs(state.groups);
      return;
    }
    if (state.more) {
      loadOther();
      return;
    }
    if (qEl && qEl.value.trim() && ownN === 0 && otherTotal > 0) {
      var q = encodeURIComponent(qEl.value.trim());
      fetch(otherURL + '?q=' + q, { credentials: 'same-origin', headers: { 'Accept': 'application/json' } })
        .then(function (r) { return r.ok ? r.json() : { total: 0 }; })
        .then(function (data) { applyHint(data && data.total ? data.total : 0); })
        .catch(function () { applyHint(0); });
      return;
    }
    applyHint(0);
    syncMoreLabel();
  }

  if (btn) btn.addEventListener('click', function (ev) {
    ev.preventDefault();
    setOpen(!state.open);
  });
  if (moreBtn) moreBtn.addEventListener('click', function () {
    state.more = !state.more;
    if (state.more && !state.groups) loadOther();
    else renderOther(state.groups);
    syncMoreLabel();
    if (hintEl) hintEl.hidden = true;
  });
  root.querySelectorAll('[data-pf-all]').forEach(function (el) {
    el.addEventListener('click', function () { submitSel(''); });
  });
  root.querySelectorAll('[data-pf-own-list] [data-project-id]').forEach(function (el) {
    if (el.hasAttribute('data-pf-all')) return;
    el.addEventListener('click', function () { submitSel(el.getAttribute('data-project-id')); });
  });
  if (qEl) qEl.addEventListener('input', pfDebounce(onSearch, 150));

  document.addEventListener('click', function (ev) {
    if (!state.open) return;
    var t = ev.target;
    if (!t || !t.isConnected) return;
    if (root.contains(t)) return;
    setOpen(false);
  });
  document.addEventListener('keydown', function (ev) {
    if (!state.open) return;
    if (ev.key === 'Escape') {
      ev.preventDefault();
      setOpen(false);
      return;
    }
    var opts = options().filter(function (o) { return !o.hidden; });
    if (!opts.length) return;
    if (ev.key === 'ArrowDown') {
      ev.preventDefault();
      state.hi = Math.min(state.hi + 1, opts.length - 1);
      if (state.hi < 0) state.hi = 0;
      opts[state.hi].focus();
    } else if (ev.key === 'ArrowUp') {
      ev.preventDefault();
      state.hi = Math.max(state.hi - 1, 0);
      opts[state.hi].focus();
    } else if (ev.key === 'Enter' && document.activeElement && panel.contains(document.activeElement)) {
      ev.preventDefault();
      document.activeElement.click();
    }
  });
}

function bindProjectFilters(doc) {
  doc = doc || document;
  var list = doc.querySelectorAll('[data-project-filter]');
  for (var i = 0; i < list.length; i++) initProjectFilter(list[i]);
}

if (typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { bindProjectFilters(document); });
  } else {
    bindProjectFilters(document);
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    pfDebounce: pfDebounce,
    pfOwnMatches: pfOwnMatches,
    pfShouldHintOther: pfShouldHintOther,
    initProjectFilter: initProjectFilter,
    bindProjectFilters: bindProjectFilters
  };
}
