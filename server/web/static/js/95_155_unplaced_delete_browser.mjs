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

function dupBtn(page) {
  return page.locator('#unplanned [data-unplaced-dup]').first();
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const page = await loginPage(browser, { width: 1280, height: 800 });
  await page.goto(base + kanbanPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor({ timeout: 15000 });
  const n0 = await page.locator('#unplanned .board-pc').count();
  if (n0 < 5) fail('중복 5건 없음', n0);
  if (await page.locator('#unplanned [data-dup-badge]').count() === 0) fail('같은 업무 배지 없음');
  const src = await page.content();
  if (src.indexOf('window.confirm') >= 0 && /confirm\(/.test(await page.evaluate(() => document.querySelector('script[src*="unplaced_delete"]') ? 'has' : 'has'))) {
    /* script is external; check runtime */
  }
  const hasConfirm = await page.evaluate(() => {
    const s = Array.from(document.scripts).map((x) => x.src).join(' ');
    return s.indexOf('unplaced_delete') >= 0;
  });
  if (!hasConfirm) fail('unplaced_delete.js 없음');

  await dupBtn(page).click();
  await page.locator('#unplaced-del-mask:not([hidden])').waitFor({ timeout: 8000 });
  const focused = await page.evaluate(() => document.activeElement && document.activeElement.id);
  if (focused !== 'unplaced-del-cancel') fail('미리보기 취소 포커스', focused);
  await page.locator('#unplaced-del-ok').click();
  await page.waitForFunction(() => document.querySelectorAll('#unplanned .board-pc').length === 1, null, { timeout: 8000 });
  await page.screenshot({ path: path.join(shotDir, '95-155-kanban-after-dup.png') });

  await page.locator('#unplanned [data-unplaced-del]:not([disabled])').click();
  await page.locator('#unplaced-del-mask:not([hidden])').waitFor();
  const f2 = await page.evaluate(() => document.activeElement && document.activeElement.id);
  if (f2 !== 'unplaced-del-cancel') fail('단건 취소 포커스', f2);
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => {
    const m = document.getElementById('unplaced-del-mask');
    return m && m.hidden;
  });
  await page.locator('#unplanned [data-unplaced-del]:not([disabled])').click();
  await page.locator('#unplaced-del-ok').click();
  await page.waitForFunction(() => document.querySelectorAll('#unplanned .board-pc').length === 0, null, { timeout: 8000 });

  await page.goto(base + dayPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor({ timeout: 10000 });
  if (await page.locator('#unplanned [data-unplaced-bulk]').count() === 0) fail('일정표 선택 삭제 없음');
  await page.screenshot({ path: path.join(shotDir, '95-155-day.png') });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(base + kanbanPath, { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor();
  const overflow = await page.evaluate(() => {
    const box = document.getElementById('unplanned');
    if (!box) return 'no';
    return box.scrollWidth > box.clientWidth + 2 ? (box.scrollWidth + '>' + box.clientWidth) : '';
  });
  if (overflow) fail('390px 가로 넘침', overflow);
  await page.screenshot({ path: path.join(shotDir, '95-155-390.png') });
  console.log('95-155 browser ok');
} finally {
  await browser.close();
}
