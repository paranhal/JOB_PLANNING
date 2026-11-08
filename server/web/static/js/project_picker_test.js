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
const all = projectPicker({
  SiteOnly: true,
  ShowAllSiteRows: true,
  ShowSiteNote: true,
  SiteCount: 12,
  Site: Array.from({ length: 12 }, function (_, i) { return { project_id: 'P' + i, name: 'n' + i }; }),
  Own: Array.from({ length: 12 }, function (_, i) { return { project_id: 'P' + i, name: 'n' + i }; }),
});
assert(all.siteRowsVisible.length === 12, '사이트 전건 표시');
assert(all.siteMore === false, '더 보기 없음');
assert(all.siteNote.indexOf('12건') >= 0, 'N건 안내');
const visitEmpty = projectPicker({
  SiteOnly: true, ShowSiteNote: true, SiteCount: 0, Site: [], Own: [],
  EmptyListHint: '이 사이트가 등록된 사업이 없습니다. 사업명을 검색해 연결하세요',
});
assert(visitEmpty.siteNote.indexOf('사업명을 검색해 연결하세요') >= 0, '방문 0건 안내');
assert(visitEmpty.siteNote.indexOf('사이트 등록에서') < 0, '옛 문구');
visitEmpty.remoteSearch = projectPicker({}).remoteSearch;
console.log('project_picker_test.js ok');
