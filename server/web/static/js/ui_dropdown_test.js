const { uiDropdown } = require('./ui_dropdown.js');

function ctx(opts) {
  const items = (opts.links || []).map((href) => {
    const a = { href: href, focus() { this.focused = true; }, click() { this.clicked = true; } };
    return a;
  });
  const self = uiDropdown();
  self.$refs = {
    btn: { focus() { self.btnFocused = true; } },
    menu: { querySelectorAll() { return items; } }
  };
  self.$nextTick = function (fn) { fn(); };
  return self;
}

const d = ctx({ links: ['/a', '/b'] });
d.onKey({ key: 'ArrowDown', preventDefault() {} });
if (!d.open) {
  console.error('ArrowDown 이 열지 않음');
  process.exit(1);
}
d.onKey({ key: 'Escape', preventDefault() {} });
if (d.open || !d.btnFocused) {
  console.error('Esc 닫기/포커스 복귀 실패', d.open);
  process.exit(1);
}

const d2 = ctx({ links: ['/a', '/b', '/c'] });
d2.toggle();
if (d2.i !== 0) {
  console.error('열리면 첫 항목', d2.i);
  process.exit(1);
}
d2.onKey({ key: 'ArrowDown', preventDefault() {} });
if (d2.i !== 1) {
  console.error('아래 화살표', d2.i);
  process.exit(1);
}
console.log('ok');
