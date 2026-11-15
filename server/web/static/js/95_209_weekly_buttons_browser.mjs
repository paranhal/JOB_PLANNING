// 95-209 주간 업무 보고서: 상단 링크가 주 버튼, 이 화면의 모든 버튼 높이 동일(44px), 다른 화면은 그대로. 1440/820/390.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const W = '/stats/company-weekly?week=2026-10-19&org=O03';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + W, { waitUntil: 'load' });
  // 상단 줄: 새 버튼이 보이고 엑셀·HWPX 저장과 높이가 같다
  const hs = await page.evaluate(() => {
    const q = (sel) => Array.from(document.querySelectorAll(sel)).filter((e) => e.offsetParent !== null).map((e) => ({ t: (e.innerText || e.textContent || '').trim().slice(0, 14), h: Math.round(e.getBoundingClientRect().height) }));
    return { top: q('.weekly-uniform > div a.btn-primary, .weekly-uniform summary.btn-primary'), all: q('.weekly-uniform a.btn-primary, .weekly-uniform a.btn-secondary, .weekly-uniform button.btn-primary, .weekly-uniform button.btn-secondary, .weekly-uniform summary.btn-primary, .weekly-uniform .btn'), move: q('.weekly-uniform a.w-11.h-11') };
  });
  const names = hs.top.map((x) => x.t).join('|');
  ok(names.includes('다른 출력물') && names.includes('예전 화면으로') && names.includes('엑셀·HWPX 저장'), label + ' 상단 버튼', hs.top);
  const bad = hs.all.filter((x) => Math.abs(x.h - 44) > 1);
  ok(bad.length === 0, label + ' 44px 가 아닌 버튼', bad);
  ok(hs.move.length === 2 && hs.move.every((x) => x.h === 44), label + ' ‹ › 높이', hs.move);
  ok(hs.all.length >= 6, label + ' 버튼 수 부족', hs.all.length);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), label + ' 가로 넘침');
  // 링크 동작 불변: href
  const hrefs = await page.evaluate(() => Array.from(document.querySelectorAll('.weekly-uniform a.btn-primary')).map((a) => a.getAttribute('href')));
  ok(hrefs.includes('/stats/reports#outputs') && hrefs.includes('/stats/reports'), label + ' href 불변', hrefs);
  await page.screenshot({ path: path.join(shotDir, '95-209-top-' + label + '.png') });
  if (label === '1440') {
    // 클릭 이동: 예전 화면으로 → /stats/reports
    await Promise.all([page.waitForURL(/\/stats\/reports/), page.locator('a.btn-primary', { hasText: '예전 화면으로' }).click()]);
    // 다른 화면: 버튼 높이가 44 로 바뀌지 않았다(.btn-sm 은 30 안팎)
    const other = await page.evaluate(() => { const b = document.createElement('button'); b.className = 'btn-secondary btn-sm'; b.textContent = '시험'; document.body.appendChild(b); const h = Math.round(b.getBoundingClientRect().height); const c = document.createElement('button'); c.className = 'btn-primary'; c.textContent = '시험'; document.body.appendChild(c); const h2 = Math.round(c.getBoundingClientRect().height); b.remove(); c.remove(); return [h, h2]; });
    ok(other.length === 2 && other.every((h) => h < 40), '다른 화면 버튼 높이가 바뀌었다', other);
    await page.screenshot({ path: path.join(shotDir, '95-209-other-reports-1440.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
