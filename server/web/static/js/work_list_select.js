(function (root) {
  function cookieValue(name) {
    if (typeof document === 'undefined' || !document.cookie) return '';
    var parts = ('; ' + document.cookie).split('; ' + name + '=');
    if (parts.length < 2) return '';
    return decodeURIComponent(parts.pop().split(';').shift() || '');
  }

  function metaCsrf() {
    if (typeof document === 'undefined' || !document.querySelector) return '';
    var meta = document.querySelector('meta[name="csrf-token"]');
    return (meta && meta.getAttribute('content')) || '';
  }

  function workListSelect(csrf) {
    return {
      csrf: csrf || '',
      n: 0,
      modal: false,
      busy: false,
      deletable: [],
      skipped: [],
      ids: [],
      delN: 0,
      lastError: '',
      rootEl() {
        return this.$root || (typeof document !== 'undefined' ? document.getElementById('work-list-select') : null);
      },
      boxes() {
        var el = this.rootEl();
        if (!el || !el.querySelectorAll) return [];
        return Array.prototype.slice.call(el.querySelectorAll('.js-wl-check'));
      },
      sync() {
        var boxes = this.boxes();
        this.n = boxes.filter(function (b) { return b.checked; }).length;
        var el = this.rootEl();
        var all = el && el.querySelector ? el.querySelector('.js-wl-all') : null;
        if (all) {
          all.checked = boxes.length > 0 && this.n === boxes.length;
          all.indeterminate = this.n > 0 && this.n < boxes.length;
        }
      },
      toggleAll(on) {
        this.boxes().forEach(function (b) { b.checked = !!on; });
        this.sync();
      },
      clear() { this.toggleAll(false); },
      selectedIds() {
        return this.boxes().filter(function (b) { return b.checked; }).map(function (b) { return b.value; });
      },
      readCsrf() {
        return cookieValue('jp_csrf') || metaCsrf() || this.csrf || '';
      },
      applyCsrf(tok) {
        if (!tok) return;
        this.csrf = tok;
        if (typeof document === 'undefined' || !document.querySelector) return;
        var meta = document.querySelector('meta[name="csrf-token"]');
        if (meta) meta.setAttribute('content', tok);
      },
      showErr(msg, reload) {
        this.lastError = String(msg || '');
        this.modal = false;
        this.busy = false;
        if (typeof window !== 'undefined' && window.showFlashError) {
          window.showFlashError(this.lastError, !!reload || /새로고침/.test(this.lastError));
        }
      },
      refreshCsrf() {
        var self = this;
        if (typeof fetch !== 'function') return Promise.resolve(false);
        return fetch('/csrf', {
          method: 'GET',
          credentials: 'same-origin',
          headers: { 'Accept': 'application/json' }
        }).then(function (r) {
          if (!r.ok) return false;
          return r.json().then(function (d) {
            if (d && d.csrf) {
              self.applyCsrf(d.csrf);
              return true;
            }
            return false;
          });
        }).catch(function () { return false; });
      },
      failPreview(status, d) {
        var msg = (d && d.error) ? d.error : '삭제 대상을 확인하지 못했습니다. 페이지를 새로고침한 뒤 다시 시도해 주세요.';
        if (status) msg = msg + ' (' + status + ')';
        this.showErr(msg, true);
      },
      preview() {
        var ids = this.selectedIds();
        if (!ids.length) return;
        this.ids = ids;
        this.applyCsrf(this.readCsrf());
        this.fetchPreview(ids, false);
      },
      fetchPreview(ids, retried) {
        var body = ids.map(function (id) { return 'ids=' + encodeURIComponent(id); }).join('&');
        body += '&csrf=' + encodeURIComponent(this.csrf);
        var self = this;
        if (typeof fetch !== 'function') {
          this.showErr('삭제 대상을 확인하지 못했습니다.');
          return Promise.resolve();
        }
        return fetch('/workboard/tasks/bulk-delete/preview', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Accept': 'application/json',
            'X-CSRF-Token': this.csrf
          },
          body: body
        }).then(function (r) {
          return r.json().then(function (d) {
            return { ok: r.ok, status: r.status, d: d || {} };
          }).catch(function () {
            return { ok: false, status: r.status, d: { error: '확인 결과를 받지 못했습니다' } };
          });
        }).then(function (res) {
          var d = res.d || {};
          if (!res.ok || d.error) {
            if (!retried && (res.status === 403 || /보안 확인/.test(d.error || ''))) {
              return self.refreshCsrf().then(function (ok) {
                if (ok) return self.fetchPreview(ids, true);
                self.failPreview(res.status, d);
              });
            }
            self.failPreview(res.status, d);
            return;
          }
          var del = d.deletable || [];
          var skip = d.skipped || [];
          if (ids.length > 0 && del.length + skip.length === 0) {
            self.showErr('확인 결과를 받지 못했습니다', true);
            return;
          }
          self.deletable = del;
          self.skipped = skip;
          self.delN = del.length;
          self.busy = false;
          self.lastError = '';
          self.modal = true;
        }).catch(function () {
          self.showErr('삭제 대상을 확인하지 못했습니다. 페이지를 새로고침한 뒤 다시 시도해 주세요.', true);
        });
      }
    };
  }
  root.workListSelect = workListSelect;
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = workListSelect;
  }
})(typeof window !== 'undefined' ? window : (typeof global !== 'undefined' ? global : this));
