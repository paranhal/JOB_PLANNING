import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day = process.env.WORK_DATE;
if (!base || !cookie || !day) {
  console.error('BASE_URL/AUTH_COOKIE/WORK_DATE 없음');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  const tries = [{ channel: 'chrome' }, {}];
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

const dayURL = base + '/workboard/board?view=day&date=' + day + '&auto=0&mine=0';
const browser = await launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, hasTouch: false });
const u = new URL(base);
await context.addCookies([
  { name: 'token', value: cookie, url: base },
  { name: 'token', value: cookie, domain: u.hostname, path: '/' },
]);
const page = await context.newPage();

async function openDay() {
  await page.goto(dayURL, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-board-bar][data-title="짧은바30"]');
}

await openDay();
const short = page.locator('[data-board-bar][data-title="짧은바30"]');
await short.hover();
const act = short.locator('a.board-bar-act').first();
await act.waitFor({ state: 'visible' });
const box = await act.boundingBox();
const hit = await page.evaluate(({ x, y }) => {
  const el = document.elementFromPoint(x, y);
  return el ? (el.className || '').toString() : '';
}, { x: box.x + box.width / 2, y: box.y + box.height / 2 });
if (hit.indexOf('board-bar-act') < 0) {
  console.error('조치 버튼 위에 다른 요소: ' + hit);
  process.exit(1);
}
await act.click();
await page.waitForTimeout(400);
if (!(page.url() || '').includes('/tasks/')) {
  console.error('조치 클릭 후 이동 없음 ' + page.url());
  process.exit(1);
}

await openDay();
const bar = page.locator('[data-board-bar][data-title="짧은바30"]');
const zone = page.locator('[data-dayzone]').first();
const zb = await zone.boundingBox();
const handle = bar.locator('[data-resize]');
const hb = await handle.boundingBox();
if (!hb || hb.height < 8) {
  console.error('손잡이 영역 없음 ' + JSON.stringify(hb));
  process.exit(1);
}
const y10 = zb.y + ((10 * 60 - 7 * 60) * 64) / 60;
await page.mouse.move(hb.x + hb.width / 2, hb.y + Math.max(1, hb.height / 2));
await page.mouse.down();
await page.mouse.move(hb.x + hb.width / 2, y10, { steps: 18 });
await page.mouse.up();
await page.waitForTimeout(900);
await openDay();
let st = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-start');
let en = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-end');
if (st !== '09:00' || en !== '10:00') {
  console.error('손잡이 09:30→10:00 실패 ' + st + '-' + en);
  process.exit(1);
}

const bar2 = page.locator('[data-board-bar][data-title="짧은바30"]');
const bb = await bar2.boundingBox();
await page.mouse.move(bb.x + bb.width / 2, bb.y + 10);
await page.mouse.down();
await page.mouse.move(bb.x + bb.width / 2, bb.y + 10 + 16, { steps: 10 });
await page.mouse.up();
await page.waitForTimeout(900);
await openDay();
st = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-start');
if (st !== '09:15') {
  console.error('세로 드래그 15분 스냅 실패 start=' + st);
  process.exit(1);
}

const overlapURL = await page.locator('[data-board-bar][data-title="짧은바30"]').evaluate(async (el) => {
  const form = document.getElementById('board-move-form');
  document.getElementById('board-m-kind').value = el.getAttribute('data-kind') || 'task';
  document.getElementById('board-m-ref').value = el.getAttribute('data-ref') || '';
  document.getElementById('board-m-title').value = el.getAttribute('data-title') || '';
  document.getElementById('board-m-assignee').value = el.getAttribute('data-assignee') || '';
  document.getElementById('board-m-from-col').value = '';
  document.getElementById('board-m-dur').value = '60';
  document.getElementById('board-m-date').value = el.getAttribute('data-date') || '';
  document.getElementById('board-m-start').value = '14:00';
  document.getElementById('board-m-end').value = '15:00';
  const body = new URLSearchParams();
  Array.from(form.elements).forEach(function (x) { if (x.name) body.set(x.name, x.value); });
  const res = await fetch(form.action, { method: 'POST', body: body, credentials: 'same-origin', redirect: 'manual' });
  return res.headers.get('Location') || String(res.status);
});
if (String(overlapURL).indexOf('err=') < 0 && String(overlapURL) !== '409') {
  console.error('겹침 거부 없음 ' + overlapURL);
  process.exit(1);
}
st = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-start');
if (st !== '09:15') {
  console.error('겹침 후 원위치 아님 ' + st);
  process.exit(1);
}

console.log('desktop-mouse-ok');
await browser.close();
