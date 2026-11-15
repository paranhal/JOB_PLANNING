// 95-203: 하단 고정 「저장」 버튼이 모바일 하단 탭바에 가리지 않는다. 클릭은 반드시 실제 마우스 클릭(JS 클릭·force 금지).
// MODE=repro 이면 가림(수정 전)을 확인하는 용도, MODE=verify 이면 수정 뒤 동작·좌표·스크롤·safe-area·회귀를 본다.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, MODE = process.env.MODE || 'verify';
const CASES = JSON.parse(process.env.CASES);
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
const VP = { 390: 844, 820: 1000, 1023: 900, 1024: 900, 1440: 900 };

async function open(w, safeInset) {
  const context = await browser.newContext({ viewport: { width: w, height: VP[w] } });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => fail('브라우저 대화상자: ' + d.message()));
  return { context, page };
}
const scrollAll = (page, where) => page.evaluate((where) => {
  const els = [document.getElementById('main-content'), document.scrollingElement, ...document.querySelectorAll('.as-action-right,.pw-action-mid,[data-pw-scroll]')];
  els.forEach((el) => { if (el) el.scrollTop = where === 'bottom' ? 1e7 : (where === 'middle' ? 600 : 0); });
}, where);
const geo = (page, sel) => page.evaluate((sel) => {
  const btn = [...document.querySelectorAll(sel)].find((b) => b.offsetParent !== null || getComputedStyle(b).position === 'fixed');
  if (!btn) return null;
  const b = btn.getBoundingClientRect(), tab = document.getElementById('mobile-tab-bar');
  const t = tab && getComputedStyle(tab).display !== 'none' ? tab.getBoundingClientRect() : null;
  const cx = b.x + b.width / 2, cy = b.y + b.height / 2;
  const hit = document.elementFromPoint(cx, cy);
  return { top: b.top, bottom: b.bottom, tabTop: t ? t.top : null, tabH: t ? t.height : 0, vh: innerHeight, vw: innerWidth, hitTab: !!(hit && hit.closest('#mobile-tab-bar')), hitBtn: !!(hit && btn.contains(hit)) };
}, sel);

const results = [];
for (const c of CASES) {
  for (const w of c.widths) {
    const { context, page } = await open(w);
    await page.goto(base + c.url, { waitUntil: 'load' });
    if (c.prep) { await page.evaluate(c.prep).catch(() => {}); }
    await page.waitForTimeout(300);
    const btn = page.locator(c.sel).first();

    if (MODE === 'repro') {
      await scrollAll(page, 'bottom');
      await page.waitForTimeout(150);
      if (c.shotName && (w === 390 || w === 820 || w === 1440)) await page.screenshot({ path: path.join(shotDir, `95-203-before-${c.shotName}-${w}-bottom.png`) });
      // 실제 마우스 클릭이 가로막힌다(수정 전): Playwright 의 actionability 가 `intercepts pointer events` 로 실패해야 한다
      let blocked = false, msg = '';
      try { await btn.click({ timeout: 2500, noWaitAfter: true, trial: true }); } catch (e) { blocked = /intercepts pointer events/.test(e.message); msg = e.message.split('\n').slice(0, 1).join(''); }
      results.push({ screen: c.name, w, blocked });
      await context.close();
      continue;
    }

    // verify: ① 모든 스크롤 위치에서 버튼이 탭바 위에 완전히 보인다 ② 실제 마우스 클릭이 된다(trial 아님)
    for (const where of ['top', 'middle', 'bottom']) {
      await scrollAll(page, where);
      await page.waitForTimeout(150);
      const g = await geo(page, c.sel);
      ok(g, `${c.name} ${w} ${where}: 버튼이 없다`);
      ok(g.top >= 0 && g.bottom <= g.vh, `${c.name} ${w} ${where}: 버튼이 화면 밖`, g);
      if (w < 1024) {
        ok(g.tabTop !== null, `${c.name} ${w}: 탭바가 없다`);
        ok(g.bottom <= g.tabTop, `${c.name} ${w} ${where}: 버튼 하단(${g.bottom}) 이 탭바 상단(${g.tabTop}) 아래로 들어갔다`, g);
        ok(!g.hitTab && g.hitBtn, `${c.name} ${w} ${where}: 버튼 중앙을 탭바가 잡는다`, g);
      } else {
        ok(g.tabTop === null, `${c.name} ${w}: lg 이상인데 탭바가 보인다`);
        ok(g.hitBtn, `${c.name} ${w} ${where}: 버튼 중앙을 다른 요소가 잡는다`, g);
        // lg 이상은 수정 전과 같은 좌표여야 한다(0단계 측정값)
        if (c.expTop !== undefined) ok(Math.abs(g.top - c.expTop) < 1.5 && Math.abs(g.bottom - c.expBottom) < 1.5, `${c.name} ${w}: lg 이상 좌표가 변했다(${g.top}~${g.bottom}, 기대 ${c.expTop}~${c.expBottom})`, g);
      }
    }
    await scrollAll(page, 'bottom');
    await page.waitForTimeout(150);
    // ③ 콘텐츠 마지막 항목이 하단 액션 영역(푸터)에 가려지지 않는다
    // (lg 이상은 변경 금지 범위라 마지막 항목 검사는 모바일 구간만. lg 이상 접수 폼의 기존 하단 여백 문제는 보고서에 따로 적는다)
    if (c.lastSel && c.barSel && w < 1024) {
      const r = await page.evaluate(({ lastSel, barSel }) => {
        const cont = document.querySelector(lastSel); let el = cont && cont.lastElementChild;
        while (el && (el.offsetParent === null || el.getBoundingClientRect().height === 0)) el = el.previousElementSibling;
        const bar = document.querySelector(barSel);
        if (!el || !bar) return null;
        return { lastBottom: el.getBoundingClientRect().bottom, barTop: bar.getBoundingClientRect().top };
      }, { lastSel: c.lastSel, barSel: c.barSel });
      ok(r, `${c.name} ${w}: 마지막 항목·바를 찾지 못했다`);
      ok(r.lastBottom <= r.barTop + 1, `${c.name} ${w}: 콘텐츠 마지막 항목(${Math.round(r.lastBottom)})이 하단 바(${Math.round(r.barTop)})에 가려진다`, r);
    }
    // ④ 실제 마우스 클릭(JS 클릭·force 금지): 버튼 중앙을 눌러 포인터 이벤트가 닿는지 본다(저장 동작은 별도 시험)
    if (c.clickProbe) {
      const probe = await page.evaluate((sel) => {
        const b = [...document.querySelectorAll(sel)].find((x) => x.offsetParent !== null || getComputedStyle(x).position === 'fixed');
        window.__probe = 0; b.addEventListener('click', (e) => { e.preventDefault(); window.__probe++; });
        const r = b.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
      }, c.sel);
      await page.mouse.click(probe.x, probe.y); // 좌표 클릭 = 사용자가 손가락/마우스로 누르는 것과 같다
      ok(await page.evaluate(() => window.__probe) === 1, `${c.name} ${w}: 실제 마우스 클릭이 저장 버튼에 닿지 않았다`);
    }
    if (c.shotName && (w === 390 || w === 820 || w === 1440)) await page.screenshot({ path: path.join(shotDir, `95-203-${c.shotName}-${w}-bottom.png`) });
    // ⑤ safe-area(홈 인디케이터)가 있다고 가정: 변수 한 곳을 키워도 가려지지 않는다
    if (w < 1024) {
      await page.addStyleTag({ content: ':root{--mobile-tab-h:calc(45px + 34px)} #mobile-tab-bar{padding-bottom:34px}' });
      await page.waitForTimeout(200);
      for (const where of ['top', 'bottom']) {
        await scrollAll(page, where); await page.waitForTimeout(100);
        const g2 = await geo(page, c.sel);
        ok(g2 && g2.bottom <= g2.tabTop && !g2.hitTab && g2.hitBtn, `${c.name} ${w} safe-area ${where}: 가려진다`, g2);
      }
      await scrollAll(page, 'bottom');
    }
    const gb = await geo(page, c.sel);
    results.push({ screen: c.name, w, top: Math.round(gb.top), bottom: Math.round(gb.bottom), tabTop: gb.tabTop === null ? null : Math.round(gb.tabTop) });
    await context.close();
  }
}
// ── 탭바 동작·z-index 회귀(390): 더보기 시트 열기/닫기, 메뉴 이동, 시트가 하단 액션 영역 위에 놓인다
if (MODE === 'verify' && process.env.TABBAR_URL) {
  const { context, page } = await open(390);
  await page.goto(base + process.env.TABBAR_URL, { waitUntil: 'load' });
  const z = await page.evaluate(() => ({ tab: getComputedStyle(document.getElementById('mobile-tab-bar')).zIndex, sheet: getComputedStyle(document.querySelector('#mobile-more-sheet > div:nth-child(2)')).zIndex, overlay: getComputedStyle(document.querySelector('#mobile-more-sheet > div:nth-child(1)')).zIndex }));
  ok(z.tab === '30' && z.sheet === '50' && z.overlay === '40', 'z-index 순서가 바뀌었다(탭바 30 < 오버레이 40 < 시트 50)', z);
  await page.getByRole('button', { name: '더보기' }).click();        // 실제 마우스 클릭
  const sheet = page.locator('#mobile-more-sheet [role="dialog"]');
  await sheet.waitFor({ state: 'visible', timeout: 3000 });
  const covered = await page.evaluate(() => { const r = document.querySelector('#mobile-more-sheet [role="dialog"]').getBoundingClientRect(); const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height - 20); return !!(hit && hit.closest('#mobile-more-sheet')); });
  ok(covered, '더보기 시트가 하단 액션 영역·탭바 위에 놓이지 않는다');
  await page.mouse.click(20, 20);                                   // 오버레이 클릭으로 닫기
  await sheet.waitFor({ state: 'hidden', timeout: 3000 });
  await page.getByRole('link', { name: '내 업무' }).click();          // 탭바 메뉴 이동
  await page.waitForURL(/\/work/, { timeout: 5000 });
  await context.close();
  console.log('TABBAR_OK');
}
console.log('RESULTS ' + JSON.stringify(results));
console.log('OK');
await browser.close();
