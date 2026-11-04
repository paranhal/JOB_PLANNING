const { pfOwnMatches, pfShouldHintOther, pfDebounce } = require('./project_filter.js');

const names = ['도서관 운영', '방화벽 업그레이드'];
const m = pfOwnMatches(names, '방화벽');
if (m.length !== 1 || m[0] !== '방화벽 업그레이드') {
  console.error('own filter', m);
  process.exit(1);
}
if (pfOwnMatches(names, '').length !== 2) {
  console.error('empty q');
  process.exit(1);
}
if (!pfShouldHintOther(0, 3, '전관방송', false)) {
  console.error('hint should show');
  process.exit(1);
}
if (pfShouldHintOther(0, 3, '전관방송', true)) {
  console.error('hint hidden when more open');
  process.exit(1);
}
if (pfShouldHintOther(1, 3, '전관방송', false)) {
  console.error('hint hidden when own hits');
  process.exit(1);
}

let n = 0;
const d = pfDebounce(function () { n += 1; }, 20);
d(); d(); d();
setTimeout(function () {
  if (n !== 1) {
    console.error('debounce', n);
    process.exit(1);
  }
  console.log('ok');
}, 80);
