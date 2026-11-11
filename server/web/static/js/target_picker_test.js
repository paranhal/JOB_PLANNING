const { createTargetPicker, splitDefaultTitle } = require('./target_picker.js');

function assert(cond, msg) {
  if (!cond) {
    console.error('fail:', msg);
    process.exit(1);
  }
}

const fresh = createTargetPicker({ customerId: 'C1', selected: [] });
assert(fresh.state.open === false && fresh.state.q === '' && fresh.state.applied.length === 0, 'empty first');
assert(fresh.state.applied.length === 0, 'unlinked');
fresh.setQuery('가');
assert(fresh.canSearch() === false && fresh.state.open === false, '1 char');
fresh.setQuery('');
assert(fresh.canSearch() === false && fresh.state.hint === '', 'blank query stays closed');

const p = createTargetPicker({ customerId: 'C1' });
p.setQuery('스위치');
p.receiveHits([
  { id: 'A1', name: '스위치 SW', customer_id: 'C1', sub: '본관' },
  { id: 'A9', name: '다른자산', customer_id: 'C2' },
  { asset_id: 'A2', product_name: '발급기', model_name: '부분', customer_id: 'C1' }
]);
assert(p.state.open === true, 'open after search');
assert(p.state.hits.length === 2, 'other customer hidden ' + p.state.hits.length);
p.toggle('A1');
p.toggle('A2');
p.selectAll();
assert(p.state.draft.A1 && p.state.draft.A2, 'select all');
p.clearDraft();
assert(!p.state.draft.A1 && !p.state.draft.A2, 'clear all');
p.toggle('A1');
p.toggle('A2');
assert(p.apply() === true, 'apply');
assert(p.state.open === false, 'apply closes');
assert(p.state.applied.length === 2, 'chips');
assert(p.state.applied[0].name.indexOf('스위치') >= 0, 'chip name');
p.remove('A1');
assert(p.state.applied.length === 1 && p.state.applied[0].id === 'A2', 'chip remove');
p.cancel();
assert(p.state.applied.length === 0, 'cancel restores saved empty');

const edited = createTargetPicker({
  customerId: 'C1',
  selected: [{ id: 'A1', name: '기존' }]
});
edited.setQuery('스위치');
edited.receiveHits([{ id: 'A2', name: '추가', customer_id: 'C1' }]);
edited.toggle('A2');
edited.apply();
assert(edited.state.applied.map(function (h) { return h.id; }).join(',') === 'A1,A2', 'apply keeps previous');
edited.cancel();
assert(edited.state.applied.length === 1 && edited.state.applied[0].id === 'A1', 'cancel after apply');

const one = createTargetPicker({ multi: false, customerId: 'C1' });
one.setQuery('장비');
one.receiveHits([
  { id: 'A1', name: '하나', customer_id: 'C1' },
  { id: 'A2', name: '둘', customer_id: 'C1' }
]);
one.toggle('A1');
one.toggle('A2');
assert(one.state.rowHint.indexOf('장비 추가') >= 0, 'single row hint');
assert(!one.state.draft.A2, 'second not checked');
one.selectAll();
assert(one.state.rowHint.indexOf('장비 추가') >= 0, 'select all blocked');

const keys = createTargetPicker({ customerId: 'C1' });
keys.setQuery('스위치');
keys.receiveHits([
  { id: 'A1', name: '하나', customer_id: 'C1' },
  { id: 'A2', name: '둘', customer_id: 'C1' }
]);
assert(keys.onKey('ArrowDown') === true, 'down prevent');
assert(keys.state.hi === 1, 'down moves');
assert(keys.onKey('Enter') === true && keys.state.draft.A2, 'enter selects');
assert(keys.onKey('Escape') === true && keys.state.open === false, 'esc closes');
keys.setQuery('스위치');
keys.receiveHits([{ id: 'A1', name: '하나', customer_id: 'C1' }]);
assert(keys.reclickBox() === true && keys.state.open === false, 'reclick closes');
keys.setQuery('스위치');
keys.receiveHits([{ id: 'A1', name: '하나', customer_id: 'C1' }]);
assert(keys.clickOutside() === true && keys.state.open === false, 'outside closes');

const none = createTargetPicker({ customerId: 'C-empty' });
none.setQuery('없는자산');
none.receiveHits([]);
assert(none.state.open === true && none.state.hits.length === 0, 'zero assets still opens');
assert(none.state.hint.indexOf('없습니다') >= 0, 'zero hint');
assert(none.apply() === true && none.state.applied.length === 0, 'apply none');

const moved = createTargetPicker({
  customerId: 'C1',
  selected: [{ id: 'A1', name: '기존' }]
});
moved.setCustomer('C2');
assert(moved.state.applied.length === 0, 'customer change clears');
moved.setCustomer('');
assert(moved.canSearch() === false && moved.state.hint.indexOf('거래처') >= 0, 'no customer');

assert(splitDefaultTitle('스위치 점검', 'SW-100') === '스위치 점검 - SW-100', 'asset title');
assert(splitDefaultTitle('현장 방문', '계룡고') === '현장 방문 - 계룡고', 'site title');
assert(splitDefaultTitle('  제목  ', '  자산  ') === '제목 - 자산', 'trim title');

console.log('target_picker_test ok');
