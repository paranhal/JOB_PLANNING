// 95-188: 작업 등록 모달(project_work)에서 package_id 입력 개수·제출 순서를 재현하고, 하부 업무를 고르고 저장한다.
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, token = process.env.AUTH_COOKIE;
const mode = process.env.MODE || 'repro'; // repro: 수치만 출력 / assert: 수정 후 기대값 확인
async function launch() {
  const { chromium } = require('playwright');
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) {} }
  throw new Error('launch');
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
const browser = await launch();
const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
const u = new URL(base);
await context.addCookies([{ name: 'token', value: token, url: base }, { name: 'token', value: token, domain: u.hostname, path: '/' }]);
const page = await context.newPage();
page.on('dialog', (d) => d.accept());
const posts = [];
page.on('request', (r) => { if (r.method() === 'POST') posts.push({ url: r.url(), body: r.postData() || '' }); });
await page.goto(base + '/workboard/board?view=day&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
await page.locator('button[data-new-work]').click();
const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
await dlg.waitFor({ timeout: 10000 });
await dlg.locator('[data-kind="project_work"]').click();
await dlg.getByRole('button', { name: /다음 단계/ }).click();
await page.waitForSelector('#new-work-form-slot [data-project-picker]', { timeout: 10000 });
const search = page.locator('#new-work-form-slot [data-project-picker] input[type="search"]').first();
await search.click({ force: true });
await page.evaluate(() => { const root = document.querySelector('#new-work-form-slot [data-project-picker]'); Alpine.$data(root).pick('WP-O01', '도서관사업', ''); });
const form = page.locator('#new-work-form-slot form').filter({ has: page.locator('[data-package-picker]') }).first();
const panel = form.locator('[data-package-picker]').first();
await panel.waitFor({ state: 'visible', timeout: 8000 });
await page.waitForTimeout(800); // 수행 단계 패널(structure/options) 장착 대기
const stepSel = form.locator('select[name="plan_pick"]');
const hasStep = await stepSel.count();
if (hasStep) { await stepSel.selectOption(''); } // 「단계 없이 등록」
await panel.locator('[data-package-select]').selectOption({ index: 1 });
await panel.locator('[data-package-grid] input[data-id]').first().waitFor({ timeout: 8000 });
await panel.locator('[data-package-all]').check();
await panel.locator('[data-package-add]').click();
const info = await form.evaluate((f) => ({
  pkgInputs: [...f.querySelectorAll('[name=package_id]')].map((e) => ({ v: e.value, inPicker: !!e.closest('[data-package-picker]'), disabled: e.disabled })),
  planInputs: [...f.querySelectorAll('[name=plan_id]')].map((e) => e.value),
  sites: f.querySelectorAll('[name=site_id]').length,
  step: !!f.querySelector('[name=plan_pick]'),
}));
console.log('INFO', JSON.stringify(info));
await form.locator('input[name="title"]').fill('95-188 재현');
const radio = form.locator('input[name="work_subtype"]').first();
if (await radio.count()) await radio.check();
await Promise.all([page.waitForLoadState('load').catch(() => {}), form.locator('button[type="submit"]').click()]);
await page.waitForTimeout(1500);
const post = posts.filter((p) => p.url.includes('/workboard/tasks')).pop();
const pkgInBody = post ? [...post.body.matchAll(/package_id=([^&]*)/g)].map((m) => decodeURIComponent(m[1])) : null;
const text = (await page.locator('body').innerText()).slice(0, 4000);
const errShown = text.includes('하부 업무를 선택하세요');
console.log('POST package_id values in order:', JSON.stringify(pkgInBody), 'url:', page.url(), 'errShown:', errShown);
await browser.close();
if (mode === 'assert') {
  const live = info.pkgInputs.filter((x) => !x.disabled);
  if (live.length !== 1) fail('제출되는 package_id 가 1개여야 한다', info.pkgInputs);
  if (!live[0].v) fail('제출되는 package_id 값이 비어 있다', info.pkgInputs);
  if (errShown) fail('하부 업무를 골랐는데 오류가 난다');
  if (!page.url().includes('ok=task')) fail('등록 성공 표시(ok=task)가 없다', page.url());
}
console.log('OK');
