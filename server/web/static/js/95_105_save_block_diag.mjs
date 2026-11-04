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

const html = process.env.DIAG_HTML || join(here, '95_105_save_block_diag.html');
const browser = await launch();
const page = await browser.newPage();
const consoleMsgs = [];
const requests = [];
page.on('console', (m) => consoleMsgs.push(m.type() + ': ' + m.text()));
page.on('pageerror', (e) => consoleMsgs.push('pageerror: ' + e.message));
page.on('request', (r) => {
  if (r.method() === 'POST') requests.push(r.url() + ' ' + (r.postData() || '').slice(0, 400));
});
await page.goto(pathToFileURL(html).href);
await page.waitForFunction(() => document.getElementById('as-action-form'));

const before = await page.evaluate(() => {
  const form = document.getElementById('as-action-form');
  const ta = document.getElementById('as-action-taken');
  const inv = Array.from(form.querySelectorAll(':invalid')).map((el) => ({
    name: el.name, id: el.id, required: !!el.required, value: el.value,
    hidden: el.classList.contains('hidden') || el.type === 'hidden',
    display: el.ownerDocument.defaultView.getComputedStyle(el).display
  }));
  return {
    validity: form.checkValidity(),
    taRequired: !!(ta && ta.required),
    taValue: ta ? ta.value : null,
    setText: (document.querySelector('.as-set-text') || {}).value || '',
    invalid: inv
  };
});
console.log('BEFORE_CLICK ' + JSON.stringify(before, null, 2));

await page.click('#as-save');
await page.waitForTimeout(300);

const after = await page.evaluate(() => {
  const form = document.getElementById('as-action-form');
  const inv = Array.from(form.querySelectorAll(':invalid')).map((el) => ({
    name: el.name, id: el.id, required: !!el.required, value: el.value,
    hidden: el.classList.contains('hidden') || el.type === 'hidden',
    display: el.ownerDocument.defaultView.getComputedStyle(el).display
  }));
  return {
    validity: form.checkValidity(),
    invalid: inv,
    taRequired: !!(document.getElementById('as-action-taken') && document.getElementById('as-action-taken').required),
    taValue: document.getElementById('as-action-taken').value
  };
});
console.log('AFTER_CLICK ' + JSON.stringify(after, null, 2));
console.log('CONSOLE ' + JSON.stringify(consoleMsgs, null, 2));
console.log('POSTS ' + JSON.stringify(requests, null, 2));
await browser.close();
if (process.env.DIAG_EXPECT_POST === '1' && !requests.length) {
  console.error('expected POST missing');
  process.exit(1);
}
if (process.env.DIAG_EXPECT_BLOCK === '1') {
  if (after.validity !== false || requests.length) {
    console.error('expected blocked submit');
    process.exit(1);
  }
}
