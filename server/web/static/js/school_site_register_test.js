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

const t1 = schoolSiteRegister({ projectId: 'WP', canWrite: true, type: 'library' });
t1.regions = [
  { sido: '세종', sigungu: ['세종시'] },
  { sido: '충남', sigungu: ['천안시', '아산시'] }
];
t1.items = [
  { id: 'sj', name: '세종도서관', sido: '세종', sigungu: '세종시', registered: false },
  { id: 'cn', name: '천안도서관', sido: '충남', sigungu: '천안시', registered: false },
  { id: 'as', name: '아산도서관', sido: '충남', sigungu: '아산시', registered: false }
];
t1.toggleSido('세종');
if (t1.sidoAll || !t1.sidoSel['세종'] || t1.sidoSel['충남']) {
  console.error('T1 세종 선택', t1.sidoAll, t1.sidoSel);
  process.exit(1);
}
if (!t1.sigunguAll || Object.keys(t1.sigunguSel).length) {
  console.error('T1 시군 초기화', t1.sigunguAll, t1.sigunguSel);
  process.exit(1);
}
if (t1.sigunKeys().join(',') !== '세종|세종시') {
  console.error('T1 시군 키', t1.sigunKeys());
  process.exit(1);
}
if (t1.visible().length !== 1 || t1.visible()[0].id !== 'sj') {
  console.error('T1 visible', t1.visible());
  process.exit(1);
}
t1.toggleSido('ALL');
if (!t1.sidoAll || t1.visible().length !== 3) {
  console.error('T1 전체 복귀', t1.sidoAll, t1.visible().length);
  process.exit(1);
}
t1.toggleSido('세종');
t1.toggleSido('세종');
if (!t1.sidoAll) {
  console.error('T1 마지막 해제 전체', t1.sidoAll, t1.sidoSel);
  process.exit(1);
}

if (typeof FormData === 'undefined') {
  global.FormData = function () { this.pairs = []; };
  global.FormData.prototype.append = function (k, v) { this.pairs.push([k, v]); };
}

(async function () {
  const t3 = schoolSiteRegister({ projectId: 'WP', canWrite: true, type: 'library' });
  t3.regions = [{ sido: '세종', sigungu: ['세종시'] }];
  t3.items = [
    { id: 'a', name: 'A도서관', sido: '세종', sigungu: '세종시', registered: false },
    { id: 'b', name: 'B도서관', sido: '세종', sigungu: '세종시', registered: false }
  ];
  t3.picked = { a: true, b: true };
  t3.siteTotal = 0;
  global.fetch = function () {
    return Promise.resolve({
      ok: true,
      json: function () {
        return Promise.resolve({
          added: 1,
          skipped: 1,
          siteTotal: 4,
          addedIDs: ['a'],
          skippedItems: [{ customer_id: 'b', reason: '같은 이름 사이트가 있음', name: 'B도서관' }]
        });
      }
    });
  };
  await t3.register();
  if (!t3.items[0].registered || t3.items[1].registered) {
    console.error('T3 registered', t3.items);
    process.exit(1);
  }
  if (t3.picked.a || !t3.picked.b) {
    console.error('T3 picked', t3.picked);
    process.exit(1);
  }
  if (t3.modalFail || t3.siteTotal !== 4 || t3.modalBody.indexOf('1곳을 등록했습니다') < 0 || t3.modalBody.indexOf('같은 이름') < 0) {
    console.error('T3 modal', t3.modalFail, t3.modalBody);
    process.exit(1);
  }

  const z = schoolSiteRegister({ projectId: 'WP', canWrite: true, type: 'library' });
  z.regions = t3.regions;
  z.items = [
    { id: 'a', name: 'A도서관', sido: '세종', sigungu: '세종시', registered: false },
    { id: 'b', name: 'B도서관', sido: '세종', sigungu: '세종시', registered: false }
  ];
  z.picked = { a: true, b: true };
  z.siteTotal = 2;
  global.fetch = function () {
    return Promise.resolve({
      ok: true,
      json: function () {
        return Promise.resolve({
          added: 0,
          skipped: 2,
          siteTotal: 2,
          addedIDs: [],
          skippedItems: [
            { customer_id: 'a', reason: '이미 이 사업에 등록됨' },
            { customer_id: 'b', reason: '같은 이름 사이트가 있음' }
          ]
        });
      }
    });
  };
  await z.register();
  if (z.items[0].registered || z.items[1].registered) {
    console.error('T3 zero registered', z.items);
    process.exit(1);
  }
  if (!z.picked.a || !z.picked.b) {
    console.error('T3 zero picked', z.picked);
    process.exit(1);
  }
  if (!z.modalFail || z.modalBody.indexOf('등록했습니다') >= 0 || z.modalBody.indexOf('등록된 곳이 없습니다') < 0) {
    console.error('T3 zero modal', z.modalFail, z.modalBody);
    process.exit(1);
  }
  console.log('ok');
})().catch(function (e) {
  console.error(e);
  process.exit(1);
});
