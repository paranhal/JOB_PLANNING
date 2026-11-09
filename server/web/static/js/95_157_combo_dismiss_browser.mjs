import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const editPath = process.env.EDIT_PATH || '';
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie) {
  console.error('BASE_URL/AUTH_COOKIE 없음');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  const tries = [{ channel: 'chrome' }, {}];
  let last = null;
  for (const extra of tries) {
    try {
      return await chromium.launch(Object.assign({ headless: true }, extra));
    } catch (e) {
      last = e;
    }
  }
  throw last;
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

async function loginPage(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  return context.newPage();
}

async function openAdminForm(page) {
  await page.locator('button[data-new-work]').click();
  const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
  await dlg.waitFor({ timeout: 10000 });
  await dlg.locator('[data-kind="admin"]').click();
  await dlg.getByRole('button', { name: /다음 단계/ }).click();
  await page.locator('#new-work-form-slot [data-jp-combo]').first().waitFor({ timeout: 12000 });
}

function listDisplay(sel) {
  return `(() => {
    const root = document.querySelector(${JSON.stringify(sel)});
    if (!root) return 'no-root';
    const ul = root.querySelector('ul.combobox-list');
    if (!ul) return 'no-ul';
    const cs = getComputedStyle(ul);
    const d = window.Alpine && Alpine.$data ? Alpine.$data(root) : null;
    return (d && d.open ? 'open' : 'closed') + ':' + cs.display + ':' + (d && d.q != null ? d.q : '');
  })()`;
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const page = await loginPage(browser, { width: 1280, height: 800 });
  await page.goto(base + '/workboard/board?view=day&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  await openAdminForm(page);
  const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
  const cust = '#new-work-form-slot [data-customer-combo]';
  const proj = '#new-work-form-slot [data-project-picker]';
  const custInp = page.locator(cust + ' input[type="search"]').first();
  const projInp = page.locator(proj + ' input[type="search"]').first();

  await custInp.click();
  await page.waitForTimeout(150);
  await custInp.fill('충남');
  await page.waitForTimeout(250);
  let st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('open:')) fail('거래처 목록이 안 열림', st);
  if (!st.includes('충남')) fail('검색어가 비었다', st);

  await page.locator('#nw-title').click();
  await page.waitForTimeout(150);
  st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('closed:')) fail('바깥 클릭에 목록이 안 닫힘', st);
  if (!st.includes('충남')) fail('닫은 뒤 검색어 소실', st);
  if (!(await dlg.isVisible())) fail('모달이 닫혔다');

  await custInp.click();
  await page.waitForTimeout(150);
  st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('open:')) fail('재클릭 열기 실패', st);
  await custInp.click();
  await page.waitForTimeout(150);
  st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('closed:')) fail('같은 칸 재클릭에 안 닫힘', st);

  await custInp.click();
  await page.waitForTimeout(150);
  await custInp.press('Escape');
  await page.waitForTimeout(150);
  st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('closed:')) fail('Esc에 목록이 안 닫힘', st);
  if (!(await dlg.isVisible())) fail('Esc에 모달이 닫혔다');
  if (!st.includes('충남')) fail('Esc 후 검색어 소실', st);

  await custInp.click();
  await page.waitForTimeout(150);
  await custInp.press('Tab');
  await page.waitForTimeout(150);
  st = await page.evaluate(listDisplay(cust));
  if (!st.startsWith('closed:')) fail('Tab에 목록이 안 닫힘', st);
  if (!(await dlg.isVisible())) fail('Tab에 모달이 닫혔다');

  await custInp.click();
  await page.waitForTimeout(150);
  await projInp.click();
  await page.waitForTimeout(200);
  const both = await page.evaluate(() => {
    const c = document.querySelector('#new-work-form-slot [data-customer-combo]');
    const p = document.querySelector('#new-work-form-slot [data-project-picker]');
    const cd = Alpine.$data(c);
    const pd = Alpine.$data(p);
    return { c: !!cd.open, p: !!pd.open, q: cd.q };
  });
  if (both.c) fail('다른 콤보를 열었는데 거래처 목록이 남음', JSON.stringify(both));
  if (!both.p) fail('사업명 목록이 안 열림', JSON.stringify(both));
  if (both.q !== '충남') fail('다른 콤보 연 뒤 거래처 검색어 소실', JSON.stringify(both));

  const pick = await page.evaluate(() => {
    const root = document.querySelector('#new-work-form-slot [data-project-picker]');
    const btns = [...root.querySelectorAll('ul.combobox-list button')];
    const b = btns.find((el) => (el.textContent || '').includes('도서관사업'));
    if (!b) return { err: 'no-btn', n: btns.length };
    b.click();
    const d = Alpine.$data(root);
    return { err: '', id: d.selectedId, name: d.selectedName, open: d.open };
  });
  if (pick.err) fail('사업 선택 버튼 없음', JSON.stringify(pick));
  if (pick.id !== 'WP-O01' || pick.open) fail('사업 선택 회귀 실패', JSON.stringify(pick));
  await page.screenshot({ path: path.join(shotDir, '95-157-desktop.png') });

  if (editPath) {
    await page.goto(base + editPath, { waitUntil: 'domcontentloaded' });
    const editCust = page.locator('[data-customer-combo][data-jp-combo] input[type="search"]').first();
    await editCust.waitFor({ timeout: 8000 });
    await editCust.click();
    await page.waitForTimeout(200);
    let est = await page.evaluate(listDisplay('[data-customer-combo][data-jp-combo]'));
    if (!est.startsWith('open:')) fail('수정 화면 거래처 목록 안 열림', est);
    const box = await page.locator('[data-customer-combo][data-jp-combo]').first().boundingBox();
    if (!box) fail('수정 화면 콤보 위치 없음');
    await page.mouse.click(Math.max(8, box.x + 8), Math.max(8, box.y - 20));
    await page.waitForTimeout(150);
    est = await page.evaluate(listDisplay('[data-customer-combo][data-jp-combo]'));
    if (!est.startsWith('closed:')) fail('수정 화면 바깥 클릭에 안 닫힘', est);
  }

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(base + '/workboard/board?view=day&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  await openAdminForm(page);
  await page.locator('#new-work-form-slot [data-customer-combo] input[type="search"]').first().click();
  await page.waitForTimeout(200);
  const overflow = await page.evaluate(() => {
    const box = document.querySelector('[aria-labelledby="nw-title"]');
    if (!box) return 'no';
    const panel = box.querySelector(':scope > div') || box;
    return panel.scrollWidth > panel.clientWidth + 2 ? (panel.scrollWidth + '>' + panel.clientWidth) : '';
  });
  if (overflow) fail('390px 가로 넘침', overflow);
  await page.screenshot({ path: path.join(shotDir, '95-157-390.png') });
  console.log('95-157 browser ok');
} finally {
  await browser.close();
}
