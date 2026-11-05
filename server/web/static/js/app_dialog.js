(function () {
  var lastFocus = null;
  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )).filter(function (el) { return el.offsetParent !== null || el === document.activeElement; });
  }
  window.appDialogTrap = function (root) {
    if (!root) return function () {};
    lastFocus = document.activeElement;
    var list = focusables(root);
    if (list.length) list[0].focus();
    function onKey(e) {
      if (e.key === 'Tab') {
        var items = focusables(root);
        if (!items.length) return;
        var i = items.indexOf(document.activeElement);
        if (e.shiftKey) {
          if (i <= 0) { e.preventDefault(); items[items.length - 1].focus(); }
        } else if (i === items.length - 1 || i < 0) {
          e.preventDefault(); items[0].focus();
        }
      }
    }
    root.addEventListener('keydown', onKey);
    return function () {
      root.removeEventListener('keydown', onKey);
      if (lastFocus && lastFocus.focus) lastFocus.focus();
    };
  };
})();
