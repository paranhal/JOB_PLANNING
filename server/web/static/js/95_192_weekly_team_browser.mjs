// 95-192 주간업무보고: 진행율 폼 제거, 팀 전체 금주·차주 입력/저장, 엑셀·HWPX 내려받기. 390/820/1440.
import { createRequire } from 'node:module';
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
  ok(await page.locator('form[action$="/progress"]').count() === 0, label + ' 진행율 폼이 남아 있다');
  ok(!(await page.locator('body').innerText()).includes('진행율 저장'), label + ' 「진행율 저장」 문구');
  const team = page.locator('[data-weekly-team]');
  ok(await team.count() === 1 && await team.isVisible(), label + ' 팀 전체 칸이 없다');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8), label + ' 가로 넘침');
  if (label === '1440') {
    await page.locator('[data-team-done]').fill('팀 금주 첫째\n팀 금주 둘째');
    await page.locator('[data-team-plan]').fill('팀 차주 첫째');
    ok((await page.locator('[data-weekly-dirty-msg]').innerText()).includes('저장하지 않은 수정'), '수정 표시');
    await Promise.all([page.waitForURL(/edit=1/), page.locator('[data-weekly-save]').click()]);
    await page.waitForLoadState('load');
    ok((await page.locator('[data-team-done]').inputValue()).includes('팀 금주 둘째'), '저장한 금주가 칸에 없다');
    ok((await page.locator('[data-team-plan]').inputValue()).includes('팀 차주 첫째'), '저장한 차주가 칸에 없다');
    await page.screenshot({ path: path.join(shotDir, '95-192-edit-1440.png') });
    // 읽기 모드: 팀 전체 구역
    await page.goto(base + W + '&edit=0', { waitUntil: 'load' });
    ok((await page.locator('[data-weekly-team-read]').innerText()).includes('팀 금주 첫째'), '읽기 모드 팀 전체');
    // 다른 저장(다시 집계) 뒤에도 팀 전체가 남는다
    await page.goto(base + W, { waitUntil: 'load' });
    await Promise.all([page.waitForLoadState('load'), page.locator('[data-weekly-reagg]').click()]);
    await page.waitForLoadState('load');
    ok((await page.locator('[data-team-done]').inputValue()).includes('팀 금주 둘째'), '다시 집계 뒤 팀 전체가 사라졌다');
    // 엑셀·HWPX 저장
    await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    const [dx] = await Promise.all([page.waitForEvent('download'), page.locator('a[href*="/export?"][href*="scope=team"]').first().click()]);
    ok(dx.suggestedFilename().endsWith('.xlsx'), '엑셀 이름');
    if (!(await page.locator('details:has(> summary:has-text("엑셀·HWPX 저장"))').evaluate((d) => d.open))) await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    const [dh] = await Promise.all([page.waitForEvent('download'), page.locator('[data-export-hwpx]').click()]);
    ok(dh.suggestedFilename().endsWith('.hwpx'), 'HWPX 이름');
    ok(fs.statSync(await dh.path()).size > 2000, 'HWPX 크기');
  } else {
    await page.screenshot({ path: path.join(shotDir, '95-192-edit-' + label + '.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
