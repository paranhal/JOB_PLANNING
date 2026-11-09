import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const staffCookie = process.env.STAFF_COOKIE;
const headCookie = process.env.HEAD_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !staffCookie || !headCookie) {
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

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const staff = await open(browser, staffCookie, { width: 1280, height: 800 });
  await staff.page.goto(base + '/leaves', { waitUntil: 'domcontentloaded' });
  const remain0 = await staff.page.locator('[data-remain]').first().innerText();
  if (!remain0.includes('5일') && !remain0.includes('일')) fail('목록 잔여', remain0);

  await staff.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  await staff.page.locator('form[action="/leaves"]').evaluate((form) => {
    const set = (name, v) => { const el = form.querySelector('[name="' + name + '"]'); if (el) el.value = v; };
    const annual = form.querySelector('input[name="type_key"][value="annual"]');
    if (annual) annual.checked = true;
    const days = form.querySelector('input[name="period_mode"][value="days"]');
    if (days) days.checked = true;
    set('start_date', '2026-08-10');
    set('end_date', '2026-08-14');
    set('reason', '161B부족');
    const c = form.querySelector('[data-absence-consent]');
    if (c) c.checked = true;
    form.submit();
  });
  await staff.page.waitForURL(/\/leaves\/LR-/, { timeout: 15000, waitUntil: 'domcontentloaded' });
  const leaveURL = staff.page.url();

  const head = await open(browser, headCookie, { width: 1280, height: 800 });
  await head.page.goto(base + '/approvals/inbox', { waitUntil: 'domcontentloaded' });
  await Promise.all([
    head.page.waitForURL(/\/approvals\//, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    head.page.getByRole('link', { name: /근태허가원/ }).first().click(),
  ]);
  await Promise.all([
    head.page.waitForURL(/\/approvals\/inbox/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    head.page.getByRole('button', { name: '승인' }).click(),
  ]);

  await staff.page.goto(base + '/leaves', { waitUntil: 'domcontentloaded' });
  const remain1 = await staff.page.locator('[data-remain]').first().innerText();
  if (remain1.includes('5일')) fail('승인 후 잔여 그대로', remain1);

  await staff.page.goto(leaveURL, { waitUntil: 'domcontentloaded' });
  const cancel = staff.page.locator('[data-cancel-leave]');
  if (await cancel.count()) {
    staff.page.once('dialog', (d) => d.accept());
    await Promise.all([
      staff.page.waitForURL(/\/leaves/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
      cancel.click(),
    ]);
  }
  await staff.page.goto(base + '/leaves', { waitUntil: 'domcontentloaded' });
  const remain2 = await staff.page.locator('[data-remain]').first().innerText();
  if (!remain2.includes('5일') && !remain2.includes('일')) fail('복원 잔여', remain2);
  await staff.page.screenshot({ path: path.join(shotDir, '95-161B-desktop.png'), fullPage: true });

  const mobile = await open(browser, staffCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/leaves', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침 목록');
  await mobile.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침 작성');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-161B-390.png'), fullPage: true });
  console.log('95_161b_leave_remain_browser.mjs ok');
} finally {
  await browser.close();
}
