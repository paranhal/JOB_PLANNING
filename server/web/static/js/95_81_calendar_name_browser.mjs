import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day = process.env.WORK_DATE;
const expectName = process.env.EXPECT_NAME || '김현우';
if (!base || !cookie || !day) {
  console.error('BASE_URL/AUTH_COOKIE/WORK_DATE 없음');
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
const widths = [390, 360];
const views = ['day', 'week', 'month'];
for (const width of widths) {
  const context = await browser.newContext({ viewport: { width, height: 844 } });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror', e.message));
  for (const view of views) {
    const url = base + '/workboard/board?view=' + view + '&date=' + day + '&auto=0';
    await page.goto(url, { waitUntil: 'domcontentloaded' });
    const html = await page.content();
    if (!html.includes('data-assignee-line')) {
      console.error(width + 'px ' + view + ' 담당자 줄 없음');
      process.exit(1);
    }
    if (!html.includes(expectName)) {
      console.error(width + 'px ' + view + ' 이름 없음');
      process.exit(1);
    }
    const overflow = await page.evaluate(() => {
      const el = document.querySelector('[data-assignee-line]');
      if (!el) return 'missing';
      const r = el.getBoundingClientRect();
      if (r.width < 0) return 'bad';
      return 'ok';
    });
    if (overflow !== 'ok' && view !== 'month') {
      console.error(width + 'px ' + view + ' 담당자 줄 측정 실패 ' + overflow);
      process.exit(1);
    }
  }
  await context.close();
}
await browser.close();
