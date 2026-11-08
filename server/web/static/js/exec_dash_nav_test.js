const nav = require('./exec_dash_nav.js');

const mem = {};
global.sessionStorage = {
  getItem(k) { return Object.prototype.hasOwnProperty.call(mem, k) ? mem[k] : null; },
  setItem(k, v) { mem[k] = String(v); },
  removeItem(k) { delete mem[k]; }
};
global.window = global;

function reset() {
  for (const k of Object.keys(mem)) delete mem[k];
}

reset();
nav.save('O01', { scrollY: 500, ts: Date.now() });
const got = nav.restore();
if (!got || got.scrollY !== 500 || got.orgId !== 'O01') {
  console.error('restore', got);
  process.exit(1);
}
if (mem[nav.KEY]) {
  console.error('복원 후 저장값이 남았다');
  process.exit(1);
}

reset();
if (nav.restore() !== null) {
  console.error('저장값 없음');
  process.exit(1);
}

reset();
nav.save('O02', { scrollY: 80, ts: Date.now() - nav.TTL - 1000 });
if (nav.restore({ now: Date.now() }) !== null) {
  console.error('5분 만료가 안 된다');
  process.exit(1);
}

if (!nav.pathIsExecHome('http://x/exec') || nav.pathIsExecHome('http://x/exec/work')) {
  console.error('pathIsExecHome');
  process.exit(1);
}

let backed = 0;
const a = { listeners: {} };
a.addEventListener = function (ev, fn) { this.fn = fn; };
const doc = {
  getElementById(id) { return id === 'exec-dash-back' ? a : null; }
};
nav.bindBack({
  document: doc,
  history: { back() { backed++; } },
  referrer() { return 'http://x/exec?date=2026-11-04'; }
});
const ev = { metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, button: 0, preventDefault() { this.pd = true; } };
a.fn(ev);
if (backed !== 1 || !ev.pd) {
  console.error('history.back 실패', backed, ev);
  process.exit(1);
}

backed = 0;
nav.bindBack({
  document: {
    getElementById() { return a; }
  },
  history: { back() { backed++; } },
  referrer() { return 'http://x/meeting'; }
});
const ev2 = { metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, button: 0, preventDefault() { this.pd = true; } };
a.fn(ev2);
if (backed !== 0 || ev2.pd) {
  console.error('직접 진입은 링크 이동이어야 한다', backed);
  process.exit(1);
}

reset();
let focused = '';
const card = {
  focus() { focused = 'O02'; },
  scrollIntoView() {},
  classList: { add() {}, remove() {} }
};
nav.save('O02', { scrollY: 120, ts: Date.now() });
const gotFocus = nav.restore({
  document: { querySelector(sel) { return sel.indexOf('O02') >= 0 ? card : null; } },
  scrollTo() {},
  setTimeout() {}
});
if (!gotFocus || focused !== 'O02') {
  console.error('복귀 포커스', gotFocus, focused);
  process.exit(1);
}

focused = '';
nav.focusOrgCard('O02', {
  document: { querySelector(sel) { return sel.indexOf('O02') >= 0 ? card : null; } },
  setTimeout() {}
});
if (focused !== 'O02') {
  console.error('갱신 후 포커스', focused);
  process.exit(1);
}

reset();
nav.save('O03', { scrollY: 10, ts: Date.now() - nav.TTL - 1 });
focused = '';
if (nav.restore({
  now: Date.now(),
  document: { querySelector() { focused = 'hit'; return card; } },
  scrollTo() {}
}) !== null || focused === 'hit') {
  console.error('TTL 지난 저장값을 썼다');
  process.exit(1);
}

console.log('ok');
