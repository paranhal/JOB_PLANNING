// 95-225 작업 등록 폼의 「배정」 줄. 하부 업무(이 업무에 고른 하부 업무) 또는 사이트(대상 사이트에 담은 곳)를 다른 사용자에게 배정한다.
// 서버로는 assign_type[] · assign_target[] · assign_to[] 로 보낸다. 상대가 수락해야 상대 업무에 등록된다.
(function (root) {
  'use strict';
  function assignRows() {
    return {
      rows: [], users: [], usersLoaded: false,
      form: function () { return this.$el.closest('form'); },
      pkg: function () {
        var f = this.form(); if (!f) return null;
        var inp = f.querySelector('input[name="package_id"]'), sel = f.querySelector('[data-package-select]');
        if (!inp || !inp.value) return null;
        var name = sel && sel.selectedOptions && sel.selectedOptions[0] ? sel.selectedOptions[0].textContent.split(' · ')[0] : inp.value;
        return { id: inp.value, name: name };
      },
      siteList: function () {
        var f = this.form(); if (!f) return [];
        return Array.prototype.map.call(f.querySelectorAll('input[name="site_id"]'), function (i) {
          var nm = i.parentElement ? i.parentElement.textContent.replace('×', '').replace(' · 사업 대상', '').trim() : i.value;
          return { id: i.value, name: nm };
        });
      },
      options: function () {
        var out = [], p = this.pkg();
        if (p) out.push({ type: 'sub', id: p.id, label: '하부 업무 · ' + p.name });
        this.siteList().forEach(function (s) { out.push({ type: 'site', id: s.id, label: '사이트 · ' + s.name }); });
        return out;
      },
      add: function () {
        var self = this;
        this.rows.push({ ref: '', to: '' });
        if (!this.usersLoaded) {
          this.usersLoaded = true;
          fetch('/workboard/assignments/users', { credentials: 'same-origin', headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' } })
            .then(function (r) { return r.ok ? r.json() : []; }).then(function (l) { self.users = l || []; }).catch(function () { self.usersLoaded = false; });
        }
      },
      remove: function (i) { this.rows.splice(i, 1); },
      typeOf: function (ref) { return ref ? ref.split('|')[0] : ''; },
      idOf: function (ref) { return ref ? ref.split('|').slice(1).join('|') : ''; }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { assignRows: assignRows };
  root.assignRows = assignRows;
})(typeof window !== 'undefined' ? window : globalThis);
// 95-225 공동 작업 입력(체크박스 + 범위 + 지정 사용자).
(function (root) {
  'use strict';
  function collabFields() {
    return {
      on: false, scope: 'org', users: [], picked: [], loaded: false,
      loadUsers: function () {
        var self = this;
        if (this.loaded) return;
        this.loaded = true;
        fetch('/workboard/assignments/users', { credentials: 'same-origin', headers: { 'Accept': 'application/json', 'X-Requested-With': 'fetch' } })
          .then(function (r) { return r.ok ? r.json() : []; }).then(function (l) { self.users = l || []; }).catch(function () { self.loaded = false; });
      },
      pick: function (ev) {
        var id = ev.target.value; ev.target.value = '';
        if (!id || this.picked.some(function (u) { return u.id === id; })) return;
        var u = this.users.find(function (x) { return x.id === id; });
        if (u) this.picked = this.picked.concat([u]);
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports.collabFields = collabFields;
  root.collabFields = collabFields;
})(typeof window !== 'undefined' ? window : globalThis);
