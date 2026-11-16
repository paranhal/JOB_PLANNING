// 95-211 주간 업무 보고서 「팀 전체」 박스: 보기(내용 없음)·편집(카드 0개)·읽기 전용 에서 항상 보이고, 편집에서 입력·저장·확정이 된다. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const tag = process.env.SHOT_TAG || 'after', name = process.env.NAME, pth = process.env.PAGE_PATH, expect = process.env.EXPECT; // read | edit
const strict = tag === 'after';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (strict && !c) fail(m, x); if (!strict && !c) console.log('(재현) ' + m); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + pth, { waitUntil: 'load' });
  await page.waitForTimeout(500);
  const sel = expect === 'edit' ? '[data-weekly-team]' : '[data-weekly-team-read]';
  const n = await page.locator(sel).count();
  ok(n === 1, `${label} ${name}: 「팀 전체」 박스(${sel})가 없다`, n);
  if (n === 1) {
    ok(await page.locator(sel).isVisible(), `${label} ${name}: 박스가 안 보인다`);
    const txt = await page.locator(sel).innerText();
    ok(txt.includes('전주 추진내역') && txt.includes('차주 추진계획'), `${label} ${name}: 두 칸 제목`, txt);
    if (expect === 'read') ok(txt.includes('입력된 내용이 없습니다'), `${label} ${name}: 빈 내용 안내`, txt);
    ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), `${label} ${name}: 가로 넘침`);
    if (expect === 'edit') {
      // 좁은 화면에서는 한 줄 1열, 넓은 화면에서는 2열
      const cols = await page.evaluate(() => { const a = document.querySelector('[data-team-done]').getBoundingClientRect(), b = document.querySelector('[data-team-plan]').getBoundingClientRect(); return Math.abs(a.top - b.top) < 4 ? 2 : 1; });
      ok(cols === (vp.width >= 768 ? 2 : 1), `${label} ${name}: 열 배치`, cols);
    }
  }
  await page.screenshot({ path: path.join(shotDir, `${name}-${label}-${tag}.png`), fullPage: label !== '820' });
  if (strict && label === '1440' && expect === 'edit' && n === 1) {
    // 입력 → 저장 → 다시 열어 보이는지 → 다시 집계해도 유지
    await page.locator('[data-team-done]').fill('긴급 확인 금주\n둘째 줄');
    await page.locator('[data-team-plan]').fill('긴급 확인 차주');
    ok((await page.locator('[data-weekly-dirty-msg]').innerText()).includes('저장하지 않은 수정'), '수정 표시');
    await Promise.all([page.waitForURL(/edit=1/), page.locator('[data-weekly-save]').click()]);
    await page.waitForLoadState('load');
    ok((await page.locator('[data-team-done]').inputValue()).includes('둘째 줄'), '저장한 금주가 입력칸에 없다');
    await Promise.all([page.waitForLoadState('load'), page.locator('[data-weekly-reagg]').click()]);
    await page.waitForLoadState('load');
    ok((await page.locator('[data-team-plan]').inputValue()).includes('긴급 확인 차주'), '다시 집계 뒤 팀 전체가 사라졌다');
    // 확정 후 보기: 같은 박스에 줄이 보인다
    await Promise.all([page.waitForLoadState('load'), page.locator('form[action$="ui-confirm"] button').first().click()]);
    await page.goto(base + pth.replace('edit=1', 'edit=0'), { waitUntil: 'load' });
    ok((await page.locator('[data-weekly-team-read]').innerText()).includes('· 긴급 확인 금주'), '확정 뒤 보기에 팀 전체 줄이 없다');
    await page.screenshot({ path: path.join(shotDir, `${name}-after-confirmed-1440.png`), fullPage: true });
  }
  await context.close();
}
await browser.close();
console.log('OK');
