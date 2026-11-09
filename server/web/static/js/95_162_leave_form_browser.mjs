import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const staffCookie = process.env.STAFF_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !staffCookie) {
  console.error('env');
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

async function open(browser, cookie, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  return { context, page };
}

async function noOverflow(page) {
  return page.evaluate(() => {
    const w = window.innerWidth;
    const html = document.documentElement.scrollWidth <= w + 8;
    const main = document.querySelector('main');
    const inner = !main || main.scrollWidth <= main.clientWidth + 8;
    return html && inner;
  });
}

async function goNew(page) {
  await page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const staff = await open(browser, staffCookie, { width: 1280, height: 800 });

  await goNew(staff.page);
  if (await staff.page.locator('input[name="period_mode"][type="radio"]').count()) fail('기간 라디오가 있다');
  await staff.page.locator('[data-days-start]').fill('2026-08-10');
  await staff.page.locator('#reason').fill('연차하루');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);

  await goNew(staff.page);
  await staff.page.locator('[data-days-start]').fill('2026-08-11');
  await staff.page.locator('[data-days-end]').fill('2026-08-12');
  await staff.page.locator('#reason').fill('연차여러날');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);
  const multi = await staff.page.locator('body').innerText();
  if (!multi.includes('2026-08-11') || !multi.includes('2026-08-12')) fail('여러 날', multi);

  await goNew(staff.page);
  await staff.page.locator('input[name="type_key"][value="half"]').check();
  await staff.page.locator('[data-half-start]').waitFor({ state: 'visible' });
  await staff.page.locator('[data-half-start]').fill('2026-08-13');
  await staff.page.locator('[data-half-time]').fill('09:00');
  await staff.page.locator('[data-half-time]').dispatchEvent('change');
  await staff.page.waitForFunction(() => {
    const el = document.querySelector('[data-half-end]');
    return el && el.value === '14:00';
  }, null, { timeout: 5000 });
  const am = await staff.page.locator('[data-half-part]').innerText();
  if (!am.includes('오전')) fail('오전 표시', am);
  await staff.page.locator('#reason').fill('반차오전');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);
  const halfAm = await staff.page.locator('body').innerText();
  if (!halfAm.includes('09:00') || !halfAm.includes('14:00')) fail('반차 09-14', halfAm);

  await goNew(staff.page);
  await staff.page.locator('input[name="type_key"][value="half"]').check();
  await staff.page.locator('[data-half-start]').waitFor({ state: 'visible' });
  await staff.page.locator('[data-half-start]').fill('2026-08-14');
  await staff.page.locator('[data-half-time]').fill('14:00');
  await staff.page.locator('[data-half-time]').dispatchEvent('change');
  await staff.page.waitForFunction(() => {
    const el = document.querySelector('[data-half-end]');
    return el && el.value === '18:00';
  }, null, { timeout: 5000 });
  const pm = await staff.page.locator('[data-half-part]').innerText();
  if (!pm.includes('오후')) fail('오후 표시', pm);
  await staff.page.locator('#reason').fill('반차오후');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);
  const halfPm = await staff.page.locator('body').innerText();
  if (!halfPm.includes('18:00')) fail('반차 14-18', halfPm);

  await goNew(staff.page);
  await staff.page.locator('input[name="type_key"][value="half"]').check();
  await staff.page.locator('[data-half-start]').waitFor({ state: 'visible' });
  await staff.page.locator('[data-half-start]').fill('2026-08-18');
  await staff.page.locator('[data-half-time]').fill('12:30');
  await staff.page.locator('[data-half-time]').dispatchEvent('change');
  await staff.page.waitForFunction(() => {
    const el = document.querySelector('[data-half-end]');
    return el && el.value === '17:00';
  }, null, { timeout: 5000 });
  await staff.page.locator('#reason').fill('반차점심보정');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);
  const lunch = await staff.page.locator('body').innerText();
  if (!lunch.includes('13:00') || !lunch.includes('17:00')) fail('12:30 보정', lunch);

  await goNew(staff.page);
  await staff.page.locator('input[name="type_key"][value="early_leave"]').check();
  await staff.page.locator('[data-time-start]').waitFor({ state: 'visible' });
  await staff.page.locator('[data-time-start]').fill('2026-08-19');
  await staff.page.locator('[data-time-from]').fill('11:00');
  await staff.page.locator('[data-time-to]').fill('14:00');
  await staff.page.locator('#reason').fill('조퇴점심');
  await Promise.all([
    staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    staff.page.getByRole('button', { name: '상신' }).click(),
  ]);

  await staff.page.screenshot({ path: path.join(shotDir, '95-162-desktop.png'), fullPage: true });
  const mobile = await open(browser, staffCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-162-390.png'), fullPage: true });
  console.log('95_162_leave_form_browser.mjs ok');
} finally {
  await browser.close();
}
