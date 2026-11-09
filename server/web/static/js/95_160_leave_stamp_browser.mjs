import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const staffCookie = process.env.STAFF_COOKIE;
const headCookie = process.env.HEAD_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !staffCookie || !headCookie) {
  console.error('BASE_URL/STAFF_COOKIE/HEAD_COOKIE 없음');
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

const atRe = /\d{4}-\d{2}-\d{2} \d{2}:\d{2}/;
const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const staff = await open(browser, staffCookie, { width: 1280, height: 800 });
  await staff.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  await staff.page.locator('form[action="/leaves"]').evaluate((form) => {
    const set = (name, v) => { const el = form.querySelector('[name="' + name + '"]'); if (el) el.value = v; };
    const annual = form.querySelector('input[name="type_key"][value="annual"]');
    if (annual) annual.checked = true;
    const days = form.querySelector('input[name="period_mode"][value="days"]');
    if (days) days.checked = true;
    set('start_date', '2026-11-20');
    set('end_date', '2026-11-20');
    set('reason', '160브라우저결재일시');
    form.submit();
  });
  await staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' });
  const leaveURL = staff.page.url();
  await staff.page.locator('[data-stamps]').waitFor({ timeout: 10000 });
  const pending = await staff.page.locator('[data-stamps]').innerText();
  if (!pending.includes('승인 공란') && !pending.includes('공란')) {
    fail('상신 후 승인 공란 없음', pending);
  }
  await staff.page.screenshot({ path: path.join(shotDir, '95-160-submit.png'), fullPage: true });

  const head = await open(browser, headCookie, { width: 1280, height: 800 });
  await head.page.goto(base + '/approvals/inbox', { waitUntil: 'domcontentloaded' });
  await head.page.getByRole('link', { name: /근태허가원/ }).first().click();
  await Promise.all([
    head.page.waitForURL(/\/approvals\/inbox/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    head.page.getByRole('button', { name: '승인' }).click(),
  ]);

  await staff.page.goto(leaveURL, { waitUntil: 'domcontentloaded' });
  await staff.page.locator('[data-stamp-at]').first().waitFor({ timeout: 10000 });
  const stamps = await staff.page.locator('[data-stamps]').innerText();
  if (!atRe.test(stamps)) fail('승인 일시 없음', stamps);
  if (stamps.includes('승인 공란')) fail('승인 후에도 공란', stamps);
  await staff.page.screenshot({ path: path.join(shotDir, '95-160-desktop.png'), fullPage: true });

  const mobile = await open(browser, staffCookie, { width: 390, height: 844 });
  await mobile.page.goto(leaveURL, { waitUntil: 'domcontentloaded' });
  await mobile.page.locator('[data-stamp-at]').first().waitFor({ timeout: 10000 });
  const mtxt = await mobile.page.locator('[data-stamps]').innerText();
  if (!atRe.test(mtxt)) fail('390px 일시 없음', mtxt);
  await mobile.page.screenshot({ path: path.join(shotDir, '95-160-390.png'), fullPage: true });
  console.log('95_160_leave_stamp_browser.mjs ok');
} finally {
  await browser.close();
}
