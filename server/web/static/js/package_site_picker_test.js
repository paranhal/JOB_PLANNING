const { packageSitePicker } = require('./package_site_picker.js');

function assert(cond, msg) {
  if (!cond) {
    console.error('fail:', msg);
    process.exit(1);
  }
}

const p = packageSitePicker();
p.sites = [];
p.maxSites = 3;
p.items = [
  { id: 'A', name: '홍성고', sido: '충남', sigungu: '홍성군', type: 'school' },
  { id: 'B', name: '갈산중', sido: '충남', sigungu: '홍성군', type: 'school' },
  { id: 'C', name: '천안초', sido: '충남', sigungu: '천안시', type: 'school' },
  { id: 'D', name: '홍천초', sido: '강원', sigungu: '홍천군', type: 'school' },
  { id: 'L', name: '군립도서관', sido: '충남', sigungu: '홍성군', type: 'library' },
];
assert(p.firstType() === 'school', 'first type');
assert(p.typeCount('library') === 1 && p.typeCount('private') === 0, 'type counts');
assert(p.visible().length === 4, 'school only ' + p.visible().length);
p.setType('library');
assert(p.visible().length === 1 && p.sidoNames().join() === '충남', 'library tab');
p.setType('school');

// 시도·시군 필터
p.toggleSido('강원');
assert(p.visible().length === 1 && p.visible()[0].id === 'D', 'sido filter');
p.toggleSido('ALL');
assert(p.visible().length === 4, 'sido all');
p.toggleSigungu('충남|홍성군');
assert(p.visible().map((d) => d.id).join() === 'A,B', 'sigungu filter ' + p.visible().map((d) => d.id));
p.toggleSigungu('ALL');

// 검색
p.q = '천안'; p.search();
assert(p.visible().length === 1 && p.visible()[0].id === 'C', 'search');
p.q = ''; p.search();

// 전체 선택 → 담기, 최대 개수
p.toggleAll(true);
assert(p.pickedCount() === 4, 'picked all');
p.addPicked();
assert(p.sites.length === 3, 'max cap ' + p.sites.length);
assert(p.note.indexOf('3곳을 담았습니다') >= 0 && p.note.indexOf('1곳은 담지 못했습니다') >= 0, 'note ' + p.note);
assert(p.inCart('A') && p.freeVisible().length === 1, 'cart reflected');

// 하부 업무를 바꾸면 새 하부 업무에 없는 사이트는 빠진다
p.dropForeign(['A', 'B']);
assert(p.sites.map((s) => s.site_id).join() === 'A,B', 'drop foreign ' + p.sites.map((s) => s.site_id));
console.log('package_site_picker_test ok');
