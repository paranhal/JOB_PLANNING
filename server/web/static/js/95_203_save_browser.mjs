// 95-203: 390·820·1023 폭에서 AS 조치를 입력하고 「저장」을 실제 마우스 클릭(Playwright click: 가림 검사 포함, force·JS 클릭 금지)으로 눌러 저장한다.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const CASES = JSON.parse(process.env.CASES); // { "390": "R203-390", ... }
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
const H = { 390: 844, 820: 1000, 1023: 900 };
for (const w of [390, 820, 1023]) {
  const id = CASES[w];
  const context = await browser.newContext({ viewport: { width: w, height: H[w] } });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', async (d) => { if (d.type() === 'beforeunload') { await d.accept(); return; } fail('브라우저 대화상자(' + d.type() + '): ' + d.message()); });
  await page.goto(`${base}/as/${id}/action`, { waitUntil: 'load' });
  // 조치 내용 입력(조치 세트 필수 칸)
  await page.locator('select[name="s1_cause_cat1"]').selectOption('klas');
  const cat2 = page.locator('select[name="s1_cause_cat2"]');
  await cat2.locator('option[value="klas.kdevice"]').waitFor({ state: 'attached', timeout: 5000 });
  await cat2.selectOption('klas.kdevice');
  await page.locator('select[name="s1_cause_cat3"] option[value="klas.kdevice.install"]').waitFor({ state: 'attached', timeout: 5000 });
  await page.locator('select[name="s1_cause_cat3"]').selectOption('klas.kdevice.install');
  await page.locator('[name="s1_action_taken"]').fill('모바일 폭에서 실제 마우스 클릭으로 저장하는 조치 내용입니다');
  await page.locator('select[name="s1_process_type"]').selectOption('remote');
  // 근무구분(내근): force 없이 실제 마우스 클릭으로 고른다
  const place = page.locator('input.as-set-place[value="office"]');
  await place.scrollIntoViewIfNeeded();
  await place.check({ timeout: 5000 }).catch(async () => { await page.locator('label:has(input.as-set-place[value="office"])').click({ timeout: 5000 }); });
  if (!(await page.locator('[name="s1_result_code"]').count())) {
    const names = await page.evaluate(() => [...document.querySelectorAll('#as-action-form select, #as-action-sets select')].map((e) => e.name + '|' + e.id));
    fail(w + ': 결과 select 가 없다 url=' + page.url() + ' title=' + (await page.title()), names);
  }
  await page.locator('[name="s1_result_code"]').selectOption('done');
  await page.waitForTimeout(600);
  // 저장 버튼: 스크롤하지 않고도 탭바 위에 보여야 한다. 실제 마우스 클릭(actionability: 가림이 있으면 intercepts pointer events 로 실패)
  const save = page.locator('#as-action-bar button[type=submit]');
  const box = await save.boundingBox();
  const tabTop = await page.evaluate(() => document.getElementById('mobile-tab-bar').getBoundingClientRect().top);
  ok(box && box.y + box.height <= tabTop, `${w}: 저장 버튼(${box && Math.round(box.y + box.height)})이 탭바(${Math.round(tabTop)}) 아래로 들어갔다`);
  await page.screenshot({ path: path.join(shotDir, `95-203-save-before-click-${w}.png`) });
  const respP = page.waitForResponse((r) => r.url().includes(`/as/${id}/update`) && r.request().method() === 'POST', { timeout: 20000 });
  await save.click({ timeout: 5000 }); // ← force 없음, JS 클릭 없음
  const resp = await respP;
  ok(resp.status() === 303 && !(resp.headers()['location'] || '').includes('err='), `${w}: 저장 응답`, { s: resp.status(), loc: resp.headers()['location'] });
  await page.waitForLoadState('load');
  await page.screenshot({ path: path.join(shotDir, `95-203-save-after-${w}.png`) });
  await context.close();
}
await browser.close();
console.log('OK');
