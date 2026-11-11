function attachCombo(api) {
  if (typeof window !== 'undefined' && window.jpCombo && window.jpCombo.bind) {
    return window.jpCombo.bind(api);
  }
  if (typeof module !== 'undefined' && typeof require === 'function') {
    try {
      return require('./combobox_dismiss.js').bind(api);
    } catch (e) { /* node 테스트에서 같은 폴더 */ }
  }
  api.comboClose = function () { this.open = false; };
  api.comboOpen = function () { this.open = true; };
  api.comboToggleField = function () {
    if (this.open) this.comboClose();
    else {
      this.comboOpen();
      if (typeof this.revealList === 'function') this.revealList();
    }
  };
  api.comboOnTab = function () { if (this.open) this.comboClose(); };
  api.comboOnArrow = function (d) {
    if (!this.open) this.comboOpen();
    if (typeof this.move === 'function') this.move(d);
  };
  return api;
}

function projectPicker(cfg) {
  cfg = cfg || {};
  var own = cfg.Own || cfg.own || [];
  var api = {
    q: '',
    open: false,
    hi: 0,
    selectedId: cfg.Selected || cfg.selected || '',
    selectedName: cfg.SelectedName || cfg.selectedName || '',
    emptyLabel: cfg.EmptyLabel || cfg.emptyLabel || '— 선택 (해당 시) —',
    items: own,
    pending: !!cfg.Pending || !!cfg.pending,
    canOther: !!cfg.CanPickOther || !!cfg.canOther,
    otherCount: cfg.OtherCount || cfg.otherCount || 0,
    otherURL: cfg.OtherURL || cfg.otherURL || '/workboard/projects/picker-other',
    includeClosed: !!(cfg.IncludeClosed || cfg.includeClosed),
    showSiteNote: !!(cfg.ShowSiteNote || cfg.showSiteNote),
    siteOnly: !!(cfg.SiteOnly || cfg.siteOnly),
    showAllSiteRows: !!(cfg.ShowAllSiteRows || cfg.showAllSiteRows),
    siteCount: (cfg.SiteCount != null ? cfg.SiteCount : cfg.siteCount),
    siteItems: cfg.Site || cfg.site || [],
    siteListOpen: false,
    searchHits: [],
    confirmOpen: false,
    pendingProject: null,
    linkErr: '',
    emptyListHint: cfg.EmptyListHint || cfg.emptyListHint || '',
    confirmLead: cfg.ConfirmLead || cfg.confirmLead || '',
    dispatchSelected: !!(cfg.DispatchSelected || cfg.dispatchSelected),
    preferYear: cfg.PreferYear || cfg.preferYear || 0,
    get siteNote() {
      if (!this.showSiteNote) return '';
      var n = this.siteCount;
      if (n == null || n === '') n = (this.siteItems || []).length;
      n = Number(n);
      if (!n) {
        if (this.emptyListHint) return this.emptyListHint;
        if (this.siteOnly) return '연결된 사업이 없습니다. 사업명을 검색해 이 거래처를 사이트로 등록하세요';
        return '이 사이트가 등록된 사업이 없습니다. 사업 → 사이트 등록에서 이 사이트를 먼저 등록하세요';
      }
      return '이 사이트가 등록된 사업 ' + n + '건';
    },
    get siteRows() {
      if (this.siteOnly && this.items && this.items.length) return this.items;
      var rows = this.siteItems || [];
      if (rows.length) return rows;
      var n = Number(this.siteCount);
      if (this.showSiteNote && n > 0) return (this.items || []).slice(0, n);
      return [];
    },
    get siteRowsVisible() {
      var rows = this.siteRows;
      if (this.showAllSiteRows || this.siteOnly) return rows;
      if (rows.length > 10 && !this.siteListOpen) return rows.slice(0, 5);
      return rows;
    },
    get siteMore() {
      if (this.showAllSiteRows || this.siteOnly) return false;
      return this.siteRows.length > 10 && !this.siteListOpen;
    },
    otherOpen: false,
    otherGroups: [],
    otherLoading: false,
    otherErr: '',
    otherDenied: false,
    get filtered() {
      var q = String(this.q || '').trim().toLowerCase();
      if (!q) return this.items;
      return this.items.filter(function (p) {
        return String(p.name || '').toLowerCase().indexOf(q) >= 0;
      });
    },
    move: function (d) {
      var n = this.filtered.length;
      if (!n) { this.hi = 0; return; }
      this.hi = (this.hi + d + n) % n;
    },
    revealList: function () {
      var self = this;
      if (this.$nextTick) {
        this.$nextTick(function () { self._scrollList(); });
      } else {
        self._scrollList();
      }
    },
    _scrollList: function () {
      var el = this.$refs && this.$refs.list;
      if (el && el.scrollIntoView) el.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    },
    pickHighlight: function () {
      var list = (this.siteOnly && !this.siteRows.length) ? this.searchHits : this.filtered;
      var p = list[this.hi];
      if (!p) return;
      if (this.siteOnly && !this.siteRows.length) this.pickHit(p);
      else this.pick(p.project_id, p.name, p.project_kind);
    },
    linkCustomerIds: function () {
      var root = this.$el && this.$el.closest && this.$el.closest('[data-task-edit-projects]');
      var st = root && root._x_dataStack && root._x_dataStack[0];
      if (st && typeof st.linkIds === 'function') return st.linkIds();
      var form = this.$el && this.$el.closest && this.$el.closest('form');
      var el = form && form.querySelector('[name="customer_id"]');
      if (el && String(el.value || '').trim()) return [String(el.value).trim()];
      return [];
    },
    remoteSearch: function () {
      var q = String(this.q || '').trim();
      var self = this;
      if (!q || Array.from(q).length < 2) {
        this.searchHits = [];
        this.comboOpen();
        return;
      }
      if (typeof fetch !== 'function') return;
      var url = '/workboard/projects/search?q=' + encodeURIComponent(q);
      if (this.preferYear) url += '&year=' + this.preferYear;
      fetch(url, {
        credentials: 'same-origin', headers: { Accept: 'application/json' }
      }).then(function (r) { return r.json(); }).then(function (j) {
        self.searchHits = j.items || [];
        self.comboOpen();
        self.hi = 0;
      }).catch(function () { self.searchHits = []; });
    },
    pickHit: function (p) {
      if (!p) return;
      this.pendingProject = p;
      this.confirmOpen = true;
      this.linkErr = '';
      this.open = false;
    },
    cancelLink: function () {
      this.confirmOpen = false;
      this.pendingProject = null;
      this.linkErr = '';
    },
    confirmLink: function () {
      var p = this.pendingProject;
      if (!p) return;
      var ids = this.linkCustomerIds();
      var body = new URLSearchParams();
      body.set('confirm', '1');
      body.set('project_id', p.project_id);
      ids.forEach(function (id) { if (id) body.append('customer_id', id); });
      var self = this;
      if (typeof fetch !== 'function') return;
      fetch('/workboard/projects/link-site', {
        method: 'POST', credentials: 'same-origin',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
        body: body.toString()
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t || '등록에 실패했습니다'); });
        return r.json();
      }).then(function () {
        self.confirmOpen = false;
        self.pendingProject = null;
        self.pick(p.project_id, p.name, p.project_kind);
        self.reloadFromSite();
      }).catch(function (err) {
        self.linkErr = (err && err.message) ? err.message : '등록에 실패했습니다';
      });
    },
    reloadFromSite: function () {
      var ids = this.linkCustomerIds();
      var include = this.selectedId || '';
      var self = this;
      if (typeof fetch !== 'function') return;
      fetch('/workboard/projects/for-site?customer_id=' + encodeURIComponent(ids.join(',')) + '&include=' + encodeURIComponent(include), {
        credentials: 'same-origin', headers: { Accept: 'application/json' }
      }).then(function (r) { return r.json(); }).then(function (j) {
        self.siteItems = j.site || j.own || [];
        self.siteCount = j.count || 0;
        self.items = j.own || [];
      }).catch(function () {});
    },
    sitesPanelHtml: '',
    announceSelected: function () {
      if (!this.selectedId || !this.dispatchSelected || typeof this.$dispatch !== 'function') return;
      var pk = '';
      var rows = this.items || [];
      for (var i = 0; i < rows.length; i++) {
        if (rows[i].project_id === this.selectedId) {
          pk = rows[i].project_kind || '';
          break;
        }
      }
      this.$dispatch('project-picked', { id: this.selectedId, name: this.selectedName || '', kind: pk });
    },
    pick: function (id, name, kind) {
      this.selectedId = id || '';
      this.selectedName = name || '';
      this.open = false;
      this.q = '';
      if (typeof this.$dispatch === 'function') {
        this.$dispatch('project-picked', { id: this.selectedId, name: name || '', kind: kind || '' });
      }
      this.loadSitesPanel();
      var self = this;
      if (this.$nextTick) {
        this.$nextTick(function () {
          var el = self.$el && self.$el.querySelector('[data-picked-name]');
          if (el && el.scrollIntoView) el.scrollIntoView({ block: 'nearest' });
        });
      }
    },
    loadSitesPanel: function () {
      var id = this.selectedId || '';
      if (!id) {
        this.sitesPanelHtml = '';
        return;
      }
      var self = this;
      if (typeof document === 'undefined' || typeof fetch !== 'function') return;
      fetch('/projects/' + encodeURIComponent(id) + '/sites-panel', {
        credentials: 'same-origin',
        headers: { 'HX-Request': 'true', 'Accept': 'text/html' }
      }).then(function (r) {
        if (!r.ok) {
          self.sitesPanelHtml = '';
          return '';
        }
        return r.text();
      }).then(function (html) {
        self.sitesPanelHtml = html || '';
      }).catch(function () {
        self.sitesPanelHtml = '';
      });
    },
    clear: function () {
      this.pick('', '', '');
    },
    setPending: function (on) {
      this.pending = !!on;
      if (this.pending) this.clear();
    },
    denyOther: function () {
      this.otherDenied = true;
      this.otherOpen = false;
    },
    toggleOther: function () {
      if (!this.canOther) {
        this.denyOther();
        return;
      }
      this.otherDenied = false;
      this.otherOpen = !this.otherOpen;
      if (this.otherOpen && !this.otherGroups.length) this.loadOther();
    },
    loadOther: function () {
      var self = this;
      self.otherLoading = true;
      self.otherErr = '';
      var u = self.otherURL + (self.otherURL.indexOf('?') >= 0 ? '&' : '?') + 'q=' + encodeURIComponent(self.q || '');
      if (self.includeClosed) u += '&closed=1';
      fetch(u, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
        .then(function (r) {
          if (r.status === 403) {
            self.otherDenied = true;
            self.otherOpen = false;
            return null;
          }
          return r.json();
        })
        .then(function (j) {
          self.otherLoading = false;
          if (!j) return;
          self.otherGroups = j.groups || [];
          if (typeof j.total === 'number') self.otherCount = j.total;
        })
        .catch(function () {
          self.otherLoading = false;
          self.otherErr = '타 부서 사업을 불러오지 못했습니다';
        });
    }
  };
  return attachCombo(api);
}
function taskEditSiteProjects(opts) {
  opts = opts || {};
  return {
    customerId: opts.customerId || '',
    siteIds: opts.siteIds || [],
    include: opts.include || '',
    linkIds: function () {
      var ids = [this.customerId].concat(this.siteIds || []);
      var seen = {};
      var out = [];
      ids.forEach(function (id) {
        id = String(id || '').trim();
        if (!id || seen[id]) return;
        seen[id] = true;
        out.push(id);
      });
      return out;
    },
    applyPicker: function (j) {
      var el = this.$el && this.$el.querySelector('[data-project-picker]');
      var st = el && el._x_dataStack && el._x_dataStack[0];
      if (!st) return;
      st.siteItems = (j && (j.site || j.own)) || [];
      st.siteCount = (j && j.count) || 0;
      st.items = (j && j.own) || [];
      st.showSiteNote = true;
      st.siteOnly = true;
      st.showAllSiteRows = true;
    },
    reload: function () {
      var ids = this.linkIds();
      var self = this;
      if (typeof fetch !== 'function') return;
      fetch('/workboard/projects/for-site?customer_id=' + encodeURIComponent(ids.join(',')) + '&include=' + encodeURIComponent(this.include || ''), {
        credentials: 'same-origin', headers: { Accept: 'application/json' }
      }).then(function (r) { return r.json(); }).then(function (j) { self.applyPicker(j); }).catch(function () {});
    },
    onCustomer: function (d) {
      this.customerId = (d && d.id) || '';
      this.reload();
    },
    onSites: function (ids) {
      this.siteIds = ids || [];
      this.reload();
    }
  };
}
if (typeof module !== 'undefined') {
  module.exports = { projectPicker: projectPicker, taskEditSiteProjects: taskEditSiteProjects };
}
