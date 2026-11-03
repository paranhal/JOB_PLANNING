import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
if (!base || !cookie) {
  console.error('BASE_URL/AUTH_COOKIE 없음');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  const tries = [{ channel: 'chrome' }, {}, { channel: 'msedge' }];
  let last = null;
  for (const extra of tries) {
    try {
      return await chromium.launch(Object.assign({ headless: true }, extra));
    } catch (e) {
      last = e;
    }
  }
  throw last;
}

function overlapVisible(page) {
  return page.evaluate(() => {
    const m = document.getElementById('overlap-resolve');
    return !!(m && !m.hidden && !m.classList.contains('hidden'));
  });
}

async function openOverlap(page) {
  await page.click('#nw-save');
  await page.waitForFunction(() => {
    const m = document.getElementById('overlap-resolve');
    return m && !m.hidden && !m.classList.contains('hidden');
  }, null, { timeout: 8000 });
}

async function elementFromPointActions(page) {
  return page.evaluate(() => {
    const btns = [...document.querySelectorAll('[data-overlap-action]')];
    return btns.map((b) => {
      const r = b.getBoundingClientRect();
      const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return { action: b.getAttribute('data-overlap-action'), hit: !!(top && (top === b || b.contains(top))) };
    });
  });
}

const browser = await launch();
const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
await context.addCookies([{ name: 'token', value: cookie, url: base }]);
const page = await context.newPage();
page.on('console', (m) => console.error('console', m.type(), m.text()));
page.on('pageerror', (e) => console.error('pageerror', e.message));
page.on('response', (r) => {
  if (!r.ok()) console.error('http', r.status(), r.url());
});
const u = new URL(base);
await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
await page.goto(base + '/__t74/form', { waitUntil: 'domcontentloaded' });
const hasSave = await page.$('#nw-save');
const hasFn = await page.evaluate(() => typeof window.showOverlapResolve);
if (!hasSave || hasFn !== 'function') {
  console.error('fixture', { hasSave: !!hasSave, hasFn, html: (await page.content()).slice(0, 500) });
  process.exit(1);
}

await page.fill('#title', '브새-취소');
await page.focus('#nw-save');
await openOverlap(page);

const focused = await page.evaluate(() => document.activeElement && document.activeElement.getAttribute('data-overlap-action'));
if (focused !== 'edit') {
  console.error('포커스가 선택 화면이 아님', focused);
  process.exit(1);
}
const hits = await elementFromPointActions(page);
for (const h of hits) {
  if (!h.hit) {
    console.error('elementFromPoint 실패', h.action);
    process.exit(1);
  }
}

await page.keyboard.press('Escape');
await page.waitForTimeout(100);
if (await overlapVisible(page)) {
  console.error('Esc 후 선택 화면이 남음');
  process.exit(1);
}
const back = await page.evaluate(() => document.activeElement && document.activeElement.id);
if (back !== 'nw-save') {
  console.error('Esc 후 포커스 복귀 실패', back);
  process.exit(1);
}
const kept = await page.inputValue('#title');
if (kept !== '브새-취소') {
  console.error('Esc 후 입력값 유실', kept);
  process.exit(1);
}

await openOverlap(page);
await page.click('[data-overlap-action="cancel"]');
await page.waitForTimeout(150);
if (await overlapVisible(page)) {
  console.error('취소 후 선택 화면이 남음');
  process.exit(1);
}
if (await page.inputValue('#title') !== '브새-취소') {
  console.error('취소 후 입력값 유실');
  process.exit(1);
}

await page.fill('#title', '브새-수정');
await page.evaluate(() => {
  document.getElementById('start').value = '11:00';
  document.getElementById('end').value = '11:30';
});
await openOverlap(page);
await page.click('[data-overlap-action="edit"]');
await page.waitForURL(/\/workboard\/tasks\/.+\/edit/, { timeout: 8000 });
const editBody = await page.content();
if (!editBody.includes('새 업무는 저장하지 않았습니다')) {
  console.error('수정 안내 없음');
  process.exit(1);
}

await page.goto(base + '/__t74/form', { waitUntil: 'networkidle' });
await page.fill('#title', '브새-삭제');
await page.evaluate(() => {
  document.getElementById('start').value = '13:00';
  document.getElementById('end').value = '13:30';
});
await openOverlap(page);
await page.click('[data-overlap-action="delete"]');
await page.waitForURL(/\/workboard\/board/, { timeout: 8000 });

await page.goto(base + '/__t74/form', { waitUntil: 'networkidle' });
await page.fill('#title', '브새-제외');
await page.evaluate(() => {
  document.getElementById('start').value = '15:00';
  document.getElementById('end').value = '15:30';
});
await openOverlap(page);
await page.click('[data-overlap-action="leave"]');
await page.waitForURL(/\/workboard\/board/, { timeout: 8000 });

await browser.close();
console.log('31a browser ok');
