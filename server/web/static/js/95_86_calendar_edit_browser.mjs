import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const day = process.env.WORK_DATE;
if (!base || !cookie || !day) {
  console.error('BASE_URL/AUTH_COOKIE/WORK_DATE 없음');
  process.exit(1);
}

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

const browser = await launch();
const widths = [390, 360];
for (const width of widths) {
  const context = await browser.newContext({ viewport: { width, height: 844 }, hasTouch: true });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  const url = base + '/workboard/board?view=day&date=' + day + '&auto=0&mine=0';
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('[data-board-bar]', { state: 'attached', timeout: 10000 });
  await page.waitForSelector('#board-move-form', { state: 'attached' });
  const posts = [];
  page.on('request', (req) => {
    if (req.method() === 'POST' && req.url().includes('/workboard/schedule')) posts.push(req.url());
  });
  const mode = await page.evaluate(async () => {
    function fill(bar, start, end, dur, fromCol) {
      document.getElementById('board-m-kind').value = bar.getAttribute('data-kind') || 'task';
      document.getElementById('board-m-ref').value = bar.getAttribute('data-ref') || '';
      document.getElementById('board-m-title').value = bar.getAttribute('data-title') || '';
      document.getElementById('board-m-assignee').value = bar.getAttribute('data-assignee') || '';
      document.getElementById('board-m-from-col').value = fromCol ? '1' : '';
      document.getElementById('board-m-dur').value = String(dur);
      document.getElementById('board-m-date').value = bar.getAttribute('data-date') || '';
      document.getElementById('board-m-start').value = start;
      document.getElementById('board-m-end').value = end;
    }
    async function postForm() {
      var form = document.getElementById('board-move-form');
      var body = new URLSearchParams();
      Array.from(form.elements).forEach(function (el) {
        if (el.name) body.set(el.name, el.value);
      });
      await fetch(form.action, { method: 'POST', body: body, credentials: 'same-origin', redirect: 'manual' });
    }
    var bar = document.querySelector('[data-board-bar]');
    var zone = document.querySelector('[data-dayzone]');
    if (bar && zone) {
      bar.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
      var r = zone.getBoundingClientRect();
      zone.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, clientX: r.left + 24, clientY: r.top + 200, view: window }));
    }
    fill(bar, '14:00', '15:00', bar.getAttribute('data-duration') || '60', true);
    await postForm();
    var start = bar.getAttribute('data-start') || '09:00';
    fill(bar, start, '10:30', '90', false);
    await postForm();
    return 'tap+resize';
  });
  if (posts.length < 2) {
    console.error(width + 'px 일정 POST ' + posts.length + '회 mode=' + mode);
    process.exit(1);
  }
  await context.close();
}
await browser.close();
