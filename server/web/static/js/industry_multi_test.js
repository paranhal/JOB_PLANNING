// node web/static/js/industry_multi_test.js
const assert = require('assert');
const { industryMulti } = require('./industry_multi.js');
const c = industryMulti({ options: ['도서관', '학교', '공공기관', '민간'], extras: ['교육청'], selected: ['학교'] });
assert.strictEqual(c.summary(), '학교');
c.toggleValue('도서관');
assert.deepStrictEqual(c.sel, ['도서관', '학교'], '고정 순서로 보관');
assert.strictEqual(c.summary(), '도서관, 학교');
c.toggleValue('민간');
assert.strictEqual(c.summary(), '도서관 외 2', '3개 이상은 첫 항목 외 N');
c.toggleValue('교육청');
assert.deepStrictEqual(c.sel, ['도서관', '학교', '민간', '교육청'], '기타 기존 값은 뒤에');
c.toggleValue('학교'); c.toggleValue('도서관'); c.toggleValue('민간'); c.toggleValue('교육청');
assert.strictEqual(c.summary(), '선택하세요');
assert.ok(c.all.find((o) => o.value === '교육청' && o.extra), '목록 밖 값은 기타 표시');
console.log('industry_multi_test ok');
