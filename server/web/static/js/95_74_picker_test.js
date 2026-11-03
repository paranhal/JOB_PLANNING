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
  id = String(id || '').trim();
  if (id) return id;
  return '이름 미등록';
}

const uid = 'USR-1792139370211635000';
if (nameOf(uid, { names: { [uid]: '양현모' } }) !== '양현모') {
  console.error('PickerNames 실패');
  process.exit(1);
}
if (nameOf(uid, {}) !== uid) {
  console.error('로그인 아이디 fallback 실패');
  process.exit(1);
}
if (nameOf(uid, { names: { [uid]: uid } }) !== uid) {
  console.error('USR- 사전값은 아이디로');
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
console.log('ok');
