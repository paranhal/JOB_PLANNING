const fs = require('fs');
const path = require('path');
const vm = require('vm');

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

const src = fs.readFileSync(path.join(__dirname, 'as_action_sets.js'), 'utf8');
const ctx = { window: {}, document: { getElementById: function () { return null; } } };
ctx.window = ctx;
ctx.globalThis = ctx;
vm.createContext(ctx);
vm.runInContext(src, ctx);

const api = ctx.ASActionSets;
assert(api, 'ASActionSets 없음');
assert(api.needsResult({ requireResult: true }, '') === true, '결과 필수면 막음');
assert(api.needsResult({ requireResult: false }, '') === false, '임시 저장은 결과 없이 보냄');
assert(api.needsResult({ requireResult: true }, 'done') === false, '결과 있으면 통과');

const fd = { n: [], append: function (k, v) { this.n.push(k + '=' + v); } };
api.appendDraftFlag(fd, { requireResult: false });
assert(fd.n.indexOf('draft=1') >= 0, 'draft=1');
const fd2 = { n: [], append: function (k, v) { this.n.push(k + '=' + v); } };
api.appendDraftFlag(fd2, { requireResult: true });
assert(fd2.n.length === 0, '자동 저장은 draft 없음');

const dest = { value: '' };
api.copyWorkDateToVisit('2026-11-03', dest);
assert(dest.value === '2026-11-03', 'mirrorLast 작업일 → as-visit-date');

assert(api.missingLine(['result_code', 'process_type']).indexOf('결과') >= 0, 'missing 안내');

console.log('as_action_sets_test.js ok');
