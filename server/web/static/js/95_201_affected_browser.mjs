// 95-201: 조치 화면 「영향 장비」 — 영역 열기 → 장비 2개 선택 + 직접 입력 → 조치 저장 → 상세에서 확인 → 자산 상세 이력 「영향」 배지. 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const CASES = JSON.parse(process.env.CASES); // { "1440": { id, show, asNumber }, ... }
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
const VPS = { '390': { width: 390, height: 844 }, '820': { width: 820, height: 1000 }, '1440': { width: 1440, height: 900 } };
const noOverflow = (p) => p.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 8);

async function pick(page, query, name) {
  const root = page.locator('[data-affected-details] .js-target-picker');
  const q = root.locator('.tp-q');
  await q.fill(query);
  const row = root.locator('.tp-list li label', { hasText: name }).first();
  await row.waitFor({ state: 'visible', timeout: 8000 });
  await row.locator('input.tp-check').check();
  await root.locator('.tp-apply').click();
  await root.locator('.tp-chips', { hasText: name }).waitFor({ timeout: 5000 });
}

for (const label of ['390', '820', '1440']) {
  const c = CASES[label];
  const context = await browser.newContext({ viewport: VPS[label] });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => fail(label + ' 브라우저 대화상자: ' + d.message()));

  await page.goto(`${base}/as/${c.id}/action`, { waitUntil: 'load' });
  const details = page.locator('[data-affected-details]');
  ok(await details.count() === 1, label + ' 영향 장비 영역이 없다');
  ok(!(await details.evaluate((el) => el.open)), label + ' 기본이 펼쳐져 있다');
  ok(await page.locator('[data-affected-block]').innerText().then((t) => t.includes('영향 장비 (선택)')), label + ' 제목');
  await details.locator('summary').scrollIntoViewIfNeeded();
  await details.locator('summary').click();
  ok(await details.evaluate((el) => el.open), label + ' 영역이 열리지 않는다');
  ok((await details.innerText()).includes('영향 장비는 업무를 만들지 않고 건수에도 더해지지 않습니다'), label + ' 안내 문구');
  ok(await noOverflow(page), label + ' 열린 영역 가로 넘침');

  await pick(page, 'EZ', 'EZ-SRP 프린터');
  await pick(page, '회원', '회원가입PC');
  await details.locator('textarea[name="affected_label"]').fill('열람실 PC');
  await details.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, `95-201-action-${label}.png`) });

  // 조치 세트 필수 칸: KLAS > KDevice(클라이언트 프로그램) > 설치·재설치 를 1→2→3차로 고른다(95-201 원인 분류)
  const set = page.locator('#as-action-sets');
  await set.scrollIntoViewIfNeeded();
  await page.locator('select[name="s1_cause_cat1"]').selectOption('klas');
  const cat2 = page.locator('select[name="s1_cause_cat2"]');
  await cat2.locator('option[value="klas.kdevice"]').waitFor({ state: 'attached', timeout: 5000 });
  ok((await cat2.locator('option[value="klas.kdevice"]').innerText()).includes('KDevice(클라이언트 프로그램)'), label + ' 2차에 KDevice 가 없다');
  await cat2.selectOption('klas.kdevice');
  const cat3 = page.locator('select[name="s1_cause_cat3"]');
  await cat3.locator('option[value="klas.kdevice.install"]').waitFor({ state: 'attached', timeout: 5000 });
  const opts3 = (await cat3.locator('option').allInnerTexts()).map((t) => t.trim());
  ok(['설치·재설치', '설정·자동 로그인', '장비 연동 오류', '기타'].every((t) => opts3.includes(t)), label + ' 3차 선택지', opts3);
  await cat3.selectOption('klas.kdevice.install');
  await page.locator('[name="s1_action_taken"]').fill('KDevice 프로그램을 재설치하고 연동 장비를 확인했습니다');
  await page.locator('select[name="s1_process_type"]').selectOption('remote');
  await page.locator('input.as-set-place[value="office"]').check({ force: true }).catch(() => {});
  const result = page.locator('[name="s1_result_code"]');
  const tag = await result.first().evaluate((el) => el.tagName + ':' + (el.type || ''));
  if (tag.startsWith('SELECT')) await result.first().selectOption('done');
  else await page.locator('[name="s1_result_code"][value="done"]').check({ force: true });
  await page.waitForTimeout(800);
  const dbg = await page.evaluate(() => ({ val: document.querySelector('[name="s1_result_code"]')?.value, err: document.querySelector('.as-set-err')?.innerText, miss: document.querySelector('.as-set-missing')?.innerText, opts: [...document.querySelectorAll('[name="s1_result_code"] option')].map((o) => o.value + ':' + o.textContent.trim()) }));
  console.log(label + ' 결과 선택 상태', JSON.stringify(dbg));
  await page.screenshot({ path: path.join(shotDir, `95-201-cause-${label}.png`) });

  // 조치 저장(같은 폼): 별도 저장 버튼 없음
  ok(await details.locator('button[type=submit]').count() === 0, label + ' 영향 장비 영역에 별도 저장 버튼이 있다');
  const saveBtn = page.locator('button[type=submit][form="as-action-form"]', { hasText: /^\s*저장\s*$/ }).first();
  const respP = page.waitForResponse((r) => r.url().includes(`/as/${c.id}/update`) && r.request().method() === 'POST', { timeout: 20000 });
  try {
    await saveBtn.click({ noWaitAfter: true, timeout: 4000 });
  } catch (e) {
    // 알려진 기존 문제(95-201 보고서 참고): 390 폭에서 하단 모바일 탭바가 「저장」 버튼을 가린다. 영향 장비와 무관하므로 JS 클릭으로 대신한다.
    if (!/intercepts pointer events/.test(e.message)) fail(label + ' 클릭 실패: ' + e.message.split('Call log:')[1]);
    console.log('[알림] ' + label + ': 저장 버튼이 모바일 탭바에 가려져 JS 클릭으로 대신한다');
    await saveBtn.evaluate((el) => el.click());
  }
  const resp = await respP.catch(async (e) => {
    const ce = await page.locator('#as-action-client-err').innerText().catch(() => '(없음)');
    const bad = await page.evaluate(() => [...(document.getElementById('as-action-form')?.elements || [])].filter((x) => x.willValidate && !x.checkValidity()).map((x) => (x.name || x.id) + ':' + x.validationMessage));
    fail(label + ' 저장 요청이 서버에 가지 않았다. 클라이언트 오류=' + ce, bad);
  });
  console.log(label + ' 저장 응답', resp.status(), resp.headers()['location'] || '');
  ok(resp.status() === 303 && !(resp.headers()['location'] || '').includes('err='), label + ' 저장 응답 오류', { s: resp.status(), loc: resp.headers()['location'] });
  await page.waitForLoadState('load');
  await page.waitForFunction(() => document.querySelector('[data-affected-details]') && document.readyState === 'complete', null, { timeout: 15000 });
  ok(!page.url().includes('err='), label + ' 저장 오류 ' + page.url());
  // 저장 뒤 조치 화면: 값이 채워져 있고 영역이 열려 있다
  const d2 = page.locator('[data-affected-details]');
  ok(await d2.evaluate((el) => el.open), label + ' 저장 뒤 영역이 접혀 있다');
  const chips = await d2.locator('.tp-chips').innerText();
  ok(chips.includes('EZ-SRP 프린터') && chips.includes('회원가입PC'), label + ' 저장 뒤 칩', chips);
  ok(await d2.locator('textarea[name="affected_label"]').inputValue() === '열람실 PC', label + ' 직접 입력이 유지되지 않는다');

  // 상세에서 「영향 장비: 이름 외 N대」
  await page.goto(`${base}${c.show}`, { waitUntil: 'load' });
  const line = page.locator('[data-affected-line]');
  ok(await line.count() === 1, label + ' 상세에 영향 장비 줄이 없다');
  const text = (await line.innerText()).trim();
  ok(/외 2대$/.test(text), label + ' 한 줄 형식', text);
  ok(await noOverflow(page), label + ' 상세 가로 넘침');
  await line.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, `95-201-show-${label}.png`) });

  // 자산 상세 이력: 영향 장비로 연결된 자산에는 「영향」 배지, 주 대상 자산에는 없음
  await page.goto(`${base}/assets/A201-PC`, { waitUntil: 'load' });
  const row = page.locator('tr', { hasText: c.asNumber }).first();
  ok(await row.count() === 1, label + ' 자산 이력에 접수가 없다');
  ok(await row.locator('[data-affected-badge]').count() === 1, label + ' 「영향」 배지가 없다');
  ok((await row.locator('[data-affected-badge]').innerText()).trim() === '영향', label + ' 배지 문구');
  ok(await noOverflow(page), label + ' 자산 이력 가로 넘침');
  await row.scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(shotDir, `95-201-asset-history-${label}.png`) });
  await page.goto(`${base}/assets/A201-MAIN`, { waitUntil: 'load' });
  ok(await page.locator('[data-affected-badge]').count() === 0, label + ' 주 대상 자산 이력에 배지가 있다');
  await context.close();
}
await browser.close();
console.log('OK');
