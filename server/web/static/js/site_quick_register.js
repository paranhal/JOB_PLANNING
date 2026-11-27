// 95-224 §2 업무 등록 화면 「사이트 간이 등록」 팝업.
// 사업에 등록된 사이트가 0곳이면 안내와 [사이트 등록] 버튼을 보이고, 같은 페이지의 모달에서 이 사업의 고객에 연결된
// 하위 기관을 검색·선택해 [사업 등록]하면 기존 POST /projects/:id/sites/add-schools 로 등록한다.
// 페이지를 떠나지 않으므로 작성 중이던 업무 입력은 그대로 남는다. [취소]는 아무것도 저장하지 않는다.
(function (root) {
  'use strict';
  function csrf() {
    var m = document.querySelector('meta[name="csrf-token"]');
    return m ? m.getAttribute('content') || '' : '';
  }
  function hdr() {
    return { 'Accept': 'application/json', 'X-Requested-With': 'fetch', 'X-CSRF-Token': csrf() };
  }
  function siteQuickRegister() {
    return {
      projectId: '', siteTotal: -1, canRegister: false, canRequestOnce: false,
      items: [], chips: [], q: '', industry: '', picked: {},
      modalOpen: false, busy: false, err: '', unlockPw: '', ok: '',
      onProject: function (id) {
        var self = this;
        this.projectId = id || '';
        this.siteTotal = -1; this.items = []; this.chips = []; this.picked = {}; this.err = ''; this.ok = '';
        if (!this.projectId) return Promise.resolve();
        return fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/quick', { credentials: 'same-origin', headers: hdr() })
          .then(function (r) { return r.ok ? r.json() : null; })
          .then(function (j) {
            if (!j || j.projectId !== self.projectId) return;
            self.siteTotal = j.siteTotal;
            self.canRegister = !!j.canRegister;
            self.canRequestOnce = !!j.canRequestOnce;
            self.items = ((j.children && j.children.items) || []).filter(function (it) { return !it.registered; });
            self.chips = (j.children && j.children.chips) || [];
          })
          .catch(function () { /* 안내만 못 보일 뿐 업무 등록은 계속된다 */ });
      },
      needNotice: function () { return !!this.projectId && this.siteTotal === 0; },
      visible: function () {
        var q = (this.q || '').trim().toLowerCase(), ind = this.industry;
        return this.items.filter(function (it) {
          if (ind && (it.industry || '(분류 없음)') !== ind) return false;
          if (!q) return true;
          return (it.name + ' ' + (it.sido || '') + ' ' + (it.sigungu || '')).toLowerCase().indexOf(q) >= 0;
        });
      },
      toggle: function (id, on) {
        var next = Object.assign({}, this.picked);
        if (on) next[id] = true; else delete next[id];
        this.picked = next;
      },
      pickedCount: function () {
        var self = this;
        return this.items.filter(function (it) { return self.picked[it.id]; }).length;
      },
      openModal: function () { this.err = ''; this.ok = ''; this.modalOpen = true; },
      cancel: function () { this.modalOpen = false; this.picked = {}; this.q = ''; this.industry = ''; this.err = ''; this.unlockPw = ''; },
      requestOnce: function () {
        var self = this, fd = new FormData();
        fd.append('unlock_password', this.unlockPw);
        this.busy = true; this.err = '';
        return fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/unlock', { method: 'POST', body: fd, credentials: 'same-origin', headers: hdr() })
          .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }).catch(function () { return { ok: r.ok, j: {} }; }); })
          .then(function (res) {
            self.busy = false; self.unlockPw = '';
            if (!res.ok) { self.err = (res.j && res.j.error) || '1회 등록 요청에 실패했습니다'; return; }
            self.canRegister = true; self.canRequestOnce = false; self.ok = '1회 등록 권한이 부여되었습니다(30분, 1회 사용)';
          }).catch(function () { self.busy = false; self.err = '1회 등록 요청에 실패했습니다'; });
      },
      register: function () {
        var self = this, fd = new FormData(), chosen = [];
        this.items.forEach(function (it) { if (self.picked[it.id]) { fd.append('customer_id', it.id); chosen.push(it); } });
        if (!chosen.length || this.busy) return Promise.resolve();
        this.busy = true; this.err = '';
        return fetch('/projects/' + encodeURIComponent(this.projectId) + '/sites/add-schools', { method: 'POST', body: fd, credentials: 'same-origin', headers: hdr() })
          .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }).catch(function () { return { ok: r.ok, j: {} }; }); })
          .then(function (res) {
            self.busy = false;
            if (!res.ok) { self.err = (res.j && res.j.error) || '사이트를 등록하지 못했습니다'; return; }
            var added = {};
            ((res.j && res.j.addedIDs) || []).forEach(function (id) { added[id] = true; });
            var linked = chosen.filter(function (it) { return added[it.id]; }).map(function (it) {
              return { site_id: it.id, site_name: it.name, on_project: true };
            });
            self.siteTotal = (res.j && res.j.siteTotal) || linked.length;
            self.canRegister = false; // 1회 권한은 사용 후 소멸(정식 권한자는 다음 호출에서 다시 true)
            self.items = self.items.filter(function (it) { return !added[it.id]; });
            self.cancel();
            // 방금 등록한 사이트를 업무 폼의 대상 사이트에 자동 연결한다(부모 폼이 이 이벤트를 받는다).
            if (typeof self.$dispatch === 'function') self.$dispatch('site-quick-added', { sites: linked });
            self.onProject(self.projectId);
          }).catch(function () { self.busy = false; self.err = '사이트를 등록하지 못했습니다'; });
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { siteQuickRegister: siteQuickRegister };
  root.siteQuickRegister = siteQuickRegister;
})(typeof window !== 'undefined' ? window : globalThis);
