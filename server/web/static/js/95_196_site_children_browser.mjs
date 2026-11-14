// 95-196: 사업 상세 빈 상태 → 고객 하위 기관 카드(도서관 탭) → 선택·확인·확정 등록, 학교 사업 기본 탭·직접 탭 선택. 390/820/1440.
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
async function open(vp) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
const activeType = (p) => p.locator('[data-site-type-tabs] button.btn-primary').getAttribute('data-type');

// 1) 390/820: 빈 상태 카드·하위 기관 카드가 줄바꿈되어 가로로 넘치지 않는다
for (const [label, vp] of [['390', { width: 390, height: 844 }], ['820', { width: 820, height: 1000 }]]) {
  const { context, page } = await open(vp);
  await page.goto(base + '/projects/P196-LIB', { waitUntil: 'load' });
  ok(await page.locator('[data-site-empty-card]').isVisible(), label + ' 빈 상태 카드');
  ok(await noOverflow(page), label + ' 사업 상세 가로 넘침');
  await page.locator('[data-site-empty-card]').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, '95-196-empty-' + label + '.png') });
  await page.goto(base + '/projects/P196-LIB/sites#children', { waitUntil: 'load' });
  await page.locator('[data-site-children-card]').waitFor({ state: 'visible', timeout: 5000 });
  ok(await noOverflow(page), label + ' 사이트 등록 화면 가로 넘침');
  await page.screenshot({ path: path.join(shotDir, '95-196-children-' + label + '.png') });
  await context.close();
}

// 2) 1440: 전체 흐름
const { context, page } = await open({ width: 1440, height: 900 });
await page.goto(base + '/projects/P196-LIB', { waitUntil: 'load' });
ok(await page.locator('[data-site-progress-reg-btn]').isVisible(), '제목 줄 사이트 등록 버튼');
ok((await page.locator('[data-site-empty-card]').innerText()).includes('이 사업에 등록된 사이트가 없습니다'), '빈 상태 문구');
const childBtn = page.locator('[data-site-children-btn]');
ok((await childBtn.innerText()).includes('7곳'), '하위 기관 버튼 수', await childBtn.innerText());
await page.screenshot({ path: path.join(shotDir, '95-196-empty-1440.png') });
await Promise.all([page.waitForURL(/\/sites#children/), childBtn.click()]);
await page.waitForLoadState('load');
ok(await activeType(page) === 'library', '하위 기관 업종 최다(도서관)로 열려야 한다', await activeType(page));
const card = page.locator('[data-site-children-card]');
await card.waitFor({ state: 'visible', timeout: 5000 });
ok((await card.locator('[data-children-title]').innerText()).includes('하위 기관 7곳 (미등록 7곳)'), '카드 제목', await card.locator('[data-children-title]').innerText());
const chips = await card.locator('[data-children-chips] span').allInnerTexts();
ok(chips.includes('도서관 5') && chips.includes('공공기관 1') && chips.includes('(분류 없음) 1'), '업종 칩', chips);
// 선택 → 확인 단계 → 취소(등록 안 됨) → 다시 확인 → 확정
await card.locator('[data-children-select-all]').click();
ok((await card.locator('[data-children-register]').innerText()).includes('(7)'), '미등록 전체 선택 → 7');
await card.locator('[data-children-register]').click();
await card.locator('[data-children-confirm]').waitFor({ state: 'visible', timeout: 3000 });
ok((await card.locator('[data-children-confirm-text]').innerText()).includes('등록 7곳'), '확인 단계 문구', await card.locator('[data-children-confirm-text]').innerText());
await page.screenshot({ path: path.join(shotDir, '95-196-confirm-1440.png') });
await card.locator('[data-children-cancel]').click();
await card.locator('[data-children-confirm]').waitFor({ state: 'hidden', timeout: 3000 }).catch(() => fail('취소하면 확인 단계가 닫힌다'));
ok((await card.locator('[data-children-title]').innerText()).includes('미등록 7곳'), '취소했는데 등록됐다');
await card.locator('[data-children-register]').click();
await Promise.all([card.locator('[data-children-result]').waitFor({ state: 'visible', timeout: 8000 }), card.locator('[data-children-confirm-btn]').click()]);
ok((await card.locator('[data-children-result]').innerText()).includes('7곳을 등록했습니다'), '등록 결과', await card.locator('[data-children-result]').innerText());
ok((await card.locator('[data-children-title]').innerText()).includes('미등록 0곳'), '등록 뒤 미등록 0');
ok(await card.locator('[data-children-grid] input:disabled').count() === 7, '등록된 곳은 비활성');
ok((await page.locator('[data-site-total]').first().innerText()).trim() === '7', '등록된 사이트 수 갱신');
await page.screenshot({ path: path.join(shotDir, '95-196-done-1440.png') });
// 사업 상세에 반영
await page.goto(base + '/projects/P196-LIB', { waitUntil: 'load' });
ok(await page.locator('[data-site-empty-card]').count() === 0, '등록 뒤 빈 상태 카드');
ok((await page.locator('[data-site-reg-note]').innerText()).includes('등록 7곳'), '사업 상세 등록 7곳');

// 3) 학교 사업: 학교 탭 유지, 상단 유형 탭 직접 선택
await page.goto(base + '/projects/P196-SCH/sites', { waitUntil: 'load' });
ok(await activeType(page) === 'school', '학교 사업은 학교 탭');
await page.locator('[data-site-type-tabs] [data-type="public"]').click();
ok(await activeType(page) === 'public', '유형 탭을 직접 고를 수 있어야 한다');
await context.close();
await browser.close();
console.log('OK');
