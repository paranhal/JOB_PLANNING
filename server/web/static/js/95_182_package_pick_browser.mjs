import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const token = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !token) {
  console.error('env');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  for (const extra of [{ channel: 'chrome' }, {}]) {
    try {
      return await chromium.launch(Object.assign({ headless: true }, extra));
    } catch (e) { /* 다음 */ }
  }
  throw new Error('launch');
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

async function openForm(browser, kind, width) {
  const context = await browser.newContext({ viewport: { width, height: 1000 } });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: token, url: base },
    { name: 'token', value: token, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.goto(base + '/workboard/board?view=day&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  const btn = page.locator('button[data-new-work]');
  if (!(await btn.count())) fail('등록 버튼 없음');
  await btn.click();
  const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
  await dlg.waitFor({ timeout: 10000 });
  await dlg.locator('[data-kind="' + kind + '"]').click();
  await dlg.getByRole('button', { name: /다음 단계/ }).click();
  await page.waitForSelector('#new-work-form-slot [data-project-picker]', { timeout: 10000 });
  return { context, page };
}

async function pickProject(page, id, name) {
  const search = page.locator('#new-work-form-slot [data-project-picker] input[type="search"]').first();
  await search.click({ force: true });
  const ok = await page.evaluate(([i, n]) => {
    const root = document.querySelector('#new-work-form-slot [data-project-picker]');
    const data = window.Alpine && Alpine.$data ? Alpine.$data(root) : null;
    if (!data || !data.pick) return false;
    data.pick(i, n, '');
    return true;
  }, [id, name]);
  if (!ok) fail('사업 선택 불가');
}

async function noOverflow(page) {
  return page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  for (const kind of ['project_work', 'admin']) {
    const { context, page } = await openForm(browser, kind, 1280);
    const form = page.locator('#new-work-form-slot form').filter({ has: page.locator('[data-package-picker]') }).first();
    const panel = form.locator('[data-package-picker]').first();

    // 사업을 고르기 전에는 하부 업무 선택이 보이지 않는다
    if (await panel.isVisible()) fail(kind + ': 사업 선택 전에 하부 업무가 보임');

    // 수행 구조가 있는 사업 → 하부 업무 필수
    await pickProject(page, 'WP-O01', '도서관사업');
    await panel.waitFor({ state: 'visible', timeout: 8000 });
    const select = panel.locator('[data-package-select]');
    if ((await select.getAttribute('required')) === null) fail(kind + ': 하부 업무가 필수가 아님');
    if ((await panel.locator('[data-package-grid]').isVisible())) fail(kind + ': 하부 업무 전에 사이트 목록이 보임');

    // 하부 업무를 고르면 그 하부 업무의 사이트만 나온다(유형 탭, 시도·시군 체크, 검색)
    await select.selectOption({ index: 1 });
    await panel.locator('[data-package-grid] input[data-id]').first().waitFor({ timeout: 8000 });
    const n = await panel.locator('[data-package-grid] input[data-id]').count();
    if (n !== 2) fail(kind + ': 후보 수 ' + n);
    if (!(await panel.locator('[data-package-types] button').count())) fail(kind + ': 유형 탭 없음');
    if (!(await panel.locator('[data-package-sido] input').count())) fail(kind + ': 시도 체크 없음');
    if (!(await panel.locator('[data-package-sigungu] input').count())) fail(kind + ': 시군 체크 없음');

    // 검색
    await panel.locator('[data-package-q]').fill('홍성');
    await panel.locator('[data-package-search]').click();
    const hit = await panel.locator('[data-package-grid] input[data-id]').count();
    if (hit !== 1) fail(kind + ': 검색 결과 ' + hit);
    await panel.locator('[data-package-q]').fill('');
    await panel.locator('[data-package-search]').click();

    // 전체 선택 → 담기 → 대상 사이트(숨은 site_id)로 들어간다
    await panel.locator('[data-package-all]').check();
    const addBtn = panel.locator('[data-package-add]');
    if (!(await addBtn.innerText()).includes('(2)')) fail(kind + ': 담기 수', await addBtn.innerText());
    await addBtn.click();
    const sids = await form.locator('input[name="site_id"]').count();
    if (sids !== 2) fail(kind + ': 담은 사이트 숨은 입력 ' + sids);
    const pkgVal = await form.locator('input[name="package_id"]').first().inputValue();
    if (!pkgVal) fail(kind + ': package_id 없음');
    if (!(await panel.locator('[data-package-note]').innerText()).includes('2곳을 담았습니다')) fail(kind + ': 담김 안내');
    await page.screenshot({ path: path.join(shotDir, kind + '-picked-1280.png'), fullPage: true });

    // 구조 없는 사업으로 바꾸면 하부 업무 선택이 사라지고 기존 검색으로 돌아간다
    await pickProject(page, 'WP-O02', '구조없는사업');
    await page.waitForFunction(() => !document.querySelector('#new-work-form-slot [data-package-picker]') || document.querySelector('#new-work-form-slot [data-package-picker]').offsetParent === null, null, { timeout: 8000 })
      .catch(() => fail(kind + ': 구조 없는 사업인데 하부 업무가 남음'));
    await context.close();

    // 좁은 화면
    const m = await openForm(browser, kind, 390);
    await pickProject(m.page, 'WP-O01', '도서관사업');
    const mp = m.page.locator('#new-work-form-slot [data-package-picker]').first();
    await mp.waitFor({ state: 'visible', timeout: 8000 });
    await mp.locator('[data-package-select]').selectOption({ index: 1 });
    await mp.locator('[data-package-grid] input[data-id]').first().waitFor({ timeout: 8000 });
    if (!(await noOverflow(m.page))) fail(kind + ': 390px 가로 넘침');
    await m.page.screenshot({ path: path.join(shotDir, kind + '-picked-390.png'), fullPage: true });
    await m.context.close();
  }
  console.log('95_182_package_pick_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
