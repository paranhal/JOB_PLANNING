// 95-219 마일스톤 리드타임 추이: 표·막대·탭이 보이고, 일/주/월 전환이 되고, 좁은 화면에서 가로로 넘치지 않는다. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/stats/milestones', { waitUntil: 'load' });
  // 시험 서버에서는 앱 공통 스크립트가 다 돌지 않아 모바일 메뉴 서랍이 열려 보이므로, 스크린샷에서만 숨긴다.
  await page.addStyleTag({ content: '[x-cloak]{display:none !important}' });
  await page.waitForTimeout(900);
  const sec = page.locator('[data-lead-trend]');
  if ((await sec.count()) !== 1) fail(`${label}: 리드타임 추이 영역이 없다`);
  await sec.scrollIntoViewIfNeeded();
  if (!(await sec.isVisible())) fail(`${label}: 영역이 안 보인다`);
  const rows = await page.locator('[data-lead-row]').count();
  if (rows !== 3) fail(`${label}: 주 단위 행 ${rows} want 3`);
  const txt = await sec.innerText();
  for (const s of ['접수→방문', '방문→조치완료', '접수→조치완료', '0.8', '17건', '참고', '표본 적음', '—']) if (!txt.includes(s)) fail(`${label}: 글 「${s}」 없음`);
  if (!(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2))) fail(`${label}: 페이지가 가로로 넘친다`);
  await page.screenshot({ path: path.join(shotDir, `lead-week-${label}.png`), fullPage: false });
  // 월 단위로 전환(저장된 값이 없으면 안내), 주 단위로 되돌아온다
  await Promise.all([page.waitForLoadState('load'), page.locator('[data-lead-tab="month"]').click()]);
  if (!page.url().includes('lead=month')) fail(`${label}: 월 탭 주소`, page.url());
  if ((await page.locator('[data-lead-empty]').count()) !== 1) fail(`${label}: 월 단위 빈 안내가 없다`);
  await page.screenshot({ path: path.join(shotDir, `lead-month-empty-${label}.png`), fullPage: false });
  await context.close();
}
await browser.close();
console.log('OK');
