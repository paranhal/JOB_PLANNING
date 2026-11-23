// 95-221 정기점검 일일업무: 날짜만 바꿔 저장해도 담당 유지, 「선택 해제」는 명시 플래그(assignee_cleared)를 켜고 다시 고르면 끈다. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const editPath = process.env.EDIT_PATH, newDate = process.env.NEW_DATE;
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const only = process.env.ONLY_VP;
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  if (only && only !== label) continue;
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => { if (!/Unexpected token 'var'/.test(e.message)) console.error('pageerror:', e.message); });
  await page.goto(base + editPath, { waitUntil: 'load' });
  await page.addStyleTag({ content: '[x-cloak]{display:none !important}' });
  await page.waitForTimeout(700);
  const form = page.locator('form[action$="/update"]').first();
  const picker = page.locator('[x-data*="assigneePicker"]').first();
  if (!(await picker.innerText()).includes('양기헌')) fail(`${label}: 선택기에 현재 담당이 없다`, await picker.innerText());
  const flag = () => form.evaluate((f) => (f.querySelector('input[name="assignee_cleared"]') || {}).value);
  if ((await flag()) !== '') fail(`${label}: 처음에는 해제 플래그가 꺼져 있어야 한다`, await flag());
  // 선택 해제 → 플래그 켜짐, 다시 고르면 꺼짐
  await picker.locator('button').first().click();
  await page.waitForTimeout(250);
  await picker.locator('[data-assignee-clear]').click();
  if ((await flag()) !== '1') fail(`${label}: 선택 해제 뒤 플래그`, await flag());
  await picker.locator('label', { hasText: '양기헌' }).first().click();
  await page.waitForTimeout(250);
  if ((await flag()) !== '') fail(`${label}: 다시 고르면 플래그가 꺼져야 한다`, await flag());
  // 날짜만 바꿔 저장 → 담당 유지
  await page.locator('input[name="work_date"]').first().fill(newDate);
  await form.evaluate((f) => { f.querySelectorAll('[name=delay_reason]').forEach((e) => { e.value = '일정 이동 시험'; e.dispatchEvent(new Event('input', { bubbles: true })); }); });
  await Promise.all([page.waitForLoadState('load'), form.locator('button[type=submit]', { hasText: '저장' }).first().click()]);
  await page.waitForTimeout(500);
  if (!page.url().includes('ok=saved')) fail(`${label}: 저장되지 않았다`, page.url());
  await page.addStyleTag({ content: '[x-cloak]{display:none !important}' });
  await page.waitForTimeout(500);
  const after = await page.locator('[x-data*="assigneePicker"]').first().innerText();
  if (!after.includes('양기헌')) fail(`${label}: 저장 뒤 담당이 비었다`, after);
  if (!(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2))) fail(`${label}: 가로 넘침`);
  await page.screenshot({ path: path.join(shotDir, `saved-${label}.png`) });
  await context.close();
}
await browser.close();
console.log('OK');
