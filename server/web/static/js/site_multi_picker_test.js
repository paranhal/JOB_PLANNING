const { siteMultiPicker } = require('./site_multi_picker.js');

function assert(cond, msg) {
  if (!cond) {
    console.error('fail:', msg);
    process.exit(1);
  }
}

const p = siteMultiPicker({ max: 20, extraKey: 'symptom' });
p.pick({ site_id: 'A', site_name: '가' });
p.pick({ site_id: 'A', site_name: '가' });
assert(p.sites.length === 1, 'duplicate');
p.pick({ site_id: 'B', site_name: '나' });
assert(p.sites.length === 2, 'add');
p.removeSite('A');
assert(p.sites.length === 1 && p.sites[0].site_id === 'B', 'remove');
for (let i = 0; i < 25; i++) {
  p.pick({ site_id: 'S' + i, site_name: 's' + i });
}
assert(p.sites.length === 20, 'max 20 got ' + p.sites.length);
assert(p.hint.indexOf('최대 20곳') >= 0, 'cap hint');
const r = siteMultiPicker({ max: 100 });
for (let i = 0; i < 101; i++) {
  r.pick({ site_id: 'T' + i, site_name: 't' + i });
}
assert(r.sites.length === 100, 'max 100 got ' + r.sites.length);
assert(r.hint.indexOf('최대 100곳') >= 0, 'cap 100 hint');
const q = siteMultiPicker({ excludeId: 'X' });
q.pick({ site_id: 'X', site_name: '제외' });
assert(q.sites.length === 0, 'exclude');

const k = siteMultiPicker({ max: 100 });
k.cands = [{ site_id: 'A1', site_name: '하나만' }];
k.pickHighlight();
assert(k.sites.length === 1 && k.sites[0].site_id === 'A1', 'enter 1');
assert(k.q === '' && k.cands.length === 0, 'enter 1 clear');

k.cands = [
  { site_id: 'B1', site_name: '하나' },
  { site_id: 'B2', site_name: '둘' },
  { site_id: 'B3', site_name: '셋' }
];
k.hi = 0;
k.pickHighlight();
assert(k.sites[k.sites.length - 1].site_id === 'B1', 'enter many first');

k.cands = [
  { site_id: 'C1', site_name: '하나' },
  { site_id: 'C2', site_name: '둘' },
  { site_id: 'C3', site_name: '셋' }
];
k.move(1);
k.move(1);
assert(k.hi === 2, 'arrow down');
k.move(-1);
assert(k.hi === 1, 'arrow up');
k.pickHighlight();
assert(k.sites[k.sites.length - 1].site_id === 'C2', 'enter after arrows');

const before = k.sites.length;
k.cands = [];
k.pickHighlight();
assert(k.sites.length === before, 'enter 0');

const ev = { prevented: false, preventDefault: function () { this.prevented = true; } };
k.onEnter(ev);
assert(ev.prevented, 'enter preventDefault');

k.q = '다음';
k.cands = [{ site_id: 'D1', site_name: '연속1' }];
k.pickHighlight();
assert(k.q === '', 'cleared for next type');
k.cands = [{ site_id: 'D2', site_name: '연속2' }];
k.pickHighlight();
assert(k.sites.filter(function (s) { return s.site_id === 'D1' || s.site_id === 'D2'; }).length === 2, 'consecutive add');

k.closeCands();
assert(k.cands.length === 0 && k.hi === 0, 'esc');

console.log('ok');
