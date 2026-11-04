const fs = require('fs');
const path = require('path');
const vm = require('vm');

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

const src = fs.readFileSync(path.join(__dirname, 'as_keywords.js'), 'utf8');
const ctx = {
  window: {},
  document: { getElementById: function () { return null; } }
};
ctx.window = ctx;
ctx.globalThis = ctx;
vm.createContext(ctx);
vm.runInContext(src, ctx);

const api = ctx.ASKeywords;
assert(api, 'ASKeywords 없음');
assert(api.newKey({ keyword: '재기동', kw_group: '조치' }) === 'new:재기동|조치', 'newKey');
const d1 = {
  items: [
    { keyword_id: 'K1', keyword: 'A', kw_group: '대상' },
    { keyword_id: 'K2', keyword: 'B', kw_group: '증상' }
  ],
  new: [{ keyword: '재기동', kw_group: '조치' }]
};
const d2 = {
  items: [
    { keyword_id: 'K1', keyword: 'A', kw_group: '대상' },
    { keyword_id: 'K2', keyword: 'B', kw_group: '증상' }
  ],
  new: [{ keyword: '재기동', kw_group: '조치' }]
};
assert(api.itemSig(d1) === api.itemSig(d2), '같은 후보 집합 sig');
assert(api.itemSig(d1) !== api.itemSig({ items: [{ keyword_id: 'K3' }], new: [] }), '다른 집합');
console.log('as_keywords_test.js ok');
