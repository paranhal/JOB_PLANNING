// 95-198: 업무 등록 화면의 활동 분류 칸 — 실제로 고르고 저장하면 상세에 활동이 보이고, 수정 화면에서 바꾼다. 390/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, ASSIGNEE = process.env.ASSIGNEE;
const EDIT_ID = process.env.EDIT_ID;
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
for (const [label, vp, title, kindVal, kindLabel, st, en] of [
  ['390', { width: 390, height: 844 }, '브라우저 활동 등록 390', 'service', '고객서비스', '09:00', '09:30'],
  ['1440', { width: 1440, height: 900 }, '브라우저 활동 등록 1440', 'mgmt', '경영지원', '10:00', '10:30']]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/workboard/register', { waitUntil: 'load' });
  await page.getByRole('button', { name: '+ 일일 업무 등록' }).click();
  await page.getByRole('button', { name: /행정 \/ 지원업무/ }).click();
  const form = page.locator('form:has([data-activity-kind-select])').first();
  const sel = form.locator('[data-activity-kind-select]');
  try { await sel.waitFor({ state: 'visible', timeout: 10000 }); } catch (e) {
    const d = await page.locator('[data-new-work-dialog]').count() ? (await page.locator('[data-new-work-dialog]').innerText()).slice(0, 500) : 'no dialog';
    fail(label + ' 활동 분류 칸이 안 보인다: ' + d + ' | selects=' + await page.locator('[data-activity-kind-select]').count());
  }
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  const opts = await sel.locator('option').allInnerTexts();
  ok(opts.length === 8 && opts.includes('역량강화') && opts.includes('업무지원'), label + ' 선택지', opts);
  await page.screenshot({ path: path.join(shotDir, '95-198-register-' + label + '.png') });
  await form.locator('input[name="title"]:visible').first().fill(title);
  await sel.selectOption(kindVal);
  await form.locator('input[name="start_time"]').first().fill(st);
  await form.locator('input[name="end_time"]').first().fill(en);
  const asg = form.locator('input[name="assignee"]');
  if (await asg.count()) { await asg.first().evaluate((el, v) => { el.value = v; }, ASSIGNEE); }
  await Promise.all([page.waitForLoadState('load'), form.locator('button[type=submit]:visible').last().click()]);
  await page.waitForLoadState('load');
  ok(!page.url().includes('err='), label + ' 저장 오류 ' + page.url());
  await context.close();
}
// 수정 화면에서 활동을 바꾼다
{
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  await page.goto(base + '/workboard/tasks/' + EDIT_ID + '/edit', { waitUntil: 'load' });
  const sel = page.locator('[data-activity-kind-select]:visible').first();
  await sel.waitFor({ state: 'visible', timeout: 5000 });
  ok(await sel.inputValue() === 'service', '수정 화면에 현재 활동이 선택돼 있지 않다');
  await sel.selectOption('growth');
  await page.screenshot({ path: path.join(shotDir, '95-198-edit-1440.png') });
  const dr = page.locator('form[action$="/update"] [name="delay_reason"]').first();
  if (await dr.count()) {
    const tag = await dr.evaluate((el) => el.tagName);
    if (tag === 'SELECT') { const v = await dr.locator('option:not([value=""])').first().getAttribute('value'); await dr.selectOption(v); }
    else { await dr.evaluate((el) => { el.removeAttribute('disabled'); }); await dr.fill('브라우저 시험'); }
  }
  const info = await page.evaluate(() => [...document.querySelectorAll('form[action$="/update"]')].map((f) => ({ n: f.querySelectorAll('button[type=submit]').length, bad: [...f.elements].filter((e) => e.willValidate && !e.checkValidity()).map((e) => e.name + ':' + e.validationMessage) })));
  console.log('forms', JSON.stringify(info));
  await page.locator('form[action$="/update"] button[type=submit]').first().click();
  await page.waitForTimeout(2500);
  const afterUrl = page.url();
  await page.goto(base + '/workboard/tasks/' + EDIT_ID, { waitUntil: 'load' });
  ok(await page.locator('[data-activity-label]').count() === 1, '수정 뒤 상세에 활동 표시가 없다 (저장 뒤 주소 ' + afterUrl + ')');
  ok((await page.locator('[data-activity-label]').innerText()).includes('역량강화'), '수정 뒤 상세에 역량강화가 없다');
  await context.close();
}
await browser.close();
console.log('OK');
