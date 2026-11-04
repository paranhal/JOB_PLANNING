const { projectPicker } = require('./project_picker.js');

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

const p = projectPicker({
  Own: [{ project_id: 'A', name: '도서관' }, { project_id: 'B', name: '미디어' }],
  Selected: 'A',
  SelectedName: '도서관',
});
p.pick('A', '도서관', '');
assert(p.q === '', '선택 후 검색어는 비어야 한다');
assert(p.selectedName === '도서관', '선택명');
p.q = '미디';
assert(p.filtered.length === 1 && p.filtered[0].project_id === 'B', '검색은 입력칸만');
p.q = '';
assert(p.filtered.length === 2, '포커스 시 전체');
p.setPending(true);
assert(p.pending === true && p.selectedId === '', '나중에 매칭이면 사업 비움');
p.denyOther();
assert(p.otherDenied === true, '타 부서 거절');
console.log('project_picker_test.js ok');
