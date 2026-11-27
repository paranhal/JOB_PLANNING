// 95-197: 활동 블록(값 있는 것만·사업 뒤) + 연결된 업무의 사업·활동 변경을 실제 클릭으로 본다. 390/1440.
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
const titles = (page) => page.locator('article.weekly-card b.block').allInnerTexts();
for (const [label, vp] of [['390', { width: 390, height: 844 }], ['1440', { width: 1440, height: 900 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + W, { waitUntil: 'load' });
  let t = (await titles(page)).map((s) => s.trim());
  // 처음에는 값 있는 활동만(경영지원·영업), 사업 블록이 먼저
  ok(t.includes('경영지원활동') && t.includes('영업활동'), label + ' 활동 블록 없음', t);
  ok(!t.includes('자산관리활동') && !t.includes('역량강화활동'), label + ' 값 없는 활동이 보인다', t);
  ok(t.indexOf('시험 사업') >= 0 && t.indexOf('시험 사업') < t.indexOf('영업활동'), label + ' 사업이 활동보다 앞이 아니다', t);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  await page.screenshot({ path: path.join(shotDir, '95-197-blocks-' + label + '.png') });
  // 연결된 업무 → 연결 변경 → 활동 고르기
  await page.locator('[data-weekly-linked] summary').click();
  const row = page.locator('[data-linked-task="T197-T"]');
  ok(await row.count() === 1, label + ' 연결된 업무 행 없음');
  await row.locator('[data-weekly-link]').click();
  await page.locator('#weekly-link-modal').waitFor({ state: 'visible', timeout: 3000 });
  const submit = page.locator('[data-weekly-link-submit]');
  ok(await submit.isDisabled(), label + ' 선택 전에 연결 버튼이 켜져 있다');
  // 사업을 고르면 활동이 해제되고(그 반대도) 하나만 선택된다
  await page.locator('input[name="project_id"]').first().check();
  await page.locator('input[name="activity_kind"][value="growth"]').check();
  ok(await page.locator('input[name="project_id"]:checked').count() === 0, label + ' 활동을 골랐는데 사업이 남아 있다');
  ok(await submit.isEnabled(), label + ' 활동을 골라도 연결 버튼이 꺼져 있다');
  await page.screenshot({ path: path.join(shotDir, '95-197-modal-' + label + '.png') });
  if (label === '1440') {
    await Promise.all([page.waitForLoadState('load'), submit.click()]);
    await page.waitForLoadState('load');
    t = (await titles(page)).map((s) => s.trim());
    ok(t.includes('역량강화활동'), '변경 뒤 역량강화활동 블록이 없다', t);
    ok((await page.locator('body').innerText()).includes('역량강화 활동'), '연결 완료 안내가 없다');
    // 같은 업무를 다시 사업으로 옮긴다(활동 해제)
    await page.locator('[data-weekly-linked] summary').click();
    await page.locator('[data-linked-task="T197-T"] [data-weekly-link]').click();
    await page.locator('#weekly-link-modal').waitFor({ state: 'visible', timeout: 3000 });
    await page.locator('input[name="project_id"][value="WPT197"]').check();
    await Promise.all([page.waitForLoadState('load'), page.locator('[data-weekly-link-submit]').click()]);
    await page.waitForLoadState('load');
    t = (await titles(page)).map((s) => s.trim());
    ok(!t.includes('역량강화활동'), '사업으로 옮겼는데 활동 블록이 남아 있다', t);
    await page.screenshot({ path: path.join(shotDir, '95-197-after-1440.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
