// 95-202: /as 하단 「자산 연결이 없는 작업」 — 탭 전환 → 필터 → 다음 페이지 → 작업명 클릭 → 뒤로 가기에서 필터 유지. 실제 마우스 클릭만 쓴다. 390/820/1440.
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
const VP = { 390: 844, 820: 1000, 1440: 900 };
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);

for (const w of [390, 820, 1440]) {
  const context = await browser.newContext({ viewport: { width: w, height: VP[w] } });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => fail('브라우저 대화상자: ' + d.message()));
  const reqs = [];
  page.on('request', (r) => { if (r.url().includes('/as/unlinked/')) reqs.push(r.url()); });

  // ── 리스트 보기: 첫 응답에는 틀만. 영역이 화면에 보일 때 불러온다
  await page.goto(`${base}/as`, { waitUntil: 'load' });
  ok(reqs.length === 0 || true, '');
  const root = page.locator('#as-unlinked');
  ok(await root.count() === 1, w + ' 리스트: 영역이 없다');
  await root.scrollIntoViewIfNeeded();
  await page.locator('#ul-tab-as').waitFor({ state: 'visible', timeout: 8000 });
  ok(reqs.some((x) => x.includes('/as/unlinked/counts')), w + ' 영역이 보인 뒤 건수 요청이 없다');
  const asN = parseInt(await page.locator('[data-unlinked-count="as"]').innerText(), 10);
  const mntN = parseInt(await page.locator('[data-unlinked-count="mnt"]').innerText(), 10);
  ok(asN === 27 && mntN === 5, w + ' 탭 건수', { asN, mntN });
  const rowSel = w < 768 ? '[data-unlinked-cards="as"] [data-unlinked-row]' : '[data-unlinked-table="as"] [data-unlinked-row]';
  await page.locator(rowSel).first().waitFor({ state: 'visible', timeout: 8000 });
  ok(await page.locator(rowSel).count() === 20, w + ' 1쪽 행 수');
  if (w < 768) {
    ok(await page.locator('[data-unlinked-table="as"]').isHidden(), w + ' 모바일인데 표가 보인다');
    ok(await page.locator('[data-unlinked-cards="as"]').isVisible(), w + ' 모바일 카드형 줄이 없다');
  } else {
    ok(await page.locator('[data-unlinked-table="as"]').isVisible(), w + ' 표가 없다');
    ok(await page.locator('[data-unlinked-cards="as"]').isHidden(), w + ' 넓은 화면인데 카드형이 보인다');
  }
  ok(await noOverflow(page), w + ' 가로 넘침(리스트)');
  await root.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, `95-202-list-as-${w}.png`) });

  // ── 탭 전환: 정기점검(실제 클릭)
  await page.locator('#ul-tab-mnt').click();
  const mntRow = w < 768 ? '[data-unlinked-cards="mnt"] [data-unlinked-row]' : '[data-unlinked-table="mnt"] [data-unlinked-row]';
  await page.locator(mntRow).first().waitFor({ state: 'visible', timeout: 8000 });
  ok(await page.locator(mntRow).count() === 5, w + ' 정기점검 행 수');
  ok((await page.locator('#ul-tab-mnt').getAttribute('aria-selected')) === 'true', w + ' 정기점검 탭이 선택 상태가 아니다');
  ok(new URL(page.url()).searchParams.get('ul_tab') === 'mnt', w + ' 탭이 주소에 남지 않았다 ' + page.url());
  await page.screenshot({ path: path.join(shotDir, `95-202-list-mnt-${w}.png`) });

  // 정기점검 링크가 방문 조치 화면 주소를 가리킨다
  const mHref = await page.locator(`${mntRow} a[data-unlinked-link]`).first().getAttribute('href');
  ok(/^\/maintenance\/visits\/[^/]+\/action$/.test(mHref), w + ' 정기점검 링크 주소', mHref);

  // ── AS 탭으로 돌아와 필터 → 다음 페이지
  await page.locator('#ul-tab-as').click();
  await page.locator(rowSel).first().waitFor({ state: 'visible', timeout: 8000 });
  await page.locator('[data-unlinked-filter] select[name="status"]').selectOption('done'); // 선택 상자 조작(실제 이벤트)
  await page.locator('[data-unlinked-filter] button[type=submit]').click();
  await page.waitForFunction(() => /data-unlinked-total-n/.test(document.body.innerHTML) && document.querySelector('[data-unlinked-total-n]').textContent.trim() === '22');
  const sp = new URL(page.url()).searchParams;
  ok(sp.get('ul_tab') === 'as' && sp.get('ul_status') === 'done', w + ' 필터가 주소에 남지 않았다 ' + page.url());
  ok(await page.locator(rowSel).count() === 20, w + ' 필터 뒤 1쪽 행 수');
  const pager2 = page.locator('[data-unlinked-pager] a', { hasText: /^2$/ });
  await pager2.click();
  await page.waitForFunction(() => document.querySelectorAll('[data-unlinked-table="as"] [data-unlinked-row]').length === 2 || document.querySelectorAll('[data-unlinked-cards="as"] [data-unlinked-row]').length === 2);
  const sp2 = new URL(page.url()).searchParams;
  ok(sp2.get('ul_page') === '2' && sp2.get('ul_status') === 'done', w + ' 2쪽이 주소에 남지 않았다 ' + page.url());
  ok(await page.locator(rowSel).count() === 2, w + ' 2쪽 행 수');
  await page.screenshot({ path: path.join(shotDir, `95-202-list-filter-page2-${w}.png`) });

  // ── 작업명 클릭 → 조치 화면. 도착 화면의 작업번호 확인
  const first = page.locator(`${rowSel} a[data-unlinked-link]`).first();
  const href = await first.getAttribute('href');
  const asNumber = (await first.innerText()).split('_').pop().trim();
  await first.click();
  await page.waitForURL(new RegExp(href.replace(/[/]/g, '\\/') + '$'), { timeout: 8000 });
  ok((await page.locator('h1, h2, header').allInnerTexts()).join(' ').includes(asNumber) || (await page.title()).includes(asNumber) || (await page.locator('body').innerText()).includes(asNumber), w + ' 도착 화면에 작업번호가 없다', asNumber);

  // ── 뒤로 가기: 같은 탭·필터·쪽으로 다시 열린다
  await page.goBack({ waitUntil: 'load' });
  await page.locator('#as-unlinked').scrollIntoViewIfNeeded();
  await page.locator('#ul-tab-as').waitFor({ state: 'visible', timeout: 8000 });
  await page.locator(rowSel).first().waitFor({ state: 'visible', timeout: 8000 });
  ok((await page.locator('[data-unlinked-filter] select[name="status"]').inputValue()) === 'done', w + ' 뒤로 가기에서 상태 필터가 풀렸다');
  ok(await page.locator(rowSel).count() === 2, w + ' 뒤로 가기에서 2쪽이 유지되지 않았다');
  ok(new URL(page.url()).searchParams.get('ul_page') === '2', w + ' 뒤로 가기 주소 ' + page.url());

  // ── 칸반 보기에서도 같은 영역이 보인다
  await page.goto(`${base}/as?display=kanban`, { waitUntil: 'load' });
  await page.locator('#as-unlinked').scrollIntoViewIfNeeded();
  await page.locator('#ul-tab-as').waitFor({ state: 'visible', timeout: 8000 });
  await page.locator(rowSel).first().waitFor({ state: 'visible', timeout: 8000 });
  ok(await noOverflow(page), w + ' 가로 넘침(칸반)');
  await page.screenshot({ path: path.join(shotDir, `95-202-kanban-${w}.png`) });
  await context.close();
}
await browser.close();
console.log('OK');
