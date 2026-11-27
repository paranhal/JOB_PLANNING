// 95-229: AS 접수 화면 사업 입력란 — 기관을 고르면 등록된 사업 후보를 불러오고, 후보가 1개면 자동 선택. 1440/390.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/as/new', { waitUntil: 'networkidle' });
  const sel = page.locator('[data-as-project-select]');
  await sel.waitFor({ state: 'visible', timeout: 10000 });
  ok((await sel.locator('option').count()) === 1, label + ' 기관 선택 전에는 후보가 없다');
  // 후보가 1개인 기관 → 자동 선택
  await page.evaluate(() => { document.getElementById('as-customer_id').value = 'C151'; });
  await page.waitForFunction(() => document.querySelector('[data-as-project-select]').value === 'WP-A', null, { timeout: 8000 });
  ok((await page.locator('[data-as-project-note]').innerText()).includes('자동으로 골랐습니다'), label + ' 자동 선택 안내');
  await page.screenshot({ path: path.join(shotDir, '95-229-form-auto-' + label + '.png') });
  // 후보가 여러 개인 기관 → 직접 고르게 한다
  await page.evaluate(() => { document.getElementById('as-customer_id').value = 'C151b'; });
  await page.waitForFunction(() => document.querySelectorAll('[data-as-project-select] option').length === 3, null, { timeout: 8000 });
  ok(await sel.inputValue() === '', label + ' 후보가 여러 개인데 자동 선택됐다');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  await context.close();
}
await browser.close();
console.log('OK');
