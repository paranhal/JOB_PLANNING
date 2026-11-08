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
const actionPath = process.env.ACTION_PATH;
if (!base || !token || !actionPath) fail('BASE_URL/AUTH_COOKIE/ACTION_PATH 필요');

const browser = await launch();
const page = await browser.newPage();
await page.context().addCookies([{ name: 'token', value: token, url: base.endsWith('/') ? base : base + '/' }]);
await page.setExtraHTTPHeaders({ Cookie: 'token=' + token });
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(base + actionPath + '#project', { waitUntil: 'networkidle' });
await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, token);
await page.locator('#as-action-side-mobile').evaluate((el) => { el.open = true; });

const body = await page.content();
if (!body.includes('접수 및 고객 정보') || !body.includes('id="project"')) fail('조치 화면 아님', body.slice(0, 400));
if (body.includes('타 부서 사업 보기')) fail('타 부서 보기 있음');
if (!body.includes('사업명을 검색해 연결하세요')) fail('새 안내 없음');

const search = page.locator('#project [data-project-picker] input[type="search"]').first();
await search.click();
const waitSearch = page.waitForResponse((r) => r.url().includes('/workboard/projects/search') && r.ok(), { timeout: 8000 }).catch(() => null);
await search.fill('');
await search.pressSequentially('사업A', { delay: 40 });
const searchRes = await waitSearch;
if (!searchRes) fail('검색 요청 없음');
await page.waitForTimeout(300);
const hit = page.locator('#project [data-project-picker] .combobox-list button', { hasText: '사업A' }).first();
if (!(await hit.count())) fail('검색 결과 없음', await page.locator('#project').innerHTML());
await hit.click();
const confirm = page.getByRole('button', { name: '등록하고 연결' });
await confirm.waitFor({ state: 'visible', timeout: 5000 }).catch(() => fail('확인 상자 없음'));
await confirm.click();
await page.waitForTimeout(400);
const waitSave = page.waitForResponse((r) => r.url().includes('/as/R151-b/project') && (r.status() === 303 || r.status() === 200), { timeout: 8000 }).catch(() => null);
await page.locator('#as-action-side-mobile #project button[type="submit"]').click();
if (!(await waitSave)) fail('사업 저장 요청 없음');
await page.waitForTimeout(400);
await page.goto(base + actionPath + '#project', { waitUntil: 'networkidle' });
const after = await page.content();
if (!after.includes('사업A')) fail('새로고침 후 사업명 없음', after.slice(0, 600));

fs.mkdirSync(shotDir, { recursive: true });
await page.screenshot({ path: path.join(shotDir, '95-151-t6-390.png'), fullPage: true });
await page.setViewportSize({ width: 1280, height: 800 });
await page.screenshot({ path: path.join(shotDir, '95-151-t6-desktop.png'), fullPage: true });

await browser.close();
console.log('95-151 browser ok');
