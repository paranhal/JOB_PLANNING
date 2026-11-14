// 95-191: 완료·취소 업무의 등록정보 수정 — 관리자는 링크를 눌러 고치고, 수행 담당은 링크 없이 안내를 본다. 390/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, ADMIN = process.env.ADMIN_COOKIE, OWNER = process.env.OWNER_COOKIE;
const DONE = process.env.DONE_ID, PROG = process.env.PROG_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
async function open(cookie, vp) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['390', { width: 390, height: 844 }]]) {
  // 수행 담당(관리자급 아님): 링크 없음 + 안내, 직접 /edit 은 안내 화면(상세)으로
  {
    const { context, page } = await open(OWNER, vp);
    await page.goto(base + '/workboard/tasks/' + DONE, { waitUntil: 'load' });
    ok(await page.locator('[data-registration-edit]').count() === 0, label + ' 수행 담당에게 수정 링크');
    ok(await page.locator('[data-registration-lock]').count() === 1, label + ' 안내 문구 없음');
    ok((await page.locator('[data-registration-lock]').innerText()).includes('완료된 업무는 수정할 수 없습니다'), label + ' 안내 내용');
    ok(await noOverflow(page), label + ' 가로 넘침');
    await page.screenshot({ path: path.join(shotDir, '95-191-owner-' + label + '.png') });
    const r = await page.goto(base + '/workboard/tasks/' + DONE + '/edit', { waitUntil: 'load' });
    ok(r && r.status() === 200, label + ' /edit 직접 접근 상태 ' + (r && r.status()) + ' ' + (r && r.url()) + ' from ' + (r && r.request().redirectedFrom() && r.request().redirectedFrom().url()));
    ok(!(await page.locator('body').innerText()).includes('"message"'), label + ' JSON 이 보인다');
    ok(page.url().includes('/workboard/tasks/' + DONE) && !page.url().includes('/edit'), label + ' 상세로 돌아오지 않았다 ' + page.url());
    ok((await page.locator('body').innerText()).includes('완료된 업무는 수정할 수 없습니다'), label + ' 이동 뒤 안내');
    await page.screenshot({ path: path.join(shotDir, '95-191-owner-direct-' + label + '.png') });
    // 진행 중 건은 정상 수정
    await page.goto(base + '/workboard/tasks/' + PROG, { waitUntil: 'load' });
    ok(await page.locator('[data-registration-edit]').count() === 1, label + ' 진행 중 건 수정 링크');
    await context.close();
  }
  // 관리자: 링크 → 수정 화면 → 저장
  {
    const { context, page } = await open(ADMIN, vp);
    await page.goto(base + '/workboard/tasks/' + DONE, { waitUntil: 'load' });
    const link = page.locator('[data-registration-edit]');
    ok(await link.count() === 1, label + ' 관리자 수정 링크 없음');
    await link.click();
    await page.waitForLoadState('load');
    ok(page.url().includes('/edit'), label + ' 수정 화면이 아님 ' + page.url());
    ok(!(await page.locator('body').innerText()).includes('"message"'), label + ' JSON');
    if (label === '1440') {
      const title = page.locator('form[action$="/update"] input[name="title"]:visible').first();
      ok(await title.count() === 1, '제목 입력칸');
      await title.fill('완료 업무 브라우저 수정');
      await page.screenshot({ path: path.join(shotDir, '95-191-admin-edit-1440.png') });
      await Promise.all([page.waitForLoadState('load'), page.locator('form[action$="/update"] button[type=submit]').first().click()]);
      await page.waitForLoadState('load');
      await page.goto(base + '/workboard/tasks/' + DONE, { waitUntil: 'load' });
      ok((await page.locator('body').innerText()).includes('완료 업무 브라우저 수정'), '저장한 제목이 보이지 않는다');
    }
    ok(await noOverflow(page), label + ' 관리자 화면 가로 넘침');
    await context.close();
  }
}
await browser.close();
console.log('OK');
