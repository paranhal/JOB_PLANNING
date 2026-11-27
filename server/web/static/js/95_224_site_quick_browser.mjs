// 95-224 §2: 업무(작업) 등록 중 사이트 간이 팝업 — 열기→취소(미저장)→열기→등록→복귀(입력 보존·자동 연결). 390/1440.
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
for (const [label, vp, pid, childName, childId] of [
  ['1440', { width: 1440, height: 900 }, 'P224A', 'A가도서관', 'C224A1'],
  ['390', { width: 390, height: 844 }, 'P224B', 'B가도서관', 'C224B1']]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/workboard/register', { waitUntil: 'load' });
  await page.getByRole('button', { name: '+ 일일 업무 등록' }).click();
  await page.getByRole('button', { name: /구축·공사 작업/ }).click();
  const form = page.locator('form:has(input[name="work_type"][value="project_work"])').first();
  const titleBox = form.locator('input[name="title"]').first();
  await titleBox.fill('간이팝업 시험 ' + label);
  const quick = form.locator('[data-site-quick]');
  await quick.evaluate((el, id) => el.dispatchEvent(new CustomEvent('project-picked', { bubbles: true, detail: { id, kind: '' } })), pid);
  const notice = form.locator('[data-site-quick-notice]');
  try { await notice.waitFor({ state: 'visible', timeout: 8000 }); } catch (e) { fail(label + ' 사이트 0건 안내가 안 보인다'); }
  await page.screenshot({ path: path.join(shotDir, '95-224-quick-notice-' + label + '.png') });
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  // 1) 열기 → 선택 → 취소: 아무것도 저장하지 않고 입력은 그대로
  await form.locator('[data-site-quick-open]').click();
  const modal = form.locator('[data-site-quick-modal]');
  await modal.waitFor({ state: 'visible', timeout: 5000 });
  ok(await modal.locator('[data-site-quick-list] label').count() === 2, label + ' 하위 기관 목록 수', await modal.locator('[data-site-quick-list] label').count());
  await modal.locator('[data-site-quick-list] label', { hasText: childName }).locator('input').check();
  await page.screenshot({ path: path.join(shotDir, '95-224-quick-modal-' + label + '.png') });
  await modal.locator('[data-site-quick-cancel]').click();
  await modal.waitFor({ state: 'hidden', timeout: 5000 });
  ok(await titleBox.inputValue() === '간이팝업 시험 ' + label, label + ' 취소 뒤 입력이 사라졌다');
  const afterCancel = await page.evaluate(async (id) => (await (await fetch('/projects/' + id + '/sites/quick', { headers: { Accept: 'application/json' } })).json()).siteTotal, pid);
  ok(afterCancel === 0, label + ' 취소했는데 사이트가 저장됐다', afterCancel);
  ok(await form.locator('input[name="site_id"]').count() === 0, label + ' 취소했는데 대상 사이트가 붙었다');
  // 2) 다시 열기 → 검색 → 등록: 모달이 닫히고 입력 보존, 등록한 사이트가 대상 사이트에 자동 연결
  await form.locator('[data-site-quick-open]').click();
  await modal.waitFor({ state: 'visible', timeout: 5000 });
  await modal.locator('[data-site-quick-q]').fill(childName);
  ok(await modal.locator('[data-site-quick-list] label').count() === 1, label + ' 검색 결과 수');
  await modal.locator('[data-site-quick-list] label').first().locator('input').check();
  await modal.locator('[data-site-quick-register]').click();
  await modal.waitFor({ state: 'hidden', timeout: 8000 });
  ok(await titleBox.inputValue() === '간이팝업 시험 ' + label, label + ' 등록 뒤 입력이 사라졌다');
  const sid = await form.locator('input[name="site_id"]').evaluateAll((els) => els.map((e) => e.value));
  ok(sid.length === 1 && sid[0] === childId, label + ' 등록한 사이트가 대상 사이트에 연결되지 않았다', sid);
  ok(await page.url().includes('/workboard/register'), label + ' 페이지를 벗어났다 ' + page.url());
  const after = await page.evaluate(async (id) => (await (await fetch('/projects/' + id + '/sites/quick', { headers: { Accept: 'application/json' } })).json()).siteTotal, pid);
  ok(after === 1, label + ' 등록 뒤 사이트 수', after);
  ok(await notice.isHidden(), label + ' 등록 뒤에도 0건 안내가 보인다');
  await page.screenshot({ path: path.join(shotDir, '95-224-quick-done-' + label + '.png') });
  await context.close();
}
await browser.close();
console.log('OK');
