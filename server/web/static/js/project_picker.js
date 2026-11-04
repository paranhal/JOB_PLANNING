function projectPicker(cfg) {
  cfg = cfg || {};
  var own = cfg.Own || cfg.own || [];
  return {
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
      var p = this.filtered[this.hi];
      if (p) this.pick(p.project_id, p.name, p.project_kind);
    },
    sitesPanelHtml: '',
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
}
if (typeof module !== 'undefined') {
  module.exports = { projectPicker: projectPicker };
}
