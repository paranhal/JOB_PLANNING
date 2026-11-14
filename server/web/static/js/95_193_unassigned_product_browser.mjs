// 95-193 주간업무보고 「사업이 연결되지 않은 업무」 제품 라벨: 표시·390/820/1440 넘침 없음·연결 모달 동작 그대로.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
const W = '/stats/company-weekly?week=2026-10-19&org=O03&edit=0';
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
  await page.goto(base + W, { waitUntil: 'load' });
  const rows = page.locator('[data-weekly-row]');
  ok(await rows.count() === 5, label + ' 미연결 행 수 ' + (await rows.count()));
  const labels = await page.locator('[data-weekly-row] [data-product-label]').allInnerTexts();
  ok(labels.length === 3, label + ' 제품 라벨 개수(정기점검 1·AS 1·행정 1)', labels);
  ok(labels.some((t) => t.trim() === 'KLAS'), label + ' KLAS 없음', labels);
  ok(labels.some((t) => t.includes('전자도서관 · LX-1') && !t.includes('외')), label + ' AS 라벨', labels);
  ok(labels.some((t) => t.includes('외 1')), label + ' 여러 자산 라벨', labels);
  // 제품을 모르는 행에는 라벨이 없다
  ok(await page.locator('[data-weekly-row]:has-text("자산 없는 행정") [data-product-label]').count() === 0, label + ' 자산 없는 행정에 라벨');
  ok(await page.locator('[data-weekly-row]:has-text("R193N") [data-product-label]').count() === 0, label + ' 자산 없는 AS 에 라벨');
  // 라벨이 화면 안에 보이고 가로로 넘치지 않는다
  const first = page.locator('[data-weekly-row] [data-product-label]').first();
  await first.scrollIntoViewIfNeeded();
  const b = await first.boundingBox();
  ok(b && b.x >= 0 && b.x + b.width <= vp.width + 1, label + ' 라벨이 화면 밖', b);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  if (label === '1440') {
    // 연결 동작 그대로: 「사업 연결」 버튼이 모달을 연다
    await page.locator('[data-weekly-row]:has-text("자산 있는 행정") [data-weekly-link]').click();
    await page.locator('#weekly-link-modal').waitFor({ state: 'visible', timeout: 3000 });
    ok((await page.locator('#weekly-link-modal').innerText()).includes('자산 있는 행정'), '연결 모달에 업무 제목이 없다');
    await page.screenshot({ path: path.join(shotDir, '95-193-link-modal-1440.png') });
    await page.keyboard.press('Escape');
  }
  await page.locator('[data-weekly-unassigned]').first().scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, '95-193-unassigned-' + label + '.png') });
  await context.close();
}
await browser.close();
console.log('OK');
