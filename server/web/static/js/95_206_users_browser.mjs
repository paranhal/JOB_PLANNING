// 95-206 사용자 관리: 아이디 항상 표시, 「수정」을 가로 스크롤 없이 누름, 이름 클릭·서랍 저장. 1440/1280/820/390. TEST 계정만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const rowOf = (page, user) => page.locator('#rows tr[data-user="' + user + '"]');
// 단추가 실제로 눈에 보이고(다른 요소에 가려지지 않고) 화면 안에 있는지
async function reallyVisible(page, loc) {
  return await loc.evaluate((el) => {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.left < 0 || r.right > window.innerWidth + 1 || r.top < 0 || r.bottom > window.innerHeight + 1) return { ok: false, r: [r.left, r.right, r.top, r.bottom], vw: window.innerWidth, vh: window.innerHeight };
    const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return { ok: !!top && (top === el || el.contains(top)), hit: top && top.tagName + '.' + top.className };
  });
}
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['1280', { width: 1280, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  await page.goto(base + '/admin/users', { waitUntil: 'load' });
  // 1) 아이디 표시(메일이 있는 사람 포함)
  const t1 = await rowOf(page, 'mail206').locator('[data-user-id]').innerText();
  ok(t1.includes('mail206'), label + ' 메일 있는 사용자의 아이디가 안 보인다', t1);
  ok((await rowOf(page, 'nomail206').locator('[data-user-id]').innerText()).includes('nomail206'), label + ' 메일 없는 사용자 아이디');
  ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), label + ' 페이지 가로 넘침');
  // 2) 수정 버튼: 가로 스크롤 없이 보이고 눌린다
  const btn = rowOf(page, 'mail206').locator('button.edit');
  await btn.scrollIntoViewIfNeeded();
  const v = await reallyVisible(page, btn);
  ok(v.ok, label + ' 「수정」이 화면 안에 보이지 않는다', v);
  const winX = await page.evaluate(() => window.scrollX);
  ok(winX === 0, label + ' 페이지가 가로로 밀렸다(scrollX)', winX);
  const wrapScrollLeft = await page.evaluate(() => document.querySelector('[data-users-scroll]').scrollLeft);
  ok(wrapScrollLeft === 0, label + ' 가로 스크롤 없이 보여야 한다', wrapScrollLeft);
  // 3) 열 숨김: 최근 활동은 2xl(1536) 이상에서만
  const recentVisible = await page.locator('#rows tr[data-user="mail206"] td').last().evaluate((td) => td.previousElementSibling && getComputedStyle(td.previousElementSibling).display !== 'none');
  ok(recentVisible === (vp.width >= 1536), label + ' 최근 활동 열 표시 규칙(2xl)', recentVisible);
  // 4) 안내: 표가 넘칠 때만
  const overflow = await page.evaluate(() => { const s = document.querySelector('[data-users-scroll]'); return s.scrollWidth > s.clientWidth + 2; });
  const hintShown = await page.locator('[data-users-hscroll-hint]').isVisible();
  ok(hintShown === overflow, label + ' 스크롤 안내 표시 규칙', { overflow, hintShown });
  await page.locator('#rows').scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, '95-206-after-' + label + '.png') });
  // 5) 「수정」 클릭 → 서랍(상단에 현재 아이디)
  await btn.click();
  await page.waitForFunction(() => document.getElementById('drawer').getBoundingClientRect().right <= window.innerWidth + 1);
  ok((await page.locator('#dSub').innerText()).includes('mail206'), label + ' 서랍 상단에 현재 아이디가 없다');
  const save = page.locator('#edit-form button[type="submit"]');
  const sv = await reallyVisible(page, save);
  ok(sv.ok, label + ' 서랍 저장 버튼이 가려졌다', sv);
  if (label === '390' || label === '820') await page.screenshot({ path: path.join(shotDir, '95-206-after-' + label + '-drawer.png') });
  await page.locator('#dCancel').click();
  // 6) 이름 클릭으로도 열린다
  await rowOf(page, 'nomail206').locator('.edit-name').click();
  await page.waitForFunction(() => document.getElementById('drawer').getBoundingClientRect().right <= window.innerWidth + 1);
  ok((await page.locator('#dName').inputValue()) === '메일없음', label + ' 이름 클릭 서랍에 이름');
  await page.locator('#dCancel').click();
  // 7) 저장(이름·메일) 후 목록 반영 — 1440 은 mail206, 390 은 nomail206 로
  if (label === '1440' || label === '390') {
    const user = label === '1440' ? 'mail206' : 'nomail206';
    const newName = '저장확인' + label, newMail = 'saved' + label + '@example.test';
    await rowOf(page, user).locator('button.edit').click();
    await page.waitForFunction(() => document.getElementById('drawer').getBoundingClientRect().right <= window.innerWidth + 1);
    await page.locator('#dName').fill(newName);
    await page.locator('#dEmail').fill(newMail);
    await Promise.all([page.waitForNavigation({ waitUntil: 'load' }), page.locator('#edit-form button[type="submit"]').click()]);
    await page.goto(base + '/admin/users', { waitUntil: 'load' });
    const rtxt = await rowOf(page, user).innerText();
    ok(rtxt.includes(newName) && rtxt.includes(newMail) && rtxt.includes(user), label + ' 저장한 이름·메일·아이디가 목록에 없다', rtxt);
  }
  // 8) 아이디 변경(390, 마지막 폭): 서랍의 아이디 변경 영역
  if (label === '390') {
    await rowOf(page, 'nomail206').locator('button.edit').click();
    await page.waitForFunction(() => document.getElementById('drawer').getBoundingClientRect().right <= window.innerWidth + 1);
    ok(await page.locator('#dRename').isVisible(), '아이디 변경 입력란이 안 보인다');
    await page.locator('#dRename').fill('renamed206');
    page.once('dialog', (d) => d.accept());
    await Promise.all([page.waitForLoadState('load'), page.locator('#btn-rename').click()]);
    await page.waitForTimeout(500);
    await page.goto(base + '/admin/users', { waitUntil: 'load' });
    ok((await page.locator('#rows').innerText()).includes('아이디: renamed206'), '아이디 변경 뒤 목록에 새 아이디가 없다');
    await page.screenshot({ path: path.join(shotDir, '95-206-after-390-renamed.png') });
  }
  await context.close();
}
await browser.close();
console.log('OK');
