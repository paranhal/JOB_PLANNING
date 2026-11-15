// 95-205 주간업무보고: 연속·반복 업무 1줄 축약(기간), 같은 문구 중복 제거, HWPX 「## ##」 없음. 390/820/1440.
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
const W = '/stats/company-weekly?week=2026-10-19&org=O03';
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
  const context = await browser.newContext({ viewport: vp, acceptDownloads: true });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + W, { waitUntil: 'load' });
  const vals = await page.evaluate(() => Array.from(document.querySelectorAll('textarea')).map((t) => t.name + '=' + t.value));
  const joined = vals.join('\n');
  ok(joined.includes('제안서 작성(10/20~10/22)'), label + ' 묶음 기간 줄이 칸에 없다', vals);
  ok(!/제안서 작성 3회/.test(joined), label + ' 완료 줄에 N회가 남았다');
  ok((joined.match(/신구대조표 작성 3회\(이월\)/g) || []).length === 1, label + ' 이월 줄이 1줄이 아니다', vals);
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  if (label === '1440') {
    await page.screenshot({ path: path.join(shotDir, '95-205-edit-1440.png'), fullPage: true });
    // 묶음 줄을 고쳐 저장 → 다시 집계해도 고친 줄 유지
    const sel = 'textarea[name^="done_"]';
    const ta = page.locator(sel).filter({ hasText: '제안서 작성(10/20~10/22)' }).first();
    await ta.fill('제안서 작성(10/20~10/22) 직접 수정');
    await Promise.all([page.waitForURL(/edit=1/), page.locator('[data-weekly-save]').click()]);
    await page.waitForLoadState('load');
    ok((await page.locator(sel).filter({ hasText: '직접 수정' }).count()) >= 1, '저장한 직접 수정이 없다');
    await Promise.all([page.waitForLoadState('load'), page.locator('[data-weekly-reagg]').click()]);
    await page.waitForLoadState('load');
    const after = (await page.evaluate(() => Array.from(document.querySelectorAll('textarea')).map((t) => t.value))).join('\n');
    ok(after.includes('제안서 작성(10/20~10/22) 직접 수정'), '다시 집계 뒤 직접 수정이 사라졌다');
    ok((after.match(/신구대조표 작성 3회\(이월\)/g) || []).length === 1, '재집계 뒤 이월 줄이 1줄이 아니다');
    await page.screenshot({ path: path.join(shotDir, '95-205-after-reagg-1440.png'), fullPage: true });
    // HWPX 내려받기 → 파일을 열어 줄 확인
    await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    if (!(await page.locator('details:has(> summary:has-text("엑셀·HWPX 저장"))').evaluate((d) => d.open))) await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    const [dh] = await Promise.all([page.waitForEvent('download'), page.locator('[data-export-hwpx]').click()]);
    ok(dh.suggestedFilename().endsWith('.hwpx'), 'HWPX 이름');
    const file = await dh.path();
    const xml = execFileSync('unzip', ['-p', file, 'Contents/section0.xml']).toString('utf8');
    ok(!xml.includes('## ##'), 'HWPX 에 「## ##」가 있다');
    ok(xml.includes('## 기술영업활동'), 'HWPX 활동 행 이름');
    ok(xml.includes('제안서 작성(10/20~10/22)'), 'HWPX 에 묶음 기간 줄이 없다(직접 수정 전 자동 줄)') || 0;
  } else {
    await page.screenshot({ path: path.join(shotDir, '95-205-edit-' + label + '.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
