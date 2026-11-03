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
await page.route('**/*', function (route) {
  const headers = Object.assign({}, route.request().headers(), { cookie: 'token=' + cookie });
  route.continue({ headers: headers });
});

async function openDay() {
  await page.goto(dayURL, { waitUntil: 'domcontentloaded' });
  try {
    await page.waitForSelector('[data-board-bar][data-title="짧은바30"]', { state: 'attached', timeout: 15000 });
  } catch (err) {
    const html = await page.content();
    console.error('url ' + page.url());
    console.error(html.slice(0, 2000));
    throw err;
  }
  const bound = await page.locator('#board-root').getAttribute('data-board-sched-bound');
  if (bound !== '1') {
    const info = await page.evaluate(() => ({
      url: location.href,
      scripts: Array.from(document.scripts).map(function (s) { return s.src; }),
      form: !!document.getElementById('board-move-form'),
      bound: document.getElementById('board-root') && document.getElementById('board-root').getAttribute('data-board-sched-bound'),
    }));
    console.error('board_schedule 미바인딩 ' + JSON.stringify(info));
    process.exit(1);
  }
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
const zb2 = await zone.boundingBox();
const y915 = zb2.y + ((9 * 60 + 15 - 7 * 60) * 64) / 60;
await page.mouse.move(bb.x + Math.min(24, bb.width / 2), bb.y + 8);
await page.mouse.down();
await page.mouse.move(bb.x + Math.min(24, bb.width / 2), y915, { steps: 12 });
await page.mouse.up();
await page.waitForTimeout(900);
await openDay();
st = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-start');
if (st !== '09:15') {
  console.error('세로 드래그 15분 스냅 실패 start=' + st);
  process.exit(1);
}

const overlapFields = await page.locator('[data-board-bar][data-title="짧은바30"]').evaluate((el) => {
  const form = document.getElementById('board-move-form');
  return {
    action: form ? form.action : '/workboard/schedule',
    kind: el.getAttribute('data-kind') || 'task',
    ref_id: el.getAttribute('data-ref') || '',
    title: el.getAttribute('data-title') || '',
    assignee: el.getAttribute('data-assignee') || '',
    work_date: el.getAttribute('data-date') || '',
    redirect: form ? (form.querySelector('[name=redirect]') || {}).value : '',
  };
});
const overlapRes = await page.request.post(overlapFields.action, {
  form: {
    kind: overlapFields.kind,
    ref_id: overlapFields.ref_id,
    title: overlapFields.title,
    assignee: overlapFields.assignee,
    assignee_from_column: '',
    duration_min: '60',
    work_date: overlapFields.work_date,
    start_time: '14:00',
    end_time: '15:00',
    redirect: overlapFields.redirect || dayURL,
  },
  headers: { Cookie: 'token=' + cookie },
  maxRedirects: 0,
});
const overlapURL = overlapRes.headers()['location'] || String(overlapRes.status());
if (String(overlapURL).indexOf('err=') < 0 && String(overlapURL) !== '409') {
  console.error('겹침 거부 없음 ' + overlapURL);
  process.exit(1);
}
st = await page.locator('[data-board-bar][data-title="짧은바30"]').getAttribute('data-start');
if (st !== '09:15') {
  console.error('겹침 후 원위치 아님 ' + st);
  process.exit(1);
}

const doneBar = page.locator('[data-board-bar][data-title="완료고정"]');
if (await doneBar.count()) {
  const can = await doneBar.getAttribute('data-can-move');
  const rz = await doneBar.getAttribute('data-can-resize');
  if (can === '1' || rz === '1') {
    console.error('완료 바가 이동·손잡이 가능');
    process.exit(1);
  }
}

const oaCol = page.locator('[data-dayzone][data-assignee]').first();
const otherCol = page.locator('[data-dayzone][data-assignee]').nth(1);
if (await otherCol.count()) {
  const fromName = await oaCol.getAttribute('data-assignee');
  const toName = await otherCol.getAttribute('data-assignee');
  if (fromName && toName && fromName !== toName) {
    const src = page.locator('[data-board-bar][data-title="짧은바30"]');
    const sb = await src.boundingBox();
    const tb = await otherCol.boundingBox();
    await page.mouse.move(sb.x + sb.width / 2, sb.y + 12);
    await page.mouse.down();
    await page.mouse.move(tb.x + Math.min(40, tb.width / 2), tb.y + ((9 * 60 + 15 - 7 * 60) * 64) / 60, { steps: 16 });
    await page.mouse.up();
    await page.waitForTimeout(900);
    await openDay();
    const moved = page.locator('[data-board-bar][data-title="짧은바30"]');
    const asg = await moved.getAttribute('data-assignee');
    if (asg !== toName) {
      console.error('담당자 열 이동 실패 ' + asg + ' want ' + toName);
      process.exit(1);
    }
  }
}

const weekURL = base + '/workboard/board?view=week&date=' + day + '&auto=0&mine=0';
await page.goto(weekURL, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('#board-root[data-board-sched-bound="1"]', { state: 'attached' });
const weekBar = page.locator('[data-board-bar][data-title="긴바60"]').first();
const dragAttr = await weekBar.getAttribute('draggable');
if (dragAttr !== 'true') {
  console.error('주 보기 HTML5 draggable 없음 ' + dragAttr);
  process.exit(1);
}

const kanURL = base + '/workboard/board?view=kanban&date=' + day + '&auto=0&mine=0';
await page.goto(kanURL, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('[data-kanban-bound="1"]', { timeout: 10000 });

console.log('desktop-mouse-ok');
await browser.close();
