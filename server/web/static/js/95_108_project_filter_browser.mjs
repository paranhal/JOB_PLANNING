import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const otherName = process.env.OTHER_NAME || '';
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

const pre = await fetch(base + '/workboard/projects/other', {
  headers: { Cookie: 'token=' + cookie, Accept: 'application/json' },
  redirect: 'manual',
});
if (pre.status !== 200) {
  console.error('node other', pre.status);
  process.exit(1);
}
const otherJSON = await pre.text();

const browser = await launch();
try {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.route('**/workboard/projects/other**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json; charset=utf-8',
      body: otherJSON,
    });
  });
  await page.goto(base + '/workboard/board?view=day&auto=0', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-project-filter]', { timeout: 10000 });
  const html = await page.content();
  if (otherName && html.includes(otherName)) {
    console.error('초기 HTML 에 타 부서 사업명');
    process.exit(1);
  }
  await page.locator('[data-pf-open]').click();
  await page.waitForSelector('[data-pf-panel]', { state: 'visible' });
  const more = page.locator('[data-pf-more]');
  await more.waitFor({ state: 'visible', timeout: 10000 });
  await more.click();
  await page.waitForSelector('[data-pf-other] .pf-gh, [data-pf-other] .pf-it', { timeout: 10000 });
  const gh = page.locator('[data-pf-other] .pf-gh');
  if (await gh.count()) {
    await gh.first().click();
  }
  await page.waitForSelector('[data-pf-other] .pf-it[data-project-id]', { state: 'visible', timeout: 10000 });
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  if (overflow > 2) {
    console.error('가로 넘침', overflow);
    process.exit(1);
  }
  await Promise.all([
    page.waitForURL(/project=/, { timeout: 10000 }),
    page.evaluate(() => {
      var it = document.querySelector('[data-pf-other] .pf-it[data-project-id]');
      if (!it) throw new Error('항목 없음');
      it.click();
    }),
  ]);
  await context.close();
} finally {
  await browser.close();
}
