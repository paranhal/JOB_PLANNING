// 95-185 1단계 칸의 「+ 하부 업무」 버튼·단계 안 추가 패널: 실제 클릭(임시 DB), 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const P = process.env.PROJECT_ID;
const PH1 = process.env.PHASE_ID;
const PH2 = process.env.PHASE2_ID;
const PKG = process.env.PKG_ID; // 계획이 있는 「홍성」
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !P || !PH1 || !PH2 || !PKG) { console.error('env'); process.exit(1); }

async function launch() {
  const { chromium } = require('playwright');
  let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) {
    try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; }
  }
  throw last;
}
function fail(msg, extra) { console.error('FAIL:', msg, extra === undefined ? '' : extra); process.exit(1); }
function ok(c, msg, extra) { if (!c) fail(msg, extra); }
async function open(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('dialog', (d) => d.accept());
  page.on('response', (r) => { if (r.status() === 404) console.error('404:', r.url()); });
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('console', (m) => { if (m.type() === 'error' || m.type() === 'warning') console.error('console.' + m.type() + ':', m.text()); });
  return { context, page };
}
async function visit(page, p) {
  const r = await page.goto(base + p, { waitUntil: 'domcontentloaded' });
  ok(r && r.status() === 200, p + ' status ' + (r && r.status()));
  await page.waitForLoadState('load');
}
const noOverflow = (page) => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
const panel = (page, ph) => page.locator('[data-add-package-panel="' + ph + '"]');
const btn = (page, ph) => page.locator('#phase-' + ph + ' [data-add-package-btn-row] [data-add-package], #phase-' + ph + ' button[data-add-package="' + ph + '"]').first();
async function shown(page, ph) {
  try { await panel(page, ph).waitFor({ state: 'visible', timeout: 2500 }); return true; } catch (e) { return false; }
}
async function hidden(page, ph) {
  try { await panel(page, ph).waitFor({ state: 'hidden', timeout: 2500 }); return true; } catch (e) { return false; }
}
async function shot(page, name) { await page.screenshot({ path: path.join(shotDir, name), fullPage: true }); }
async function submit(page, ph) {
  await Promise.all([page.waitForNavigation(), panel(page, ph).locator('button[type="submit"]').click()]);
  await page.waitForLoadState('load');
}
async function fill(page, ph, f) {
  const p = panel(page, ph);
  if (f.name !== undefined) await p.locator('input[name="name"]').fill(f.name);
  if (f.track) await p.locator('select[name="track_mode"]').selectOption(f.track);
  if (f.badge !== undefined) await p.locator('input[name="badge"]').fill(f.badge);
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;

  // 1) 단계마다 버튼이 있고 패널은 처음에 닫혀 있다
  await visit(page, '/projects/' + P);
  for (const ph of [PH1, PH2]) {
    ok(await page.locator('#phase-' + ph + ' button[data-add-package="' + ph + '"]').first().isVisible(), '버튼이 안 보인다 ' + ph);
    ok(!(await panel(page, ph).isVisible()), '패널이 처음부터 열려 있다 ' + ph);
  }
  ok(await page.locator('[data-no-packages]').isVisible(), '0개 단계 안내가 없다');
  ok((await page.locator('#phase-' + PH2 + ' button[data-add-package="' + PH2 + '"]').count()) === 2, '0개 단계에 버튼이 2개가 아니다');
  await shot(page, 'after-1440.png');

  // 2) 열기·포커스·취소
  const b1 = page.locator('#phase-' + PH1 + ' button[data-add-package="' + PH1 + '"]').first();
  await b1.click();
  ok(await shown(page, PH1), '버튼을 눌러도 패널이 안 열린다');
  ok(await hidden(page, PH2), '다른 단계 패널이 같이 열렸다');
  ok(await page.evaluate(() => document.activeElement && document.activeElement.getAttribute('name') === 'name'), '이름 칸에 포커스가 없다');
  await shot(page, 'after-panel-open-1440.png');
  await fill(page, PH1, { name: '임시' });
  await panel(page, PH1).locator('button', { hasText: '취소' }).click();
  ok(await hidden(page, PH1), '취소해도 안 닫힌다');
  await b1.click();
  ok((await panel(page, PH1).locator('input[name="name"]').inputValue()) === '', '취소 뒤 입력값이 남았다');

  // 3) 사이트 기준 추가 → 새 줄로 이동
  await fill(page, PH1, { name: '현장 점검', track: 'site', badge: '사전' });
  await submit(page, PH1);
  ok(/#package-PK-\d+$/.test(page.url()), '새 하부 업무 앵커로 안 갔다', page.url());
  const newId = page.url().split('#package-')[1];
  const row = page.locator('#package-' + newId);
  ok(await row.isVisible(), '새 줄이 안 보인다');
  const top = await row.evaluate((e) => e.getBoundingClientRect().top);
  ok(top >= 0 && top < 900, '새 줄이 화면 밖이다', top);
  ok((await page.locator('[data-struct-ok]').innerText()).includes('하부 업무를 추가했습니다'), '성공 문구 없음');
  ok(await row.locator('[data-pkg-sites-link]').count(), '사이트 기준 줄에 사이트 등록·관리가 없다');
  ok((await row.innerText()).includes('사전'), '배지가 안 보인다');
  ok(await hidden(page, PH1), '추가 뒤 패널이 열려 있다');
  await shot(page, 'after-added-1440.png');

  // 4) 수량만 추가
  await page.locator('#phase-' + PH1 + ' button[data-add-package="' + PH1 + '"]').first().click();
  await fill(page, PH1, { name: '장비 수량', track: 'phase' });
  await submit(page, PH1);
  const qid = page.url().split('#package-')[1];
  const qrow = page.locator('#package-' + qid);
  ok((await qrow.locator('[data-qty-line]').innerText()).includes('수량만'), '수량만 표기가 없다');
  ok((await qrow.locator('[data-pkg-sites-link]').count()) === 0, '수량만 줄에 사이트 등록·관리가 있다');

  // 5) 하부 업무 0개 단계: 안내의 버튼은 그 단계 패널을 열고, 추가하면 그 단계에 생긴다
  ok((await page.locator('#phase-' + PH2 + ' [data-package-id]').count()) === 0, '2단계에 이미 하부 업무가 있다');
  await page.locator('#phase-' + PH2 + ' [data-no-packages] button[data-add-package]').click();
  ok((await shown(page, PH2)) && (await hidden(page, PH1)), '안내 버튼이 엉뚱한 단계를 열었다');
  await fill(page, PH2, { name: '구축 1', track: 'site' });
  await submit(page, PH2);
  ok((await page.locator('#phase-' + PH2 + ' [data-package-id]').count()) === 1, '구축 단계에 안 생겼다');
  ok((await page.locator('#phase-' + PH1 + ' [data-package-id]', { hasText: '구축 1' }).count()) === 0, '엉뚱한 단계에 생겼다');
  ok((await page.locator('#phase-' + PH2 + ' [data-no-packages]').count()) === 0, '하부 업무가 생겼는데 없음 안내가 남았다');

  // 6) 오류: 중복 → 패널이 열린 채 입력값 유지
  const errCase = async (ph, f, want, prep) => {
    await visit(page, '/projects/' + P);
    await page.locator('#phase-' + ph + ' button[data-add-package="' + ph + '"]').first().click();
    if (prep) await prep();
    await fill(page, ph, f);
    await submit(page, ph);
    const msg = await page.locator('#phase-flash').innerText();
    ok(msg.includes(want), '오류 문구: ' + want, msg);
    ok(page.url().endsWith('#add-package-' + ph), '오류 뒤 위치', page.url());
    ok(await shown(page, ph), '오류 뒤 패널이 닫혔다');
    if (f.name && f.name.trim()) ok((await panel(page, ph).locator('input[name="name"]').inputValue()) === f.name.trim(), '입력값이 사라졌다');
    return msg;
  };
  await errCase(PH1, { name: '현장 점검', track: 'phase' }, '같은 이름의 하부 업무가 있습니다');
  await shot(page, 'after-error-1440.png');
  await errCase(PH1, { name: '현장(점검)', track: 'site' }, '같은 이름의 하부 업무가 있습니다'); // 정규화 중복
  const no8 = async () => { await panel(page, PH1).locator('input[name="badge"]').evaluate((e) => e.removeAttribute('maxlength')); };
  await errCase(PH1, { name: '배지 시험', badge: '123456789' }, '8자까지', no8);
  const noReq = async () => { await panel(page, PH1).locator('input[name="name"]').evaluate((e) => e.removeAttribute('required')); };
  await errCase(PH1, { name: '   ' }, '하부 업무명을 입력하세요', noReq);
  // 비활성 중복: 계획이 있는 「홍성」을 삭제(비활성)하고 같은 이름으로 추가
  await visit(page, '/projects/' + P);
  await Promise.all([page.waitForNavigation(), page.locator('#package-' + PKG + ' [data-pkg-actions] button', { hasText: '삭제' }).click()]);
  ok((await page.locator('[data-struct-ok]').innerText()).includes('비활성'), '비활성 준비 실패');
  const m = await errCase(PH1, { name: '홍성' }, '되돌리거나');
  ok(m.includes('비활성'), '비활성 안내 문구', m);

  // 7) 폭별: 가로 넘침 없음, 패널 열린 화면 스크린샷
  for (const [name, vp] of [['390', { width: 390, height: 844 }], ['820', { width: 820, height: 1000 }]]) {
    const w = await open(browser, vp);
    await visit(w.page, '/projects/' + P);
    await w.page.locator('#phase-' + PH1 + ' button[data-add-package="' + PH1 + '"]').first().click();
    ok(await shown(w.page, PH1), name + ' 패널이 안 열린다');
    ok(await noOverflow(w.page), name + ' 가로 넘침');
    await shot(w.page, 'after-' + name + '.png');
    await w.context.close();
  }
  console.log('OK 95-185 browser');
} finally {
  await browser.close();
}
