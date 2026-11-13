// 95-187 A: 사업 상세 헤더 버튼 높이 측정(임시 DB), 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, P = process.env.PROJECT_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !P) { console.error('env'); process.exit(1); }
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('dialog', (d) => d.accept());
  await page.goto(base + '/projects/' + P, { waitUntil: 'load' });
  const r = await page.$$eval('[data-project-header-btn]', (els) => els.map((e) => {
    const b = e.getBoundingClientRect(); const cs = getComputedStyle(e);
    return { t: e.textContent.trim(), h: Math.round(b.height * 10) / 10, fs: cs.fontSize, pad: cs.paddingTop + '/' + cs.paddingLeft };
  }));
  if (r.length < 6) fail(label + ' 버튼 수', r);
  const hs = new Set(r.map((x) => x.h)), fss = new Set(r.map((x) => x.fs)), pads = new Set(r.map((x) => x.pad));
  if (hs.size !== 1 || fss.size !== 1 || pads.size !== 1) fail(label + ' 크기 불일치', r);
  const ov = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
  if (!ov) fail(label + ' 가로 넘침');
  console.log(label, 'buttons', r.length, 'height', [...hs][0], 'font', [...fss][0], 'pad', [...pads][0]);
  await page.screenshot({ path: path.join(shotDir, '95-187-header-' + label + '.png') });
  await context.close();
}
await browser.close();
console.log('OK');
