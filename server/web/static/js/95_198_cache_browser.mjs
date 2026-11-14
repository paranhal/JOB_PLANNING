// 95-198: 배포 뒤 옛 weekly-link.js 캐시 시나리오. 같은 브라우저(HTTP 캐시 유지)에서
// ① 옛 빌드 주소(?v=OLD)로 열어 옛 스크립트를 캐시 → 활동을 골라도 [연결]이 꺼져 있다(증상 재현)
// ② 서버 빌드를 바꾸고(배포) 다시 열면 ?v=NEW 주소로 새 스크립트를 받아 [연결]이 켜진다.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const shotDir = process.env.SCREENSHOT_DIR || '.';
const W = base + '/stats/company-weekly?week=2026-10-19&org=O03&edit=0';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
function ok(c, m, x) { if (!c) fail(m, x); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const u = new URL(base);
await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
const page = await context.newPage();
await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
page.on('pageerror', (e) => console.error('pageerror:', e.message));
const hits = async () => (await (await context.request.get(base + '/__hits')).json());
const linkSrc = () => page.locator('script[src*="weekly-link.js"]').evaluateAll((els) => els.map((e) => e.getAttribute('src')));

async function openAndPickActivity() {
  await page.goto(W, { waitUntil: 'load' });
  await page.locator('[data-weekly-linked] summary').click();
  await page.locator('[data-linked-task="T197-T"] [data-weekly-link]').click();
  await page.locator('#weekly-link-modal').waitFor({ state: 'visible', timeout: 3000 });
  await page.locator('input[name="activity_kind"][value="growth"]').check();
  return page.locator('[data-weekly-link-submit]');
}

// ① 옛 빌드
const old = await (await context.request.get(base + '/__build')).json();
let src = await linkSrc();
ok(src.length === 0, '예상: 첫 로드 전에는 페이지가 없다');
let submit = await openAndPickActivity();
src = await linkSrc();
ok(src.length === 1 && src[0].includes('?v=' + old.q), '① 옛 빌드 주소에 ?v= 가 없다', src);
ok(await submit.isDisabled(), '① 옛 스크립트라면 활동을 골라도 [연결]이 꺼져 있어야 한다(증상 재현)');
let h = await hits();
ok(h[old.q] === 1, '① 옛 스크립트를 한 번 받았다', h);
await page.screenshot({ path: path.join(shotDir, '95-198-cache-old-1440.png') });

// 같은 빌드로 다시 열면 캐시를 쓴다(서버를 다시 치지 않는다)
await openAndPickActivity();
h = await hits();
ok(h[old.q] === 1, '같은 빌드 재방문은 캐시를 써야 한다(서버 적중 불변)', h);

// ② 배포: 서버 빌드가 바뀐다
const nw = await (await context.request.get(base + '/__setbuild')).json();
ok(nw.q !== old.q, '빌드 값이 바뀌지 않았다', nw);
submit = await openAndPickActivity();
src = await linkSrc();
ok(src.length === 1 && src[0].includes('?v=' + nw.q) && !src[0].includes(old.q), '② 새 빌드 주소로 바뀌지 않았다', src);
h = await hits();
ok(h[nw.q] === 1, '② 새 주소로 새 스크립트를 받아야 한다', h);
ok(await submit.isEnabled(), '② 새 스크립트에서는 활동을 고르면 [연결]이 켜져야 한다');
await page.screenshot({ path: path.join(shotDir, '95-198-cache-new-1440.png') });
await browser.close();
console.log('OK');
