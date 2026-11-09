(function (root) {
  var ATTR = 'data-jp-combo';
  var closingPointer = false;

  function stack(el) {
    var s = el && el._x_dataStack;
    if (!s || !s.length) return null;
    for (var i = 0; i < s.length; i++) {
      if (s[i] && typeof s[i].open === 'boolean' && typeof s[i].comboClose === 'function') return s[i];
    }
    for (var j = 0; j < s.length; j++) {
      if (s[j] && typeof s[j].open === 'boolean') return s[j];
    }
    return s[0];
  }

  function hosts() {
    if (typeof document === 'undefined') return [];
    return Array.prototype.slice.call(document.querySelectorAll('[' + ATTR + ']'));
  }

  function closeHost(el) {
    var st = stack(el);
    if (!st || !st.open) return false;
    if (typeof st.comboClose === 'function') st.comboClose();
    else st.open = false;
    return true;
  }

  function closeAll(except) {
    var n = 0;
    hosts().forEach(function (el) {
      if (except && (el === except || (except.nodeType && el.contains(except)))) return;
      if (closeHost(el)) n++;
    });
    return n;
  }

  function bind(api) {
    api = api || {};
    api.comboClose = function () {
      this.open = false;
    };
    api.comboOpen = function () {
      var el = this.$el;
      closeAll(el);
      this.open = true;
    };
    api.comboToggleField = function () {
      if (this.open) {
        this.comboClose();
        return;
      }
      this.comboOpen();
      if (typeof this.search === 'function' && !(this.items && this.items.length)) this.search(false);
      if (typeof this.revealList === 'function') this.revealList();
    };
    api.comboOnTab = function () {
      if (this.open) this.comboClose();
    };
    api.comboOnArrow = function (d) {
      if (!this.open) this.comboOpen();
      if (typeof this.move === 'function') this.move(d);
    };
    return api;
  }

  function install() {
    if (typeof document === 'undefined' || document.__jpComboDismiss) return;
    document.__jpComboDismiss = true;
    document.addEventListener('mousedown', function (e) {
      var t = e.target;
      var inside = t && t.closest ? t.closest('[' + ATTR + ']') : null;
      if (inside) {
        closeAll(inside);
        return;
      }
      closingPointer = closeAll() > 0;
    }, true);
    document.addEventListener('click', function (e) {
      if (!closingPointer) return;
      e.stopPropagation();
      e.stopImmediatePropagation();
      closingPointer = false;
    }, true);
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      if (closeAll() > 0) {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
      }
    }, true);
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install);
    else install();
  }

  var api = { attr: ATTR, bind: bind, closeAll: closeAll, install: install };
  if (root) root.jpCombo = api;
  if (typeof module !== 'undefined') module.exports = api;
})(typeof window !== 'undefined' ? window : (typeof global !== 'undefined' ? global : this));
