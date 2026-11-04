import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

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

const base = process.env.BASE_URL;
const token = process.env.AUTH_COOKIE;
if (!base || !token) fail('BASE_URL/AUTH_COOKIE 필요');

const browser = await launch();
const page = await browser.newPage();
const u = new URL(base);
await page.context().addCookies([{ name: 'token', value: token, url: base.endsWith('/') ? base : base + '/' }]);
await page.setExtraHTTPHeaders({ Cookie: 'token=' + token });
page.on('response', async (r) => {
  if (r.url().includes('new-work') || r.status() >= 400) {
    console.error('RESP', r.status(), r.url());
  }
});
await page.goto(base + '/workboard/board');
await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, token);
const btn = page.locator('[data-new-work]').first();
if (!(await btn.count())) fail('등록 버튼 없음', await page.title());
await btn.click();
await page.click('text=행정/지원 업무');
await page.click('text=다음 단계');
await page.waitForTimeout(2500);
const slotHTML = await page.locator('#new-work-form-slot').innerHTML().catch(() => '');
const errVis = await page.locator('text=폼을 불러오지 못했습니다').count();
if (!(await page.locator('[data-project-picker]').count())) {
  fail('피커 없음 err=' + errVis + ' slot=' + slotHTML.slice(0, 800));
}
const search = page.locator('[data-project-picker] input[type="search"]').first();
await search.click({ force: true });
const picked = await page.evaluate(() => {
  const root = document.querySelector('[data-project-picker]');
  const btns = [...root.querySelectorAll('button')];
  const b = btns.find((el) => {
    const t = (el.textContent || '').replace(/\s+/g, ' ').trim();
    return t === '도서관사업' || t.startsWith('도서관사업 ');
  });
  if (!b) return { err: 'missing', n: btns.length };
  b.click();
  const data = window.Alpine && Alpine.$data ? Alpine.$data(root) : null;
  if (data && data.pick) data.pick('WP-O01', '도서관사업', '');
  return { err: '', id: data ? data.selectedId : '', name: data ? data.selectedName : '', q: data ? data.q : '' };
});
if (picked.err) fail('목록에 도서관사업 버튼 없음', JSON.stringify(picked));
if (picked.id !== 'WP-O01') fail('선택 id', JSON.stringify(picked));
const selected = await page.locator('[data-project-picker]').first().innerText();
if (!selected.includes('도서관사업')) fail('선택 표시', selected);
const q = await search.inputValue();
if (q !== '') fail('선택명이 검색어가 되면 안 됨', q);
const hid = await page.locator('[data-project-picker] input[name="project_id"]').first().inputValue();
if (hid !== 'WP-O01') fail('hidden project_id', hid);
await search.evaluate((el) => el.focus());
await search.fill('없는사업xyz');
const empty = page.locator('[data-project-picker]', { hasText: '등록된 사업이 없습니다' });
if (!(await empty.count())) fail('0건 안내 없음');
await browser.close();
console.log('95_113_picker_browser ok');
