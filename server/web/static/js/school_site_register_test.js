const {
  schoolSiteRegister,
  ssrFiltered,
  ssrResetChecked,
  ssrToggleMap,
  ssrSyncRegionsToSearch,
  ssrSigunKeys
} = require('./school_site_register.js');

const regions = [
  { sido: '대전', sigungu: ['서구'] },
  { sido: '충남', sigungu: ['천안시', '아산시', '계룡시'] }
];
const items = [
  { id: 'a', name: '천안초', sido: '충남', sigungu: '천안시', registered: false },
  { id: 'b', name: '아산초', sido: '충남', sigungu: '아산시', registered: false },
  { id: 'c', name: '계룡고', sido: '충남', sigungu: '계룡시', registered: true },
  { id: 'd', name: '서구초', sido: '대전', sigungu: '서구', registered: false }
];

function stBase() {
  return {
    regions: regions,
    items: items,
    sidoAll: true,
    sidoSel: {},
    sigunguAll: true,
    sigunguSel: {},
    appliedQ: '',
    picked: {}
  };
}

let st = stBase();
ssrResetChecked(st);
if (!st.picked.a || !st.picked.b || st.picked.c || !st.picked.d) {
  console.error('초기 체크', st.picked);
  process.exit(1);
}

ssrToggleMap('sidoAll', 'sidoSel', '충남', ['대전', '충남'], st);
st.sigunguAll = true;
st.sigunguSel = {};
ssrResetChecked(st);
if (ssrFiltered(st).some(function (x) { return x.sido !== '충남'; })) {
  console.error('시도 필터');
  process.exit(1);
}
if (ssrSigunKeys(st).join(',') !== '충남|천안시,충남|아산시,충남|계룡시') {
  console.error('시군 목록', ssrSigunKeys(st));
  process.exit(1);
}

ssrToggleMap('sidoAll', 'sidoSel', '충남', ['대전', '충남'], st);
if (!st.sidoAll) {
  console.error('시도 모두 해제 시 전체 복귀');
  process.exit(1);
}

st = stBase();
ssrSyncRegionsToSearch(st, '천안');
if (st.sidoAll || !st.sidoSel['충남'] || st.sidoSel['대전']) {
  console.error('천안 검색 시도', st);
  process.exit(1);
}
if (st.sigunguAll || !st.sigunguSel['충남|천안시'] || st.sigunguSel['충남|아산시']) {
  console.error('천안 검색 시군', st);
  process.exit(1);
}
st.appliedQ = '천안';
if (ssrFiltered(st).length !== 1 || ssrFiltered(st)[0].id !== 'a') {
  console.error('천안 목록', ssrFiltered(st));
  process.exit(1);
}

st = stBase();
ssrSyncRegionsToSearch(st, '충남');
if (!st.sidoAll && Object.keys(st.sidoSel).join() !== '충남') {
  console.error('충남 검색 시도', st);
  process.exit(1);
}
if (!st.sigunguAll) {
  console.error('충남 검색 시군은 전체여야 함', st);
  process.exit(1);
}

const keep = stBase();
keep.sidoAll = false;
keep.sidoSel = { 대전: true };
ssrSyncRegionsToSearch(keep, '없는검색어');
if (keep.sidoAll || !keep.sidoSel['대전']) {
  console.error('0건 검색 시 지역 유지');
  process.exit(1);
}

st = stBase();
ssrSyncRegionsToSearch(st, '천안');
ssrSyncRegionsToSearch(st, '');
if (!st.sidoAll || !st.sigunguAll) {
  console.error('빈 검색 전체 복귀');
  process.exit(1);
}

const s = schoolSiteRegister({ projectId: 'WP', canWrite: true, type: 'school' });
s.items = items;
s.regions = regions;
ssrResetChecked(s);
s.picked.a = false;
if (s.selectedCount() !== 2) {
  console.error('선택 수', s.selectedCount());
  process.exit(1);
}

console.log('ok');
