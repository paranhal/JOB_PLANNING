import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day = process.env.WORK_DATE;
const org = process.env.ORG_ID || 'O03';
const shotDir = process.env.SCREENSHOT_DIR || '.';
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

async function loginPage(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  return { context, page };
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

const browser = await launch();
try {
  const { context, page } = await loginPage(browser, { width: 1280, height: 800 });
  await page.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  const card = page.locator('a[data-exec-org="' + org + '"]');
  await card.waitFor({ timeout: 10000 });
  await page.evaluate(() => window.scrollTo(0, 400));
  const before = await page.evaluate(() => window.scrollY);
  await card.click();
  await page.waitForURL(/\/meeting\/board/, { timeout: 15000 });
  if (!(await page.locator('#page-back').count())) fail('이전 없음', page.url());
  const q = page.url();
  if (q.indexOf('src=exec') < 0 || q.indexOf('view_org_id=' + org) < 0) fail('보드 쿼리', q);
  await page.locator('#page-back').click();
  await page.waitForURL(/\/exec/, { timeout: 15000 });
  await page.waitForSelector('a[data-exec-org="' + org + '"]');
  await page.waitForFunction((id) => {
    const el = document.activeElement;
    return el && el.getAttribute && el.getAttribute('data-exec-org') === id;
  }, org, { timeout: 8000 });
  await page.waitForTimeout(200);
  const active = await page.evaluate(() => {
    const el = document.activeElement;
    return el && el.getAttribute ? el.getAttribute('data-exec-org') : '';
  });
  if (active !== org) fail('이전 후 포커스', active);
  const afterBtn = await page.evaluate(() => window.scrollY);
  if (before >= 8 && Math.abs(afterBtn - before) > 80) fail('이전 스크롤', before + ' ' + afterBtn);

  await page.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  await page.locator('a[data-exec-org="' + org + '"]').click();
  await page.waitForURL(/\/meeting\/board/);
  await page.goBack({ waitUntil: 'domcontentloaded' });
  await page.waitForURL(/\/exec/);
  await page.waitForTimeout(400);
  const active2 = await page.evaluate(() => document.activeElement && document.activeElement.getAttribute && document.activeElement.getAttribute('data-exec-org'));
  if (active2 !== org) fail('뒤로 가기 포커스', active2);

  await page.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  await page.locator('[data-exec-view-btn="sales"]').click();
  await page.waitForSelector('[data-exec-sales-stub]', { timeout: 8000 });
  const salesBody = await page.content();
  if (salesBody.indexOf('매출 현황은 준비 중입니다') < 0) fail('매출 준비 중 없음');
  await page.locator('[data-exec-view-btn="progress"]').click();
  await page.waitForSelector('a[data-exec-org="' + org + '"]', { timeout: 8000 });
  await page.locator('a[data-exec-org="' + org + '"]').click();
  await page.waitForURL(/\/meeting\/board/);
  await page.locator('#page-back').click();
  await page.waitForURL(/\/exec/, { timeout: 15000 });
  await page.waitForSelector('a[data-exec-org="' + org + '"]');
  await page.waitForFunction((id) => {
    const u = new URL(location.href);
    const view = u.searchParams.get('view') || 'progress';
    const el = document.activeElement;
    const orgId = el && el.getAttribute && el.getAttribute('data-exec-org');
    return view !== 'sales' && orgId === id;
  }, org, { timeout: 8000 }).catch(() => {});
  const view = await page.evaluate(() => {
    const u = new URL(location.href);
    return u.searchParams.get('view') || 'progress';
  });
  const active3 = await page.evaluate(() => document.activeElement && document.activeElement.getAttribute && document.activeElement.getAttribute('data-exec-org'));
  if (view === 'sales') fail('진행 보기로 안 돌아옴', view);
  if (active3 !== org) fail('토글 후 포커스', active3);

  fs.mkdirSync(shotDir, { recursive: true });
  await page.screenshot({ path: path.join(shotDir, '95-152-t6-desktop.png'), fullPage: true });

  const { context: mobile, page: mp } = await loginPage(browser, { width: 390, height: 844 });
  await mp.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  const overflow = await mp.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  if (overflow > 2) fail('가로 넘침', String(overflow));
  await mp.screenshot({ path: path.join(shotDir, '95-152-t6-390.png'), fullPage: true });
  await mobile.close();
  await context.close();
  console.log('95-152 browser ok');
} finally {
  await browser.close();
}
