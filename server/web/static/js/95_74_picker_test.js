function nameOf(id, opts) {
  opts = opts || {};
  const names = opts.names || {};
  if (names[id] && String(names[id]).indexOf('USR-') !== 0 && names[id] !== '담당자') return names[id];
  const gmem = (opts.groups || []).flatMap((g) => g.members || []);
  const hit = gmem.find((m) => m.user_id === id);
  if (hit && hit.name && String(hit.name).indexOf('USR-') !== 0 && hit.name !== '담당자') return hit.name;
  const el = opts.checkbox;
  if (el) {
    const dn = el.getAttribute('data-name');
    if (dn && String(dn).indexOf('USR-') !== 0 && dn !== '담당자') return dn;
  }
  return '이름 미등록';
}

const uid = 'USR-1792139370211635000';
if (nameOf(uid, { names: { [uid]: '양현모' } }) !== '양현모') {
  console.error('PickerNames 실패');
  process.exit(1);
}
if (nameOf(uid, {}) !== '이름 미등록') {
  console.error('USR- fallback 은 이름 미등록');
  process.exit(1);
}
if (nameOf(uid, { names: { [uid]: uid } }) !== '이름 미등록') {
  console.error('USR- 사전값은 이름 미등록');
  process.exit(1);
}
if (nameOf('', {}) !== '이름 미등록') {
  console.error('빈값 이름 미등록');
  process.exit(1);
}
if (nameOf(uid, { names: { [uid]: '담당자' } }) === '담당자') {
  console.error('담당자 단독 반환');
  process.exit(1);
}

const src = require('fs').readFileSync(require('path').join(__dirname, 'assignee_picker.js'), 'utf8');
const vm = require('vm');
const ctx = { window: {}, document: { getElementById: function () { return null; } } };
ctx.window = ctx;
ctx.globalThis = ctx;
vm.createContext(ctx);
vm.runInContext(src, ctx);
const extra = ctx.window.assigneePickerInitialExtra({ ownerId: uid, selectedIds: [] });
if (!extra[uid]) {
  console.error('ownerId hydrate extra 실패');
  process.exit(1);
}
const empty = ctx.window.assigneePickerInitialExtra({});
if (Object.keys(empty).length) {
  console.error('빈 cfg 에 extra 가 생기면 안 된다');
  process.exit(1);
}
const ph = ctx.window.assigneePicker({ external: '담당자' });
if (ph.summaryLabel() === '담당자') {
  console.error('external 담당자가 버튼 문구가 되면 안 된다');
  process.exit(1);
}
console.log('ok');
