var cause = require('./as_action_set_cause.js');
var api = global.ASActionSetCause;
if (!api) {
  console.error('ASActionSetCause 없음');
  process.exit(1);
}
var cats = [
  { code: 'server', label: '서버', level: 1, parent_code: '', is_active: true },
  { code: 'server.hw', label: 'H/W', level: 2, parent_code: 'server', is_active: true },
  { code: 'server.hw.disk', label: '디스크', level: 3, parent_code: 'server.hw', is_active: true },
  { code: 'rfid', label: 'RFID', level: 1, parent_code: '', is_active: true },
  { code: 'rfid.hw', label: 'HW', level: 2, parent_code: 'rfid', is_active: true },
  { code: 'rfid.hw.part_check', label: '부품 점검', level: 3, parent_code: 'rfid.hw', is_active: true },
  { code: 'klas', label: 'KLAS', level: 1, parent_code: '', is_active: true },
  { code: 'klas.book_stats', label: '장서통계', level: 2, parent_code: 'klas', is_active: true }
];
var l2 = api.childrenOf(cats, 'server', 2);
if (l2.length !== 1 || l2[0].code !== 'server.hw') {
  console.error('1→2 실패', l2);
  process.exit(1);
}
var l3 = api.childrenOf(cats, 'server.hw', 3);
if (l3.length !== 1 || l3[0].code !== 'server.hw.disk') {
  console.error('2→3 실패', l3);
  process.exit(1);
}
var rfid2 = api.childrenOf(cats, 'rfid', 2);
if (rfid2.length !== 1 || rfid2[0].code !== 'rfid.hw') {
  console.error('RFID 1→2 실패', rfid2);
  process.exit(1);
}
var rfid3 = api.childrenOf(cats, 'rfid.hw', 3);
if (rfid3.length !== 1 || rfid3[0].code !== 'rfid.hw.part_check') {
  console.error('RFID 2→3 실패', rfid3);
  process.exit(1);
}
if (api.childrenOf(cats, 'klas.book_stats', 3).length !== 0) {
  console.error('하위 없음 실패');
  process.exit(1);
}
if (api.childrenOf(cats, 'custom', 2).length !== 0) {
  console.error('직접 입력 하위가 있다');
  process.exit(1);
}
console.log('as_action_set_cause_test ok');
