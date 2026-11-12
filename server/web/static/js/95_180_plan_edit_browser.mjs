import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const projectID = process.env.PROJECT_ID;
const planID = process.env.PLAN_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !projectID || !planID) {
  console.error('env');
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

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

async function open(browser, viewport) {
  const context = await browser.newContext({ viewport });
  const u = new URL(base);
  await context.addCookies([
    { name: 'token', value: cookie, url: base },
    { name: 'token', value: cookie, domain: u.hostname, path: '/' },
  ]);
  const page = await context.newPage();
  await page.addInitScript((t) => { document.cookie = 'token=' + t + '; path=/'; }, cookie);
  return { context, page };
}

async function noOverflow(page) {
  return page.evaluate(() => {
    const w = window.innerWidth;
    const html = document.documentElement.scrollWidth <= w + 8;
    const main = document.querySelector('main');
    const inner = !main || main.scrollWidth <= main.clientWidth + 8;
    return html && inner;
  });
}


async function visit(page, p) {
  const r = await page.goto(base + p, { waitUntil: 'domcontentloaded' });
  if (!r || r.status() !== 200) fail(p + ' status ' + (r && r.status()));
}

async function openEdit(page) {
  await visit(page, '/projects/' + projectID);
  const d = page.locator('[data-plan-edit="' + planID + '"]');
  if (!(await d.count())) fail('계획 수정 접기가 없음');
  await d.locator('summary').click();
  return d;
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;
  let d = await openEdit(page);
  await page.screenshot({ path: path.join(shotDir, 'plan-edit-1440.png'), fullPage: true });

  // 사유 없이 마감 연장 → 저장 거절 문구
  await d.locator('input[name="plan_end"]').fill('2026-12-31');
  await Promise.all([page.waitForLoadState('domcontentloaded'), d.getByRole('button', { name: '저장' }).click()]);
  if (!(await page.locator('body').innerText()).includes('연장 사유')) fail('사유 없는 연장이 거절 안내 없이 넘어감');

  // 사유와 함께 연장 + 이름 변경
  d = await openEdit(page);
  await d.locator('input[name="name"]').fill('홍천 지역 50곳');
  await d.locator('input[name="plan_end"]').fill('2026-12-31');
  await d.locator('[data-extend-reason]').fill('현장 일정 지연');
  await Promise.all([page.waitForLoadState('domcontentloaded'), d.getByRole('button', { name: '저장' }).click()]);
  if (!(await page.locator('body').innerText()).includes('홍천 지역 50곳')) fail('이름 변경이 화면에 없음');

  await visit(page, '/projects/' + projectID + '/changes');
  const txt = await page.locator('body').innerText();
  if (!txt.includes('현장 일정 지연') || !txt.includes('2026-12-31')) fail('변경내역에 연장 이력이 없음');
  await page.screenshot({ path: path.join(shotDir, 'changes-1440.png'), fullPage: true });

  for (const w of [390, 820, 1440]) {
    const v = await open(browser, { width: w, height: 900 });
    await openEdit(v.page);
    if (!(await noOverflow(v.page))) fail(w + 'px 가로 넘침');
    if (w === 390) await v.page.screenshot({ path: path.join(shotDir, 'plan-edit-390.png'), fullPage: true });
    await visit(v.page, '/projects/' + projectID + '/changes');
    if (!(await noOverflow(v.page))) fail(w + 'px 변경내역 가로 넘침');
    await v.context.close();
  }
  await desk.context.close();
  console.log('95_180_plan_edit_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
