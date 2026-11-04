import { pathToFileURL } from 'node:url';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const here = dirname(fileURLToPath(import.meta.url));

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

const browser = await launch();
const page = await browser.newPage();
const posts = [];
page.on('request', (r) => {
  if (r.method() === 'POST' && r.url().includes('/update')) {
    posts.push(r.postData() || '');
  }
});
await page.route('**/as/*/update', async (route) => {
  await route.fulfill({ status: 303, headers: { Location: '/as/AS1/action?ok=1' }, body: '' });
});

await page.goto(pathToFileURL(join(here, '95_105_save_ok.html')).href);
await page.waitForFunction(() => window.ASActionSets && window.ASActionSets.prepareSubmit);

const hidReq = await page.evaluate(() => {
  const form = document.getElementById('as-action-form');
  const ta = document.getElementById('as-action-taken');
  if (ta) { ta.required = false; ta.removeAttribute('required'); }
  if (window.ASActionSets.prepareSubmit) window.ASActionSets.prepareSubmit();
  return Array.from(form.querySelectorAll('[required]')).filter((el) => {
    return el.classList.contains('hidden') || el.type === 'hidden' || el.closest('.hidden, [hidden], [aria-hidden="true"]');
  }).map((el) => el.id || el.name);
});
if (hidReq.length) fail('T2 숨은 required', hidReq);

posts.length = 0;
await page.click('#as-save');
await page.waitForTimeout(250);
if (!posts.length) fail('T1 저장 POST 없음', await page.evaluate(() => document.getElementById('as-action-form').checkValidity()));
const body = posts[posts.length - 1];
if (!body.includes('action_taken=') || !decodeURIComponent(body.replace(/\+/g, ' ')).includes('원격으로 재기동')) {
  fail('T1 action_taken 미도달', body);
}
if (!body.includes('process_type=remote') && !body.includes('s1_process_type=remote')) fail('T1 처리유형 미도달', body);
if (!body.includes('s1_work_date=2026-11-03')) fail('T1 작업일 미도달', body);
if (!body.includes('result_code=done') && !body.includes('s1_result_code=done')) fail('T1 결과 미도달', body);

await page.goto(pathToFileURL(join(here, '95_105_save_ok.html')).href);
await page.waitForFunction(() => window.ASActionSets);
await page.fill('.as-set-text', '짧음');
posts.length = 0;
const dlg = page.waitForEvent('dialog');
const clickShort = page.click('#as-save');
const dialog = await dlg;
await dialog.dismiss();
await clickShort;
await page.waitForTimeout(500);
if (!posts.length) {
    const st = await page.evaluate(() => {
      const form = document.getElementById('as-action-form');
      const ta = document.getElementById('as-action-taken');
      return {
        shortOk: (document.getElementById('as-action-short-ok') || {}).value,
        validity: form ? form.checkValidity() : null,
        invalid: form ? Array.from(form.querySelectorAll(':invalid')).map((el) => ({ id: el.id, name: el.name, required: !!el.required })) : [],
        ta: ta ? ta.value : null,
        taReq: ta ? ta.required : null,
        text: (document.querySelector('.as-set-text') || {}).value,
        result: (document.querySelector('.as-set-result') || {}).value
      };
    });
    fail('T3 확인창 뒤 POST 없음', JSON.stringify(st));
  }
const b2 = posts[posts.length - 1];
if (!b2.includes('s1_process_type=remote') || !b2.includes('s1_work_date=2026-11-03')) fail('T3 처리유형·작업일 미도달', b2);
if (!b2.includes('s1_result_code=done') && !b2.includes('result_code=done')) fail('T3 결과 미도달', b2);
if (!b2.includes('action_short_ok=1')) fail('T3 short_ok 없음', b2);

await page.goto(pathToFileURL(join(here, '95_105_save_ok.html')).href);
await page.waitForFunction(() => window.ASActionSets);
await page.selectOption('.as-set-result', '');
posts.length = 0;
await page.click('#as-save');
await page.waitForTimeout(200);
const err = await page.locator('#as-action-client-err').textContent();
if (!err || !err.includes('결과')) fail('T4 필수 안내 없음', err);
const focused = await page.evaluate(() => document.activeElement && document.activeElement.className);
if (!String(focused).includes('as-set-result')) fail('T4 포커스 없음', focused);
if (posts.length) fail('T4 빈 결과인데 POST 됨', posts);

await browser.close();
console.log('95_105_save_ok.mjs ok');
