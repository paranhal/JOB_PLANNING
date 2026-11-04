import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day0 = process.env.DAY0;
if (!base || !cookie || !day0) {
  console.error('BASE_URL/AUTH_COOKIE/DAY0 없음');
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

const browser = await launch();
try {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.goto(base + '/workboard/board?view=list&date=' + day0 + '&auto=0', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#schedule-show', { timeout: 10000 });
  const act = page.locator('[data-show-bit="act"]');
  if (await act.count()) {
    await act.first().click();
    await page.waitForLoadState('domcontentloaded');
  }
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  if (overflow > 8) {
    console.error('가로 넘침', overflow);
    process.exit(1);
  }
} finally {
  await browser.close();
}
