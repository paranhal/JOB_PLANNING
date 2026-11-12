import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const projectID = process.env.PROJECT_ID;
const fromOrg = process.env.FROM_ORG || 'O01';
const toOrg = process.env.TO_ORG || 'O02';
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !projectID) {
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

async function goEdit(page) {
  await page.goto(base + '/projects/' + projectID + '/edit', { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => window.Alpine, null, { timeout: 8000 });
}

async function reasonVisible(page) {
  const el = page.locator('[name="org_move_reason"]');
  if (!(await el.count())) return false;
  return el.first().isVisible();
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1280, height: 800 });
  const page = desk.page;

  await goEdit(page);
  if (await reasonVisible(page)) fail('처음에도 이관 칸이 보임');

  await page.locator('#org-picker-owner').selectOption(toOrg);
  try {
    await page.waitForFunction(() => {
      const el = document.querySelector('[name="org_move_reason"]');
      return el && el.offsetParent !== null;
    }, null, { timeout: 5000 });
  } catch (e) {
    await page.screenshot({ path: path.join(shotDir, 'after-org-change.png') });
    fail('주관 부서 변경 후 이관 사유 칸이 안 보임');
  }
  await page.screenshot({ path: path.join(shotDir, 'after-org-change.png') });

  await page.getByRole('button', { name: '저장' }).click();
  await page.waitForTimeout(400);
  if (!page.url().includes('/edit')) fail('사유 없이 저장이 넘어감 ' + page.url());
  if (!(await reasonVisible(page))) fail('사유 없이 저장 후 칸 숨김');
  const blocked = await page.evaluate(() => {
    const t = (document.body.innerText || '');
    return t.includes('이관 사유');
  });
  if (!blocked) fail('사유 필수 안내 없음');

  await page.locator('[name="org_move_reason"]').fill('95-164 이관 테스트');
  await Promise.all([
    page.waitForURL(new RegExp('/projects/' + projectID), { timeout: 15000, waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: '저장' }).click(),
  ]);
  if (page.url().includes('/edit')) fail('사유 입력 후 이관 실패 ' + page.url());

  await goEdit(page);
  const cur = await page.locator('#org-picker-owner').inputValue();
  if (cur !== toOrg) fail('이관 후 주관', cur);
  // 이관 저장 뒤 현재 팀은 toOrg 다. 거기서 다른 팀(fromOrg)을 고르면 다시 이관이므로 이관 칸이 **보여야** 하고,
  // 현재 팀(cur)으로 되돌리면 **숨어야** 한다. 시작 상태가 「숨김」이라 숨김만 기다리면 Alpine 갱신 전에
  // 우연히 통과하므로, 반드시 보임을 먼저 기다린 뒤 숨김을 기다린다.
  await page.locator('#org-picker-owner').selectOption(fromOrg);
  await page.waitForFunction(() => {
    const el = document.querySelector('[name="org_move_reason"]');
    return !!el && el.offsetParent !== null;
  }, null, { timeout: 5000 }).catch(() => fail('이관 후 다른 팀을 골랐는데 이관 칸이 안 보임'));
  await page.locator('#org-picker-owner').selectOption(cur);
  await page.waitForFunction(() => {
    const el = document.querySelector('[name="org_move_reason"]');
    return !el || el.offsetParent === null;
  }, null, { timeout: 5000 }).catch(() => fail('현재 팀으로 되돌렸는데 이관 칸이 남음'));

  const mobile = await open(browser, { width: 390, height: 844 });
  await goEdit(mobile.page);
  await mobile.page.locator('#org-picker-owner').selectOption(fromOrg === cur ? toOrg : fromOrg);
  await mobile.page.waitForTimeout(200);
  if (!(await noOverflow(mobile.page))) fail('390px 가로 넘침');
  await mobile.page.screenshot({ path: path.join(shotDir, 'mobile-390.png') });
  await mobile.context.close();
  await desk.context.close();
  console.log('95_164_org_move_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
