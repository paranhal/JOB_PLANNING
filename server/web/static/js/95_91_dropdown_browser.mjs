import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
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

const browser = await launch();
try {
  const context = await browser.newContext({ viewport: { width: 400, height: 844 } });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.goto(base + '/workboard/board?auto=0', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-ui-dd="scope"]', { timeout: 10000 });
  await page.locator('[data-ui-dd="scope"] button').click();
  await page.waitForSelector('[data-ui-dd="scope"] [role="listbox"]', { state: 'visible' });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  if (overflow > 2) {
    console.error('가로 넘침', overflow);
    process.exit(1);
  }
  const box = await page.locator('[data-ui-dd="scope"] [role="listbox"]').boundingBox();
  if (!box || box.x < -1 || box.x + box.width > 401) {
    console.error('메뉴가 화면 밖', box);
    process.exit(1);
  }
  await page.keyboard.press('Escape');
  await context.close();
} finally {
  await browser.close();
}
