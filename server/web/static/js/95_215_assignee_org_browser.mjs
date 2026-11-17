// 95-215 규칙 O: 도서관 화면에 타 팀 담당자 이름이 나오지 않고 카드는 남는다. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const pages = JSON.parse(process.env.PAGES), foreign = process.env.FOREIGN_NAME, cardTitle = process.env.CARD_TITLE;
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  for (const [name, pth] of Object.entries(pages)) {
    const page = await context.newPage();
    page.on('pageerror', (e) => console.error('pageerror:', e.message));
    await page.goto(base + pth, { waitUntil: 'load' });
    await page.waitForTimeout(400);
    const text = await page.evaluate(() => document.body.innerText);
    const html = await page.content();
    if (!html.includes(cardTitle)) fail(`${label} ${name}: 카드가 숨겨졌다`, cardTitle);
    if (text.includes(foreign) || html.includes(foreign)) fail(`${label} ${name}: 조직 밖 담당자 이름 노출`, foreign);
    const badge = await page.locator('[data-assignee-reassign]').count();
    console.log(`${label} ${name}: 재배정 표시 ${badge}건`);
    if (!(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2))) fail(`${label} ${name}: 가로 넘침`);
    await page.screenshot({ path: path.join(shotDir, `${name}-${label}.png`), fullPage: label !== '820' });
    await page.close();
  }
  await context.close();
}
await browser.close();
console.log('OK');
