// 95-184 링크를 버튼으로 + 사이트 등록 화면의 「선택한 사이트 등록」 위치: 실제 클릭(임시 DB), 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const P = process.env.PROJECT_ID;
const PH2 = process.env.PHASE2_ID;
const NEWPKG = process.env.NEW_PKG_ID;
const PLAN = process.env.PLAN_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !P || !PH2 || !NEWPKG || !PLAN) { console.error('env'); process.exit(1); }

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
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  return { context, page };
}
async function visit(page, p) {
  const r = await page.goto(base + p, { waitUntil: 'domcontentloaded' });
  ok(r && r.status() === 200, p + ' status ' + (r && r.status()));
  await page.waitForLoadState('load');
}
const noOverflow = (page) => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
async function shot(page, name) { await page.screenshot({ path: path.join(shotDir, name), fullPage: true }); }
const num = async (loc) => parseInt((await loc.innerText()).replace(/[^0-9]/g, ''), 10);
async function nav(page, fn) { await Promise.all([page.waitForNavigation(), fn()]); await page.waitForLoadState('load'); }
async function waitDisabled(loc, want) {
  for (let i = 0; i < 40; i++) {
    if ((await loc.isDisabled()) === want) return true;
    await new Promise((r) => setTimeout(r, 50));
  }
  return false;
}
const isBtn = async (loc) => loc.evaluate((e) => e.tagName === 'A' && e.classList.contains('btn') && !e.className.includes('text-primary'));

const SITES = '/projects/' + P + '/phases/' + PH2 + '/sites';
const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;

  // ---- A. 사이트 등록 화면: 선택한 사이트 등록 위치·동작 ----
  await visit(page, SITES);
  const add = page.locator('[data-add-selected]');
  ok(await add.count() === 1, '선택한 사이트 등록 버튼이 1개가 아니다');
  ok(await waitDisabled(add, true), '체크한 곳이 없는데 버튼이 활성이다');
  ok((await page.locator('#site-form [data-add-selected]').count()) === 0, '버튼이 아직 표 폼 안에 있다');
  ok((await add.getAttribute('form')) === 'site-form', 'form 속성이 없다');
  const summary = page.locator('[data-site-summary]');
  const box = async (l) => l.boundingBox();
  const bS = await box(summary), bA = await box(add), bT = await box(page.locator('#site-form table'));
  ok(Math.abs((bA.y + bA.height / 2) - (bS.y + bS.height / 2)) < 30, '버튼이 안내 문구와 같은 줄이 아니다', [bS, bA]);
  ok(bA.x > bS.x + bS.width - 4, '버튼이 안내 문구 오른쪽이 아니다', [bS, bA]);
  ok(bA.y + bA.height < bT.y, '버튼이 표 위가 아니다');
  ok((await num(page.locator('[data-registered]'))) === 0, '처음 등록 수');
  const pool0 = await num(page.locator('[data-addable]'));
  ok(pool0 >= 6, '후보 수', pool0);
  await shot(page, 'after-sites-1440.png');

  const checks = page.locator('[data-site-check]');
  await checks.nth(0).check();
  await checks.nth(1).check();
  ok(await waitDisabled(add, false), '2곳을 골랐는데 버튼이 비활성이다');
  ok((await add.innerText()).includes('(2)'), '선택 개수 표시', await add.innerText());
  await checks.nth(1).uncheck();
  ok((await add.innerText()).includes('(1)'), '1곳 표시');
  await checks.nth(1).check();
  await page.locator('[data-select-all]').check();
  ok((await add.innerText()).includes('(' + (await checks.count()) + ')'), '전체 선택 표시');
  await page.locator('[data-select-all]').uncheck();
  ok(await waitDisabled(add, true), '전체 해제 뒤 버튼이 활성이다');
  await checks.nth(0).check();
  await checks.nth(1).check();
  await shot(page, 'after-sites-selected-1440.png');
  await nav(page, () => add.click());
  ok((await page.locator('[data-flash-ok]').innerText()).includes('2곳'), '등록 문구', await page.locator('[data-flash-ok]').innerText().catch(() => ''));
  ok((await num(page.locator('[data-registered]'))) === 2, '등록 수가 안 올랐다');
  ok((await num(page.locator('[data-addable]'))) === pool0 - 2, '등록 안 된 곳 수가 안 줄었다');

  // 검색·지역 필터가 유지된 상태에서 등록
  await page.locator('[data-site-q]').fill('홍');
  await nav(page, () => page.locator('[data-site-search-form] button[type=submit]').click());
  const rows = page.locator('[data-site-row]');
  const rowCount = await rows.count();
  ok(rowCount >= 3 && rowCount < pool0 + 0 + 6, '검색 결과 수', rowCount);
  let picked = false;
  for (let i = 0; i < rowCount && !picked; i++) {
    const r = rows.nth(i);
    if ((await r.locator('[data-site-added]').count()) === 0) { await r.locator('[data-site-check]').check(); picked = true; }
  }
  ok(picked, '미등록 행이 없다');
  await nav(page, () => add.click());
  ok((await page.locator('[data-site-q]').inputValue()) === '홍', '등록 뒤 검색어가 사라졌다');
  ok((await num(page.locator('[data-registered]'))) === 3, '필터 상태 등록 수', await num(page.locator('[data-registered]')));
  ok((await page.locator('[data-flash-ok]').innerText()).includes('1곳'), '필터 등록 문구');

  // 조회 결과 전체 등록(검색 유지)
  await nav(page, () => page.locator('[data-add-all]').click());
  const regAfterAll = await num(page.locator('[data-registered]'));
  ok(regAfterAll > 3, '조회 결과 전체 등록이 동작하지 않았다', regAfterAll);
  ok((await page.locator('[data-site-q]').inputValue()) === '홍', '전체 등록 뒤 검색어');

  // 제외: 사유 없음 → 안내, 사유 있음 → 제외
  const added = page.locator('[data-site-row]').filter({ has: page.locator('[data-site-added]') }).first();
  await added.locator('[data-site-check]').check();
  await nav(page, () => page.locator('[data-remove-selected]').click());
  ok(await page.locator('[data-flash-err]').count(), '사유 없는 제외 안내가 없다');
  const before = await num(page.locator('[data-registered]'));
  await page.locator('[data-site-row]').filter({ has: page.locator('[data-site-added]') }).first().locator('[data-site-check]').check();
  await page.locator('[data-remove-reason]').fill('현장 철거');
  await nav(page, () => page.locator('[data-remove-selected]').click());
  ok((await num(page.locator('[data-registered]'))) === before - 1, '제외 뒤 등록 수');
  ok((await page.locator('[data-flash-ok]').innerText()).includes('제외'), '제외 문구');

  // 2단계(하부 업무) 사이트 등록 화면에서도 같은 위치·동작
  await visit(page, '/projects/' + P + '/packages/' + NEWPKG + '/sites');
  const add2 = page.locator('[data-add-selected]');
  ok(await waitDisabled(add2, true), '2단계 화면: 버튼이 활성이다');
  const c2 = page.locator('[data-site-check]');
  await c2.nth(0).check();
  await c2.nth(1).check();
  ok((await add2.innerText()).includes('(2)'), '2단계 화면 개수');
  await nav(page, () => add2.click());
  ok((await page.locator('[data-flash-ok]').innerText()).includes('2곳'), '2단계 화면 등록 문구');

  // ---- B. 링크를 버튼으로: 클릭 동작 ----
  await visit(page, SITES);
  const back = page.locator('a', { hasText: '‹ 수행 구조로' }).first();
  ok(await isBtn(back), '뒤로가기가 버튼이 아니다');
  await nav(page, () => back.click());
  ok(page.url().endsWith('/projects/' + P + '#project-phases'), '뒤로가기 위치', page.url());
  ok(await isBtn(page.locator('[data-changes-link]')), '변경내역이 버튼이 아니다');
  await nav(page, () => page.locator('[data-changes-link]').click());
  ok(page.url().endsWith('/projects/' + P + '/changes'), '변경내역 이동', page.url());
  await visit(page, '/projects/' + P);
  for (const k of ['week', 'month', 'today']) {
    const a = page.locator('[data-progress-report] a[href*="report=' + k + '"]');
    await nav(page, () => a.click());
    ok(page.url().includes('report=' + k) && page.url().endsWith('#project-phases'), '보고 기간 이동', page.url());
    const cur = page.locator('[data-progress-report] a[href*="report=' + k + '"]');
    ok(((await cur.getAttribute('class')) || '').includes('btn-primary') && (await cur.getAttribute('aria-current')) === 'true', k + ' 선택 표시');
    for (const o of ['today', 'week', 'month'].filter((x) => x !== k)) {
      const oc = (await page.locator('[data-progress-report] a[href*="report=' + o + '"]').getAttribute('class')) || '';
      ok(oc.includes('btn-secondary') && !oc.includes('btn-primary'), o + ' 가 선택 상태로 보인다');
    }
  }
  ok(await isBtn(page.locator('[data-phase-sites-link]').first()), '1단계 사이트 등록·관리가 버튼이 아니다');
  await nav(page, () => page.locator('[data-phase-sites-link]').first().click());
  ok(/\/phases\/PH-\d+\/sites$/.test(page.url()), '1단계 사이트 화면 이동', page.url());
  await visit(page, '/projects/' + P);
  ok(await isBtn(page.locator('[data-plan-sites-link]').first()), '계획 사이트·수량 관리가 버튼이 아니다');
  ok(await isBtn(page.locator('[data-pkg-sites-link]').first()), '하부 업무 사이트 등록·관리가 버튼이 아니다');
  ok(await isBtn(page.locator('[data-register-shortcut]').first()), '사업에 사이트 등록이 버튼이 아니다');
  await nav(page, () => page.locator('[data-plan-sites-link]').first().click());
  ok(/\/plans\/PL-\d+\/sites$/.test(page.url()), '계획 사이트 화면 이동', page.url());
  const planBack = page.locator('a', { hasText: '‹ 수행 구조로' }).first();
  ok(await isBtn(planBack), '계획 화면 뒤로가기');
  await nav(page, () => planBack.click());
  ok(page.url().includes('#plan-'), '계획 줄 위치로 돌아오지 않았다', page.url());

  // 사업 상세 머리: 연차·사이트 등록 버튼이 수정 버튼과 같은 줄
  await visit(page, '/projects/' + P);
  const yb = await page.locator('[data-project-years-link]').boundingBox();
  const eb = await page.locator('a[href$="/edit"]').first().boundingBox();
  ok(yb && eb && Math.abs(yb.y - eb.y) < 60, '연차 버튼이 헤더 버튼 줄에 없다', [yb, eb]);
  await nav(page, () => page.locator('[data-project-years-link]').click());
  ok(page.url().endsWith('/years'), '연차 이동', page.url());

  // ---- C. 폭별 ----
  for (const [name, vp] of [['390', { width: 390, height: 844 }], ['820', { width: 820, height: 1000 }], ['1440', { width: 1440, height: 900 }]]) {
    const w = await open(browser, vp);
    await visit(w.page, '/projects/' + P);
    ok(await noOverflow(w.page), name + ' 사업 상세 가로 넘침');
    await shot(w.page, 'after-project-' + name + '.png');
    await visit(w.page, SITES);
    ok(await noOverflow(w.page), name + ' 사이트 등록 화면 가로 넘침');
    const cs = w.page.locator('[data-site-check]');
    await cs.nth(0).check();
    await cs.nth(1).check();
    const a = await w.page.locator('[data-add-selected]').boundingBox();
    const s = await w.page.locator('[data-site-summary]').boundingBox();
    ok(a && a.x >= 0 && a.x + a.width <= vp.width + 1, name + ' 선택 등록 버튼이 화면 밖', a);
    if (name === '390') ok(a.y >= s.y + s.height - 2, '390: 문구 아래로 줄바꿈되지 않았다', [s, a]);
    await shot(w.page, 'after-sites-' + name + '.png');
    await visit(w.page, '/projects/' + P + '/plans/' + PLAN + '/sites');
    ok(await noOverflow(w.page), name + ' 계획 사이트 화면 가로 넘침');
    await shot(w.page, 'after-plan-sites-' + name + '.png');
    await w.context.close();
  }
  console.log('OK 95-184 browser');
} finally {
  await browser.close();
}
