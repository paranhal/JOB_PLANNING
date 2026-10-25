const fs = require('fs');
const path = require('path');
const vm = require('vm');

const src = fs.readFileSync(path.join(__dirname, 'mnt-assignee-multi.js'), 'utf8');
const ctx = { window: {}, CustomEvent: function (n, o) { this.type = n; this.detail = o && o.detail; } };
vm.createContext(ctx);
vm.runInContext(src, ctx);

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

const a = ctx.window.mntVisitAction({ start: '10:00', end: '11:30', travel: 40 });
a.calc();
assert(a.siteMin() === 90, '90분');
assert(a.durLabel().indexOf('1시간') >= 0, '라벨');
assert(a.warn === '', '경고 없음');
a.end = '09:00';
assert(a.calc() === false, '역전 거부');
assert(a.warn.length > 0, '경고 문구');

const b = ctx.window.mntVisitAction({ start: '10:00' });
b.applyChip(90);
assert(b.end === '11:30', '칩이 종료를 채움');

console.log('mnt-visit-action_test.js ok');
