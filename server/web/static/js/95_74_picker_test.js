function nameOf(id, opts) {
  opts = opts || {};
  const names = opts.names || {};
  if (names[id] && String(names[id]).indexOf('USR-') !== 0) return names[id];
  const gmem = (opts.groups || []).flatMap((g) => g.members || []);
  const hit = gmem.find((m) => m.user_id === id);
  if (hit && hit.name && String(hit.name).indexOf('USR-') !== 0) return hit.name;
  const el = opts.checkbox;
  if (el) {
    const dn = el.getAttribute('data-name');
    if (dn && String(dn).indexOf('USR-') !== 0) return dn;
  }
  return '담당자';
}

const uid = 'USR-1792139370211635000';
if (nameOf(uid, { names: { [uid]: '양현모' } }) !== '양현모') {
  console.error('PickerNames 실패');
  process.exit(1);
}
if (nameOf(uid, {}) !== '담당자') {
  console.error('id 그대로 반환');
  process.exit(1);
}
if (nameOf(uid, { names: { [uid]: uid } }) !== '담당자') {
  console.error('USR- 사전값 통과');
  process.exit(1);
}
console.log('ok');
