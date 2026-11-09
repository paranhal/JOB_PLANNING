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
  const st = await staff.page.goto(base + '/leaves/team', { waitUntil: 'domcontentloaded' });
  if (st && st.status() === 200 && (await staff.page.locator('[data-team-leave]').count())) fail('직원이 팀 휴가 열람');
  const sa = await staff.page.goto(base + '/admin/leaves', { waitUntil: 'domcontentloaded' });
  if (sa && sa.status() === 200 && (await staff.page.locator('[data-members]').count())) fail('직원이 휴가 관리 열람');

  const head = await open(browser, headCookie, { width: 1280, height: 800 });
  await head.page.goto(base + '/leaves/team', { waitUntil: 'domcontentloaded' });
  const teamTxt = await head.page.locator('[data-team-leave]').innerText();
  if (!teamTxt.includes('직원') && !teamTxt.includes('잔여')) fail('팀 표', teamTxt);
  await head.page.goto(base + '/admin/leaves', { waitUntil: 'domcontentloaded' });
  if (!(await head.page.locator('[data-unreg]').count())) fail('미등록 구역 없음');
  if (!(await head.page.locator('[data-nohire]').count())) fail('입사일 누락 구역 없음');
  if (!(await head.page.locator('[data-settle]').count())) fail('정산표 없음');
  if (!(await head.page.locator('[data-adjust]').count())) fail('조정 폼 없음');
  await head.page.screenshot({ path: path.join(shotDir, '95-161C-desktop.png'), fullPage: true });

  const mobile = await open(browser, headCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/leaves/team', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침 팀');
  await mobile.page.goto(base + '/admin/leaves', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침 관리');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-161C-390.png'), fullPage: true });
  console.log('95_161c_leave_admin_browser.mjs ok');
} finally {
  await browser.close();
}
