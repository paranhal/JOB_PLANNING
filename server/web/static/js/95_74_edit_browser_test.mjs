import { pathToFileURL } from 'node:url';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const here = dirname(fileURLToPath(import.meta.url));
const html = join(here, '../review-95-74/edit-form-behavior.html');

async function launch() {
  const { chromium } = require('playwright');
  try {
    return await chromium.launch({ headless: true, channel: 'chrome' });
  } catch (e) {
    return await chromium.launch({ headless: true, channel: 'msedge' });
  }
}

const browser = await launch();
const page = await browser.newPage({ viewport: { width: 800, height: 600 } });
await page.goto(pathToFileURL(html).href);
await page.waitForFunction(() => window.Alpine);

await page.click('#save');
const emptyMsg = await page.locator('[data-assignee-banner]').textContent();
if (emptyMsg !== '담당자를 선택하세요') {
  console.error('담당자 오류 없음', emptyMsg);
  process.exit(1);
}

await page.evaluate(() => {
  const f = document.getElementById('edit-form');
  const i = document.createElement('input');
  i.type = 'hidden';
  i.name = 'assignee_user_id';
  i.value = 'USR-1';
  f.appendChild(i);
});

let confirmed = '';
page.once('dialog', async (d) => {
  confirmed = d.message();
  await d.accept();
});
await page.click('#save');
await page.waitForTimeout(200);
if (!confirmed.includes('완료되지 않은 실행 작업')) {
  console.error('확인창 없음', confirmed);
  process.exit(1);
}
await browser.close();
console.log('edit browser ok');
