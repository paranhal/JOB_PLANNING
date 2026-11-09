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

async function approverText(page) {
  await page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  return page.locator('[data-leave-approver]').innerText();
}

async function saveHead(page, value) {
  await page.goto(base + '/admin/approval-settings', { waitUntil: 'domcontentloaded' });
  await page.locator('#org-head-user').selectOption(value);
  await Promise.all([
    page.waitForURL(/\/admin\/approval-settings/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: '팀장 설정 저장' }).click(),
  ]);
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const staff = await open(browser, staffCookie, { width: 1280, height: 800 });
  const head = await open(browser, headCookie, { width: 1280, height: 800 });

  let txt = await approverText(staff.page);
  if (!txt.includes('팀장')) fail('초기 결재자 팀장', txt);
  if (!(await staff.page.locator('[data-leave-remain]').count())) fail('잔여 칸 없음');

  await saveHead(head.page, '');
  txt = await approverText(staff.page);
  if (!txt.includes('가관리자')) fail('해제 후 조직관리자', txt);

  await saveHead(head.page, { label: '팀장' });
  txt = await approverText(staff.page);
  if (!txt.includes('팀장')) fail('재지정 후 팀장', txt);

  await staff.page.screenshot({ path: path.join(shotDir, '95-159-desktop.png'), fullPage: true });
  const mobile = await open(browser, staffCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/leaves/new', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-159-390.png'), fullPage: true });
  console.log('95_159_leave_approver_browser.mjs ok');
} finally {
  await browser.close();
}
