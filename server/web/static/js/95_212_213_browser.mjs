// 95-212 마감 표시·23:55 안내 띠, 95-213 업무 상태별 줄(진행 중·취소·차주 계획). 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
const pth213 = process.env.PAGE_PATH_213, pth212 = process.env.PAGE_PATH_212;
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror:', e.message));

  // ---- 95-213: 줄 규칙
  await page.goto(base + pth213, { waitUntil: 'load' });
  const txt = await page.locator('main, body').first().innerText();
  for (const w of ['서버 점검: 현장 방문하여 로그 확인(진행 중)', '납품 협의: 고객 요청으로 중단(취소)', '서버 점검: 재방문(10/28)', '제안서 작성(10/20~10/22)']) {
    ok(txt.includes(w), `${label} 95-213: 화면에 「${w}」 없음`);
  }
  ok(!txt.includes('서버 점검(이월)'), `${label} 95-213: 계획이 있는데 이월 줄이 남음`);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), `${label} 95-213: 가로 넘침`);
  await page.screenshot({ path: path.join(shotDir, `95-213-lines-${label}.png`), fullPage: label !== '820' });

  // ---- 95-212: 마감 표시 + 안내 띠(서버 시각 기준)
  await page.goto(base + pth212, { waitUntil: 'load' });
  const box = page.locator('[data-weekly-close]');
  ok(await box.count() === 1 && await box.isVisible(), `${label} 95-212: 마감 표시가 없다`);
  ok((await page.locator('[data-weekly-close-line]').innerText()).includes('1차 마감') , `${label} 95-212: 마감 문구`);
  const banner = page.locator('[data-weekly-close-banner]');
  ok(!(await banner.isVisible()), `${label} 95-212: 5분보다 많이 남았는데 띠가 보인다`);
  await page.screenshot({ path: path.join(shotDir, `95-212-close-${label}.png`) });
  // 마감 200초 전으로: 서버 시각 기준으로 계산한 값을 속성에 넣는다
  await page.evaluate(() => { const b = document.querySelector('[data-weekly-close]'); const off = Number(b.getAttribute('data-server-now-ms')) - Date.now(); b.setAttribute('data-close-at-ms', String(Date.now() + off + 200000)); });
  await page.waitForTimeout(1300);
  ok(await banner.isVisible(), `${label} 95-212: 5분 전 안내 띠가 안 보인다`);
  const t1 = await banner.innerText();
  ok(t1.includes('자동 집계 예정이니 저장 버튼을 눌러 데이터를 저장하세요') && /3:[0-5]\d/.test(t1), `${label} 95-212: 띠 문구·카운트다운`, t1);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), `${label} 95-212: 띠 때문에 가로 넘침`);
  await page.screenshot({ path: path.join(shotDir, `95-212-banner-${label}.png`) });
  // 마감 3초 전 → 마감 후 문구로 전환
  await page.evaluate(() => { const b = document.querySelector('[data-weekly-close]'); const off = Number(b.getAttribute('data-server-now-ms')) - Date.now(); b.setAttribute('data-close-at-ms', String(Date.now() + off + 2000)); });
  await page.waitForTimeout(4500);
  const t2 = await banner.innerText();
  ok(t2.includes('1차 마감되었습니다'), `${label} 95-212: 마감 후 문구 전환`, t2);
  await page.screenshot({ path: path.join(shotDir, `95-212-closed-${label}.png`) });
  await context.close();
}
await browser.close();
console.log('OK');
