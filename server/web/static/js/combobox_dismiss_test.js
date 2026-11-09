const jpCombo = require('./combobox_dismiss.js');

if (!jpCombo || typeof jpCombo.bind !== 'function') {
  throw new Error('jpCombo.bind 없음');
}
const api = jpCombo.bind({ open: false, items: [], searchCalls: 0, search: function () { this.searchCalls++; } });
if (typeof api.comboOpen !== 'function' || typeof api.comboToggleField !== 'function') {
  throw new Error('mixin 없음');
}
api.comboToggleField();
if (!api.open || api.searchCalls !== 1) throw new Error('열기/검색 ' + api.open + ' ' + api.searchCalls);
const q = '검색어유지';
api.q = q;
api.selectedId = 'C1';
api.comboToggleField();
if (api.open) throw new Error('재클릭에 안 닫힘');
if (api.q !== q || api.selectedId !== 'C1') throw new Error('닫을 때 값 지움');
api.comboOnTab();
if (api.open) throw new Error('탭 후 열림');
console.log('combobox_dismiss_test.js ok');
