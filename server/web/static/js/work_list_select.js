(function (root) {
  function workListSelect(csrf) {
    return {
      csrf: csrf || '',
      n: 0,
      modal: false,
      busy: false,
      deletable: [],
      skipped: [],
      delN: 0,
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
      preview() {
        var ids = this.selectedIds();
        if (!ids.length) return;
        var body = ids.map(function (id) { return 'ids=' + encodeURIComponent(id); }).join('&');
        body += '&csrf=' + encodeURIComponent(this.csrf);
        var self = this;
        fetch('/workboard/tasks/bulk-delete/preview', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Accept': 'application/json',
            'X-CSRF-Token': this.csrf
          },
          body: body
        }).then(function (r) { return r.json(); }).then(function (d) {
          self.deletable = d.deletable || [];
          self.skipped = d.skipped || [];
          self.delN = self.deletable.length;
          self.busy = false;
          self.modal = true;
        }).catch(function () {
          if (typeof window !== 'undefined' && window.showFlashError) {
            window.showFlashError('삭제 대상을 확인하지 못했습니다. 페이지를 새로고침해 주세요.');
          }
        });
      }
    };
  }
  root.workListSelect = workListSelect;
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = workListSelect;
  }
})(typeof window !== 'undefined' ? window : (typeof global !== 'undefined' ? global : this));
