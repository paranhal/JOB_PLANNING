// 95-210 링크 → 버튼 통일: 대표 화면을 실제 서버로 열어 변환된 버튼의 높이·줄바꿈·겹침·가림·클릭 이동을 확인한다. 1440/820/390. TEST 데이터만.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE, shotDir = process.env.SCREENSHOT_DIR || '.', tag = process.env.SHOT_TAG || 'after';
const pages = JSON.parse(process.env.PAGES);
const strict = tag === 'after';
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
function fail(m, x) { console.error('FAIL:', m, x === undefined ? '' : JSON.stringify(x)); process.exit(1); }
fs.mkdirSync(shotDir, { recursive: true });
const browser = await launch();
const summary = [];
for (const [label, vp] of [['1440', { width: 1440, height: 900 }], ['820', { width: 820, height: 1000 }], ['390', { width: 390, height: 844 }]]) {
  const context = await browser.newContext({ viewport: vp });
  const u = new URL(base);
  await context.addCookies([{ name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
  for (const pg of pages) {
    const page = await context.newPage();
    const errs = [];
    page.on('pageerror', (e) => errs.push(e.message));
    const resp = await page.goto(base + pg.path, { waitUntil: 'load' });
    if (resp.status() !== 200) fail(label + ' ' + pg.path + ' 응답 ' + resp.status());
    if (errs.length) fail(label + ' ' + pg.path + ' 스크립트 오류', errs);
    await page.waitForTimeout(700); // 좁은 화면에서 사이드바가 닫히는 전환(0.3s)이 끝난 뒤 측정
    if (pg.pre) { await page.getByRole('button', { name: pg.pre }).click(); await page.waitForTimeout(800); }
    // 로드 때 자동으로 뜨는 안내 모달(assign-notice-modal)은 사용자가 닫는 알림이라 측정 전에 닫는다.
    await page.evaluate(() => document.querySelectorAll('#assign-notice-modal').forEach((e) => { e.style.display = 'none'; }));
    // 변환 후보: 본문 안(사이드바·상단 제외)의 a.btn-primary / a.btn-secondary 중 문구가 pg.texts 에 있는 것
    const res = await page.evaluate((texts) => {
      const vw = window.innerWidth;
      const out = [];
      const docTop = (el) => { let y = 0, x = el; while (x) { y += x.offsetTop || 0; x = x.offsetParent; } return y; };
      for (const a of document.querySelectorAll('main a, #content a, body a')) {
        if (!/\bbtn-(primary|secondary)\b/.test(a.className) || a.offsetParent === null) continue;
        const t = (a.textContent || '').replace(/\s+/g, ' ').trim();
        if (!texts.some((x) => t.includes(x))) continue;
        a.scrollIntoView({ block: 'center' });
        const r = a.getBoundingClientRect();
        const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
        out.push({ t: t.slice(0, 20), h: Math.round(r.height), w: Math.round(r.width), l: Math.round(r.left), r: Math.round(r.right), top: Math.round(docTop(a)), bottom: Math.round(docTop(a) + r.height), covered: !(top === a || a.contains(top)), by: top ? (top.tagName + '.' + String(top.className).slice(0, 80) + '#' + top.id) : null, cls: a.className.includes('btn-sm') ? 'sm' : 'std', card: (a.closest('div.rounded-xl, section') || {}).innerText ? (a.closest('div.rounded-xl, section').innerText || '').slice(0, 30).replace(/\s+/g, ' ') : '', href: a.getAttribute('href') });
      }
      return { vw, out, hs: document.documentElement.scrollWidth > vw + 2 };
    }, pg.texts);
    for (const b of res.out) {
      const want = b.cls === 'sm' ? 30 : 44;
      if (strict && Math.abs(b.h - want) > 6) fail(`${label} ${pg.path} 「${b.t}」 높이 ${b.h} (기대 ≈${want}, 줄바꿈?)`, b);
      if (strict && b.covered) fail(`${label} ${pg.path} 「${b.t}」 가려졌다`, b);
      if (strict && (b.l < -1 || b.r > res.vw + 1)) fail(`${label} ${pg.path} 「${b.t}」 화면 밖`, b);
    }
    // 겹침(변환 버튼끼리)
    for (let i = 0; i < res.out.length; i++) for (let j = i + 1; j < res.out.length; j++) {
      const A = res.out[i], B = res.out[j];
      const ox = Math.min(A.r, B.r) - Math.max(A.l, B.l), oy = Math.min(A.bottom, B.bottom) - Math.max(A.top, B.top);
      if (strict && ox > 2 && oy > 2) fail(`${label} ${pg.path} 버튼 겹침`, [A, B]);
    }
    if (strict && res.hs) fail(`${label} ${pg.path} 페이지 가로 넘침`);
    summary.push({ label, path: pg.path, n: res.out.length, hs: res.out.map((b) => b.h) });
    const file = (pg.name || pg.path.replace(/[^a-z0-9]+/gi, '_')).replace(/^_+|_+$/g, '') || 'home';
    if (label !== '820' || pg.shot820) await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({ path: path.join(shotDir, `${file}-${label}-${tag}.png`), fullPage: label !== '820' });
    // 클릭 이동(1440 에서 첫 변환 버튼): 이동한 곳이 200
    if (label === '1440' && strict && pg.click && res.out.length) {
      const href = await page.evaluate((texts) => { for (const a of document.querySelectorAll('a')) { const t = (a.textContent || '').replace(/\s+/g, ' ').trim(); if (/\bbtn-(primary|secondary)\b/.test(a.className) && texts.some((x) => t.includes(x)) && a.offsetParent !== null) return a.getAttribute('href'); } return null; }, pg.texts);
      if (href && !href.startsWith('#') && !href.startsWith('javascript')) {
        const r2 = await context.request.get(new URL(href, base).toString());
        if (r2.status() >= 400 && r2.status() !== 403) fail(`${pg.path} 버튼 이동 ${href} → ${r2.status()}`);
      }
    }
    await page.close();
  }
  await context.close();
}
await browser.close();
console.log(JSON.stringify(summary.filter((s) => s.n > 0)));
console.log('OK');
