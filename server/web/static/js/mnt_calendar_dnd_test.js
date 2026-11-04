const dnd = require("./mnt_calendar_dnd.js");

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

assert(dnd.sameDate("2026-08-12", "2026-08-12") === true, "same date");
assert(dnd.sameDate("2026-08-12", "2026-08-13") === false, "other date");
assert(dnd.shouldPost("2026-08-12", "2026-08-12") === false, "같은 날짜 요청 없음");
assert(dnd.shouldPost("2026-08-12", "2026-08-13") === true, "다른 날짜 요청");

const locked = { getAttribute(k) { return k === "data-locked" ? "1" : (k === "draggable" ? "true" : ""); } };
assert(dnd.canStartDrag(locked) === false, "완료 칩 끌기 시작 안 됨");

const okChip = { getAttribute(k) { return k === "draggable" ? "true" : ""; } };
assert(dnd.canStartDrag(okChip) === true, "미완료 칩 끌기");

const dest = {
  kids: [],
  attrs: { "data-date": "2026-08-14" },
  getAttribute(k) { return this.attrs[k]; },
  querySelector() { return this; },
  insertBefore(n) { this.kids.unshift(n); },
  appendChild(n) { this.kids.push(n); }
};
const src = {
  kids: [],
  attrs: { "data-date": "2026-08-12" },
  getAttribute(k) { return this.attrs[k]; },
  querySelector() { return this; },
  insertBefore(n) { this.kids.unshift(n); },
  appendChild(n) { this.kids.push(n); }
};
const chip = { attrs: { "data-date": "2026-08-12" }, setAttribute(k, v) { this.attrs[k] = v; } };
src.kids.push(chip);
dnd.applyOptimistic(chip, dest);
assert(dest.kids[0] === chip, "낙관적 이동");
assert(chip.attrs["data-date"] === "2026-08-14", "낙관적 날짜");
dnd.revertChip(chip, src);
assert(src.kids.indexOf(chip) >= 0, "실패 시 원위치");
assert(chip.attrs["data-date"] === "2026-08-12", "원날짜 복원");

console.log("mnt_calendar_dnd_test.js ok");
