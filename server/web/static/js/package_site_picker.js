// 95-182 작업·행정/지원 등록 화면 공통 사이트 선택.
// 사업을 고르면 수행 구조(하부 업무) 여부를 확인하고, 있으면 하부 업무를 반드시 고르게 한다.
// 사이트 후보는 그 하부 업무에 등록된 사이트뿐이고, 사업 사이트 등록 화면처럼 시도·시군 체크와 검색, 카드 다중 선택을 쓴다.
// 부모(siteMultiPicker)의 sites 배열에 「담기」하면 기존 site_id 숨은 입력과 칩이 그대로 쓰인다.
(function (root) {
  function uniq(a) {
    var seen = {};
    return a.filter(function (x) { if (seen[x]) return false; seen[x] = true; return true; });
  }

  function packageSitePicker() {
    return {
      pkgHas: false,
      pkgList: [],
      pkgId: '',
      pkgProject: '',
      pkgErr: '',
      pkgLoading: false,
      items: [],
      type: 'school',
      types: [{ id: 'school', label: '학교' }, { id: 'public', label: '공공기관' }, { id: 'library', label: '도서관' }, { id: 'private', label: '민간업체' }],
      sidoAll: true, sidoSel: {},
      sigunguAll: true, sigunguSel: {},
      q: '', appliedQ: '',
      picked: {},
      note: '',

      // 패널이 나중에 만들어졌을 때(x-if) 이미 고른 사업이 있으면 바로 읽는다.
      init: function () {
        if (this.projectId) this.onProject(this.projectId);
      },

      onProject: function (id) {
        var self = this;
        id = id || '';
        this.pkgProject = id;
        this.pkgId = '';
        this.pkgHas = false;
        this.pkgList = [];
        this.pkgErr = '';
        this.resetSites();
        this.pkgMode = false;
        if (!id) return Promise.resolve();
        return fetch('/workboard/project-packages?project_id=' + encodeURIComponent(id), { credentials: 'same-origin' })
          .then(function (r) { return r.ok ? r.json() : { has: false, packages: [] }; })
          .then(function (j) {
            if (self.pkgProject !== id) return;
            self.pkgHas = !!j.has;
            self.pkgList = j.packages || [];
            if (self.pkgHas) self.pkgMode = true;
            else self.pkgMode = false;
          })
          .catch(function () { self.pkgHas = false; self.pkgMode = false; });
      },

      resetSites: function () {
        this.items = []; this.picked = {}; this.type = 'school';
        this.sidoAll = true; this.sidoSel = {}; this.sigunguAll = true; this.sigunguSel = {};
        this.q = ''; this.appliedQ = ''; this.note = '';
      },

      onPackage: function () {
        var self = this, pid = this.pkgProject, pk = this.pkgId;
        this.resetSites();
        this.pkgErr = '';
        if (!pk) { this.dropForeign([]); return Promise.resolve(); }
        this.pkgLoading = true;
        return fetch('/workboard/package-sites?project_id=' + encodeURIComponent(pid) + '&package_id=' + encodeURIComponent(pk), { credentials: 'same-origin' })
          .then(function (r) { if (!r.ok) throw new Error('load'); return r.json(); })
          .then(function (j) {
            if (self.pkgId !== pk) return;
            self.items = j.items || [];
            self.type = self.firstType();
            self.dropForeign(self.items.map(function (it) { return it.id; }));
          })
          .catch(function () { self.pkgErr = '사이트 목록을 불러오지 못했습니다'; })
          .then(function () { self.pkgLoading = false; });
      },

      // 하부 업무를 바꾸면, 이미 담아 둔 사이트 중 새 하부 업무에 없는 곳은 뺀다.
      dropForeign: function (allowedIds) {
        var ok = {};
        allowedIds.forEach(function (id) { ok[id] = true; });
        var before = (this.sites || []).length;
        this.sites = (this.sites || []).filter(function (s) { return !!ok[s.site_id]; });
        if (before !== this.sites.length) {
          this.note = '선택한 하부 업무에 없는 사이트 ' + (before - this.sites.length) + '곳을 뺐습니다';
        }
      },

      typeItems: function () {
        var t = this.type;
        return this.items.filter(function (d) { return d.type === t; });
      },
      typeCount: function (id) {
        return this.items.filter(function (d) { return d.type === id; }).length;
      },
      firstType: function () {
        var self = this;
        var t = this.types.find(function (x) { return self.typeCount(x.id) > 0; });
        return t ? t.id : 'school';
      },
      setType: function (id) {
        this.type = id; this.picked = {};
        this.sidoAll = true; this.sidoSel = {}; this.sigunguAll = true; this.sigunguSel = {};
      },
      sidoNames: function () {
        var o = [];
        this.typeItems().forEach(function (d) { if (o.indexOf(d.sido) < 0) o.push(d.sido); });
        return o;
      },
      activeSido: function () {
        var self = this, all = this.sidoNames();
        return this.sidoAll ? all : all.filter(function (s) { return self.sidoSel[s]; });
      },
      sigunKeys: function () {
        var out = [], act = this.activeSido();
        this.typeItems().forEach(function (d) {
          var k = d.sido + '|' + d.sigungu;
          if (act.indexOf(d.sido) >= 0 && out.indexOf(k) < 0) out.push(k);
        });
        return out;
      },
      activeSigun: function () {
        var self = this, all = this.sigunKeys();
        return this.sigunguAll ? all : all.filter(function (k) { return self.sigunguSel[k]; });
      },
      toggleSido: function (key) {
        if (key === 'ALL') { this.sidoAll = true; this.sidoSel = {}; }
        else {
          this.sidoAll = false;
          if (this.sidoSel[key]) delete this.sidoSel[key]; else this.sidoSel[key] = true;
          if (!this.sidoNames().some(function (s) { return this.sidoSel[s]; }, this)) { this.sidoAll = true; this.sidoSel = {}; }
        }
        this.sigunguAll = true; this.sigunguSel = {};
      },
      toggleSigungu: function (key) {
        if (key === 'ALL') { this.sigunguAll = true; this.sigunguSel = {}; return; }
        this.sigunguAll = false;
        if (this.sigunguSel[key]) delete this.sigunguSel[key]; else this.sigunguSel[key] = true;
        if (!this.sigunKeys().some(function (k) { return this.sigunguSel[k]; }, this)) { this.sigunguAll = true; this.sigunguSel = {}; }
      },
      search: function () { this.appliedQ = String(this.q || '').trim(); },

      visible: function () {
        var keys = this.activeSigun(), q = this.appliedQ;
        return this.typeItems().filter(function (d) {
          if (keys.indexOf(d.sido + '|' + d.sigungu) < 0) return false;
          if (q && (d.name + d.sido + d.sigungu).indexOf(q) < 0) return false;
          return true;
        });
      },
      inCart: function (id) {
        return (this.sites || []).some(function (s) { return s.site_id === id; });
      },
      freeVisible: function () {
        var self = this;
        return this.visible().filter(function (d) { return !self.inCart(d.id); });
      },
      pickedCount: function () {
        var self = this;
        return this.freeVisible().filter(function (d) { return self.picked[d.id]; }).length;
      },
      toggleOne: function (id, on) { if (on) this.picked[id] = true; else delete this.picked[id]; },
      toggleAll: function (on) {
        var self = this;
        this.picked = {};
        if (on) this.freeVisible().forEach(function (d) { self.picked[d.id] = true; });
      },

      // 고른 사이트를 대상 사이트로 담는다(최대 maxSites).
      addPicked: function () {
        var self = this, max = this.maxSites || 100;
        var add = this.freeVisible().filter(function (d) { return self.picked[d.id]; });
        if (!add.length) return;
        var room = max - (this.sites || []).length, skipped = 0;
        if (add.length > room) { skipped = add.length - room; add = add.slice(0, Math.max(room, 0)); }
        this.sites = (this.sites || []).concat(add.map(function (d) {
          return { site_id: d.id, site_name: d.name, region: (d.sido + ' ' + d.sigungu).trim() };
        }));
        this.picked = {};
        this.note = add.length + '곳을 담았습니다' + (skipped ? ' · 최대 ' + max + '곳이라 ' + skipped + '곳은 담지 못했습니다' : '');
        if (typeof this.$dispatch === 'function') {
          this.$dispatch('sites-changed', { ids: uniq((this.sites || []).map(function (s) { return s.site_id; })) });
        }
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { packageSitePicker: packageSitePicker };
  root.packageSitePicker = packageSitePicker;
})(typeof window !== 'undefined' ? window : globalThis);
