import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const base = process.env.BASE_URL;
const token = process.env.AUTH_COOKIE;
const week = process.env.WEEK;
const shotDir = process.env.SCREENSHOT_DIR || '.';
if (!base || !token || !week) {
  console.error('env');
  process.exit(1);
}

async function launch() {
  const { chromium } = require('playwright');
  for (const extra of [{ channel: 'chrome' }, {}]) {
    try {
      return await chromium.launch(Object.assign({ headless: true }, extra));
    } catch (e) { /* 다음 */ }
  }
  throw new Error('launch');
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

const browser = await launch();
try {
  fs.mkdirSync(shotDir, { recursive: true });
  for (const [w, name] of [[1440, 'desktop'], [820, 'tablet'], [390, 'mobile']]) {
    const context = await browser.newContext({ viewport: { width: w, height: 900 } });
    const u = new URL(base);
    await context.addCookies([
      { name: 'token', value: token, url: base },
      { name: 'token', value: token, domain: u.hostname, path: '/' },
    ]);
    const page = await context.newPage();
    await page.goto(base + '/stats/company-weekly?week=' + week + '&org=O03&edit=1', { waitUntil: 'domcontentloaded' });
    const doneBtn = page.locator('[data-insert-target^="done_"]').first();
    const planBtn = page.locator('[data-insert-target^="plan_"]').first();
    if (!(await doneBtn.isVisible())) fail(w + 'px 금주에 넣기 버튼이 안 보임');
    const doneTa = page.locator('textarea[name^="done_"]').first();
    const planTa = page.locator('textarea[name^="plan_"]').first();

    // 작성자가 이미 쓴 내용은 그대로 남고 끝에 추가된다
    await doneTa.fill('- 홍천교육청 협의 완료');
    await doneBtn.click();
    let v = await doneTa.inputValue();
    if (!v.startsWith('- 홍천교육청 협의 완료\n') || !v.includes('- 환경조사 120식 중 30식 완료(25%)')) fail(w + 'px 금주 칸', v);
    // 한 번 더 누르면 중복으로 들어가지 않는다
    await doneBtn.click();
    const v2 = await doneTa.inputValue();
    if (v2 !== v) fail(w + 'px 중복으로 들어감', v2);
    const hint = await page.locator('[data-insert-hint]').first().innerText();
    if (!hint.includes('이미 들어 있습니다')) fail(w + 'px 안내 문구', hint);

    await planBtn.click();
    const p = await planTa.inputValue();
    if (!p.includes('- 환경조사 남은 90식 진행')) fail(w + 'px 차주 칸', p);

    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 8);
    if (overflow) fail(w + 'px 가로 넘침');
    await page.screenshot({ path: path.join(shotDir, 'weekly-insert-' + name + '-' + w + '.png'), fullPage: true });
    await context.close();
  }
  console.log('95_181_weekly_insert_browser.mjs ok');
} catch (e) {
  fail(String(e && e.stack ? e.stack : e));
} finally {
  await browser.close();
}
