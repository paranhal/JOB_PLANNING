// 95-186 「1단계 사업 만들기」 박스: 맨 위 위치·실제 클릭(임시 DB), 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, P = process.env.PROJECT_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !P) { console.error('env'); process.exit(1); }
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : x); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
async function open(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('dialog', (d) => d.accept());
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const { context, page } = await open(browser, vp);
  await page.goto(base + '/projects/' + P, { waitUntil: 'load' });
  const box = page.locator('[data-phase-create]');
  ok(await box.count() === 1, label + ' 박스 개수');
  await box.waitFor({ state: 'visible', timeout: 3000 });
  ok((await box.innerText()).includes('1단계 사업 만들기'), label + ' 제목');
  // 맨 위: 첫 단계 줄·구조 복사 박스보다 위
  const y = async (sel) => (await page.locator(sel).first().boundingBox()).y;
  const yb = await y('[data-phase-create]');
  ok(yb < await y('[data-phase-id]'), label + ' 박스가 단계 목록보다 위');
  ok(yb < await y('[data-structure-copy]'), label + ' 박스가 구조 복사보다 위');
  ok(yb > await y('[data-changes-link]'), label + ' 박스가 변경내역 줄 아래');
  ok(await page.locator('[data-phase-create]').evaluate((e) => e.closest('details').open), label + ' 열림');
  ok(await page.locator('input[placeholder="단계명"]').count() === 1, label + ' 단계명 입력 1개');
  if (label === '1440') {
    // 오류: 마감이 시작보다 앞
    await box.locator('input[name=name]').fill('오류 시험 단계');
    await box.locator('input[name=plan_start]').fill('2027-05-01');
    await box.locator('input[name=plan_end]').fill('2027-04-01');
    await box.locator('button[type=submit]').click();
    await page.waitForLoadState('load');
    const flash = page.locator('#phase-flash');
    await flash.waitFor({ state: 'visible', timeout: 3000 });
    ok((await flash.innerText()).trim().length > 0, '오류 문구');
    ok((await flash.boundingBox()).y < (await page.locator('[data-phase-create]').boundingBox()).y, '오류 문구가 박스 위');
    ok(await page.locator('[data-phase-id]:has-text("오류 시험 단계")').count() === 0, '오류인데 단계가 생김');
    await page.screenshot({ path: path.join(shotDir, '95-186-error-1440.png') });
    // 성공
    const b2 = page.locator('[data-phase-create]');
    await b2.locator('input[name=name]').fill('새 단계 클릭');
    await b2.locator('input[name=plan_start]').fill('2027-06-01');
    await b2.locator('input[name=plan_end]').fill('2027-06-30');
    await b2.locator('button[type=submit]').click();
    await page.waitForLoadState('load');
    ok(await page.locator('[data-phase-id]:has-text("새 단계 클릭")').count() === 1, '단계가 생기지 않음');
    ok(await page.locator('[data-struct-ok]').count() === 1, '성공 문구');
    ok(page.url().includes('#project-phases'), '앵커 복귀 ' + page.url());
    ok(await page.locator('[data-phase-create] input[name=name]').inputValue() === '', '입력 비워짐');
  }
  ok(await noOverflow(page), label + ' 가로 넘침');
  await page.locator('[data-phase-create]').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, '95-186-' + label + '.png') });
  await context.close();
}
await browser.close();
console.log('OK');
