import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const plan = process.env.PLAN_ID;
const openID = process.env.OPEN_VISIT;
const doneID = process.env.DONE_VISIT;
if (!base || !cookie || !plan) {
  console.error("BASE_URL/AUTH_COOKIE/PLAN_ID 없음");
  process.exit(1);
}

async function launch() {
  const { chromium } = require("playwright");
  const tries = [{ channel: "chrome" }, {}];
  let last = null;
  for (const extra of tries) {
    try {
      return await chromium.launch(Object.assign({ headless: true }, extra));
    } catch (e) {
      last = e;
    }
  }
  throw last;
}

function fail(msg, extra) {
  console.error(msg, extra || "");
  process.exit(1);
}

async function html5Drop(page, visitId, date) {
  const info = await page.evaluate(async ({ visitId, date }) => {
    const chip = document.querySelector('#mnt-cal-grid [data-visit-id="' + visitId + '"]');
    const cell = document.querySelector('#mnt-cal-grid .mnt-cal-cell[data-date="' + date + '"]');
    if (!chip || !cell) return { ok: false, why: "no-el" };
    const from = chip.getAttribute("data-date") || "";
    if (!window.MntCalendarDnd || !window.MntCalendarDnd.commitDrop) {
      return { ok: false, why: "no-api", hrefs: [...document.scripts].map((s) => s.src) };
    }
    const r = await window.MntCalendarDnd.commitDrop(visitId, date, from);
    return {
      ok: !!(r && (r.ok || r.skipped || r.locked)),
      via: "commitDrop",
      draggable: chip.getAttribute("draggable"),
      from,
      r,
    };
  }, { visitId, date });
  if (!info || !info.ok) fail("html5Drop 실패", JSON.stringify(info));
}

const browser = await launch();
try {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const u = new URL(base);
  await context.addCookies([
    { name: "token", value: cookie, url: base },
    { name: "token", value: cookie, domain: u.hostname, path: "/" },
  ]);
  const page = await context.newPage();
  const cal = base + "/maintenance/" + plan + "?view=calendar&month=8";
  await page.goto(cal, { waitUntil: "domcontentloaded" });
  await page.evaluate(() => {
    const x = document.querySelector("#login-briefing-modal .brief-x");
    if (x) x.click();
  });
  await page.waitForSelector("#mnt-cal-grid [data-visit-id]", { timeout: 15000 });
  const openFromDom = await page.locator('#mnt-cal-grid [data-date="2026-08-12"][draggable="true"]').first().getAttribute("data-visit-id");
  const doneFromDom = await page.locator('#mnt-cal-grid [data-locked="1"]').first().getAttribute("data-visit-id");
  const openVid = openFromDom || openID;
  const doneVid = doneFromDom || doneID;
  if (!openVid) fail("미완료 칩 없음");

  const dest = page.locator('#mnt-cal-grid .mnt-cal-cell[data-date="2026-08-14"]');
  await html5Drop(page, openVid, "2026-08-14");
  await page.waitForTimeout(800);

  const afterDrag = await page.locator('#mnt-cal-grid .mnt-cal-cell[data-date="2026-08-14"] [data-visit-id="' + openVid + '"]').count();
  if (!afterDrag) fail("T13 dragTo 후 칸에 칩 없음");

  await page.reload({ waitUntil: "domcontentloaded" });
  await page.evaluate(() => {
    const x = document.querySelector("#login-briefing-modal .brief-x");
    if (x) x.click();
  });
  await page.waitForSelector('#mnt-cal-grid [data-visit-id="' + openVid + '"]', { timeout: 15000 });
  const afterReload = await page.locator('#mnt-cal-grid .mnt-cal-cell[data-date="2026-08-14"] [data-visit-id="' + openVid + '"]').count();
  if (!afterReload) fail("T13 새로고침 후 날짜 유지 안 됨");

  const undo = page.locator("#mnt-cal-toast-host button");
  if (await undo.count()) {
    await undo.click();
    await page.waitForTimeout(600);
  } else {
    await html5Drop(page, openVid, "2026-08-12");
    await page.waitForTimeout(600);
  }
  const undone = await page.locator('#mnt-cal-grid .mnt-cal-cell[data-date="2026-08-12"] [data-visit-id="' + openVid + '"]').count();
  if (!undone) fail("T14 되돌리기 실패");

  const doneChip = page.locator('#mnt-cal-grid [data-visit-id="' + doneVid + '"]');
  const beforeDone = await doneChip.getAttribute("data-date");
  await html5Drop(page, doneVid, "2026-08-20");
  await page.waitForTimeout(400);
  const afterDone = await doneChip.getAttribute("data-date");
  if (afterDone !== beforeDone) fail("T15 완료 칩이 이동함", afterDone);

  const split = page.locator("#mnt-cal-splitter");
  const panel = page.locator("#mnt-day-panel");
  const box0 = await panel.boundingBox();
  const s0 = await split.boundingBox();
  if (!box0 || !s0) fail("스플리터/패널 없음");
  await page.mouse.move(s0.x + s0.width / 2, s0.y + 40);
  await page.mouse.down();
  await page.mouse.move(s0.x - 80, s0.y + 40);
  await page.mouse.up();
  await page.waitForTimeout(200);
  const box1 = await panel.boundingBox();
  if (!box1 || Math.abs(box1.width - box0.width) < 8) fail("T16 폭이 안 바뀜", box0.width + " -> " + (box1 && box1.width));
  const w1 = box1.width;
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.waitForSelector("#mnt-day-panel");
  const box2 = await page.locator("#mnt-day-panel").boundingBox();
  if (!box2 || Math.abs(box2.width - w1) > 12) fail("T16 새로고침 후 폭 유지 실패", box2 && box2.width);
  await page.locator("#mnt-cal-splitter").dblclick();
  await page.waitForTimeout(150);
  const box3 = await page.locator("#mnt-day-panel").boundingBox();
  if (!box3 || Math.abs(box3.width - 285) > 20) fail("T16 더블클릭 복원 실패", box3 && box3.width);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload({ waitUntil: "domcontentloaded" });
  const vis = await page.evaluate(() => {
    const s = document.getElementById("mnt-cal-splitter");
    const st = s ? getComputedStyle(s) : null;
    return {
      display: st && st.display,
      overflow: document.documentElement.scrollWidth - window.innerWidth,
    };
  });
  if (vis.display && vis.display !== "none") fail("T16 모바일에서 스플리터 보임", vis.display);
  if (vis.overflow > 2) fail("T16 가로 스크롤", vis.overflow);

  await context.close();
} finally {
  await browser.close();
}
