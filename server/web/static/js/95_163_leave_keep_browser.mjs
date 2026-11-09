import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const staffCookie = process.env.STAFF_COOKIE;
const headID = process.env.HEAD_ID || '';
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
  const page = staff.page;

  await goNew(page);
  await page.locator('[data-days-start]').fill('2026-08-10');
  await page.locator('#reason').fill('유지사유');
  await page.locator('[name="destination"]').fill('현장A');
  await page.locator('[name="contact_phone"]').fill('010-1111-2222');
  if (headID) await page.locator('[name="deputy_user_id"]').selectOption(headID);
  await Promise.all([
    page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: '상신' }).click(),
  ]);

  await goNew(page);
  await page.locator('[data-days-start]').fill('2026-08-10');
  await page.locator('#reason').fill('유지사유');
  await page.locator('[name="destination"]').fill('현장A');
  await page.locator('[name="contact_phone"]').fill('010-1111-2222');
  if (headID) await page.locator('[name="deputy_user_id"]').selectOption(headID);
  await page.getByRole('button', { name: '상신' }).click();
  await page.waitForSelector('[data-form-error]', { timeout: 8000 });
  if (page.url().includes('/leaves/new?err=')) fail('redirect err');
  const err1 = await page.locator('[data-form-error]').innerText();
  if (!err1.includes('이미 신청')) fail('겹침 문구', err1);
  if ((await page.locator('[data-days-start]').inputValue()) !== '2026-08-10') fail('날짜 유지');
  if ((await page.locator('#reason').inputValue()) !== '유지사유') fail('사유 유지');
  if ((await page.locator('[name="destination"]').inputValue()) !== '현장A') fail('행선지 유지');
  await page.waitForFunction(() => {
    const days = document.querySelector('[data-period-days]');
    const half = document.querySelector('[data-period-half]');
    return days && days.offsetParent && (!half || !half.offsetParent);
  }, null, { timeout: 5000 });
  await page.locator('[data-days-start]').fill('2026-08-14');
  await page.locator('[data-days-end]').fill('');
  await page.getByRole('button', { name: '상신' }).click();
  try {
    await page.waitForURL(/\/leaves\/LR-/, { timeout: 8000, waitUntil: 'domcontentloaded' });
  } catch (e) {
    const err = await page.locator('[data-form-error]').innerText().catch(() => '');
    const st = await page.locator('[data-days-start]').inputValue().catch(() => '');
    fail('재상신 실패 ' + page.url() + ' start=' + st + ' err=' + err);
  }

  await goNew(page);
  await page.locator('input[name="type_key"][value="half"]').check();
  await page.locator('[data-half-start]').waitFor({ state: 'visible' });
  await page.locator('[data-half-start]').fill('2026-08-12');
  await page.locator('[data-half-time]').fill('08:00');
  await page.locator('[data-half-time]').dispatchEvent('change');
  await page.locator('#reason').fill('반차유지');
  await page.getByRole('button', { name: '상신' }).click();
  await page.waitForSelector('[data-form-error]', { timeout: 8000 });
  if (!(await page.locator('input[name="type_key"][value="half"]').isChecked())) fail('반차 선택');
  if ((await page.locator('[data-half-start]').inputValue()) !== '2026-08-12') fail('반차 날짜');
  await page.locator('[data-half-time]').fill('09:00');
  await page.locator('[data-half-time]').dispatchEvent('change');
  await page.waitForFunction(() => {
    const el = document.querySelector('[data-half-end]');
    return el && el.value === '14:00';
  }, null, { timeout: 5000 });
  await Promise.all([
    page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: '상신' }).click(),
  ]);

  await goNew(page);
  await page.locator('input[name="type_key"][value="early_leave"]').check();
  await page.locator('[data-time-start]').waitFor({ state: 'visible' });
  await page.locator('[data-time-start]').fill('2026-08-13');
  await page.locator('[data-time-from]').fill('14:00');
  await page.locator('[data-time-to]').fill('11:00');
  await page.locator('#reason').fill('조퇴유지');
  await page.getByRole('button', { name: '상신' }).click();
  await page.waitForSelector('[data-form-error]', { timeout: 8000 });
  if ((await page.locator('[data-time-from]').inputValue()) !== '14:00') fail('조퇴 시작');
  if ((await page.locator('[data-time-to]').inputValue()) !== '11:00') fail('조퇴 종료');
  await page.locator('[data-time-from]').fill('11:00');
  await page.locator('[data-time-to]').fill('14:00');
  await Promise.all([
    page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: '상신' }).click(),
  ]);

  await page.screenshot({ path: path.join(shotDir, '95-163-desktop.png'), fullPage: true });
  const mobile = await open(browser, staffCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-163-390.png'), fullPage: true });
  console.log('95_163_leave_keep_browser.mjs ok');
} finally {
  await browser.close();
}
