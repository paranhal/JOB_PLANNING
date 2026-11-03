import { pathToFileURL } from 'node:url';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const here = dirname(fileURLToPath(import.meta.url));
const html = process.env.PLAYWRIGHT_HTML || join(here, '../review-95-74/overlap-stack.html');
const fileUrl = pathToFileURL(html).href;

async function withPlaywright() {
  let chromium;
  try {
    ({ chromium } = require('playwright'));
  } catch (e) {
    try {
      ({ chromium } = require('@playwright/test'));
    } catch (e2) {
      return null;
    }
  }
  let browser;
  try {
    browser = await chromium.launch({ headless: true, channel: 'chrome' });
  } catch (e) {
    try {
      browser = await chromium.launch({ headless: true, channel: 'msedge' });
    } catch (e2) {
      console.error('launch', e.message);
      return null;
    }
  }
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  await page.goto(fileUrl);
  const result = await page.evaluate(() => {
    const btns = [...document.querySelectorAll('[data-overlap-action]')];
    const hits = btns.map((b) => {
      const r = b.getBoundingClientRect();
      const x = r.left + r.width / 2;
      const y = r.top + r.height / 2;
      const top = document.elementFromPoint(x, y);
      return {
        action: b.getAttribute('data-overlap-action'),
        hit: !!(top && (top === b || b.contains(top))),
      };
    });
    const summary = document.getElementById('assignee-summary');
    return {
      hits,
      usr: (summary && summary.textContent || '').indexOf('USR-') >= 0,
      z: getComputedStyle(document.getElementById('overlap-resolve')).zIndex,
    };
  });
  await page.setViewportSize({ width: 360, height: 800 });
  const mobile = await page.evaluate(() => {
    const b = document.querySelector('[data-overlap-action="cancel"]');
    const r = b.getBoundingClientRect();
    const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return !!(top && (top === b || b.contains(top))) && r.width < 400;
  });
  await page.screenshot({ path: join(here, '../review-95-74/overlap-over-newwork.png') });
  await browser.close();
  return { result, mobile };
}

const ran = await withPlaywright();
if (!ran) {
  console.error('playwright 없음');
  process.exit(1);
}
if (ran.result.usr) {
  console.error('USR- 표시');
  process.exit(1);
}
if (String(ran.result.z) !== '80') {
  console.error('z-index', ran.result.z);
  process.exit(1);
}
for (const h of ran.result.hits) {
  if (!h.hit) {
    console.error('elementFromPoint 실패', h.action);
    process.exit(1);
  }
}
if (!ran.mobile) {
  console.error('360px 버튼 클릭 불가');
  process.exit(1);
}
console.log('browser ok', ran.result.hits.map((h) => h.action).join(','));
