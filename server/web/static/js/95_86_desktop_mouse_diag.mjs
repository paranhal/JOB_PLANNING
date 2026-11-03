import { createRequire } from 'node:module';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day = process.env.WORK_DATE;
const shotDir = process.env.SHOT_DIR || '/tmp/95-86-desktop';
if (!base || !cookie || !day) {
  console.error('BASE_URL/AUTH_COOKIE/WORK_DATE 없음');
  process.exit(1);
}
mkdirSync(shotDir, { recursive: true });

async function launch() {
  const { chromium } = require('playwright');
  const tries = [{ channel: 'chrome' }, {}];
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

function describeEl(el) {
  if (!el) return null;
  return {
    tag: el.tag,
    id: el.id,
    cls: el.cls,
    text: (el.text || '').replace(/\s+/g, ' ').trim().slice(0, 80),
  };
}

const browser = await launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, hasTouch: false });
const u = new URL(base);
await context.addCookies([
  { name: 'token', value: cookie, url: base },
  { name: 'token', value: cookie, domain: u.hostname, path: '/' },
]);
const page = await context.newPage();
const url = base + '/workboard/board?view=day&date=' + day + '&auto=0&mine=0';
await page.goto(url, { waitUntil: 'domcontentloaded' });
await page.waitForSelector('[data-board-bar]', { state: 'attached', timeout: 15000 });

const bars = await page.evaluate(() => {
  return Array.from(document.querySelectorAll('[data-board-bar]')).map((el) => {
    const r = el.getBoundingClientRect();
    const acts = el.querySelector('.board-bar-acts');
    const ar = acts ? acts.getBoundingClientRect() : null;
    const handle = el.querySelector('[data-resize]');
    const hr = handle ? handle.getBoundingClientRect() : null;
    const btn = el.querySelector('[data-bar-act]');
    const br = btn ? btn.getBoundingClientRect() : null;
    const cs = getComputedStyle(el);
    return {
      title: el.getAttribute('data-title'),
      dur: el.getAttribute('data-duration'),
      short: el.getAttribute('data-short'),
      start: el.getAttribute('data-start'),
      end: el.getAttribute('data-end'),
      h: el.getAttribute('data-h'),
      top: el.getAttribute('data-top'),
      box: { x: r.x, y: r.y, w: r.width, h: r.height },
      overflow: cs.overflow,
      z: cs.zIndex,
      padB: cs.paddingBottom,
      actsOutside: ar ? ar.bottom > r.bottom + 1 || ar.top < r.top - 1 : null,
      titleOutside: false,
      handle: handle ? { w: hr.width, h: hr.height, vis: csH(handle) } : null,
      btn: btn ? { x: br.x + br.width / 2, y: br.y + br.height / 2, label: (btn.textContent || '').trim().slice(0, 20) } : null,
    };
    function csH(h) {
      const s = getComputedStyle(h);
      return { bg: s.backgroundColor, opacity: s.opacity, h: s.height };
    }
  });
});

const probe = [];
for (const bar of bars) {
  if (!bar.btn) continue;
  const hit = await page.evaluate(({ x, y }) => {
    const el = document.elementFromPoint(x, y);
    function chain(n, out) {
      while (n && out.length < 8) {
        out.push({
          tag: n.tagName,
          id: n.id || '',
          cls: (n.className && n.className.toString && n.className.toString()) || '',
          text: (n.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 40),
        });
        n = n.parentElement;
      }
      return out;
    }
    return { x, y, chain: chain(el, []) };
  }, { x: bar.btn.x, y: bar.btn.y });
  probe.push({ title: bar.title, btn: bar.btn.label, hit });
}

const shortBar = bars.find((b) => b.title && b.title.indexOf('짧은') >= 0) || bars[0];
if (shortBar) {
  const cx = shortBar.box.x + shortBar.box.w / 2;
  const cy = shortBar.box.y + Math.min(12, shortBar.box.h / 2);
  await page.mouse.move(cx, cy);
  await page.screenshot({ path: join(shotDir, 'a-hover-short.png'), fullPage: false });
  await page.mouse.down();
  await page.mouse.move(cx, cy + 80, { steps: 12 });
  await page.screenshot({ path: join(shotDir, 'b-mouse-drag.png'), fullPage: false });
  await page.mouse.up();
}

if (shortBar && shortBar.handle) {
  const hx = shortBar.box.x + shortBar.box.w / 2;
  const hy = shortBar.box.y + shortBar.box.h - 2;
  const hitH = await page.evaluate(({ x, y }) => {
    const el = document.elementFromPoint(x, y);
    return el ? { tag: el.tagName, cls: (el.className && el.className.toString()) || '', text: (el.innerText || '').slice(0, 40) } : null;
  }, { x: hx, y: hy });
  probe.push({ handlePoint: { x: hx, y: hy }, hitH });
  await page.mouse.move(hx, hy);
  await page.screenshot({ path: join(shotDir, 'c-handle.png'), fullPage: false });
}

await page.screenshot({ path: join(shotDir, 'd-overview.png'), fullPage: true });
const report = { url, bars, probe };
writeFileSync(join(shotDir, 'report.json'), JSON.stringify(report, null, 2));
console.log(JSON.stringify(report, null, 2));
await browser.close();
