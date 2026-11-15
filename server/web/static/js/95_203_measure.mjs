// 95-203 0단계: 하단 고정 액션 영역의 화면 좌표를 폭별로 측정하고 elementFromPoint 로 누가 버튼 중앙을 잡는지 본다. 읽기 전용.
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const base = process.env.BASE_URL, cookie = process.env.AUTH_COOKIE;
const SCREENS = JSON.parse(process.env.SCREENS); // [{name,url,sel}]
const WIDTHS = [[390, 844], [820, 1000], [1023, 900], [1024, 900], [1440, 900]];
async function launch() {
  const { chromium } = require('playwright'); let last = null;
  for (const extra of [{ channel: 'chrome' }, {}]) { try { return await chromium.launch(Object.assign({ headless: true }, extra)); } catch (e) { last = e; } }
  throw last;
}
const browser = await launch();
const out = [];
for (const s of SCREENS) {
  for (const [w, h] of WIDTHS) {
    const context = await browser.newContext({ viewport: { width: w, height: h } });
    const u = new URL(base);
    await context.addCookies([{ name: 'token', value: cookie, url: base }, { name: 'token', value: cookie, domain: u.hostname, path: '/' }]);
    const page = await context.newPage();
    await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
    await page.goto(base + s.url, { waitUntil: 'load' });
    await page.waitForTimeout(400);
    for (const scroll of ['top', 'bottom']) {
      const r = await page.evaluate(({ sel, scroll }) => {
        const main = document.getElementById('main-content');
        const targets = [main, document.scrollingElement, ...document.querySelectorAll('.as-action-right,.pw-action-mid,[data-pw-scroll]')];
        targets.forEach((el) => { if (el) el.scrollTop = scroll === 'bottom' ? 1e6 : 0; });
        const btn = [...document.querySelectorAll(sel)].find((b) => b.offsetParent !== null || getComputedStyle(b).position === 'fixed');
        if (!btn) return { found: false };
        const b = btn.getBoundingClientRect();
        const tab = document.getElementById('mobile-tab-bar');
        const tcs = tab ? getComputedStyle(tab) : null;
        const t = tab && tcs.display !== 'none' ? tab.getBoundingClientRect() : null;
        const cx = b.x + b.width / 2, cy = b.y + b.height / 2;
        const inView = cx >= 0 && cy >= 0 && cx <= innerWidth && cy <= innerHeight;
        const hit = inView ? document.elementFromPoint(cx, cy) : null;
        const hitDesc = hit ? (hit.closest('#mobile-tab-bar') ? '#mobile-tab-bar' : hit.id ? '#' + hit.id : hit.tagName.toLowerCase() + (hit.className && typeof hit.className === 'string' ? '.' + hit.className.split(' ')[0] : '')) : '(화면 밖)';
        return {
          found: true, btn: { y: Math.round(b.y), bottom: Math.round(b.bottom), h: Math.round(b.height), x: Math.round(b.x), w: Math.round(b.width) },
          tab: t ? { top: Math.round(t.top), h: Math.round(t.height) } : null,
          vh: innerHeight, inView, covered: inView ? !(btn.contains(hit)) : null, hit: hitDesc,
          bottomGap: t ? Math.round(t.top - b.bottom) : Math.round(innerHeight - b.bottom),
          mainScroll: main ? { sh: main.scrollHeight, ch: main.clientHeight, pb: getComputedStyle(main).paddingBottom } : null,
        };
      }, { sel: s.sel, scroll });
      out.push({ screen: s.name, w, scroll, ...r });
    }
    if (s.dump && w === 390) {
      const chain = await page.evaluate((sel) => {
        let el = [...document.querySelectorAll(sel)][0]; const rows = [];
        while (el && el !== document.documentElement) {
          const cs = getComputedStyle(el), r = el.getBoundingClientRect();
          rows.push({ el: el.tagName.toLowerCase() + (el.id ? '#' + el.id : ''), cls: String(el.className).split(' ').slice(0, 4).join(' '), disp: cs.display, pos: cs.position, ov: cs.overflowY, h: Math.round(r.height), bottom: Math.round(r.bottom), sh: el.scrollHeight, ch: el.clientHeight });
          el = el.parentElement;
        }
        return rows;
      }, s.sel);
      out.push({ screen: s.name, chain });
    }
    await context.close();
  }
}
await browser.close();
console.log('MEASURE_JSON ' + JSON.stringify(out));
