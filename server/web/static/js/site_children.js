// 95-196 사이트 등록 화면 「고객 하위 기관」 카드. 후보는 GET /projects/:id/sites/children,
// 등록은 기존 POST /projects/:id/sites/add-schools 를 그대로 쓴다(활성·중복·이름 충돌·2,000곳 상한 규칙 동일).
(function () {
  'use strict';
  function csrf() {
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') || '' : '';
  }
  // x-data='siteChildren({...})' 에서 바로 부르는 전역 함수(school_site_register.js 와 같은 방식).
  window.siteChildren = function (cfg) {
      return {
        projectId: cfg.projectId, canWrite: !!cfg.canWrite,
        items: [], chips: [], total: 0, unregistered: 0,
        picked: {}, step: 'list', busy: false, result: '', resultErr: false,
        init: function () {
          var self = this;
          fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/children', { credentials: 'same-origin', headers: { 'Accept': 'application/json' } })
            .then(function (r) { return r.ok ? r.json() : null; })
            .then(function (j) {
              if (!j) return;
              self.items = j.items || []; self.chips = j.chips || [];
              self.total = j.total || 0; self.unregistered = j.unregistered || 0;
              if (self.total > 0 && location.hash === '#children') {
                self.$nextTick(function () { if (self.$el.scrollIntoView) self.$el.scrollIntoView({ block: 'start' }); });
              }
            }).catch(function () {});
        },
        toggle: function (id, on) { if (on) this.picked[id] = true; else delete this.picked[id]; this.step = 'list'; },
        selectAll: function () {
          var p = {};
          this.items.forEach(function (it) { if (!it.registered) p[it.id] = true; });
          this.picked = p; this.step = 'list';
        },
        clearAll: function () { this.picked = {}; this.step = 'list'; },
        selectedCount: function () {
          var self = this;
          return this.items.filter(function (it) { return !it.registered && self.picked[it.id]; }).length;
        },
        registeredCount: function () { return this.items.filter(function (it) { return it.registered; }).length; },
        askConfirm: function () { if (this.selectedCount() > 0) { this.result = ''; this.step = 'confirm'; } },
        register: function () {
          var self = this, fd = new FormData(), n = 0;
          this.items.forEach(function (it) { if (!it.registered && self.picked[it.id]) { fd.append('customer_id', it.id); n++; } });
          if (!n || this.busy) return;
          this.busy = true;
          fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/add-schools', {
            method: 'POST', body: fd, credentials: 'same-origin',
            headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch', 'X-CSRF-Token': csrf() }
          }).then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }).catch(function () { return { ok: r.ok, j: {} }; }); })
            .then(function (res) {
              self.busy = false; self.step = 'list';
              if (!res.ok) { self.resultErr = true; self.result = (res.j && res.j.error) || '사이트를 등록하지 못했습니다'; return; }
              var added = res.j.added || 0, skipped = res.j.skipped || 0, ids = {};
              (res.j.addedIDs || []).forEach(function (id) { ids[id] = true; });
              self.items.forEach(function (it) { if (ids[it.id]) it.registered = true; });
              (res.j.skippedItems || []).forEach(function (s) { self.items.forEach(function (it) { if (it.id === s.customer_id) it.registered = true; }); });
              self.unregistered = self.items.filter(function (it) { return !it.registered; }).length;
              self.picked = {}; self.resultErr = false;
              self.result = added + '곳을 등록했습니다' + (skipped ? ' · ' + skipped + '곳은 건너뜀' : '');
              var t = document.querySelector('[data-site-total]');
              if (t && typeof res.j.siteTotal === 'number') { t.textContent = res.j.siteTotal; t.setAttribute('data-site-total', res.j.siteTotal); }
            }).catch(function () { self.busy = false; self.step = 'list'; self.resultErr = true; self.result = '사이트를 등록하지 못했습니다'; });
        }
      };
  };
})();
