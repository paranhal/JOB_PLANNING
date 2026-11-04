const rz = require("./mnt_calendar_resize.js");

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

assert(rz.clamp(100, 1200, 8) === rz.MIN_W, "최소 240");
assert(rz.clamp(900, 1200, 8) <= Math.min(rz.MAX_W, 1200 * 0.5), "최대");
assert(rz.clamp(400, 1200, 8) === 400, "범위 안");
const byCal = rz.clamp(500, 1400, 8);
assert(byCal <= 1400 - rz.CAL_MIN - 8, "달력 최소 560 보장");
assert(byCal >= rz.MIN_W, "패널 최소 240");

const mem = {};
global.localStorage = {
  getItem(k) { if (!Object.prototype.hasOwnProperty.call(mem, k)) throw new Error("boom"); return mem[k]; },
  setItem(k, v) { throw new Error("boom"); }
};
assert(rz.readStore() === null, "localStorage 예외 읽기");
rz.writeStore(300);

global.localStorage = {
  getItem(k) { return Object.prototype.hasOwnProperty.call(mem, k) ? mem[k] : null; },
  setItem(k, v) { mem[k] = String(v); }
};
rz.writeStore(320);
assert(rz.readStore() === 320, "저장");

const stepped = rz.clamp(285 + rz.STEP, 1400, 8);
assert(stepped === 285 + rz.STEP, "키보드 16px");
const shifted = rz.clamp(285 + rz.STEP_SHIFT, 1400, 8);
assert(shifted === 285 + rz.STEP_SHIFT, "Shift 64px");
assert(rz.clamp(rz.DEFAULT_W, 1400, 8) === rz.DEFAULT_W, "더블클릭 기본폭");

console.log("mnt_calendar_resize_test.js ok");
