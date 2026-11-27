// 95-229 AS 접수 화면의 「사업」 입력란. 기관(customer_id)을 고르면 그 기관이 사이트로 등록된 사업을 불러온다.
// 후보가 1개면 자동으로 고르고, 여러 개면 직접 고르게 한다(비워 두면 범위 규칙으로 판정). 접수 때 연결한 사업이 조치·업무·보고로 이어진다.
(function (root) {
  'use strict';
  function asFormProject(cfg) {
    cfg = cfg || {};
    return {
      cands: [], selected: cfg.initial || '', loadedFor: '', loading: false, note: '', started: false,
      init: function () {
        var self = this;
        this.check();
        // 기관 선택 칸은 숨은 입력값만 바뀌므로(이벤트 없음) 가볍게 값을 살핀다.
        this._timer = setInterval(function () { self.check(); }, 700);
      },
      customerId: function () {
        var el = document.getElementById('as-customer_id');
        return el ? (el.value || '').trim() : '';
      },
      check: function () {
        var cid = this.customerId();
        if (cid === this.loadedFor) return;
        // 기관을 바꾸면 이전 기관에서 고른 사업은 비운다(처음 불러올 때의 초기값은 유지).
        if (this.started) this.selected = '';
        this.started = true;
        this.loadedFor = cid;
        this.cands = []; this.note = '';
        if (!cid) { this.selected = ''; return; }
        var self = this;
        this.loading = true;
        fetch('/workboard/projects/for-site?customer_id=' + encodeURIComponent(cid) + '&include=' + encodeURIComponent(this.selected || ''), {
          credentials: 'same-origin', headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' }
        }).then(function (r) { return r.ok ? r.json() : null; }).then(function (j) {
          self.loading = false;
          if (!j || cid !== self.loadedFor) return;
          self.cands = (j.site || []).map(function (p) { return { id: p.project_id, name: p.name }; });
          if (self.selected && !self.cands.some(function (c) { return c.id === self.selected; })) self.selected = '';
          if (!self.selected && self.cands.length === 1) { self.selected = self.cands[0].id; self.note = '이 기관이 등록된 사업이 1개라 자동으로 골랐습니다.'; }
          else if (self.cands.length > 1) self.note = '이 기관은 사업 ' + self.cands.length + '개에 등록돼 있습니다. 해당 사업을 고르세요(비워 두면 규칙으로 판정하고, 조치 화면에서 연결할 수 있습니다).';
          else self.note = '이 기관이 등록된 사업이 없습니다.';
        }).catch(function () { self.loading = false; });
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { asFormProject: asFormProject };
  root.asFormProject = asFormProject;
})(typeof window !== 'undefined' ? window : globalThis);
