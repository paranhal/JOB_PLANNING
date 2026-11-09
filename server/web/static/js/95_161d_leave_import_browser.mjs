import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const headCookie = process.env.HEAD_COOKIE;
const xlsxPath = process.env.XLSX_PATH;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !headCookie || !xlsxPath) {
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

async function open(browser, cookie, viewport) {
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
  const head = await open(browser, headCookie, { width: 1280, height: 800 });
  await head.page.goto(base + '/admin/leaves/import', { waitUntil: 'domcontentloaded' });
  await head.page.locator('[data-import-file]').setInputFiles(xlsxPath);
  await head.page.locator('[data-import-preview]').click();
  await head.page.locator('table[data-import-preview] tbody tr').first().waitFor({ timeout: 15000 });
  const preview = await head.page.locator('table[data-import-preview]').innerText();
  if (!preview.includes('가상갑')) fail('미리보기 이름', preview);
  await head.page.locator('[data-import-confirm]').click();
  await head.page.waitForURL(/ok=/, { timeout: 15000, waitUntil: 'domcontentloaded' });
  const ok = await head.page.locator('[data-import-ok]').innerText();
  if (!ok.includes('확정')) fail('확정 메시지', ok);
  if (ok.includes('가상갑')) fail('개인값 표시', ok);
  await head.page.screenshot({ path: path.join(shotDir, '95-161D-desktop.png'), fullPage: true });

  const mobile = await open(browser, headCookie, { width: 390, height: 844 });
  await mobile.page.goto(base + '/admin/leaves/import', { waitUntil: 'domcontentloaded' });
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침');
  await mobile.page.screenshot({ path: path.join(shotDir, '95-161D-390.png'), fullPage: true });
  console.log('95_161d_leave_import_browser.mjs ok');
} finally {
  await browser.close();
}
