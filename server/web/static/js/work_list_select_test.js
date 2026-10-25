const workListSelect = require('./work_list_select.js');

function box(value) {
  return { value: value, checked: false, className: 'js-wl-check' };
}

const a = box('t1');
const b = box('t2');
const all = { checked: false, indeterminate: false };
const root = {
  querySelectorAll(sel) {
    if (sel === '.js-wl-check') return [a, b];
    return [];
  },
  querySelector(sel) {
    if (sel === '.js-wl-all') return all;
    return null;
  }
};

const ctx = workListSelect('tok');
ctx.$root = root;
ctx.sync();
if (ctx.n !== 0) {
  console.error('초기 n=', ctx.n);
  process.exit(1);
}

a.checked = true;
b.checked = true;
ctx.sync();
if (ctx.n !== 2) {
  console.error('$root 체크 후 n=', ctx.n, '(this.$el 버그면 0)');
  process.exit(1);
}
if (!all.checked || all.indeterminate) {
  console.error('전체선택 상태', all);
  process.exit(1);
}

ctx.clear();
if (ctx.n !== 0 || a.checked || b.checked) {
  console.error('선택 해제 실패', ctx.n);
  process.exit(1);
}

ctx.toggleAll(true);
if (ctx.n !== 2 || !a.checked || !b.checked) {
  console.error('전체 선택 실패', ctx.n);
  process.exit(1);
}

const off = { value: 'x', checked: false, disabled: true, className: 'js-wl-check-off' };
const mixedRoot = {
  querySelectorAll(sel) {
    if (sel === '.js-wl-check') return [a, b];
    return [];
  },
  querySelector(sel) {
    if (sel === '.js-wl-all') return all;
    if (sel === '.js-wl-check-off') return off;
    return null;
  }
};
ctx.$root = mixedRoot;
ctx.toggleAll(true);
if (ctx.n !== 2) {
  console.error('비활성 행이 선택에 포함됨', ctx.n);
  process.exit(1);
}

const five = [box('a'), box('b'), box('c'), box('d'), box('e')];
five.forEach(function (x) { x.checked = true; });
const fiveRoot = {
  querySelectorAll(sel) {
    if (sel === '.js-wl-check') return five;
    return [];
  },
  querySelector() { return null; }
};

function jsonRes(status, body) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status: status,
    json: function () { return Promise.resolve(body); }
  });
}

global.document = {
  cookie: 'jp_csrf=livecookie01234567',
  querySelector: function (sel) {
    if (sel === 'meta[name="csrf-token"]') {
      return { getAttribute: function () { return 'metatoken012345678'; }, setAttribute: function () {} };
    }
    return null;
  },
  getElementById: function () { return fiveRoot; }
};

const flashes = [];
global.window = {
  showFlashError: function (m, reload) {
    flashes.push({ m: m, reload: reload });
  }
};

const p = workListSelect('stale');
p.$root = fiveRoot;
p.sync();
if (p.n !== 5) {
  console.error('5건 n=', p.n);
  process.exit(1);
}

let fetches = [];
global.fetch = function (url, opts) {
  fetches.push({ url: url, opts: opts });
  if (String(url).indexOf('/csrf') >= 0) {
    return jsonRes(200, { csrf: 'livecookie01234567' });
  }
  return jsonRes(403, { error: '보안 확인에 실패했습니다. 페이지를 새로고침한 뒤 다시 시도해 주세요.' });
};

p.preview();
function wait(ms) {
  return new Promise(function (r) { setTimeout(r, ms); });
}

wait(40).then(function () {
  if (p.modal) {
    console.error('403 인데 모달이 열림 delN=', p.delN);
    process.exit(1);
  }
  if (!p.lastError || p.lastError.indexOf('보안 확인') < 0) {
    console.error('오류 문구 없음', p.lastError, flashes);
    process.exit(1);
  }
  if (!fetches.length || fetches[0].opts.credentials !== 'same-origin') {
    console.error('credentials 없음', fetches[0]);
    process.exit(1);
  }

  flashes.length = 0;
  p.lastError = '';
  p.modal = false;
  fetches = [];
  global.fetch = function (url, opts) {
    fetches.push({ url: url, opts: opts });
    return jsonRes(200, {
      deletable: five.map(function (x) { return { id: x.value, title: x.value }; }),
      skipped: []
    });
  };
  p.preview();
  return wait(40);
}).then(function () {
  if (!p.modal || p.delN !== 5) {
    console.error('정상 미리보기 실패', p.modal, p.delN);
    process.exit(1);
  }
  var title = '선택 ' + p.n + '건 중 삭제 ' + p.delN + '건';
  if (title !== '선택 5건 중 삭제 5건') {
    console.error('제목', title);
    process.exit(1);
  }

  p.modal = false;
  p.deletable = [];
  p.skipped = [];
  p.delN = 0;
  p.lastError = '';
  flashes.length = 0;
  global.fetch = function () { return jsonRes(200, {}); };
  p.preview();
  return wait(40);
}).then(function () {
  if (p.modal || p.delN !== 0) {
    console.error('빈 객체인데 0건 모달', p.modal, p.delN);
    process.exit(1);
  }
  if (!p.lastError || p.lastError.indexOf('삭제 대상 ID') < 0) {
    console.error('빈 객체 오류 없음', p.lastError);
    process.exit(1);
  }

  p.modal = false;
  p.lastError = '';
  flashes.length = 0;
  global.fetch = function () { return Promise.reject(new Error('network')); };
  p.preview();
  return wait(40);
}).then(function () {
  if (p.modal) {
    console.error('네트워크 실패인데 모달');
    process.exit(1);
  }
  if (!p.lastError) {
    console.error('네트워크 오류 없음');
    process.exit(1);
  }

  p.modal = false;
  p.lastError = '';
  flashes.length = 0;
  fetches = [];
  global.fetch = function (url, opts) {
    fetches.push({ url: url, opts: opts });
    return jsonRes(400, { error: '삭제 대상 ID 가 비어 있습니다' });
  };
  p.preview();
  return wait(40);
}).then(function () {
  if (p.modal) {
    console.error('400 인데 모달');
    process.exit(1);
  }
  if (!p.lastError || p.lastError.indexOf('삭제 대상 ID') < 0) {
    console.error('400 메시지 없음', p.lastError);
    process.exit(1);
  }

  const blank = box('');
  blank.checked = true;
  const blankRoot = {
    querySelectorAll(sel) {
      if (sel === '.js-wl-check') return [blank];
      return [];
    },
    querySelector() { return null; }
  };
  p.$root = blankRoot;
  p.sync();
  p.modal = false;
  p.lastError = '';
  flashes.length = 0;
  fetches = [];
  p.preview();
  if (p.modal || fetches.length) {
    console.error('빈 value 가 전송됨', fetches.length, p.modal);
    process.exit(1);
  }
  if (!p.lastError || p.lastError.indexOf('삭제 대상') < 0) {
    console.error('빈 value 오류 없음', p.lastError);
    process.exit(1);
  }
  const asRoot = {
    id: 'as-list-select',
    querySelectorAll(sel) {
      if (sel === '.js-wl-check') return [a, b];
      return [];
    },
    querySelector(sel) {
      if (sel === '.js-wl-all') return all;
      return null;
    }
  };
  const asSel = workListSelect('as-tok');
  asSel.$root = asRoot;
  a.checked = true;
  b.checked = false;
  asSel.sync();
  if (asSel.n !== 1) {
    console.error('AS 목록 공용 JS n=', asSel.n);
    process.exit(1);
  }
  console.log('ok');
}).catch(function (e) {
  console.error(e);
  process.exit(1);
});
