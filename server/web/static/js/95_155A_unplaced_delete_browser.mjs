import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
const kanbanPath = process.env.KANBAN_PATH || '/workboard/board?view=kanban&mine=0&auto=0';
const dayPath = process.env.DAY_PATH || '/workboard/board?view=day&mine=0&auto=0';
if (!base || !cookie) {
  console.error('BASE_URL/AUTH_COOKIE 없음');
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

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

async function loginPage(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  return context.newPage();
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const page = await loginPage(browser, { width: 1280, height: 800 });
  await page.goto(base + kanbanPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor({ timeout: 15000 });
  const n0 = await page.locator('#unplanned .board-pc').count();
  if (n0 !== 5) fail('부모1+하위4 아님', n0);
  const enabled = await page.locator('#unplanned [data-unplaced-del]:not([disabled])').count();
  if (enabled !== 5) fail('삭제 활성 수', enabled);
  const reasonN = await page.locator('#unplanned [data-del-reason]').count();
  if (reasonN) {
    const reason = await page.locator('#unplanned [data-del-reason]').first().textContent();
    if (reason && reason.indexOf('하위 업무가 있어') >= 0) fail('부모가 하위 때문에 막힘', reason);
  }
  const dup = page.locator('#unplanned [data-unplaced-dup]').first();
  const dupLabel = (await dup.textContent() || '').trim();
  if (dupLabel.indexOf('중복 4건 정리') < 0) fail('중복 4건 정리 없음', dupLabel);

  const parentCard = page.locator('#unplanned .board-pc').filter({ has: page.locator('[data-unplaced-dup]') });
  await parentCard.locator('[data-unplaced-del]').click();
  await page.locator('#unplaced-del-mask:not([hidden])').waitFor({ timeout: 8000 });
  await page.locator('#unplaced-del-ok').click();
  await page.waitForFunction(() => document.querySelectorAll('#unplanned .board-pc').length === 0, null, { timeout: 8000 });
  await page.screenshot({ path: path.join(shotDir, '95-155A-kanban-after-dup.png') });

  await page.goto(base + dayPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor({ timeout: 10000 });
  if (await page.locator('#unplanned [data-unplaced-bulk]').count() === 0) fail('일정표 선택 삭제 없음');
  await page.screenshot({ path: path.join(shotDir, '95-155A-day.png') });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(base + kanbanPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor();
  const overflow = await page.evaluate(() => {
    const box = document.getElementById('unplanned');
    if (!box) return 'no';
    return box.scrollWidth > box.clientWidth + 2 ? (box.scrollWidth + '>' + box.clientWidth) : '';
  });
  if (overflow) fail('390px 가로 넘침', overflow);
  await page.screenshot({ path: path.join(shotDir, '95-155A-390.png') });
  console.log('95-155A browser ok');
} finally {
  await browser.close();
}
