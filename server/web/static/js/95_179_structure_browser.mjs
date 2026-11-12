import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const cookie = process.env.AUTH_COOKIE;
const projectID = process.env.PROJECT_ID;
const phaseID = process.env.PHASE_ID;
const pkgID = process.env.PKG_ID;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !cookie || !projectID || !phaseID || !pkgID) {
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

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  const desk = await open(browser, { width: 1440, height: 900 });
  const page = desk.page;
  page.on('dialog', (d) => d.accept());

  // E: 대시보드 칩
  await visit(page, '/projects/dashboard');
  const chip = page.locator('[data-deadline-soon]');
  if ((await chip.count()) !== 1 || !(await chip.first().isVisible())) fail('마감 임박 칩이 안 보임');
  if (!(await chip.first().innerText()).includes('마감 지남')) fail('칩 문구', await chip.first().innerText());
  await page.screenshot({ path: path.join(shotDir, 'dashboard-1440.png') });

  // F: 수정 화면 카드 → 사이트 등록 화면
  await visit(page, '/projects/' + projectID + '/edit');
  const card = page.locator('[data-site-reg-card]');
  if (!(await card.isVisible())) fail('사이트 카드 안 보임');
  await Promise.all([
    page.waitForURL(new RegExp('/projects/' + projectID + '/sites$'), { timeout: 10000, waitUntil: 'domcontentloaded' }),
    card.getByRole('link', { name: /사이트 등록\/관리/ }).click(),
  ]);

  // B: 1단계에 사업 사이트 전체 가져오기 → 등록 3곳
  await visit(page, '/projects/' + projectID + '/phases/' + phaseID + '/sites');
  if (!(await page.locator('[data-register-shortcut]').count())) fail('사업에 사이트 등록 바로가기 없음');
  await Promise.all([
    page.waitForLoadState('domcontentloaded'),
    page.locator('[data-add-all]').click(),
  ]);
  await page.waitForSelector('[data-registered]');
  let reg = (await page.locator('[data-registered]').first().innerText()).trim();
  if (reg !== '3') fail('1단계 등록 수', reg);
  await page.screenshot({ path: path.join(shotDir, 'phase-sites-1440.png') });

  // 2단계 하부 업무: 1단계 사이트 중에서 전체
  await visit(page, '/projects/' + projectID + '/packages/' + pkgID + '/sites');
  await Promise.all([
    page.waitForLoadState('domcontentloaded'),
    page.locator('[data-add-all]').click(),
  ]);
  await page.waitForSelector('[data-registered]');
  reg = (await page.locator('[data-registered]').first().innerText()).trim();
  if (reg !== '3') fail('하부 업무 등록 수', reg);

  // 프로젝트 화면: 등록 N곳 중 연결, 배지, 변경내역
  await visit(page, '/projects/' + projectID);
  if (!(await page.locator('[data-site-reg-note]').innerText()).includes('등록 3곳 중 작업 연결 0곳')) fail('등록 안내 문구');
  if (!(await page.locator('[data-pkg-badge]').count())) fail('하부 업무 배지 없음');
  await Promise.all([
    page.waitForURL(/\/changes$/, { timeout: 10000, waitUntil: 'domcontentloaded' }),
    page.locator('[data-changes-link]').first().click(),
  ]);

  for (const [w, name] of [[390, 'mobile'], [820, 'tablet'], [1440, 'desktop']]) {
    const v = await open(browser, { width: w, height: 900 });
    v.page.on('dialog', (d) => d.accept());
    for (const p of ['/projects/dashboard', '/projects/' + projectID, '/projects/' + projectID + '/edit',
      '/projects/' + projectID + '/phases/' + phaseID + '/sites', '/projects/' + projectID + '/packages/' + pkgID + '/sites',
      '/projects/' + projectID + '/changes']) {
      await visit(v.page, p);
      if (!(await noOverflow(v.page))) {
        await v.page.screenshot({ path: path.join(shotDir, 'overflow-' + w + '.png'), fullPage: true });
        fail(w + 'px 가로 넘침 ' + p);
      }
    }
    await visit(v.page, '/projects/' + projectID);
    await v.page.screenshot({ path: path.join(shotDir, 'project-' + name + '-' + w + '.png'), fullPage: true });
    await v.context.close();
  }
  await desk.context.close();
  console.log('95_179_structure_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
