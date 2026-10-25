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
const q = siteMultiPicker({ excludeId: 'X' });
q.pick({ site_id: 'X', site_name: '제외' });
assert(q.sites.length === 0, 'exclude');
console.log('ok');
