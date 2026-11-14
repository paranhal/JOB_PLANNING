// 95-195 사업 등록 관리: 계약 없는 사업 표시·배지·계약 구분 필터·합치기 링크. 390/820/1440.
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
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/projects/registry', { waitUntil: 'load' });
  const row = (name) => page.locator('tbody tr:has-text("' + name + '")');
  ok(await row('계약없는 직접등록 사업').count() === 1, label + ' 계약 없는 사업이 목록에 없다');
  ok(await row('계약없는 직접등록 사업').locator('[data-uncontracted]').count() === 1, label + ' 계약 미등록 배지가 없다');
  ok(await row('계약된 사업').locator('[data-uncontracted]').count() === 0, label + ' 계약된 사업에 배지');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8 || !!document.querySelector('.overflow-x-auto')), label + ' 가로 넘침');
  if (label === '1440') {
    ok(await page.locator('[data-dup-merge]').count() >= 1, '같은 이름 안내에 합치기 링크가 없다');
    // 계약 구분 필터: 계약 미등록 → 검색
    await page.locator('[data-contract-filter]').selectOption('none');
    await Promise.all([page.waitForLoadState('load'), page.locator('form[action="/projects/registry"] button[type=submit]').first().click()]);
    await page.waitForLoadState('load');
    ok(page.url().includes('contract=none'), '필터가 주소에 반영되지 않았다 ' + page.url());
    ok(await row('계약없는 직접등록 사업').count() === 1, '미등록 필터 뒤 사업이 없다');
    ok(await row('계약된 사업').count() === 0, '미등록 필터인데 계약된 사업이 보인다');
    ok(await page.locator('[data-contract-filter]').inputValue() === 'none', '필터 선택이 유지되지 않는다');
    await page.screenshot({ path: path.join(shotDir, '95-195-filter-1440.png') });
    // 합치기 링크를 눌러 합치기 화면으로
    await page.goto(base + '/projects/registry', { waitUntil: 'load' });
    await Promise.all([page.waitForLoadState('load'), page.locator('[data-dup-merge]').first().click()]);
    ok(page.url().includes('/merge'), '합치기 화면으로 가지 않았다 ' + page.url());
  } else {
    await page.screenshot({ path: path.join(shotDir, '95-195-registry-' + label + '.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
