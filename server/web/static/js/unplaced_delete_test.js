const workListSelect = require('./work_list_select.js');
const { bindUnplacedDelete, REASON } = require('./unplaced_delete.js');
const fs = require('fs');
const path = require('path');

const src = fs.readFileSync(path.join(__dirname, 'unplaced_delete.js'), 'utf8');
if (src.indexOf('confirm(') >= 0) {
  console.error('confirm() 사용');
  process.exit(1);
}
if (REASON !== '미배치 업무 삭제') {
  console.error('사유', REASON);
  process.exit(1);
}

function el(tag, attrs) {
  const o = {
    tag: tag,
    attrs: attrs || {},
    children: [],
    hidden: false,
    disabled: false,
    textContent: '',
    className: '',
    classList: { add: function () {}, remove: function () {}, toggle: function () {} },
    listeners: {},
    getAttribute: function (k) { return this.attrs[k] || ''; },
    setAttribute: function (k, v) { this.attrs[k] = v; },
    querySelector: function (sel) {
      if (sel === '[data-unplaced-bulk]') return this.bulk;
      if (sel === '#unplaced-status') return this.status;
      if (sel === '.js-wl-all') return this.all;
      if (sel === '.board-plist') return this.list;
      if (sel === '.board-pempty') return null;
      if (sel === '.board-pc') return this.cards[0] || null;
      return null;
    },
    querySelectorAll: function (sel) {
      if (sel === '.js-wl-check') return this.checks || [];
      if (sel === '.board-pc') return this.cards || [];
      return [];
    },
    addEventListener: function (type, fn) {
      this.listeners[type] = this.listeners[type] || [];
      this.listeners[type].push(fn);
    },
    closest: function () { return this; },
    remove: function () { this.removed = true; this.parent = null; },
    appendChild: function (c) { this.children.push(c); },
    focus: function () { o._focused = this; document.activeElement = this; }
  };
  return o;
}

const cancel = el('button');
cancel.focus = function () { document.activeElement = cancel; focused.push('cancel'); };
const ok = el('button');
const mask = el('div');
mask.querySelector = function (sel) {
  if (sel === '#unplaced-del-title') return el('h3');
  if (sel === '#unplaced-del-meta') return el('p');
  if (sel === '#unplaced-del-preview') return el('div');
  if (sel === '#unplaced-del-err') return el('p');
  if (sel === '#unplaced-del-cancel') return cancel;
  if (sel === '#unplaced-del-ok') return ok;
  return null;
};

const check = { value: 'task:A', checked: true, className: 'js-wl-check' };
const delBtn = el('button', { 'data-unplaced-del': 'task:A' });
delBtn.closest = function (sel) {
  if (sel === '[data-unplaced-del]') return delBtn;
  if (sel === '.board-pc') return card;
  return null;
};
const card = el('div', { 'data-delete-key': 'task:A', 'data-title': '제목', 'data-assignee': '담당', 'data-customer': '고객' });
card.querySelector = function () { return null; };
const focused = [];
delBtn.focus = function () { document.activeElement = delBtn; focused.push('del'); };

const box = el('aside');
box.checks = [check];
box.cards = [card];
box.bulk = el('button', { 'data-unplaced-bulk': '1' });
box.status = el('p');
box.all = { checked: false, indeterminate: false };
box.list = el('div');
card.remove = function () { box.cards = box.cards.filter(function (c) { return c !== card; }); };

global.document = {
  cookie: 'jp_csrf=tok1234567890123456',
  activeElement: null,
  querySelector: function (sel) {
    if (sel === 'meta[name="csrf-token"]') {
      return { getAttribute: function () { return 'tok1234567890123456'; }, setAttribute: function () {} };
    }
    return null;
  },
  addEventListener: function (type, fn) { global.document._key = fn; },
  createElement: function () { return el('div'); }
};
global.window = {};
global.workListSelect = workListSelect;

let fetchCalls = [];
global.fetch = function (url, opts) {
  fetchCalls.push({ url: url, opts: opts });
  if (String(url).indexOf('/preview') >= 0) {
    return Promise.resolve({
      ok: true, status: 200,
      json: function () { return Promise.resolve({ deletable: [{ id: 'task:A', title: '제목' }], skipped: [] }); }
    });
  }
  return Promise.resolve({
    ok: true, status: 200,
    json: function () { return Promise.resolve({ deleted: ['task:A'], skipped: [] }); }
  });
};

const ctl = bindUnplacedDelete(box, { workListSelect: workListSelect, mask: mask, cancelBtn: cancel, okBtn: ok, csrf: 'tok1234567890123456' });
if (!ctl) {
  console.error('bind 실패');
  process.exit(1);
}

box.listeners.click[0]({ target: delBtn, preventDefault: function () {}, closest: function (sel) { return delBtn.closest(sel); } });
if (document.activeElement !== cancel) {
  console.error('열릴 때 취소 포커스 아님', document.activeElement);
  process.exit(1);
}
if (mask.hidden) {
  console.error('대화상자 안 열림');
  process.exit(1);
}
global.document._key({ key: 'Escape', preventDefault: function () {} });
if (!mask.hidden) {
  console.error('Esc 미닫힘');
  process.exit(1);
}
if (document.activeElement !== delBtn) {
  console.error('닫은 뒤 삭제 버튼 포커스 아님');
  process.exit(1);
}

box.listeners.click[0]({ target: delBtn, preventDefault: function () {}, closest: function (sel) { return delBtn.closest(sel); } });
ok.listeners.click[0]();
function wait(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }
wait(40).then(function () {
  if (box.cards.length !== 0) {
    console.error('카드 미제거', box.cards.length);
    process.exit(1);
  }
  if (!box.status.textContent || box.status.textContent.indexOf('삭제') < 0) {
    console.error('status 없음', box.status.textContent);
    process.exit(1);
  }
  const del = fetchCalls.filter(function (c) { return c.url.indexOf('/bulk-delete') >= 0 && c.url.indexOf('preview') < 0; })[0];
  if (!del || String(del.opts.body).indexOf(encodeURIComponent(REASON)) < 0) {
    console.error('reason 없음', del);
    process.exit(1);
  }
  console.log('unplaced_delete_test ok');
});
