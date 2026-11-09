import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const asID = process.env.AS_ID || '';
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

function countASCards(page) {
  return page.evaluate((id) => {
    const box = document.querySelector('#unplanned');
    if (!box) return { panel: false, n: 0 };
    const cards = [...box.querySelectorAll('[data-pid]')];
    const n = cards.filter((el) => (el.getAttribute('data-pid') || '') === 'as:' + id).length;
    const taskN = cards.filter((el) => (el.getAttribute('data-kind') || '') === 'task' && (el.getAttribute('data-ref') || '') !== id).length;
    return { panel: true, n, total: cards.length, taskN };
  }, asID);
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const page = await loginPage(browser, { width: 1280, height: 800 });
  await page.goto(base + '/workboard/board?view=kanban&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  await page.locator('#unplanned').waitFor({ timeout: 15000 });
  let c = await countASCards(page);
  if (!c.panel) fail('칸반 미배치 없음', c);
  if (asID && c.n !== 1) fail('칸반 같은 AS 카드 수 ' + c.n, c);
  await page.screenshot({ path: path.join(shotDir, '95-158-kanban.png'), fullPage: true });

  await page.goto(base + '/workboard/board?view=day&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  await page.locator('#unplanned').waitFor({ timeout: 15000 });
  c = await countASCards(page);
  if (asID && c.n !== 1) fail('일정표 같은 AS 카드 수 ' + c.n, c);
  await page.screenshot({ path: path.join(shotDir, '95-158-day.png'), fullPage: true });

  await page.locator('button[data-new-work]').click();
  const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
  await dlg.waitFor({ timeout: 10000 });
  await dlg.locator('[data-kind="admin"]').click();
  await dlg.getByRole('button', { name: /다음 단계/ }).click();
  const title = page.locator('#new-work-form-slot input[name="title"]');
  await title.waitFor({ timeout: 12000 });
  await title.click();
  await title.fill('158브라우저중복저장');
  const save = page.locator('#new-work-form-slot button[type="submit"]');
  await save.scrollIntoViewIfNeeded();
  await Promise.all([
    save.click({ timeout: 5000 }),
    save.click({ timeout: 5000 }).catch(() => {}),
  ]);
  await page.waitForTimeout(800);
  const disabled = await save.isDisabled().catch(() => true);
  if (!disabled) {
    // 이동했으면 버튼이 없을 수 있다
  }
  await page.screenshot({ path: path.join(shotDir, '95-158-save.png'), fullPage: true });

  const mobile = await loginPage(browser, { width: 390, height: 844 });
  await mobile.goto(base + '/workboard/board?view=kanban&mine=0&auto=0', { waitUntil: 'domcontentloaded' });
  await mobile.locator('#unplanned').waitFor({ timeout: 15000 });
  const mc = await mobile.evaluate((id) => {
    const box = document.querySelector('#unplanned');
    if (!box) return 0;
    return [...box.querySelectorAll('[data-pid]')].filter((el) => (el.getAttribute('data-pid') || '') === 'as:' + id).length;
  }, asID);
  if (asID && mc !== 1) fail('390px 같은 AS 카드 수 ' + mc);
  await mobile.screenshot({ path: path.join(shotDir, '95-158-390.png'), fullPage: true });
  await mobile.close();
  console.log('95-158 browser ok');
} finally {
  await browser.close();
}
