// 95-204: 「자산 연결이 없는 작업」 일괄 연결 + 정렬 — 실제 마우스 클릭만 쓴다. WIDTH(390/820/1440) 하나씩 실행.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, w = parseInt(process.env.WIDTH || '1440', 10);
const shotDir = process.env.SCREENSHOT_DIR || '.';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', w, m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const H = { 390: 844, 820: 1000, 1440: 900 }[w] || 900;
const context = await browser.newContext({ viewport: { width: w, height: H } });
const u = new URL(base);
await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
const page = await context.newPage();
await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
page.on('pageerror', (e) => console.error('pageerror:', e.message));
page.on('dialog', (d) => fail('브라우저 대화상자: ' + d.message()));
const mobile = w < 768;
const rowSel = (tab) => mobile ? `[data-unlinked-cards="${tab}"] [data-unlinked-row]` : `[data-unlinked-table="${tab}"] [data-unlinked-row]`;
const body = page.locator('#as-unlinked-body');
const num = async (sel) => parseInt((await page.locator(sel).first().innerText()).trim(), 10);
const rowIds = async (tab) => page.locator(rowSel(tab)).evaluateAll((els) => els.map((e) => e.getAttribute('data-unlinked-row')));
// 일괄 연결 버튼이 탭바·다른 요소에 가려지지 않는다(버튼 중심점의 최상위 요소가 버튼 자신)
const notCovered = (loc) => loc.evaluate((el) => {
  const r = el.getBoundingClientRect(), x = r.left + r.width / 2, y = r.top + r.height / 2;
  const top = document.elementFromPoint(x, y);
  return r.bottom <= window.innerHeight + 1 && !!top && (top === el || el.contains(top));
});

await page.goto(`${base}/as`, { waitUntil: 'load' });
await page.locator('#as-unlinked').scrollIntoViewIfNeeded();
await page.locator(rowSel('as')).first().waitFor({ state: 'visible', timeout: 8000 });

// ── AS 탭: 일괄 연결 영역
ok(await page.locator('[data-unlinked-bulkbar]').isVisible(), '일괄 연결 영역이 없다');
const open = page.locator('[data-unlinked-open]');
ok(await open.isDisabled(), '선택 0건인데 「선택 연결」이 활성이다');
const before = await rowIds('as');
const firstChk = page.locator('#as-unlinked-body [data-unlinked-check]:visible').first();
await firstChk.click();
ok((await num('[data-unlinked-selected]')) === 1 && await open.isEnabled(), '체크 1건 뒤 선택 건수·버튼');
await page.locator('[data-unlinked-checkall]').click(); // 전체 선택 한 번 → 연결 가능 건 모두
const linkable = await page.locator('#as-unlinked-body [data-unlinked-check]:visible').count();
ok(linkable === 6 && (await num('[data-unlinked-selected]')) === 6, '전체 선택', { linkable, sel: await num('[data-unlinked-selected]') });
// 대상이 아닌 행에는 체크박스가 없고 이유가 보인다
ok(await page.locator(`${rowSel('as')} [data-unlinked-nolink]`).count() === 3, '비대상 3건의 이유 표시가 없다');
await body.scrollIntoViewIfNeeded();
ok(await notCovered(open), '「선택 연결」 버튼이 가려진다');
ok(await noOverflow(), '가로 넘침(일괄 연결 영역)');
async function noOverflow() { return page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8); }
await page.screenshot({ path: path.join(shotDir, `95-204-bulk-select-${w}.png`) });

// ── 확인 창: 건수·앞 5건 미리보기·문구, 취소 → 닫힘(연결 안 됨)
await open.click();
const modal = page.locator('[data-unlinked-modal]');
await modal.waitFor({ state: 'visible', timeout: 4000 });
ok((await modal.locator('[data-unlinked-modal-n]').innerText()).trim() === '6', '확인 창 건수');
ok(await modal.locator('[data-unlinked-modal-list] li').count() === 5, '확인 창 미리보기는 5건');
ok((await modal.locator('[data-unlinked-modal-list] li').first().innerText()).includes('→'), '미리보기에 작업명 → 자산');
ok((await modal.locator('[data-unlinked-modal-more]').innerText()).includes('외 1건'), '미리보기 나머지 건수');
ok((await modal.innerText()).includes('되돌리려면 각 건 조치 화면에서 자산을 바꿔야 합니다'), '확인 문구');
ok(await notCovered(modal.locator('[data-unlinked-confirm]')), '확인 창 「연결」 버튼이 가려진다');
await page.screenshot({ path: path.join(shotDir, `95-204-bulk-modal-${w}.png`) });
await modal.locator('[data-unlinked-cancel]').click();
await modal.waitFor({ state: 'hidden', timeout: 4000 });
ok((await rowIds('as')).length === before.length, '취소했는데 목록이 바뀌었다');

// ── 연결 → 결과 요약 → 목록 갱신 → 건수 칩
await open.click();
await modal.waitFor({ state: 'visible', timeout: 4000 });
await modal.locator('[data-unlinked-confirm]').click();
const result = page.locator('[data-unlinked-result]');
await result.waitFor({ state: 'visible', timeout: 8000 });
ok((await result.innerText()).includes('연결 6건 완료'), '결과 요약', await result.innerText());
await page.waitForFunction(() => document.querySelector('[data-unlinked-count="as"]').textContent.trim() === '3');
ok((await rowIds('as')).sort().join(',') === 'M1,M2,Z1', '연결 뒤 남은 행', await rowIds('as'));
ok(await page.locator('[data-unlinked-bulkbar]').count() === 0, '대상이 없으면 일괄 연결 영역이 없어야 한다');
await page.screenshot({ path: path.join(shotDir, `95-204-bulk-result-${w}.png`) });

// ── 정렬: 선택 상자(모든 화면) → 주소·순서, 머리말 클릭(넓은 화면)
const sortSel = page.locator('[data-unlinked-filter] select[name="sort"]');
ok((await rowIds('as')).join(',') === 'M1,Z1,M2', '기본 정렬은 접수일 최신순', await rowIds('as'));
await sortSel.selectOption('recv_asc');
await page.waitForFunction(() => new URL(location.href).searchParams.get('ul_sort') === 'recv_asc');
await page.locator(rowSel('as')).first().waitFor({ state: 'visible' });
await page.waitForFunction(() => document.querySelector('[data-unlinked-sort-select]').value === 'recv_asc');
ok((await rowIds('as')).join(',') === 'M2,Z1,M1', '오래된순', await rowIds('as'));
if (!mobile) {
  await page.locator('[data-unlinked-sort="recv"]').click(); // 접수일 머리말 → 기본(최신순)으로
  await page.waitForFunction(() => !new URL(location.href).searchParams.has('ul_sort'));
  await page.waitForFunction(() => document.querySelector('[data-unlinked-sort-select]').value === 'recv_desc');
  ok((await rowIds('as')).join(',') === 'M1,Z1,M2', '머리말 클릭 뒤 최신순', await rowIds('as'));
  await page.locator('[data-unlinked-sort="customer"]').click();
  await page.waitForFunction(() => new URL(location.href).searchParams.get('ul_sort') === 'customer');
  ok((await page.locator('[data-unlinked-sort="customer"]').locator('..').getAttribute('aria-sort')) === 'ascending', '머리말 정렬 방향 표시');
}
await page.screenshot({ path: path.join(shotDir, `95-204-sort-${w}.png`) });

// ── 정기점검 탭: 제품 불일치 건은 체크박스 없음, 3건 연결
await page.locator('#ul-tab-mnt').click();
await page.locator(rowSel('mnt')).first().waitFor({ state: 'visible', timeout: 8000 });
ok(await page.locator('#as-unlinked-body [data-unlinked-check]:visible').count() === 3, '점검 연결 가능 3건');
ok((await page.locator('#as-unlinked-body').innerText()).includes('점검 제품과 자산 제품군이 다릅니다'), '제품 불일치 안내');
await page.locator('[data-unlinked-checkall]').click();
await open.click();
await modal.waitFor({ state: 'visible', timeout: 4000 });
ok((await modal.locator('[data-unlinked-modal-n]').innerText()).trim() === '3' && await modal.locator('[data-unlinked-modal-list] li').count() === 3, '점검 확인 창');
await modal.locator('[data-unlinked-confirm]').click();
await result.waitFor({ state: 'visible', timeout: 8000 });
ok((await result.innerText()).includes('연결 3건 완료'), '점검 결과 요약');
await page.waitForFunction(() => document.querySelector('[data-unlinked-count="mnt"]').textContent.trim() === '1');
ok(await noOverflow(), '가로 넘침(결과)');
await page.screenshot({ path: path.join(shotDir, `95-204-bulk-mnt-result-${w}.png`) });
await browser.close();
console.log('OK', w);
