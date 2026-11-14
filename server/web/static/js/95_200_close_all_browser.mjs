// 95-200: 「업무 모두 종료」 — 하위 상세와 상위 이력 표에서 각각 모달 → 건수 요약(미리보기 JSON 일치) → 사유 → 확정 → 결과. 390/820/1440.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const CASES = JSON.parse(process.env.CASES); // { "1440": { a:[parent,occ], b:[parent,occ] }, ... }
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
const insideModal = (p) => p.evaluate(() => { const m = document.getElementById('occ-close-modal'); return !!m && m.contains(document.activeElement); });

for (const label of ['390', '820', '1440']) {
  const vp = VPS[label], c = CASES[label];
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  page.on('pageerror', (e) => console.error('pageerror:', e.message));
  page.on('dialog', (d) => { fail(label + ' 브라우저 대화상자가 떴다(confirm 금지): ' + d.message()); });

  // ── A. 하위 업무 상세에서 종료 (manual, 이전 미완료 2건 함께 제외)
  {
    const [parent, occ] = c.a;
    await page.goto(`${base}/workboard/tasks/${occ}`, { waitUntil: 'load' });
    ok(await page.locator('[data-occ-close-all]').count() === 1, label + ' A: 하위 상세 버튼 수');
    await page.locator('[data-occ-close-all]').click();
    const modal = page.locator('#occ-close-modal');
    await modal.waitFor({ state: 'visible', timeout: 3000 });
    await page.locator('[data-occ-close-form]').waitFor({ state: 'visible', timeout: 5000 });
    // 요약 건수 = 미리보기 JSON
    const pv = await (await context.request.get(`${base}/workboard/tasks/${parent}/occurrences/${occ}/close-all/preview`)).json();
    ok(pv.later === 12 && pv.earlier === 2, label + ' A: 미리보기 JSON', pv);
    ok((await page.locator('[data-later]').innerText()).trim() === String(pv.later), label + ' A: 이후 건수 표시');
    ok((await page.locator('[data-earlier]').innerText()).trim() === String(pv.earlier), label + ' A: 이전 건수 표시');
    ok(await page.locator('[data-sum-earlier]').isVisible(), label + ' A: 이전 미완료 줄');
    ok(await page.locator('[data-include-earlier]').isChecked(), label + ' A: 「함께 제외」 기본 체크');
    ok(await page.locator('[data-final-box]').isHidden(), label + ' A: manual 인데 최종 결과 칸이 보인다');
    const submit = page.locator('[data-occ-close-submit]');
    ok(await submit.isDisabled(), label + ' A: 사유 없이 버튼이 켜져 있다');
    ok(await noOverflow(page), label + ' A: 가로 넘침');
    // Tab 순서는 모달 안에 머문다, Esc 로 닫힌다
    for (let i = 0; i < 8; i++) { await page.keyboard.press('Tab'); ok(await insideModal(page), label + ' A: Tab ' + (i + 1) + ' 번째에 모달 밖으로 나갔다'); }
    await page.keyboard.press('Shift+Tab'); ok(await insideModal(page), label + ' A: Shift+Tab 이 모달 밖으로 나갔다');
    await page.keyboard.press('Escape');
    await modal.waitFor({ state: 'hidden', timeout: 3000 });
    ok(await page.evaluate(() => document.activeElement && document.activeElement.hasAttribute('data-occ-close-all')), label + ' A: Esc 뒤 포커스가 버튼으로 돌아오지 않았다');
    await page.locator('[data-occ-close-all]').click();
    await page.locator('[data-occ-close-form]').waitFor({ state: 'visible', timeout: 5000 });
    await page.locator('[data-close-reason]').fill('고객 요청으로 조기 종료');
    ok(await submit.isEnabled(), label + ' A: 사유를 넣어도 버튼이 꺼져 있다');
    await page.screenshot({ path: path.join(shotDir, `95-200-modal-detail-${label}.png`) });
    await Promise.all([page.waitForLoadState('load'), submit.click()]);
    await page.waitForURL(new RegExp(`/workboard/tasks/${parent}\\?`), { timeout: 8000 });
    const body = await page.locator('body').innerText();
    ok(body.includes('14건 제외, 상위 업무를 완료했습니다'), label + ' A: 결과 플래시', body.slice(0, 400));
    ok(await page.locator('[data-occ-close-all]').count() === 0, label + ' A: 종료 뒤에도 버튼이 남아 있다');
    ok(await noOverflow(page), label + ' A: 결과 가로 넘침');
    await page.screenshot({ path: path.join(shotDir, `95-200-result-${label}.png`) });
  }

  // ── B. 상위 이력 표에서 종료 (require_result, 「함께 제외」 해제 → 상위는 진행 중)
  {
    const [parent, occ] = c.b;
    await page.goto(`${base}/workboard/tasks/${parent}`, { waitUntil: 'load' });
    const rowBtn = page.locator(`[data-occ-close-all][data-occ="${occ}"]`);
    ok(await rowBtn.count() === 1, label + ' B: 이력 표 행 버튼');
    ok(await page.locator('[data-occ-close-all]').count() === 15, label + ' B: 열린 회차(15건) 행마다 버튼');
    await rowBtn.scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(shotDir, `95-200-table-button-${label}.png`) });
    await rowBtn.click();
    await page.locator('[data-occ-close-form]').waitFor({ state: 'visible', timeout: 5000 });
    ok(await page.locator('[data-final-box]').isVisible(), label + ' B: require_result 인데 최종 결과 칸이 없다');
    const submit = page.locator('[data-occ-close-submit]');
    await page.locator('[data-close-reason]').fill('범위 축소로 종료');
    ok(await submit.isDisabled(), label + ' B: 최종 결과 없이 버튼이 켜져 있다');
    await page.locator('[data-final-result]').fill('신구대조표 최종본 제출');
    ok(await submit.isEnabled(), label + ' B: 최종 결과를 넣어도 꺼져 있다');
    // 이전 미완료를 남기면 상위를 완료하지 않으므로 최종 결과 칸이 사라진다
    await page.locator('[data-include-earlier]').uncheck();
    ok(await page.locator('[data-earlier-note]').isVisible(), label + ' B: 이전 미완료 유지 안내가 없다');
    ok(await page.locator('[data-final-box]').isHidden(), label + ' B: 상위를 완료하지 않는데 최종 결과 칸이 보인다');
    await page.locator('[data-include-earlier]').check();
    ok(await page.locator('[data-final-box]').isVisible(), label + ' B: 다시 함께 제외하면 최종 결과 칸이 돌아와야 한다');
    await page.locator('[data-include-earlier]').uncheck();
    await page.screenshot({ path: path.join(shotDir, `95-200-modal-table-${label}.png`) });
    await Promise.all([page.waitForLoadState('load'), submit.click()]);
    await page.waitForURL(new RegExp(`/workboard/tasks/${parent}\\?`), { timeout: 8000 });
    const body = await page.locator('body').innerText();
    ok(body.includes('이전 미완료 2건이 남아 있어 상위 업무는 진행 중으로 둡니다'), label + ' B: 부분 종료 플래시', body.slice(0, 400));
    ok(await noOverflow(page), label + ' B: 결과 가로 넘침');
  }
  await context.close();
}
await browser.close();
console.log('OK');
