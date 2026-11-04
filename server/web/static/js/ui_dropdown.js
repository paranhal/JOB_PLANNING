function uiDropdown() {
  return {
    open: false,
    i: -1,
    toggle() {
      this.open = !this.open;
      if (this.open) {
        var self = this;
        this.$nextTick(function () { self.focusAt(0); });
      } else {
        this.i = -1;
      }
    },
    close() {
      if (!this.open) return;
      this.open = false;
      this.i = -1;
      if (this.$refs.btn) this.$refs.btn.focus();
    },
    options() {
      if (!this.$refs.menu) return [];
      return Array.prototype.slice.call(this.$refs.menu.querySelectorAll('[role="option"]'));
    },
    focusAt(n) {
      var opts = this.options();
      if (!opts.length) return;
      if (n < 0) n = opts.length - 1;
      if (n >= opts.length) n = 0;
      this.i = n;
      opts[n].focus();
    },
    onKey(ev) {
      if (!this.open) {
        if (ev.key === 'ArrowDown' || ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault();
          this.toggle();
        }
        return;
      }
      if (ev.key === 'Escape') {
        ev.preventDefault();
        this.close();
        return;
      }
      if (ev.key === 'ArrowDown') {
        ev.preventDefault();
        this.focusAt(this.i + 1);
        return;
      }
      if (ev.key === 'ArrowUp') {
        ev.preventDefault();
        this.focusAt(this.i - 1);
        return;
      }
      if (ev.key === 'Enter' || ev.key === ' ') {
        var opts = this.options();
        if (opts[this.i]) {
          ev.preventDefault();
          opts[this.i].click();
        }
      }
    }
  };
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { uiDropdown: uiDropdown };
}
