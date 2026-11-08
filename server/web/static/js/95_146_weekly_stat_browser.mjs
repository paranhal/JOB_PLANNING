import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);

async function launch() {
  const { chromium } = require('playwright');
  try {
    return await chromium.launch({ headless: true, channel: 'chrome' });
  } catch (e) {
    try {
      return await chromium.launch({ headless: true, channel: 'msedge' });
    } catch (e2) {
      return await chromium.launch({ headless: true });
    }
  }
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

const base = process.env.BASE_URL;
const token = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !token) fail('BASE_URL/AUTH_COOKIE 필요');

const browser = await launch();
const page = await browser.newPage();
await page.context().addCookies([{ name: 'token', value: token, url: base.endsWith('/') ? base : base + '/' }]);
await page.setExtraHTTPHeaders({ Cookie: 'token=' + token });
await page.setViewportSize({ width: 390, height: 844 });
const url = base + '/stats/company-weekly?week=2026-08-10&org=O03';
await page.goto(url, { waitUntil: 'networkidle' });
await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, token);

const wrap = page.locator('[data-weekly-stats]').first();
if (!(await wrap.count())) fail('통계 박스 없음', await page.content().then((s) => s.slice(0, 400)));
const mnt = wrap.locator('[data-weekly-stat-box="mnt"]');
const asBox = wrap.locator('[data-weekly-stat-box="as"]');
if ((await mnt.getAttribute('aria-expanded')) !== 'false') fail('초기 접힘 아님');
await mnt.click();
await page.waitForTimeout(200);
if ((await mnt.getAttribute('aria-expanded')) !== 'true') fail('펼침 aria-expanded');
const panel = wrap.locator('[data-weekly-stat-panel="mnt"]');
if (!(await panel.isVisible())) fail('패널 안 보임');
fs.mkdirSync(shotDir, { recursive: true });
await page.screenshot({ path: path.join(shotDir, '95-146-t6-390-panel.png'), fullPage: true });
await wrap.locator('[data-weekly-stat-collapse="mnt"]').click();
await page.waitForTimeout(200);
const focused = await page.evaluate(() => document.activeElement && document.activeElement.id);
if (!focused || !focused.includes('-mnt')) fail('접기 후 박스 포커스', focused);
if ((await mnt.getAttribute('aria-expanded')) !== 'false') fail('접기 후 닫힘');
await page.screenshot({ path: path.join(shotDir, '95-146-t6-390-focus.png'), fullPage: true });
await asBox.click();
await page.waitForTimeout(200);
if ((await asBox.getAttribute('aria-expanded')) !== 'true') fail('다른 박스 펼침');
if ((await mnt.getAttribute('aria-expanded')) !== 'false') fail('한 번에 하나');
await mnt.focus();
await page.keyboard.press('Enter');
await page.waitForTimeout(200);
if ((await mnt.getAttribute('aria-expanded')) !== 'true') fail('Enter 펼침');

await page.screenshot({ path: path.join(shotDir, '95-146-t6-390-other.png'), fullPage: true });
await page.screenshot({ path: path.join(shotDir, '95-146-t6-390.png'), fullPage: true });
await page.setViewportSize({ width: 1280, height: 800 });
await page.screenshot({ path: path.join(shotDir, '95-146-t6-desktop.png'), fullPage: true });
await browser.close();
console.log('95_146_weekly_stat_browser ok');
