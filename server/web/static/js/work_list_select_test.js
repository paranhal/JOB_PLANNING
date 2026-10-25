const workListSelect = require('./work_list_select.js');

function box(value) {
  return { value: value, checked: false, className: 'js-wl-check' };
}

const a = box('t1');
const b = box('t2');
const all = { checked: false, indeterminate: false };
const root = {
  querySelectorAll(sel) {
    if (sel === '.js-wl-check') return [a, b];
    return [];
  },
  querySelector(sel) {
    if (sel === '.js-wl-all') return all;
    return null;
  }
};

const ctx = workListSelect('tok');
ctx.$root = root;
ctx.sync();
if (ctx.n !== 0) {
  console.error('초기 n=', ctx.n);
  process.exit(1);
}

a.checked = true;
b.checked = true;
ctx.sync();
if (ctx.n !== 2) {
  console.error('$root 체크 후 n=', ctx.n, '(this.$el 버그면 0)');
  process.exit(1);
}
if (!all.checked || all.indeterminate) {
  console.error('전체선택 상태', all);
  process.exit(1);
}

ctx.clear();
if (ctx.n !== 0 || a.checked || b.checked) {
  console.error('선택 해제 실패', ctx.n);
  process.exit(1);
}

ctx.toggleAll(true);
if (ctx.n !== 2 || !a.checked || !b.checked) {
  console.error('전체 선택 실패', ctx.n);
  process.exit(1);
}

const off = { value: 'x', checked: false, disabled: true, className: 'js-wl-check-off' };
const mixedRoot = {
  querySelectorAll(sel) {
    if (sel === '.js-wl-check') return [a, b];
    return [];
  },
  querySelector(sel) {
    if (sel === '.js-wl-all') return all;
    if (sel === '.js-wl-check-off') return off;
    return null;
  }
};
ctx.$root = mixedRoot;
ctx.toggleAll(true);
if (ctx.n !== 2) {
  console.error('비활성 행이 선택에 포함됨', ctx.n);
  process.exit(1);
}

console.log('ok');
