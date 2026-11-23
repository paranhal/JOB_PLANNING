// 95-218: 조치 완료 화면의 사업 후보 안내(1개 미리 선택·여러 개·0개)와 주간 보고서 저장 3번 반복 시 줄 수 불변. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const checks = JSON.parse(process.env.CHECKS), weekly = process.env.WEEKLY_PATH;
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
  page.on('pageerror', (e) => console.error('pageerror:', e.message, (e.stack || '').split('\n').slice(0,3).join(' / '), page.url()));
  for (const c of checks) {
    await page.goto(base + c.path, { waitUntil: 'load' });
    await page.waitForTimeout(300);
    for (const sel of c.present || []) if ((await page.locator(sel).count()) < 1) fail(`${label} ${c.name}: ${sel} 없음`);
    for (const sel of c.absent || []) if ((await page.locator(sel).count()) > 0) fail(`${label} ${c.name}: ${sel} 가 있으면 안 된다`);
    const txt = await page.evaluate(() => document.body.innerText);
    for (const s of c.text || []) if (!txt.includes(s)) fail(`${label} ${c.name}: 글 「${s}」 없음`);
    if (!(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2))) fail(`${label} ${c.name}: 가로 넘침`);
    await page.screenshot({ path: path.join(shotDir, `${c.name}-${label}.png`), fullPage: label !== '820' });
  }
  // 주간 보고서: 수정 없이 저장 3번 → 줄 수 불변
  await page.goto(base + weekly, { waitUntil: 'load' });
  const countLines = () => page.evaluate(() => [...document.querySelectorAll('textarea[name^="done_"],textarea[name^="plan_"]')].reduce((n, t) => n + t.value.split('\n').filter((s) => s.trim()).length, 0));
  const before = await countLines();
  if (before < 1) fail(`${label} 주간 보고서에 줄이 없다`, before);
  for (let i = 1; i <= 3; i++) {
    await Promise.all([page.waitForURL(/company-weekly/), page.locator('[data-weekly-save]').first().click()]);
    await page.waitForLoadState('load');
    if (!page.url().includes('edit=1')) await page.goto(base + weekly, { waitUntil: 'load' });
    const n = await countLines();
    if (n !== before) fail(`${label} 저장 ${i}회 뒤 줄 수 ${before} → ${n}`);
  }
  await page.screenshot({ path: path.join(shotDir, `weekly-save3-${label}.png`), fullPage: label !== '820' });
  await context.close();
}
await browser.close();
console.log('OK');
