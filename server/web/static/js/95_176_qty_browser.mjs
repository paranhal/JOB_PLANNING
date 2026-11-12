import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const projectID = process.env.PROJECT_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !projectID) {
  console.error('env');
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

async function open(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  return { context, page };
}

async function noOverflow(page) {
  return page.evaluate(() => {
    const w = window.innerWidth;
    const html = document.documentElement.scrollWidth <= w + 8;
    const main = document.querySelector('main');
    const inner = !main || main.scrollWidth <= main.clientWidth + 8;
    return html && inner;
  });
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;
  await page.goto(base + '/projects/' + projectID + '#project-phases', { waitUntil: 'domcontentloaded' });

  const line = page.locator('[data-qty-line]').first();
  if (!(await line.isVisible())) fail('수량만 줄이 안 보임');
  if (!(await line.innerText()).includes('목표 수량 미설정')) fail('목표 미설정 표시 없음', await line.innerText());
  if (await page.locator('[data-qty-record]').count()) fail('목표 전에 수량 입력 칸이 보임');

  // 1) 목표 저장 (클릭)
  await page.locator('[data-qty-target] input[name="qty_total"]').fill('120');
  await page.locator('[data-qty-target] input[name="qty_unit"]').fill('식');
  await Promise.all([
    page.waitForURL(/ok=qty-target/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.locator('[data-qty-target] button[type="submit"]').click(),
  ]);
  if (!(await page.locator('[data-qty-line]').first().innerText()).includes('0/120식 (0%)')) fail('목표 저장 뒤 0/120식 표시 없음', await page.locator('[data-qty-line]').first().innerText());
  if (!(await page.locator('[data-qty-record]').count())) fail('목표 저장 뒤 수량 입력 칸이 없음');

  // 2) 처리 수량 입력 (클릭)
  await page.locator('[data-qty-record] input[name="qty"]').fill('30');
  await Promise.all([
    page.waitForURL(/ok=qty(?!-)/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.locator('[data-qty-record] button[type="submit"]').click(),
  ]);
  const text = await page.locator('[data-qty-line]').first().innerText();
  if (!text.includes('30/120식 (25%)')) fail('입력 뒤 30/120식 (25%) 없음', text);

  // 3) 목표 초과는 막힌다
  await page.locator('[data-qty-record] input[name="qty"]').fill('91');
  await Promise.all([
    page.waitForURL(/err=/, { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.locator('[data-qty-record] button[type="submit"]').click(),
  ]);
  if (!(await page.locator('body').innerText()).includes('목표 수량(120)을 넘습니다')) fail('목표 초과 안내 없음');
  if (!(await page.locator('[data-qty-line]').first().innerText()).includes('30/120식 (25%)')) fail('초과 입력이 반영됨');

  await page.screenshot({ path: path.join(shotDir, '95-176-qty-1440.png'), fullPage: true });
  await desk.context.close();

  for (const [w, h, name] of [[820, 1100, '820'], [390, 844, '390']]) {
    const m = await open(browser, { width: w, height: h });
    await m.page.goto(base + '/projects/' + projectID + '#project-phases', { waitUntil: 'domcontentloaded' });
    if (!(await m.page.locator('[data-qty-record]').first().isVisible())) fail(w + 'px 에서 수량 입력 칸이 안 보임');
    if (!(await noOverflow(m.page))) fail(w + 'px 가로 넘침');
    await m.page.screenshot({ path: path.join(shotDir, '95-176-qty-' + name + '.png'), fullPage: true });
    await m.context.close();
  }
  console.log('95_176_qty_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
