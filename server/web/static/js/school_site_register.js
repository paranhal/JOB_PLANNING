(function (root) {
  var KINDS = [
    { id: 'school', label: '학교' },
    { id: 'public', label: '공공기관' },
    { id: 'library', label: '도서관' },
    { id: 'private', label: '민간업체' }
  ];

  function sidoList(regions) {
    return (regions || []).map(function (r) { return r.sido; });
  }

  function activeSido(st) {
    var all = sidoList(st.regions);
    if (st.sidoAll) return all;
    return all.filter(function (s) { return st.sidoSel[s]; });
  }

  function sigunKeys(st) {
    var o = [];
    activeSido(st).forEach(function (s) {
      var row = (st.regions || []).find(function (r) { return r.sido === s; });
      (row && row.sigungu ? row.sigungu : []).forEach(function (g) {
        o.push(s + '|' + g);
      });
    });
    return o;
  }

  function activeSigun(st) {
    var all = sigunKeys(st);
    if (st.sigunguAll) return all;
    return all.filter(function (k) { return st.sigunguSel[k]; });
  }

  function filtered(st) {
    var keys = activeSigun(st);
    var q = String(st.appliedQ || '').trim();
    return (st.items || []).filter(function (d) {
      if (keys.indexOf(d.sido + '|' + d.sigungu) < 0) return false;
      if (q && (d.name + d.sido + d.sigungu).indexOf(q) < 0) return false;
      return true;
    });
  }

  function resetChecked(st) {
    st.picked = {};
    filtered(st).forEach(function (d) {
      if (!d.registered) st.picked[d.id] = true;
    });
  }

  function toggleMap(allFlagName, selName, key, allKeys, st) {
    if (key === 'ALL') {
      st[selName] = {};
      st[allFlagName] = true;
      return;
    }
    st[allFlagName] = false;
    if (st[selName][key]) {
      delete st[selName][key];
    } else {
      st[selName][key] = true;
    }
    if (!allKeys.some(function (k) { return st[selName][k]; })) {
      st[allFlagName] = true;
      st[selName] = {};
    }
  }

  function syncRegionsToSearch(st, q) {
    q = String(q || '').trim();
    if (!q) {
      st.sidoAll = true;
      st.sidoSel = {};
      st.sigunguAll = true;
      st.sigunguSel = {};
      return;
    }
    var hit = (st.items || []).filter(function (d) {
      return (d.name + d.sido + d.sigungu).indexOf(q) >= 0;
    });
    if (!hit.length) return;
    var sd = {};
    var sg = {};
    hit.forEach(function (d) {
      sd[d.sido] = true;
      sg[d.sido + '|' + d.sigungu] = true;
    });
    var sdKeys = Object.keys(sd);
    if (sdKeys.length === sidoList(st.regions).length) {
      st.sidoAll = true;
      st.sidoSel = {};
    } else {
      st.sidoAll = false;
      st.sidoSel = sd;
    }
    var all = [];
    sdKeys.forEach(function (s) {
      var row = (st.regions || []).find(function (r) { return r.sido === s; });
      (row && row.sigungu ? row.sigungu : []).forEach(function (g) {
        all.push(s + '|' + g);
      });
    });
    var sgKeys = Object.keys(sg);
    if (sgKeys.length === all.length) {
      st.sigunguAll = true;
      st.sigunguSel = {};
    } else {
      st.sigunguAll = false;
      st.sigunguSel = sg;
    }
  }

  function csrf() {
    if (typeof document === 'undefined') return '';
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') || '' : '';
  }

  function schoolSiteRegister(cfg) {
    cfg = cfg || {};
    return {
      kinds: KINDS,
      type: cfg.type || 'school',
      projectId: cfg.projectId || cfg.projectID || '',
      canWrite: !!cfg.canWrite,
      siteTotal: cfg.siteTotal || 0,
      equipTotal: cfg.equipTotal || 0,
      q: '',
      appliedQ: '',
      sidoAll: true,
      sidoSel: {},
      sigunguAll: true,
      sigunguSel: {},
      regions: [],
      items: [],
      picked: {},
      loading: false,
      loadErr: '',
      busy: false,
      modalOpen: false,
      modalTitle: '',
      modalBody: '',
      modalFail: false,
      lastFocus: null,
      sidoList: function () { return sidoList(this); },
      sigunKeys: function () { return sigunKeys(this); },
      visible: function () { return filtered(this); },
      freeCount: function () {
        return this.visible().filter(function (d) { return !d.registered; }).length;
      },
      selectedCount: function () {
        var self = this;
        return this.visible().filter(function (d) { return !d.registered && self.picked[d.id]; }).length;
      },
      sumLine: function () {
        var sd = activeSido(this).join('·');
        var extra = this.sigunguAll ? '' : ' · ' + activeSigun(this).length + '개 시군';
        var vis = this.visible();
        var free = vis.filter(function (d) { return !d.registered; }).length;
        var reg = vis.length - free;
        return '지역: ' + (sd || '—') + extra + ' · 선택 ' + this.selectedCount() + ' / 등록 가능 ' + free + (reg ? ' (이미 등록 ' + reg + ')' : '');
      },
      soonLabel: function () {
        var self = this;
        var k = KINDS.find(function (x) { return x.id === self.type; });
        return k ? k.label : '';
      },
      setType: function (id) {
        this.type = id;
        if (typeof history !== 'undefined' && history.replaceState) {
          var u = new URL(window.location.href);
          u.searchParams.set('type', id);
          history.replaceState({}, '', u.toString());
        }
        if (id === 'school' && !this.items.length) this.load();
      },
      toggleSido: function (key) {
        toggleMap('sidoAll', 'sidoSel', key, sidoList(this), this);
        this.sigunguAll = true;
        this.sigunguSel = {};
        resetChecked(this);
        this.syncAllBox();
      },
      toggleSigungu: function (key) {
        toggleMap('sigunguAll', 'sigunguSel', key, sigunKeys(this), this);
        resetChecked(this);
        this.syncAllBox();
      },
      toggleOne: function (id, on) {
        this.picked[id] = !!on;
        this.syncAllBox();
      },
      toggleAll: function (on) {
        var self = this;
        this.visible().forEach(function (d) {
          if (!d.registered) self.picked[d.id] = !!on;
        });
        this.syncAllBox();
      },
      search: function () {
        this.appliedQ = this.q;
        syncRegionsToSearch(this, this.appliedQ);
        resetChecked(this);
        this.syncAllBox();
      },
      syncAllBox: function () {
        var el = this.$refs && this.$refs.allBox;
        if (!el) return;
        var free = this.freeCount();
        var n = this.selectedCount();
        el.checked = free > 0 && n === free;
        el.indeterminate = n > 0 && n < free;
      },
      load: function () {
        var self = this;
        if (!this.projectId || typeof fetch !== 'function') return;
        this.loading = true;
        this.loadErr = '';
        fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/candidates?type=school', {
          credentials: 'same-origin',
          headers: { 'Accept': 'application/json' }
        }).then(function (r) {
          if (!r.ok) throw new Error('후보를 불러오지 못했습니다');
          return r.json();
        }).then(function (data) {
          self.regions = data.regions || [];
          self.items = data.items || [];
          self.sidoAll = true;
          self.sidoSel = {};
          self.sigunguAll = true;
          self.sigunguSel = {};
          resetChecked(self);
          self.loading = false;
          if (self.$nextTick) self.$nextTick(function () { self.syncAllBox(); });
          else self.syncAllBox();
        }).catch(function (e) {
          self.loadErr = (e && e.message) || '후보를 불러오지 못했습니다';
          self.loading = false;
        });
      },
      register: function () {
        var self = this;
        if (!this.canWrite || this.busy) return;
        var ids = this.visible().filter(function (d) { return !d.registered && self.picked[d.id]; }).map(function (d) { return d.id; });
        if (!ids.length) return;
        this.lastFocus = (typeof document !== 'undefined') ? document.activeElement : null;
        this.busy = true;
        var fd = new FormData();
        ids.forEach(function (id) { fd.append('customer_id', id); });
        fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/add-schools', {
          method: 'POST',
          body: fd,
          credentials: 'same-origin',
          headers: {
            'Accept': 'application/json',
            'X-Requested-With': 'fetch',
            'X-CSRF-Token': csrf()
          }
        }).then(function (r) {
          return r.json().then(function (j) { return { ok: r.ok, j: j }; }).catch(function () {
            return { ok: r.ok, j: { error: '사이트를 등록하지 못했습니다' } };
          });
        }).then(function (res) {
          self.busy = false;
          if (!res.ok) {
            self.modalFail = true;
            self.modalTitle = '등록 실패';
            self.modalBody = '<p>' + self.esc((res.j && res.j.error) || '사이트를 등록하지 못했습니다') + '</p><p class="text-ui-sm text-ink-muted">선택한 학교와 필터는 그대로 유지됩니다.</p>';
            self.modalOpen = true;
            return;
          }
          var added = (res.j && res.j.added) || 0;
          var skipped = (res.j && res.j.skipped) || 0;
          if (typeof res.j.siteTotal === 'number') self.siteTotal = res.j.siteTotal;
          var set = {};
          ids.forEach(function (id) { set[id] = true; });
          self.items.forEach(function (d) {
            if (set[d.id]) d.registered = true;
          });
          self.modalFail = false;
          self.modalTitle = '등록 결과';
          self.modalBody = '<p>' + added + '곳을 등록했습니다.</p>' +
            (skipped ? '<p>' + skipped + '곳은 이미 등록되어 건너뛰었습니다.</p>' : '') +
            '<p class="text-ui-sm text-ink-muted">등록된 사이트 수: ' + self.siteTotal + '곳</p>';
          self.modalOpen = true;
          resetChecked(self);
          self.syncAllBox();
        }).catch(function () {
          self.busy = false;
          self.modalFail = true;
          self.modalTitle = '등록 실패';
          self.modalBody = '<p>사이트를 등록하지 못했습니다</p>';
          self.modalOpen = true;
        });
      },
      closeModal: function () {
        this.modalOpen = false;
        var t = this.lastFocus;
        var btn = this.$refs && this.$refs.regBtn;
        var q = this.$refs && this.$refs.q;
        if (this.modalFail) {
          if (t && t.focus && !t.disabled) t.focus();
          else if (q && q.focus) q.focus();
          return;
        }
        if (btn && !btn.disabled && btn.focus) btn.focus();
        else if (q && q.focus) q.focus();
      },
      esc: function (s) {
        return String(s || '').replace(/[&<>"]/g, function (c) {
          return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
        });
      },
      init: function () {
        if (this.type === 'school') this.load();
        var self = this;
        if (this.$watch) {
          this.$watch('picked', function () { self.syncAllBox(); });
        }
      }
    };
  }

  root.schoolSiteRegister = schoolSiteRegister;
  root.ssrFiltered = filtered;
  root.ssrResetChecked = resetChecked;
  root.ssrToggleMap = toggleMap;
  root.ssrSyncRegionsToSearch = syncRegionsToSearch;
  root.ssrSigunKeys = sigunKeys;
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      schoolSiteRegister: schoolSiteRegister,
      ssrFiltered: filtered,
      ssrResetChecked: resetChecked,
      ssrToggleMap: toggleMap,
      ssrSyncRegionsToSearch: syncRegionsToSearch,
      ssrSigunKeys: sigunKeys
    };
  }
})(typeof window !== 'undefined' ? window : global);
