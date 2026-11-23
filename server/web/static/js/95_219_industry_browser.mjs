// 95-219 고객 등록·수정 업종 다중 선택: 패널 열기·체크·요약·닫기(Esc·바깥 클릭·Tab)·키보드·모바일 겹침·저장 후 복원. 1440/820/390. TEST 데이터만.
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
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const vps = [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]];
let n = 0;
for (const [label, vp] of vps) {
  n++;
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  const page = await context.newPage();
  page.on('pageerror', (e) => { if (!/Unexpected token 'var'/.test(e.message)) console.error('pageerror:', e.message); });
  await page.goto(base + '/customers/new', { waitUntil: 'load' });
  await page.addStyleTag({ content: '[x-cloak]{display:none !important}' });
  await page.waitForTimeout(700);
  const root = page.locator('[data-industry-multi]');
  const btn = root.locator('button[aria-haspopup="listbox"]');
  const panel = root.locator('[role="listbox"]');
  const summary = () => root.locator('[data-industry-summary]').innerText();
  if ((await root.count()) !== 1) fail(`${label}: 업종 컴포넌트가 없다`);
  if ((await summary()).trim() !== '선택하세요') fail(`${label}: 처음 요약`, await summary());
  // 열기
  await btn.click();
  await page.waitForTimeout(200);
  if ((await btn.getAttribute('aria-expanded')) !== 'true') fail(`${label}: aria-expanded`);
  if (!(await panel.isVisible())) fail(`${label}: 패널이 안 열린다`);
  if ((await panel.locator('input[type=checkbox]').count()) !== 4) fail(`${label}: 선택지 4개`);
  // 패널이 화면 밖으로 넘치지 않고(가로), 좁은 화면에서는 하단 탭바(약 64px)에 안 가린다
  const box = await panel.boundingBox();
  if (box.x < -1 || box.x + box.width > vp.width + 1) fail(`${label}: 패널이 가로로 넘친다`, box);
  if (vp.width < 768 && box.y + box.height > vp.height - 64) fail(`${label}: 패널이 하단 탭바에 가린다`, { box, h: vp.height });
  // 항목 줄 높이 44px 이상
  const rowH = (await panel.locator('label').first().boundingBox()).height;
  if (rowH < 43.5) fail(`${label}: 항목 높이 ${rowH}`);
  // 체크 → 요약
  await panel.locator('label', { hasText: '도서관' }).click();
  if ((await summary()).trim() !== '도서관') fail(`${label}: 1개 요약`, await summary());
  await panel.locator('label', { hasText: '학교' }).click();
  if ((await summary()).trim() !== '도서관, 학교') fail(`${label}: 2개 요약`, await summary());
  await panel.locator('label', { hasText: '민간' }).click();
  if ((await summary()).trim() !== '도서관 외 2') fail(`${label}: 3개 요약`, await summary());
  await page.screenshot({ path: path.join(shotDir, `open-${label}.png`) });
  await panel.locator('label', { hasText: '민간' }).click(); // 해제 → 다시 2개
  if ((await summary()).trim() !== '도서관, 학교') fail(`${label}: 해제 요약`, await summary());
  // Esc 로 닫힘 + 버튼으로 포커스 복귀
  await page.keyboard.press('Escape');
  await page.waitForTimeout(200);
  if (await panel.isVisible()) fail(`${label}: Esc 로 안 닫힌다`);
  if (!(await btn.evaluate((b) => document.activeElement === b))) fail(`${label}: Esc 뒤 포커스가 버튼에 없다`);
  // 바깥 클릭으로 닫힘
  await btn.click();
  await page.waitForTimeout(200);
  await page.getByRole('heading', { name: '고객 등록' }).first().click();
  await page.waitForTimeout(200);
  if (await panel.isVisible()) fail(`${label}: 바깥 클릭으로 안 닫힌다`);
  // 키보드: ↓ 로 열기 → 첫 체크박스 포커스(이미 체크한 항목) → ↓ 이동 → Space 토글 → Tab 으로 나가면 닫힘
  await btn.focus();
  await page.keyboard.press('ArrowDown');
  await page.waitForTimeout(250);
  if (!(await panel.isVisible())) fail(`${label}: ↓ 로 안 열린다`);
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  const s2 = (await summary()).trim();
  if (s2 === '도서관, 학교') fail(`${label}: Space 로 토글이 안 된다`, s2);
  await page.keyboard.press('Space'); // 되돌림
  if ((await summary()).trim() !== '도서관, 학교') fail(`${label}: Space 되돌림`, await summary());
  for (let i = 0; i < 6; i++) { await page.keyboard.press('Tab'); }
  await page.waitForTimeout(200);
  if (await panel.isVisible()) fail(`${label}: Tab 으로 나가도 안 닫힌다`);
  // 제출될 값: 체크한 업종이 name="industry" 로 여러 개 들어간다(저장 규칙은 HTTP 시험이 확인한다)
  const submitted = await page.evaluate(() => new FormData(document.querySelector('form[action="/customers"]')).getAll('industry'));
  if (JSON.stringify(submitted) !== JSON.stringify(['도서관', '학교'])) fail(`${label}: 제출 값`, submitted);
  // 저장된 다중 업종 고객: 수정 화면에서 체크가 복원되고, 목록은 두 업종 필터 모두에서 찾으며 업종마다 링크가 있다
  const seed = process.env.SEED_ID;
  await page.goto(base + `/customers/${seed}/edit`, { waitUntil: 'load' });
  await page.addStyleTag({ content: '[x-cloak]{display:none !important}' });
  await page.waitForTimeout(500);
  if ((await page.locator('[data-industry-summary]').innerText()).trim() !== '도서관, 학교') fail(`${label}: 수정 화면 요약`, await page.locator('[data-industry-summary]').innerText());
  await page.locator('[data-industry-multi] button[aria-haspopup="listbox"]').click();
  await page.waitForTimeout(250);
  const checked = await page.locator('[data-industry-option]:checked').evaluateAll((a) => a.map((x) => x.value));
  if (JSON.stringify(checked) !== JSON.stringify(['도서관', '학교'])) fail(`${label}: 체크 복원`, checked);
  await page.screenshot({ path: path.join(shotDir, `edit-${label}.png`) });
  for (const ind of ['도서관', '학교']) {
    await page.goto(base + '/customers?industry=' + encodeURIComponent(ind), { waitUntil: 'load' });
    if (!(await page.locator('body').innerText()).includes('시험다중고객')) fail(`${label}: 업종 ${ind} 필터에 다중 업종 고객이 없다`);
  }
  const link = page.locator('tr:has-text("시험다중고객") a[data-industry-link]');
  if ((await link.count()) !== 2) fail(`${label}: 목록 업종 링크 2개`, await link.count());
  await page.screenshot({ path: path.join(shotDir, `list-${label}.png`) });
  await context.close();
}
await browser.close();
console.log('OK');
