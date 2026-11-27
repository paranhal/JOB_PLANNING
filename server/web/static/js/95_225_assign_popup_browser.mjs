// 95-225: 배정받은 업무 팝업 — 로그인 직후(대기+보류) · 닫기 없음 · 거부 사유 필수 · 계속 보류 · 보류 그룹 카드 · 접속 중 새 배정 1회 노출. 1440/390.
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
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; window.ASSIGN_POLL_MS = 1500; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
const mk = async (page, target) => (await page.evaluate(async (t) => (await (await fetch('/__test/assign?target=' + t)).json()).id, target));

// ── 1440 ──
{
  const { context, page } = await session({ width: 1440, height: 900 });
  await page.goto(base + '/workboard/register?view=day&date=2026-12-01', { waitUntil: 'load' });
  const pop = page.locator('[data-assign-popup]');
  await pop.waitFor({ state: 'visible', timeout: 10000 });
  const items = pop.locator('[data-assign-item]');
  ok(await items.count() === 2, '로그인 직후 팝업은 대기+보류 2건', await items.count());
  const txt = await pop.innerText();
  ok(txt.includes('배정받은 업무') && txt.includes('충남 구축 사업') && txt.includes('도서관관리자') && txt.includes('보류 중'), '팝업 표시 정보', txt);
  ok(await pop.locator('button', { hasText: /^(닫기|×|✕)$/ }).count() === 0 && await pop.locator('[aria-label="닫기"]').count() === 0, '팝업에 닫기 버튼이 있다');
  await page.screenshot({ path: path.join(shotDir, '95-225-popup-1440.png') });
  // 거부: 사유 비우면 막힘 → 사유 입력 후 확정
  const pendingItem = pop.locator('[data-assign-item]', { hasNotText: '보류 중' }).first();
  await pendingItem.locator('[data-assign-reject]').click();
  await pendingItem.locator('[data-assign-reject-confirm]').click();
  ok(await pop.locator('[data-assign-err]').isVisible(), '빈 사유인데 오류 안내가 없다');
  ok(await items.count() === 2, '빈 사유 거부가 처리됐다');
  await pendingItem.locator('[data-assign-reason]').fill('일정이 겹칩니다');
  await pendingItem.locator('[data-assign-reject-confirm]').click();
  await page.waitForFunction(() => document.querySelectorAll('[data-assign-item]').length === 1, null, { timeout: 8000 });
  // 보류 항목: 계속 보류 → 팝업이 사라지고 다시 안 뜸(이 세션)
  await pop.locator('[data-assign-keep]').click();
  await pop.waitFor({ state: 'hidden', timeout: 5000 });
  await page.reload({ waitUntil: 'load' });
  await page.waitForTimeout(1200);
  ok(await page.locator('[data-assign-popup]').isHidden(), '계속 보류한 항목이 같은 세션에서 다시 떴다');
  // 보류 그룹 카드
  const card = page.locator('[data-assign-held-card]');
  ok(await card.isVisible() && (await card.locator('[data-assign-held-count]').innerText()).trim() === '1', '보류 그룹 카드(1건)');
  await page.screenshot({ path: path.join(shotDir, '95-225-held-card-1440.png') });
  // 접속 중 새 배정: 폴링으로 한 번만 뜬다
  const newId = await mk(page, 'C3');
  await page.locator('[data-assign-popup]').waitFor({ state: 'visible', timeout: 10000 });
  ok(await page.locator('[data-assign-item="' + newId + '"]').count() === 1, '새 배정이 팝업에 없다');
  await page.waitForTimeout(3200);
  ok(await page.locator('[data-assign-item="' + newId + '"]').count() === 1, '같은 항목이 두 번 떴다');
  await page.locator('[data-assign-item="' + newId + '"] [data-assign-hold]').click();
  await page.waitForFunction(() => document.querySelectorAll('[data-assign-item]').length === 0, null, { timeout: 8000 });
  await page.reload({ waitUntil: 'load' });
  ok((await page.locator('[data-assign-held-count]').innerText()).trim() === '2', '보류 카드가 2건이 아니다');
  // 보류 카드에서 수락
  await page.locator('[data-assign-held-item]').first().locator('button', { hasText: '수락' }).click();
  await page.waitForLoadState('load');
  ok((await page.locator('[data-assign-held-count]').innerText()).trim() === '1', '수락 뒤 보류 카드 건수');
  await context.close();
}
// ── 390 ──
{
  const { context, page } = await session({ width: 390, height: 844 });
  const id = await (async () => { await page.goto(base + '/workboard/register?view=day&date=2026-12-01', { waitUntil: 'load' }); return mk(page, 'C1'); })();
  await page.reload({ waitUntil: 'load' });
  const pop = page.locator('[data-assign-popup]');
  await pop.waitFor({ state: 'visible', timeout: 10000 });
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), '390 가로 넘침');
  const box = await pop.boundingBox();
  ok(box && box.x >= 0 && box.x + box.width <= 390 + 1, '390 팝업이 화면 밖으로 나간다', box);
  ok(await pop.locator('[data-assign-item="' + id + '"] [data-assign-accept]').isVisible(), '390 수락 버튼이 안 보인다');
  await page.screenshot({ path: path.join(shotDir, '95-225-popup-390.png') });
  await context.close();
}
await browser.close();
console.log('OK');
