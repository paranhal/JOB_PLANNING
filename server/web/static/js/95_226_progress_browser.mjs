// 95-226: 조치 화면 진행률 패널 — 일자별 부분 완료·수동 진행률·연기(대기)→대기 카드→오늘 업무. 1440/390.
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
async function session(vp) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => d.accept());
  return { context, page };
}
// ── 1440: 다일(TD) 일자별 부분 완료 + 수동 진행률, 하루(T1) 연기 → 대기 카드 → 오늘 업무 ──
{
  const { context, page } = await session({ width: 1440, height: 900 });
  await page.goto(base + '/workboard/tasks/TD', { waitUntil: 'load' });
  const panel = page.locator('[data-progress-panel]');
  await panel.waitFor({ state: 'visible', timeout: 10000 });
  ok((await panel.locator('[data-progress-shown]').innerText()).trim() === '0%', '처음 진행률 0%');
  const row1 = panel.locator('[data-day-row="2026-12-01"]');
  await row1.locator('input[name="done_pct"]').fill('100');
  await Promise.all([page.waitForLoadState('load'), row1.locator('[data-day-save]').click()]);
  ok((await page.locator('[data-progress-shown]').innerText()).trim() === '33%', '1일차 완료 뒤 33%', await page.locator('[data-progress-shown]').innerText());
  await page.screenshot({ path: path.join(shotDir, '95-226-panel-1440.png') });
  await page.locator('[data-progress-manual-form] input[name="pct"]').fill('80');
  await Promise.all([page.waitForLoadState('load'), page.locator('[data-progress-manual-form] button[type="submit"]').click()]);
  ok((await page.locator('[data-progress-shown]').innerText()).trim() === '80%' && (await page.locator('[data-progress-source]').innerText()).includes('수동'), '수동 우선 표시');
  // 오늘까지만 완료 → 남은 날은 계속
  await Promise.all([page.waitForLoadState('load'), page.locator('[data-complete-today]').click()]);
  ok(await page.locator('[data-complete-choice]').count() === 1, '오늘까지만 완료 뒤에도 업무가 남아 있다');
  // 연기 → 대기 카드 → 오늘 업무로
  await page.goto(base + '/workboard/tasks/T1', { waitUntil: 'load' });
  await Promise.all([page.waitForLoadState('load'), page.locator('[data-defer-btn]').click()]);
  ok(await page.locator('[data-task-deferred]').count() === 1, '연기 뒤 대기 표시');
  await page.goto(base + '/workboard/register?view=day&date=2026-12-01', { waitUntil: 'load' });
  const card = page.locator('[data-deferred-card]');
  ok(await card.isVisible(), '대기 업무 카드');
  ok(await card.locator('[data-deferred-group="충남 구축 사업"] [data-deferred-group-count]').innerText() === '1', '대기 그룹 건수');
  await page.screenshot({ path: path.join(shotDir, '95-226-deferred-1440.png') });
  await Promise.all([page.waitForLoadState('load'), card.locator('[data-undefer-btn]').first().click()]);
  ok(await page.locator('[data-deferred-card]').count() === 0, '오늘 업무로 등록한 뒤 대기 카드가 남았다');
  await context.close();
}
// ── 390: 패널이 가로로 넘치지 않고 버튼이 보인다 ──
{
  const { context, page } = await session({ width: 390, height: 844 });
  await page.goto(base + '/workboard/tasks/TP', { waitUntil: 'load' });
  const panel = page.locator('[data-progress-panel]');
  await panel.waitFor({ state: 'visible', timeout: 10000 });
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), '390 가로 넘침');
  ok(await panel.locator('[data-kid-loads]').isVisible(), '390 하위 업무 부하량 폼');
  await page.screenshot({ path: path.join(shotDir, '95-226-panel-390.png') });
  await context.close();
}
await browser.close();
console.log('OK');
