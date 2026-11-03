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
  const tries = [{ channel: 'chrome' }, {}, { channel: 'msedge' }];
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
const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
await context.addCookies([{ name: 'token', value: cookie, url: base }]);
const page = await context.newPage();
page.on('pageerror', (e) => console.error('pageerror', e.message));
const u = new URL(base);
await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
await page.goto(base + '/__t78/form', { waitUntil: 'domcontentloaded' });
if (!(await page.$('#nw-save'))) {
  console.error('fixture 없음');
  process.exit(1);
}
await page.click('#nw-save');
await page.waitForFunction(() => {
  const m = document.getElementById('overlap-resolve');
  return m && !m.hidden && !m.classList.contains('hidden');
}, null, { timeout: 8000 });
const split = await page.$('[data-overlap-action="split"]');
if (!split) {
  console.error('끼워 넣기 버튼 없음');
  process.exit(1);
}
await split.click();
await page.waitForTimeout(1500);
await browser.close();
