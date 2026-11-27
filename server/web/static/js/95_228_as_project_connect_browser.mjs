// 95-228: 완료된 AS 조치 화면의 사업 연결 — 미연결 경고 · 후보마다 [연결] · 추천 · 연결 해제. 1440/390.
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
for (const [label, vp, as] of [['1440', { width: 1440, height: 900 }, 'R228-d'], ['390', { width: 390, height: 844 }, 'R228-m']]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => d.accept());
  await page.goto(base + '/as/' + as + '/action#project', { waitUntil: 'networkidle' });
  if (label === '390') await page.locator('#as-action-side-mobile').evaluate((el) => { el.open = true; });
  const root = page.locator(label === '390' ? '#as-action-side-mobile' : '#as-action-side').locator('[data-as-project-block]');
  ok(await root.locator('[data-as-project-warn]').isVisible(), label + ' 미연결 경고가 안 보인다');
  ok(await root.locator('[data-as-project-cand]').count() === 2, label + ' 후보 2건', await root.locator('[data-as-project-cand]').count());
  ok(await root.locator('[data-as-project-rec]').count() === 1, label + ' 추천 1건');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  await page.screenshot({ path: path.join(shotDir, '95-228-unlinked-' + label + '.png') });
  const waitConn = page.waitForResponse((r) => r.url().includes('/as/' + as + '/project') && r.status() === 303, { timeout: 8000 }).catch(() => null);
  await root.locator('[data-as-project-cand="WP-A"] [data-as-project-connect]').click();
  ok(await waitConn, label + ' 연결 요청이 없다');
  await page.waitForLoadState('networkidle');
  if (label === '390') await page.locator('#as-action-side-mobile').evaluate((el) => { el.open = true; });
  const after = page.locator(label === '390' ? '#as-action-side-mobile' : '#as-action-side').locator('[data-as-project-block]');
  ok(await after.locator('[data-as-project-linked]').isVisible(), label + ' 연결된 사업명이 안 보인다');
  ok(await after.locator('[data-as-project-warn]').count() === 0, label + ' 연결 뒤에도 경고가 남았다');
  await page.screenshot({ path: path.join(shotDir, '95-228-linked-' + label + '.png') });
  const waitUn = page.waitForResponse((r) => r.url().includes('/as/' + as + '/project') && r.status() === 303, { timeout: 8000 }).catch(() => null);
  await after.locator('[data-as-project-unlink] button').click();
  ok(await waitUn, label + ' 해제 요청이 없다');
  await page.waitForLoadState('networkidle');
  if (label === '390') await page.locator('#as-action-side-mobile').evaluate((el) => { el.open = true; });
  ok(await page.locator(label === '390' ? '#as-action-side-mobile' : '#as-action-side').locator('[data-as-project-warn]').isVisible(), label + ' 해제 뒤 경고가 안 보인다');
  await context.close();
}
await browser.close();
console.log('OK');
