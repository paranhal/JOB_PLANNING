// 95-183 수행 구조 수정: 실제 브라우저(임시 DB)에서 하부 업무 수정·순서·상태·삭제·되돌리기, 변경 내용 저장 막대, 계획 사이트·수량 화면.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const env = (k) => process.env[k];
const base = env('BASE_URL');
const cookie = env('AUTH_COOKIE');
const P = env('PROJECT_ID');
const PH = env('PHASE_ID');
const PKG = env('PKG_ID');
const QTY = env('QTY_PKG');
const EMPTY = env('EMPTY_PKG');
const PLAN = env('PLAN_ID');
const shotDir = env('SCREENSHOT_DIR') || '.';
if (!base || !cookie || !P || !PH || !PKG || !QTY || !EMPTY || !PLAN) {
  console.error('env');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) {
    try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; }
  }
  throw last;
}
function fail(msg, extra) { console.error('FAIL:', msg, extra === undefined ? '' : extra); process.exit(1); }
function ok(cond, msg, extra) { if (!cond) fail(msg, extra); }

async function open(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('dialog', (d) => d.accept()); // 삭제 확인창·이동 확인창
  return { context, page };
}
async function visit(page, p) {
  const r = await page.goto(base + p, { waitUntil: 'domcontentloaded' });
  ok(r && r.status() === 200, p + ' status ' + (r && r.status()));
  await page.waitForLoadState('load');
}
async function noOverflow(page) {
  return page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);
}
const pk = (id) => '#package-' + id;
const order = (page) => page.$$eval('[data-package-id]', (els) => els.map((e) => e.getAttribute('data-package-id')));
const lineOf = (page, id) => page.locator(pk(id) + ' > p').first().innerText();
async function openDetails(page, sel) {
  const d = page.locator(sel);
  ok(await d.count(), '접힘 없음 ' + sel);
  if (!(await d.evaluate((e) => e.open))) await d.locator('summary').first().click();
  return d;
}
async function shot(page, name) { await page.screenshot({ path: path.join(shotDir, name), fullPage: true }); }

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;

  // ---- A. 하부 업무 수정·순서·상태·삭제 ----
  await visit(page, '/projects/' + P);
  ok(!(await page.locator('#phase-' + PH + ' details[data-phase-edit]').evaluate((e) => e.open)), '1단계 수정이 접혀 있지 않다');
  for (const t of ['↑', '↓', '진행', '완료', '보류', '자동', '삭제']) {
    ok(await page.locator(pk(PKG) + ' [data-pkg-actions] button', { hasText: t }).count(), '하부 업무 버튼 없음: ' + t);
  }
  await shot(page, 'A1-overview-1440.png');

  // 이름 변경 → 같은 줄로 돌아온다
  let d = await openDetails(page, pk(EMPTY) + ' details[data-pkg-meta-edit]');
  await d.locator('input[name="name"]').fill('빈 업무 2');
  await d.locator('input[name="next_note"]').fill('검수 준비');
  await Promise.all([page.waitForNavigation(), d.locator('button[type="submit"]').click()]);
  ok(page.url().endsWith('#package-' + EMPTY), '이름 저장 뒤 위치', page.url());
  ok((await lineOf(page, EMPTY)).includes('빈 업무 2'), '이름이 바뀌지 않았다');
  const top = await page.locator(pk(EMPTY)).evaluate((e) => e.getBoundingClientRect().top);
  ok(top >= 0 && top < 900, '저장한 줄이 화면 밖이다', top);
  ok(await page.locator('[data-struct-ok]').innerText().then((s) => s.includes('저장')), '성공 문구 없음');

  // 중복 이름 → 오류 + 수정 접힘 유지
  d = await openDetails(page, pk(EMPTY) + ' details[data-pkg-meta-edit]');
  await d.locator('input[name="name"]').fill('수량 업무');
  await Promise.all([page.waitForNavigation(), d.locator('button[type="submit"]').click()]);
  ok((await page.locator('#phase-flash').innerText()).includes('같은 이름'), '중복 안내 없음');
  ok(await page.locator(pk(EMPTY) + ' details[data-pkg-meta-edit]').evaluate((e) => e.open), '오류 뒤 수정 접힘이 닫혔다');

  // 유형 변경 거절(사이트 있음)
  d = await openDetails(page, pk(PKG) + ' details[data-pkg-meta-edit]');
  await d.locator('select[name="track_mode"]').selectOption('phase');
  await Promise.all([page.waitForNavigation(), d.locator('button[type="submit"]').click()]);
  ok((await page.locator('#phase-flash').innerText()).includes('사이트 4곳'), '유형 변경 거절 안내 없음');

  // 순서
  let o = await order(page);
  ok(o[0] === PKG, '초기 순서', o);
  await Promise.all([page.waitForNavigation(), page.locator(pk(PKG) + ' button[aria-label="아래로"]').click()]);
  o = await order(page);
  ok(o[0] === QTY && o[1] === PKG, '↓ 뒤 순서', o);
  await Promise.all([page.waitForNavigation(), page.locator(pk(PKG) + ' button[aria-label="위로"]').click()]);
  o = await order(page);
  ok(o[0] === PKG, '↑ 뒤 순서', o);

  // 완료: 미완료 사이트 → 확인 패널 → 확정
  await Promise.all([page.waitForNavigation(), page.locator(pk(PKG) + ' [data-pkg-actions] button', { hasText: '완료' }).click()]);
  ok(await page.locator('[data-pkg-confirm]').isVisible(), '완료 확인 패널 없음');
  await shot(page, 'A2-done-confirm-1440.png');
  await Promise.all([page.waitForNavigation(), page.locator('[data-pkg-confirm] button', { hasText: '완료로 확정' }).click()]);
  ok((await lineOf(page, PKG)).includes('완료'), '완료 표시 없음', await lineOf(page, PKG));
  await Promise.all([page.waitForNavigation(), page.locator(pk(PKG) + ' [data-pkg-actions] button', { hasText: '자동' }).click()]);
  ok(!(await lineOf(page, PKG)).includes('· 완료 ·'), '자동으로 되돌리기 실패', await lineOf(page, PKG));
  await Promise.all([page.waitForNavigation(), page.locator(pk(QTY) + ' [data-pkg-actions] button', { hasText: '보류' }).click()]);
  ok((await lineOf(page, QTY)).includes('보류'), '보류 표시 없음', await lineOf(page, QTY));

  // 삭제: 실적 없으면 삭제, 계획이 있으면 비활성 → 비활성 목록 → 되돌리기
  await Promise.all([page.waitForNavigation(), page.locator(pk(EMPTY) + ' [data-pkg-actions] button', { hasText: '삭제' }).click()]);
  ok((await page.locator(pk(EMPTY)).count()) === 0, '삭제한 하부 업무가 남아 있다');
  await Promise.all([page.waitForNavigation(), page.locator(pk(PKG) + ' [data-pkg-actions] button', { hasText: '삭제' }).click()]);
  ok((await page.locator('[data-struct-ok]').innerText()).includes('비활성'), '비활성 안내 없음');
  const inactive = await openDetails(page, '[data-inactive-pkgs]');
  await shot(page, 'A3-inactive-1440.png');
  await Promise.all([page.waitForNavigation(), inactive.locator('button', { hasText: '되돌리기' }).click()]);
  ok(await page.locator(pk(PKG)).count(), '되돌린 하부 업무가 없다');

  // ---- B. 변경 내용 저장 막대 ----
  await visit(page, '/projects/' + P);
  const bar = page.locator('[data-struct-bar]');
  ok(!(await bar.isVisible()), '수정 전에 저장 막대가 보인다');
  const phaseName = page.locator('#phase-' + PH + ' form[data-struct-form="phase"] input[name="name"]');
  const phaseOrig = await (async () => { await openDetails(page, '#phase-' + PH + ' details[data-phase-edit]'); return phaseName.inputValue(); })();
  await phaseName.fill(phaseOrig + ' 수정');
  await openDetails(page, pk(QTY) + ' details[data-pkg-meta-edit]');
  await page.locator(pk(QTY) + ' form[data-struct-form="package"] input[name="name"]').fill('수량 업무 수정');
  await openDetails(page, '[data-plan-edit="' + PLAN + '"]');
  await page.locator('#plan-' + PLAN + ' form[data-struct-form="plan"] input[name="name"]').fill('1차 계획 수정');
  ok(await bar.isVisible(), '수정 뒤 저장 막대가 안 보인다');
  ok((await page.locator('[data-struct-count]').innerText()).includes('3곳'), '수정 개수', await page.locator('[data-struct-count]').innerText());
  ok((await page.locator('[data-dirty-badge]').count()) === 3, '「수정됨」 표시 개수', await page.locator('[data-dirty-badge]').count());
  await shot(page, 'B1-dirty-bar-1440.png');
  await phaseName.fill(phaseOrig); // 원래 값으로 되돌리면 표시가 사라진다
  ok((await page.locator('[data-struct-count]').innerText()).includes('2곳'), '원복 뒤 개수', await page.locator('[data-struct-count]').innerText());

  // 이동 확인(beforeunload)
  const guarded = await page.evaluate(() => { const e = new Event('beforeunload', { cancelable: true }); window.dispatchEvent(e); return e.defaultPrevented; });
  ok(guarded, '수정이 있는데 이동 확인이 없다');

  // 실패: 중복 이름 → 전부 취소, 입력값·접힘 유지
  await page.locator(pk(QTY) + ' form[data-struct-form="package"] input[name="name"]').fill('홍성');
  await phaseName.fill(phaseOrig + ' 취소될 이름');
  await page.locator('[data-struct-save]').click();
  await page.waitForSelector('[data-struct-err]');
  ok((await page.locator('[data-struct-msg]').innerText()).includes('같은 이름'), '실패 문구 없음', await page.locator('[data-struct-msg]').innerText());
  ok(await phaseName.inputValue() === phaseOrig + ' 취소될 이름', '실패 뒤 입력값이 사라졌다');
  ok(await bar.isVisible(), '실패 뒤 막대가 사라졌다');
  await shot(page, 'B2-batch-error-1440.png');
  await visit(page, '/projects/' + P); // 이동 확인은 위 핸들러가 수락
  await openDetails(page, '#phase-' + PH + ' details[data-phase-edit]');
  ok(await page.locator('#phase-' + PH + ' form[data-struct-form="phase"] input[name="name"]').inputValue() === phaseOrig, '실패한 일괄 저장이 일부 저장됐다');

  // 취소(원래대로)
  await page.locator(pk(QTY) + ' details[data-pkg-meta-edit] summary').click();
  const qn = page.locator(pk(QTY) + ' form[data-struct-form="package"] input[name="name"]');
  const qOrig = await qn.inputValue();
  await qn.fill('임시');
  ok(await bar.isVisible(), '취소 시험: 막대 없음');
  await page.locator('[data-struct-reset]').click();
  ok(!(await bar.isVisible()) && (await qn.inputValue()) === qOrig, '취소 뒤 원래 값으로 안 돌아왔다');

  // 성공: 여러 곳을 한 번에, 열린 접힘 유지, 이중 클릭은 한 번만
  let posts = 0;
  page.on('request', (r) => { if (r.url().includes('/structure/batch') && r.method() === 'POST') posts++; });
  await openDetails(page, '#phase-' + PH + ' details[data-phase-edit]');
  await phaseName.fill(phaseOrig + ' B');
  await qn.fill('수량 업무 B');
  await page.locator('#plan-' + PLAN + ' details[data-plan-edit] summary').click();
  await page.locator('#plan-' + PLAN + ' form[data-struct-form="plan"] input[name="name"]').fill('1차 계획 B');
  await page.evaluate(() => window.scrollTo(0, 300));
  const yBefore = await page.evaluate(() => window.scrollY);
  await Promise.all([page.waitForNavigation({ timeout: 8000 }), page.locator('[data-struct-save]').dblclick()]);
  await page.waitForLoadState('load');
  ok(posts === 1, '이중 클릭이 요청을 여러 번 보냈다', posts);
  const html = await page.content();
  ok(html.includes(phaseOrig + ' B') && html.includes('수량 업무 B') && html.includes('1차 계획 B'), '일괄 저장 값이 반영되지 않았다');
  ok((await page.locator('[data-struct-ok]').innerText()).includes('3곳을 저장했습니다'), '저장 결과 문구', await page.locator('[data-struct-ok]').innerText());
  ok(!(await bar.isVisible()), '저장 뒤에 막대가 남아 있다');
  ok(await page.locator('#plan-' + PLAN + ' details[data-plan-edit]').evaluate((e) => e.open), '열려 있던 접힘이 닫혔다');
  const yAfter = await page.evaluate(() => window.scrollY);
  ok(Math.abs(yAfter - yBefore) < 120, '스크롤 위치가 유지되지 않았다', [yBefore, yAfter]);
  await shot(page, 'B3-batch-saved-1440.png');

  // ---- C. 계획 사이트·수량 화면 ----
  await visit(page, '/projects/' + P + '/plans/' + PLAN + '/sites');
  await shot(page, 'C1-plan-sites-1440.png');
  const nCurrent = await page.locator('[data-plan-site]').count();
  ok(nCurrent === 2, '이 계획의 사이트 수', nCurrent);
  ok((await page.locator('[data-cand]').count()) === 2, '후보 수');
  await page.locator('[data-select-cands]').check();
  await Promise.all([page.waitForNavigation(), page.locator('[data-add-selected]').click()]);
  ok((await page.locator('[data-plan-site]').count()) === 4, '담은 뒤 사이트 수');
  ok((await page.locator('[data-flash-ok]').innerText()).includes('담았습니다'), '담기 문구');
  // 수량 입력·저장
  const qin = page.locator('[data-qty-input]');
  await qin.nth(0).fill('4');
  await qin.nth(1).fill('6');
  await Promise.all([page.waitForNavigation(), page.locator('[data-qty-save]').click()]);
  ok((await qin.nth(0).inputValue()) === '4' && (await qin.nth(1).inputValue()) === '6', '수량 저장값');
  // 같은 수량 적용
  await page.locator('[data-current-check]').nth(2).check();
  await page.locator('[data-current-check]').nth(3).check();
  await page.locator('[data-same-qty]').fill('7');
  await Promise.all([page.waitForNavigation(), page.locator('[data-apply-same]').click()]);
  ok((await qin.nth(2).inputValue()) === '7' && (await qin.nth(3).inputValue()) === '7', '같은 수량 적용값');
  // 제외(사유 필수)
  await page.locator('[data-current-check]').nth(3).check();
  await Promise.all([page.waitForNavigation(), page.locator('[data-exclude-selected]').click()]);
  ok((await page.locator('[data-flash-err]').innerText()).includes('사유'), '사유 없는 제외 안내');
  await page.locator('[data-current-check]').nth(3).check();
  await page.locator('[data-exclude-reason]').fill('현장 철거');
  await Promise.all([page.waitForNavigation(), page.locator('[data-exclude-selected]').click()]);
  ok((await page.locator('[data-plan-site]').count()) === 3, '제외 뒤 사이트 수');
  ok(await page.locator('[data-plan-excluded]').count(), '제외한 사이트 목록 없음');
  await shot(page, 'C2-plan-sites-after-1440.png');

  // ---- D. 폭별 가로 넘침 ----
  for (const [name, vp] of [['390', { width: 390, height: 844 }], ['820', { width: 820, height: 1000 }]]) {
    const m = await open(browser, vp);
    await visit(m.page, '/projects/' + P);
    await openDetails(m.page, '#phase-' + PH + ' details[data-phase-edit]');
    await openDetails(m.page, pk(PKG) + ' details[data-pkg-meta-edit]');
    await m.page.locator(pk(PKG) + ' form[data-struct-form="package"] input[name="name"]').fill('폭 시험');
    ok(await noOverflow(m.page), name + ' 수행 구조 가로 넘침');
    const b = await m.page.locator('[data-struct-bar]').boundingBox();
    ok(b && b.x >= -1 && b.x + b.width <= vp.width + 1, name + ' 저장 막대가 화면 밖', b);
    await shot(m.page, 'D-' + name + '-structure.png');
    await visit(m.page, '/projects/' + P + '/plans/' + PLAN + '/sites');
    ok(await noOverflow(m.page), name + ' 계획 사이트 화면 가로 넘침');
    await shot(m.page, 'D-' + name + '-plan-sites.png');
    await m.context.close();
  }
  console.log('OK 95-183 browser');
} finally {
  await browser.close();
}
