import { pathToFileURL } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const html = process.env.EDIT_HTML;
const png = process.env.EDIT_PNG;
if (!html || !png) {
  console.error('EDIT_HTML/EDIT_PNG 필요');
  process.exit(1);
}

const { chromium } = require('playwright');
let browser;
try {
  browser = await chromium.launch({ headless: true, channel: 'chrome' });
} catch (e) {
  browser = await chromium.launch({ headless: true, channel: 'msedge' });
}
const page = await browser.newPage({ viewport: { width: 1100, height: 900 } });
await page.goto(pathToFileURL(html).href);
const leaked = await page.evaluate(() => {
  const t = document.body && document.body.innerText || '';
  return /data-require-assignee|@submit=/.test(t);
});
if (leaked) {
  console.error('화면에 속성 문자열이 보인다');
  process.exit(1);
}
await page.evaluate(() => {
  document.querySelectorAll('[x-cloak]').forEach((el) => el.removeAttribute('x-cloak'));
});
const heading = page.getByRole('heading', { name: '행정/지원업무 수정' });
await heading.waitFor({ state: 'visible' });
const wrap = heading.locator('xpath=ancestor::div[contains(@class,"mx-auto")][1]');
if (await wrap.count()) {
  await wrap.screenshot({ path: png });
} else {
  await heading.screenshot({ path: png });
}
await browser.close();
console.log('screenshot ok', png);
