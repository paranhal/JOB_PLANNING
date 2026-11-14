// 95-194 사업 연결 창: 같은 이름 사업은 1개만 보이고, 고른 사업으로 실제 연결된다. 390/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
const W = '/stats/company-weekly?week=2026-10-19&org=O03&edit=0';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + W, { waitUntil: 'load' });
  await page.locator('[data-weekly-row]:has-text("' + (label === '1440' ? '자산 있는 행정' : '자산 없는 행정') + '") [data-weekly-link]').click();
  const modal = page.locator('#weekly-link-modal');
  await modal.waitFor({ state: 'visible', timeout: 3000 });
  const names = await modal.locator('[data-project-row]').evaluateAll((els) => els.map((e) => ({ name: e.getAttribute('data-name'), foreign: !!e.querySelector('[data-foreign]') })));
  const mine = (re) => names.filter((n) => re.test(n.name.replace(/\s+/g, '')) && !n.foreign).length;
  ok(mine(/세종시도서관ICT유지관리/) === 1, label + ' 세종시 ICT 가 한 번만 보여야 한다', names);
  ok(mine(/누리집도입/) === 1, label + ' 누리집 도입이 한 번만 보여야 한다', names);
  ok(names.filter((n) => n.name.includes('이름이 하나뿐인')).length === 1, label + ' 단독 사업');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  await page.screenshot({ path: path.join(shotDir, '95-194-modal-' + label + '.png') });
  if (label === '1440') {
    // 검색 → 대표 사업 선택 → 연결
    await modal.locator('[data-project-filter]').fill('누리집');
    const visible = modal.locator('[data-project-row]:visible');
    ok(await visible.count() === 1, '검색 결과 1건이어야 한다 ' + (await visible.count()));
    await visible.locator('input[type=radio]').check();
    const submit = modal.locator('[data-weekly-link-submit]');
    await page.waitForFunction(() => !document.querySelector('[data-weekly-link-submit]').disabled, null, { timeout: 3000 });
    await Promise.all([page.waitForLoadState('load'), submit.click()]);
    await page.waitForLoadState('load');
  }
  await context.close();
}
await browser.close();
console.log('OK');
