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

async function loginContext(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  return context;
}

async function openDash(page) {
  await page.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-exec-card]', { timeout: 10000 });
}

const browser = await launch();
try {
  const context = await loginContext(browser, { width: 1280, height: 800 });
  const page = await context.newPage();
  await openDash(page);
  const n = await page.locator('[data-exec-card]').count();
  if (n < 1) {
    console.error('카드 없음');
    process.exit(1);
  }
  const idx = Math.min(2, n - 1);
  const card = page.locator('[data-exec-card]').nth(idx);
  const href = await card.getAttribute('href');
  const org = await card.getAttribute('data-exec-org');
  if (!href || href.indexOf('/meeting/board') < 0 || href.indexOf('src=exec') < 0) {
    console.error('카드 href', href);
    process.exit(1);
  }
  await page.evaluate(() => window.scrollTo(0, 500));
  const before = await page.evaluate(() => window.scrollY);
  await page.evaluate((p) => {
    if (window.ExecDashNav) window.ExecDashNav.save(p.org, { scrollY: p.y });
  }, { org: org, y: before });
  await card.click();
  await page.waitForURL(/\/meeting\/board/, { timeout: 15000 });
  if (!(await page.locator('#page-back').count())) {
    console.error('이전 링크 없음', page.url());
    process.exit(1);
  }
  await page.locator('#page-back').click();
  await page.waitForURL(/\/exec/, { timeout: 15000, waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-exec-card]');
  await page.waitForFunction((y) => Math.abs((window.scrollY || 0) - y) <= 8, before, { timeout: 4000 }).catch(() => {});
  const afterBtn = await page.evaluate(() => window.scrollY);
  if (before >= 8 && Math.abs(afterBtn - before) > 8) {
    console.error('버튼 복귀 스크롤', before, afterBtn);
    process.exit(1);
  }

  await openDash(page);
  await page.evaluate(() => window.scrollTo(0, 500));
  const y2 = await page.evaluate(() => window.scrollY);
  await page.evaluate((p) => {
    if (window.ExecDashNav) window.ExecDashNav.save(p.org, { scrollY: p.y });
  }, { org: org, y: y2 });
  await card.click();
  await page.waitForURL(/\/meeting\/board/, { timeout: 15000 });
  await page.goBack({ waitUntil: 'domcontentloaded' });
  await page.waitForURL(/\/exec/, { timeout: 15000, waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-exec-card]');
  await page.waitForTimeout(400);
  const yBack = await page.evaluate(() => window.scrollY);
  if (y2 >= 8 && Math.abs(yBack - y2) > 8) {
    console.error('브라우저 뒤로 스크롤', y2, yBack);
    process.exit(1);
  }

  const mobile = await loginContext(browser, { width: 390, height: 844 });
  const mp = await mobile.newPage();
  await mp.goto(base + '/exec?date=' + day, { waitUntil: 'domcontentloaded' });
  const overflow = await mp.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  if (overflow > 2) {
    console.error('가로 넘침', overflow);
    process.exit(1);
  }
  await mobile.close();
  await context.close();
} finally {
  await browser.close();
}
