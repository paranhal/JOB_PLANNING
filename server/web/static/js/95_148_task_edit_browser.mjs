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
const editPath = process.env.EDIT_PATH;
if (!base || !token || !editPath) fail('BASE_URL/AUTH_COOKIE/EDIT_PATH 필요');

const browser = await launch();
const page = await browser.newPage();
await page.context().addCookies([{ name: 'token', value: token, url: base.endsWith('/') ? base : base + '/' }]);
await page.setExtraHTTPHeaders({ Cookie: 'token=' + token });
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(base + editPath, { waitUntil: 'networkidle' });
await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, token);

const body = await page.content();
if (!body.includes('행정/지원업무 수정') && !body.includes('업무 종료일')) fail('수정 화면 아님', body.slice(0, 400));
if (!body.includes('사업A') || !body.includes('사업B')) fail('사업 후보 없음');
const miss = page.locator('[data-customer-combo] .text-danger');
if (await miss.count() && await miss.first().isVisible() && (await miss.first().innerText()).includes('다시 선택')) {
  fail('거래처 재선택 안내');
}
const due = page.locator('#task-due-date');
if (!(await due.count())) fail('종료일 칸 없음');
const copy = page.getByRole('button', { name: '시작일과 같게' });
if (!(await copy.count())) fail('시작일과 같게 없음');
await due.fill('');
await copy.click();
await page.waitForTimeout(200);
const dueVal = await due.inputValue();
const startVal = await page.locator('form input[name="work_date"][x-model="preWork"]').first().inputValue();
if (!dueVal || dueVal !== startVal) fail('시작일과 같게 미동작', dueVal + ' / ' + startVal);

fs.mkdirSync(shotDir, { recursive: true });
await page.screenshot({ path: path.join(shotDir, '95-148-t5-390.png'), fullPage: true });

await page.setViewportSize({ width: 1280, height: 800 });
await page.screenshot({ path: path.join(shotDir, '95-148-t5-desktop.png'), fullPage: true });

await page.goto(base + editPath + '?err=due_date', { waitUntil: 'networkidle' });
const banner = await page.locator('.bg-danger-soft').first().innerText();
if (!banner.includes('업무 종료일을 입력해 주세요')) fail('원인 문장 배너 없음', banner);
if (banner.trim() === 'task' || banner.includes('>task')) fail('코드값 노출', banner);
await page.screenshot({ path: path.join(shotDir, '95-148-t5-due-err.png'), fullPage: true });

await browser.close();
console.log('95-148 browser ok');
