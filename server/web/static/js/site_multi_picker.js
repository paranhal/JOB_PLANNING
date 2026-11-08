(function (root) {
  function siteMultiPicker(opts) {
    opts = opts || {};
    return {
      sites: opts.sites || [],
      q: '',
      cands: [],
      hi: 0,
      hint: opts.hint || '학교·기관명을 2글자 이상 입력',
      projectId: opts.projectId || '',
      searchUrl: opts.searchUrl || '/workboard/work-sites/search',
      maxSites: opts.max || 20,
      extraKey: opts.extraKey || '',
      idName: opts.idName || 'site_id',
      extraName: opts.extraName || '',
      excludeId: opts.excludeId || '',
      removeSite: function (id) {
        this.sites = (this.sites || []).filter(function (s) { return s.site_id !== id; });
        if (typeof this.$dispatch === 'function') {
          this.$dispatch('sites-changed', { ids: (this.sites || []).map(function (s) { return s.site_id; }).filter(Boolean) });
        }
      },
      move: function (d) {
        var n = (this.cands || []).length;
        if (!n) { this.hi = 0; return; }
        this.hi = (this.hi + d + n) % n;
      },
      closeCands: function () {
        this.cands = [];
        this.hi = 0;
      },
      pickHighlight: function () {
        var list = this.cands || [];
        if (!list.length) return;
        var i = this.hi;
        if (i < 0 || i >= list.length) i = 0;
        this.pick(list[i]);
      },
      onEnter: function (ev) {
        if (ev && ev.preventDefault) ev.preventDefault();
        this.pickHighlight();
      },
      focusSearch: function () {
        var self = this;
        var go = function () {
          var el = self.$refs && self.$refs.siteQ;
          if (el && el.focus) el.focus();
        };
        if (this.$nextTick) this.$nextTick(go);
        else go();
      },
      pick: function (s) {
        if (!s || !s.site_id) return;
        if (this.excludeId && s.site_id === this.excludeId) return;
        if ((this.sites || []).find(function (x) { return x.site_id === s.site_id; })) return;
        if ((this.sites || []).length >= this.maxSites) {
          this.hint = '사이트는 최대 ' + this.maxSites + '곳입니다';
          return;
        }
        var row = Object.assign({}, s);
        if (this.extraKey && row[this.extraKey] == null) row[this.extraKey] = '';
        this.sites = (this.sites || []).concat([row]);
        this.q = '';
        this.cands = [];
        this.hi = 0;
        this.focusSearch();
        if (typeof this.$dispatch === 'function') {
          this.$dispatch('sites-changed', { ids: (this.sites || []).map(function (x) { return x.site_id; }).filter(Boolean) });
        }
      },
      refreshBadges: function () {
        var ids = (this.sites || []).map(function (s) { return s.site_id; }).filter(Boolean);
        if (!ids.length) return Promise.resolve();
        var url = this.searchUrl + (this.searchUrl.indexOf('?') >= 0 ? '&' : '?') +
          'project_id=' + encodeURIComponent(this.projectId || '') +
          '&selected=' + encodeURIComponent(ids.join(','));
        var self = this;
        if (typeof fetch !== 'function') return Promise.resolve();
        return fetch(url, { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (j) {
          var hit = {};
          (j || []).forEach(function (h) { hit[h.site_id] = !!h.on_project; });
          self.sites = (self.sites || []).map(function (s) {
            return Object.assign({}, s, { on_project: !!hit[s.site_id] });
          });
        }).catch(function () {});
      },
      search: function () {
        var q = (this.q || '').trim();
        if (q.length < 2) {
          this.cands = [];
          this.hi = 0;
          this.hint = '학교·기관명을 2글자 이상 입력';
          return Promise.resolve();
        }
        var url = this.searchUrl + (this.searchUrl.indexOf('?') >= 0 ? '&' : '?') +
          'q=' + encodeURIComponent(q) +
          '&project_id=' + encodeURIComponent(this.projectId || '');
        if (this.excludeId) url += '&exclude=' + encodeURIComponent(this.excludeId);
        var self = this;
        var taken = (this.sites || []).map(function (s) { return s.site_id; });
        if (typeof fetch !== 'function') return Promise.resolve();
        return fetch(url, { credentials: 'same-origin' }).then(function (r) { return r.json(); }).then(function (j) {
          self.cands = (j || []).filter(function (s) {
            return taken.indexOf(s.site_id) < 0 && s.site_id !== self.excludeId;
          });
          self.hi = 0;
          self.hint = self.cands.length ? '' : '해당하는 사이트가 없습니다';
        }).catch(function () {
          self.cands = [];
          self.hint = '';
        });
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { siteMultiPicker: siteMultiPicker };
  }
  root.siteMultiPicker = siteMultiPicker;
})(typeof window !== 'undefined' ? window : globalThis);
