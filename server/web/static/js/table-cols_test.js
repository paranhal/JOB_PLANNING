const tc = require("./table-cols.js");

function store() {
  const mem = {};
  global.localStorage = {
    getItem(k) { return Object.prototype.hasOwnProperty.call(mem, k) ? mem[k] : null; },
    setItem(k, v) { mem[k] = String(v); },
    removeItem(k) { delete mem[k]; }
  };
  return mem;
}

store();

const pg = tc.mergeWidths("product_groups", 7, [40, 120, 120, 180, 180, 140, 130]);
if (pg.length !== 7 || pg[0] < 28) {
  console.error("product_groups 저장값 호환 실패", pg);
  process.exit(1);
}

const bad = tc.mergeWidths("product_groups", 7, [1, 2]);
if (bad.join(",") !== tc.DEFAULTS.product_groups.join(",")) {
  console.error("길이 불일치 시 기본값이어야 한다", bad);
  process.exit(1);
}

const legacy = [92, 300, 120, 120, 64, 120, 200, 64, 84, 100, 130];
const proj = tc.mergeWidths("projects", 11, legacy);
if (proj.join(",") !== legacy.join(",")) {
  console.error("projects 기존 키 호환 실패", proj);
  process.exit(1);
}

const auto = tc.mergeWidths("auto:/as#0", 4, null);
if (auto.length !== 4 || auto.some((w) => w !== 120)) {
  console.error("미지정 키 기본 120", auto);
  process.exit(1);
}

if (tc.sumPx([10, 20, 30]) !== 60) {
  console.error("열 폭 합");
  process.exit(1);
}

const table = {
  getAttribute(name) { return name === "data-table-cols" ? "contracts" : null; },
  classList: { add() {}, remove() {} },
  style: {},
  querySelector(sel) {
    if (sel === "colgroup") return { querySelectorAll() { return this._cols; }, _cols: cols };
    return null;
  },
  querySelectorAll(sel) {
    if (sel === "colgroup col") return cols;
    return [];
  }
};
const cols = [
  { style: {} }, { style: {} }, { style: {} }, { style: {} }, { style: {} },
  { style: {} }, { style: {} }, { style: {} }, { style: {} }, { style: {} }
];
table.querySelector = function (sel) {
  if (sel === "colgroup") {
    return { querySelectorAll: function (s) { return s === "col" ? cols : []; } };
  }
  return null;
};

tc.applyWidths(table, tc.DEFAULTS.contracts);
if (table.style.width !== "1390px") {
  console.error("표 너비는 열 합 px 이어야 한다", table.style.width);
  process.exit(1);
}
if (String(table.style.width).indexOf("%") >= 0) {
  console.error("width% 금지");
  process.exit(1);
}

const skipTable = {
  tagName: "TABLE",
  hasAttribute(n) { return n === "data-table-cols-skip"; },
  getAttribute(n) { return n === "data-table-cols-skip" ? "1" : null; },
  tHead: { rows: [{ cells: [{}, {}, {}] }] },
  querySelector() { return null; }
};
if (tc.isListTable(skipTable)) {
  console.error("skip 표는 적용하면 안 된다");
  process.exit(1);
}

tc.writeStore("contracts", [100, 100, 100, 100, 100, 100, 100, 100, 100, 100]);
const stored = tc.readStore("contracts");
if (!stored || stored[0] !== 100) {
  console.error("table_cols:contracts 저장 실패", stored);
  process.exit(1);
}

console.log("ok");
