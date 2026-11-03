import { pathToFileURL } from 'node:url';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const here = dirname(fileURLToPath(import.meta.url));
const html = join(here, '95_92_as_action_fixture.html');

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

const browser = await launch();
const page = await browser.newPage({ viewport: { width: 390, height: 800 } });
const posts = [];
await page.route('**/as/*/action-sets', async (route) => {
  const req = route.request();
  const body = req.postData() || '';
  posts.push(body);
  const draft = body.includes('draft=1') || body.includes('name="draft"');
  const hasResult = /s1_result_code=done/.test(body) || body.includes('s1_result_code') && body.includes('done');
  if (!draft && !hasResult) {
    await route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: JSON.stringify({ error: '조치 결과를 먼저 선택해 주세요', code: 'result_code' })
    });
    return;
  }
  if (body.includes('s1_work_date=') && !body.includes('s1_work_date=2026') && hasResult && !body.includes('s1_work_date=2026-11-03')) {
    await route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: JSON.stringify({ error: '작업일을 입력하세요.', code: 'work_date_required' })
    });
    return;
  }
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      ok: true, seq: 1, set_id: 'ASET1', state: draft ? 'draft' : 'saved',
      missing: draft ? ['result_code'] : [],
      message: draft ? '조치 1 임시 저장됨 (결과 미선택 — 최종 저장 전에 결과를 선택해 주세요)' : '조치 1 임시 저장됨'
    })
  });
});

await page.goto(pathToFileURL(html).href);
await page.waitForFunction(() => window.ASActionSets);

await page.click('.as-set-save');
await page.waitForTimeout(200);
const toast1 = await page.locator('#as-set-toast').textContent();
if (!toast1.includes('임시 저장됨')) {
  console.error('임시 저장 토스트 없음', toast1);
  process.exit(1);
}
if (!posts.some((p) => p.includes('draft=1') || p.includes('draft'))) {
  const last = posts[posts.length - 1] || '';
  if (!last.includes('draft')) {
    console.error('draft=1 미전송', posts);
    process.exit(1);
  }
}

await page.selectOption('.as-set-result', 'done');
await page.waitForTimeout(200);
const errVisit = await page.locator('.as-set-err').textContent();
if ((errVisit || '').includes('방문일')) {
  console.error('방문일 오류가 나왔다', errVisit);
  process.exit(1);
}

await page.fill('.as-set-work-date', '');
await page.selectOption('.as-set-result', '');
await page.selectOption('.as-set-result', 'done');
await page.waitForTimeout(200);

const addErrHidden = await page.locator('#as-add-set-err').evaluate((el) => el.classList.contains('hidden'));
await page.selectOption('.as-set-result', '');
await page.click('#as-add-set');
await page.waitForTimeout(100);
const addMsg = await page.locator('.as-set-err').textContent();
if (!addMsg.includes('결과를 먼저 선택')) {
  console.error('조치 추가 안내 없음', addMsg);
  process.exit(1);
}

const toastBox = await page.locator('#as-set-toast').boundingBox();
if (toastBox && toastBox.y < 0) {
  console.error('토스트가 화면 밖', toastBox);
  process.exit(1);
}

await browser.close();
console.log('95-92 browser ok', addErrHidden);
