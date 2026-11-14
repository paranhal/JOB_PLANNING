// 95-190 주간업무보고: 기본 편집·고정 저장 막대·저장 뒤 편집 유지·이동 확인·엑셀/HWPX 저장, 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, KEYS = (process.env.ROW_KEYS || '').split(',');
const shotDir = process.env.SCREENSHOT_DIR || '.';
const W = '/stats/company-weekly?week=2026-10-19&org=O03';
if (!base || !cookie || !KEYS[0]) { console.error('env'); process.exit(1); }
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
async function open(vp) {
  const context = await browser.newContext({ viewport: vp, acceptDownloads: true });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const { context, page } = await open(vp);
  await page.goto(base + W, { waitUntil: 'load' });
  // 기본 편집
  const form = page.locator('[data-weekly-form]');
  ok(await form.count() === 1, label + ' 기본 편집이 아님');
  const bar = page.locator('[data-weekly-savebar]');
  // 고정 저장 막대: 페이지 맨 위에서도 화면 안에 보이고, 아래로 스크롤해도 그대로 보인다
  const inView = async () => { const b = await bar.boundingBox(); return b && b.y >= 0 && b.y + b.height <= vp.height + 1; };
  ok(await inView(), label + ' 저장 막대가 화면 안에 없다(맨 위)', await bar.boundingBox());
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await page.waitForTimeout(150);
  ok(await inView(), label + ' 저장 막대가 화면 안에 없다(맨 아래)', await bar.boundingBox());
  await page.evaluate(() => window.scrollTo(0, 0));
  ok(await noOverflow(page), label + ' 가로 넘침');
  ok((await bar.locator('[data-weekly-dirty-msg]').innerText()).includes('수정한 내용이 없습니다'), label + ' 초기 문구');
  if (label === '1440') {
    // 수정 → 표시 → 이동 시 확인(취소하면 그대로)
    const ta = page.locator('textarea[name="done_' + KEYS[0] + '"]');
    await ta.fill('브라우저에서 쓴 금주 내용');
    ok((await bar.locator('[data-weekly-dirty-msg]').innerText()).includes('저장하지 않은 수정'), '수정 표시');
    let seen = null;
    page.once('dialog', async (d) => { seen = d.type(); await d.dismiss(); });
    await page.locator('a[href*="week=2026-10-12"]').first().click({ noWaitAfter: true }).catch(() => {});
    await page.waitForTimeout(500);
    ok(seen === 'beforeunload', '이동 확인(beforeunload)이 뜨지 않았다', seen);
    ok(page.url().includes('week=2026-10-19'), '이동 확인을 취소했는데 이동했다 ' + page.url());
    // 저장 → 같은 화면(편집 유지), 내용 유지, 막대 문구 초기화
    await Promise.all([page.waitForURL(/edit=1/), page.locator('[data-weekly-save]').click()]);
    await page.waitForLoadState('load');
    ok(await page.locator('[data-weekly-form]').count() === 1, '저장 뒤 편집이 유지되지 않았다');
    ok((await page.locator('textarea[name="done_' + KEYS[0] + '"]').inputValue()).includes('브라우저에서 쓴 금주 내용'), '저장한 내용이 없다');
    ok((await page.locator('[data-weekly-dirty-msg]').innerText()).includes('수정한 내용이 없습니다'), '저장 뒤 문구');
    await page.screenshot({ path: path.join(shotDir, '95-190-edit-1440.png') });
    // 저장 뒤에는 이동 확인이 없다 → 이전 주로 가고, 편집 모드(기본)가 유지된다
    await page.locator('a[href*="week=2026-10-12"]').first().click();
    await page.waitForLoadState('load');
    ok(page.url().includes('week=2026-10-12'), '주차 이동 실패 ' + page.url());
    // 이번 주 보고 줄이 없는 주차: 편집 안 열려도 오류 없이 열림
    await page.goto(base + W, { waitUntil: 'load' });
    // 저장 메뉴: 엑셀·HWPX
    await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    const [dx] = await Promise.all([page.waitForEvent('download'), page.locator('a[href*="/export?"][href*="scope=team"]').first().click()]);
    ok(dx.suggestedFilename().endsWith('.xlsx'), '엑셀 파일 이름 ' + dx.suggestedFilename());
    if (!(await page.locator('details:has(> summary:has-text("엑셀·HWPX 저장"))').evaluate((d) => d.open))) await page.locator('summary:has-text("엑셀·HWPX 저장")').click();
    const [dh] = await Promise.all([page.waitForEvent('download'), page.locator('[data-export-hwpx]').click()]);
    ok(dh.suggestedFilename().endsWith('.hwpx'), 'HWPX 파일 이름 ' + dh.suggestedFilename());
    const p = await dh.path(); const st = fs.statSync(p);
    ok(st.size > 2000, 'HWPX 크기 ' + st.size);
    ok(fs.readFileSync(p).subarray(0, 2).toString() === 'PK', 'HWPX 가 zip 이 아니다');
    // 읽기로 보기 → 편집 끝내기 링크 → 읽기, 편집 다시
    await page.locator('details:has(> summary:has-text("엑셀·HWPX 저장"))').evaluate((d) => { d.open = false; });
    await page.locator('a:has-text("편집 끝내기")').click();
    await page.waitForLoadState('load');
    ok(await page.locator('[data-weekly-form]').count() === 0, '편집 끝내기 후에도 편집');
    ok(page.url().includes('edit=0'), 'edit=0 이 아님 ' + page.url());
    await page.locator('a[data-weekly-nav], a[href*="week=2026-10-12"]').first().click();
    await page.waitForLoadState('load');
    ok(page.url().includes('edit=0') && await page.locator('[data-weekly-form]').count() === 0, '읽기 모드가 이동 때 이어지지 않는다 ' + page.url());
  } else {
    await page.screenshot({ path: path.join(shotDir, '95-190-edit-' + label + '.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
