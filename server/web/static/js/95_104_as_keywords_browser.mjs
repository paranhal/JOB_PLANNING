import { pathToFileURL } from 'node:url';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const here = dirname(fileURLToPath(import.meta.url));
const html = join(here, '95_104_as_keywords_fixture.html');

async function launch() {
  const { chromium } = require('playwright');
  try {
    return await chromium.launch({ headless: true, channel: 'chrome' });
  } catch (e) {
    try {
      return await chromium.launch({ headless: true, channel: 'msedge' });
    } catch (e2) {
      return await chromium.launch({ headless: true });
    }
  }
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

const browser = await launch();
const page = await browser.newPage({ viewport: { width: 390, height: 800 } });
await page.goto(pathToFileURL(html).href);
await page.waitForFunction(() => window.ASKeywords && window.ASKeywords._ui && window.ASKeywords._ui.renderCount() >= 1);

const checkedAfterLoad = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value));
if (!checkedAfterLoad.includes('K1')) fail('T6/T11 로드 직후 서버 checked 가 풀림', checkedAfterLoad);

await page.check('#as-kw-list input[value="K2"]');
const nFetch0 = await page.evaluate(() => window.__fetchCalls.length);
await page.evaluate(() => window.ASKeywords._ui.render(window.__suggest));
let checked = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value).sort());
if (checked.join(',') !== 'K1,K2') fail('T1 같은 응답 재 render 후 체크 유실', checked);

await page.evaluate(() => {
  window.__suggest = { items: [{ keyword_id: 'K9', keyword: 'Z', kw_group: '원인' }], new: [] };
  window.ASKeywords._ui.render(window.__suggest);
});
checked = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value).sort());
if (!checked.includes('K1') || !checked.includes('K2')) fail('T2 다른 후보여도 체크 유지', checked);
const labels = await page.$$eval('#as-kw-list label', (els) => els.map((e) => e.getAttribute('data-label')));
if (!labels.includes('A') || !labels.includes('B')) fail('T2 체크 항목이 목록에서 사라짐', labels);

await page.evaluate(() => {
  window.__suggest = {
    items: [
      { keyword_id: 'K1', keyword: 'A', kw_group: '대상' },
      { keyword_id: 'K2', keyword: 'B', kw_group: '증상' },
      { keyword_id: 'K3', keyword: 'C', kw_group: '조치' }
    ],
    new: [{ keyword: '재기동', kw_group: '조치' }]
  };
  window.ASKeywords._ui.render(window.__suggest, true);
});
await page.check('#as-kw-new input[value="new:재기동|조치"]');
await page.evaluate(() => window.ASKeywords._ui.render(window.__suggest));
const newOn = await page.$eval('#as-kw-new input[value="new:재기동|조치"]', (el) => el.checked);
if (!newOn) fail('T3 새 키워드 후보 체크 유실');

const nBeforeManual = await page.evaluate(() => window.__fetchCalls.length);
await page.fill('#as-kw-manual', 'DB(대상)');
await page.waitForTimeout(500);
const nAfterManual = await page.evaluate(() => window.__fetchCalls.length);
if (nAfterManual !== nBeforeManual) fail('T4 직접 입력 시 서버 재조회', { nBeforeManual, nAfterManual, nFetch0 });
const chosen = await page.locator('#as-kw-chosen').textContent();
if (!chosen.includes('A(') || !chosen.includes('DB(대상)')) fail('T4 선택된 키워드 요약', chosen);
checked = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value));
if (!checked.includes('K1') || !checked.includes('K2')) fail('T4 직접 입력 후 체크 유실', checked);

const renders = await page.evaluate(() => window.ASKeywords._ui.renderCount());
const nBeforeText = await page.evaluate(() => window.__fetchCalls.length);
await page.fill('.as-set-text', '게이트를 교체 했음');
await page.waitForTimeout(150);
const mid = await page.evaluate(() => window.__fetchCalls.length);
if (mid !== nBeforeText) fail('T5 디바운스 전에 fetch', { mid, nBeforeText });
await page.waitForTimeout(450);
const afterText = await page.evaluate(() => window.__fetchCalls.length);
if (afterText !== nBeforeText + 1) fail('T5 조치내용 입력 재조회 1회가 아님', { afterText, nBeforeText });
checked = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value));
if (!checked.includes('K1') || !checked.includes('K2')) fail('T5 체크 유실', checked);

const r2 = await page.evaluate(() => {
  const n = window.ASKeywords._ui.renderCount();
  window.ASKeywords._ui.render(window.__suggest);
  return { before: n, after: window.ASKeywords._ui.renderCount() };
});
if (r2.after !== r2.before) fail('T7 같은 후보면 DOM 재작성', r2);
if (renders < 1) fail('render 한 번도 없음');

const form = await page.evaluate(() => {
  const fd = new FormData(document.getElementById('as-action-form'));
  return Array.from(fd.entries());
});
if (!form.some((x) => x[0] === 'kw_action' && x[1] === 'K1') || !form.some((x) => x[0] === 'kw_action' && x[1] === 'K2')) {
  fail('T10 저장 kw_action', form);
}
if (!form.some((x) => x[0] === 'kw_manual' && String(x[1]).includes('DB'))) {
  fail('T10 저장 kw_manual', form);
}
if (!form.some((x) => x[0] === 'kw_suggested_action')) {
  fail('T10 kw_suggested_action 없음', form);
}

await page.reload();
await page.waitForFunction(() => window.ASKeywords && window.ASKeywords._ui);
await page.waitForFunction(() => (window.__fetchCalls || []).length >= 1);
const afterReload = await page.$$eval('#as-kw-list input:checked', (els) => els.map((e) => e.value));
if (!afterReload.includes('K1')) fail('T10/T11 재진입 후 체크 유실', afterReload);

const box = await page.locator('#as-kw-box').boundingBox();
if (!box || box.x < -1 || box.x + box.width > 392) fail('T12 390px 키워드 영역 넘침', box);

await browser.close();
console.log('95_104_as_keywords_browser.mjs ok');
