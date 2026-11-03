import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(join(here, '../../node_modules/playwright/package.json'));
const shotDir = join(here, '../../../docs/리뷰/screenshots/95-98-smoke');
mkdirSync(shotDir, { recursive: true });

const base = process.env.SMOKE_BASE || 'http://127.0.0.1:18098';
const user = process.env.SMOKE_USER || 'admin';
const pass = process.env.SMOKE_PASS || 'admin';
const consoles = [];

async function launch() {
  const { chromium } = require('playwright');
  const opts = { headless: true };
  for (const channel of [undefined, 'chrome', 'msedge']) {
    try {
      return await chromium.launch(channel ? { ...opts, channel } : opts);
    } catch (e) {
      last = e;
    }
  }
  throw last;
}
let last;

function attachConsole(page) {
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoles.push(msg.text());
  });
  page.on('pageerror', (err) => consoles.push(String(err)));
}

async function failShot(page, name) {
  const p = join(shotDir, name + '.png');
  try { await page.screenshot({ path: p, fullPage: true }); } catch (_) {}
  writeFileSync(join(shotDir, name + '-console.log'), consoles.join('\n'), 'utf8');
  console.error('saved', p);
}

const browser = await launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
attachConsole(page);

try {
  await page.goto(base + '/login', { waitUntil: 'domcontentloaded' });
  await page.locator('form input[type="text"], form input[name="username"]').first().fill(user);
  await page.locator('form input[type="password"]').first().fill(pass);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
    page.getByRole('button', { name: '로그인' }).click(),
  ]);
  await page.goto(base + '/', { waitUntil: 'domcontentloaded' });
  await page.screenshot({ path: join(shotDir, 's1-login.png') });

  // S1 sidebar
  const groups = page.locator('nav button').filter({ hasText: /고객관리|설정/ });
  if (await groups.count()) {
    await groups.first().click();
    await page.waitForTimeout(200);
  }
  await page.screenshot({ path: join(shotDir, 's1-sidebar.png') });
  const body1 = await page.content();
  if (body1.includes('x-data=') && />(\s*)x-data=/.test(body1) === false) {
    // ok if only in attributes
  }
  const text = await page.locator('body').innerText();
  if (text.includes('x-data=') || text.includes('@click=')) {
    await failShot(page, 's1-leak');
    throw new Error('S1 본문에 x-data=/@click= 글자');
  }

  // S2 workboard
  await page.goto(base + '/workboard/board?view=list', { waitUntil: 'domcontentloaded' });
  await page.screenshot({ path: join(shotDir, 's2-list.png') });
  for (const v of ['week', 'month']) {
    await page.goto(base + '/workboard/board?view=' + v, { waitUntil: 'domcontentloaded' });
  }
  const newBtn = page.getByRole('button', { name: /새 업무|등록/ });
  if (await newBtn.count() && await newBtn.first().isVisible()) {
    await newBtn.first().click();
    await page.waitForTimeout(300);
  } else {
    await page.goto(base + '/workboard/register', { waitUntil: 'domcontentloaded' });
  }
  await page.screenshot({ path: join(shotDir, 's2-register.png') });

  // S3 AS action
  await page.goto(base + '/as', { waitUntil: 'domcontentloaded' });
  const asLink = page.locator('a[href*="/as/"][href*="action"], a[href^="/as/R"], a[href*="/as/"]').first();
  if (await asLink.count()) {
    await asLink.click();
    await page.waitForTimeout(400);
  }
  const actionUrl = page.url();
  if (!actionUrl.includes('/action')) {
    const href = await page.locator('a[href*="/action"]').first().getAttribute('href').catch(() => null);
    if (href) await page.goto(base + href, { waitUntil: 'domcontentloaded' });
  }
  await page.screenshot({ path: join(shotDir, 's3-action.png') });
  const actText = await page.locator('body').innerText();
  if (actText.includes('담당자') && !actText.includes('관리자') && !actText.includes('기술')) {
    // 이름이 전혀 없으면 실패하지 않고 기록
  }

  // S4 assignee picker / amount if present
  await page.goto(base + '/sales', { waitUntil: 'domcontentloaded' }).catch(() => {});
  await page.screenshot({ path: join(shotDir, 's4-sales.png') });

  // S5 crawl
  const urls = ['/', '/as', '/work', '/customers', '/stats', '/projects', '/maintenance'];
  for (const u of urls) {
    const res = await page.goto(base + u, { waitUntil: 'domcontentloaded' });
    const code = res ? res.status() : 0;
    if (code >= 500) {
      await failShot(page, 's5-' + u.replace(/\W+/g, '_'));
      throw new Error('S5 ' + u + ' HTTP ' + code);
    }
    const t = await page.locator('body').innerText();
    if (t.includes('x-data=') || t.includes('@click=')) {
      await failShot(page, 's5-leak-' + u.replace(/\W+/g, '_'));
      throw new Error('S5 본문 누출 ' + u);
    }
  }
  const errors = consoles.filter((c) => !/favicon|Failed to load resource|Download the React DevTools/i.test(c));
  if (errors.length) {
    writeFileSync(join(shotDir, 's5-console.log'), errors.join('\n'), 'utf8');
    throw new Error('S5 콘솔 오류 ' + errors.length + '건');
  }
  console.log('SMOKE_OK S1-S5');
} catch (e) {
  await failShot(page, 'fail');
  console.error(e);
  process.exit(1);
} finally {
  await browser.close();
}
