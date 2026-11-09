import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

async function launch() {
  const { chromium } = require('playwright');
  try {
    return await chromium.launch({ headless: true, channel: 'chrome' });
  } catch (e) {
    try {
      return await chromium.launch({ headless: true, channel: 'msedge' });
    } catch (e2) {
      return await chromium.launch({ headless: true });
    }
  }
}

function fail(msg, extra) {
  console.error(msg, extra || '');
  process.exit(1);
}

const base = process.env.BASE_URL;
const techTok = process.env.AUTH_COOKIE;
const oaTok = process.env.AUTH_COOKIE_OA;
const supTok = process.env.AUTH_COOKIE_SUP;
if (!base || !techTok || !oaTok || !supTok) fail('BASE_URL/AUTH_COOKIE* 필요');

function cookie(token) {
  return { name: 'token', value: token, url: base.endsWith('/') ? base : base + '/' };
}

const kindLabel = {
  admin: '행정/지원 업무',
  sales: '영업 활동',
  project_work: '작업 수행',
  as: 'AS 조치',
};

async function openKind(page, kind) {
  const dlg = page.locator('[data-new-work-dialog]').filter({ visible: true });
  if (!(await dlg.isVisible().catch(() => false))) {
    await page.locator('button[data-new-work]').click();
    await page.waitForTimeout(400);
  }
  await dlg.waitFor({ timeout: 10000 });
  await page.evaluate(() => {
    const root = document.getElementById('board-root');
    const st = root && window.Alpine && Alpine.$data ? Alpine.$data(root) : (root && root._x_dataStack && root._x_dataStack[0]);
    if (st) {
      st.newWorkStep = 1;
      var slot = document.getElementById('new-work-form-slot');
      if (slot) slot.replaceChildren();
      if (typeof st.syncNewWorkDialog === 'function') st.syncNewWorkDialog();
    }
  });
  await page.waitForTimeout(200);
  await dlg.locator('[data-kind="' + kind + '"]').click();
  await dlg.getByRole('button', { name: /다음 단계/ }).click();
  await page.waitForTimeout(2200);
  if (kind === 'as') return;
  const n = await page.locator('#new-work-form-slot [data-project-picker]').count();
  if (!n) fail('피커 없음 kind=' + kind);
}

function measureFn() {
  const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
  if (!pp) return { err: 'no picker' };
  const inp = pp.querySelector('input[type="search"]');
  const ul = pp.querySelector('ul.combobox-list');
  const overlay = document.querySelector('[aria-labelledby="nw-title"]');
  const panel = overlay ? overlay.querySelector(':scope > div') : null;
  const scroller = overlay ? overlay.querySelector('.overflow-y-auto') : null;
  if (!inp || !ul || !panel) return { err: 'missing el' };
  const ir = inp.getBoundingClientRect();
  const ur = ul.getBoundingClientRect();
  const pr = (scroller || panel).getBoundingClientRect();
  const gap = ur.top - ir.bottom;
  const dx = Math.abs(ur.left - ir.left);
  const widthRatio = ir.width ? ur.width / ir.width : 0;
  const inPanel = ur.top >= pr.top - 2 && ur.left >= pr.left - 2 &&
    ur.right <= pr.right + 4 && ur.bottom <= pr.bottom + 8;
  const cs = getComputedStyle(ul);
  const visTop = Math.max(ur.top, pr.top) + 8;
  const visBot = Math.min(ur.bottom, pr.bottom) - 4;
  const sampleY = visBot > visTop ? (visTop + visBot) / 2 : ur.top + 8;
  const sampleX = (ur.left + ur.right) / 2;
  const hit = document.elementFromPoint(sampleX, sampleY);
  const hitOk = !!(hit && ul.contains(hit));
  const data = window.Alpine && Alpine.$data ? Alpine.$data(pp) : null;
  return {
    err: '',
    open: data ? !!data.open : false,
    filtered: data && data.filtered ? data.filtered.length : -1,
    display: cs.display,
    inp: { t: ir.top, b: ir.bottom, l: ir.left, w: ir.width },
    ul: { t: ur.top, b: ur.bottom, l: ur.left, w: ur.width, h: ur.height },
    panel: { t: pr.top, b: pr.bottom, l: pr.left, r: pr.right },
    gap, dx, widthRatio, inPanel, hitOk,
    hitTag: hit ? (hit.tagName + ' ' + (hit.textContent || '').trim().slice(0, 40)) : '',
  };
}

function assertPos(label, m) {
  if (m.err) fail(label + ' 측정 실패', JSON.stringify(m));
  if (!m.open) fail(label + ' open=false', JSON.stringify(m));
  if (m.display === 'none' || m.ul.h < 8) fail(label + ' 목록이 안 보임', JSON.stringify(m));
  if (m.gap < -6 || m.gap > 24) fail(label + ' 목록이 입력칸 바로 아래가 아님 gap=' + m.gap, JSON.stringify(m));
  if (m.dx > 12) fail(label + ' 가로 시작이 입력칸과 안 겹침 dx=' + m.dx, JSON.stringify(m));
  if (m.widthRatio < 0.85) fail(label + ' 목록 폭이 입력칸과 안 맞음', JSON.stringify(m));
  if (!m.inPanel) fail(label + ' 목록이 모달 패널 밖', JSON.stringify(m));
  if (!m.hitOk) fail(label + ' elementFromPoint 가 목록이 아님', JSON.stringify(m));
  console.log('OK', label, 'gap=' + m.gap.toFixed(1), 'w=' + m.widthRatio.toFixed(2), 'hit=' + m.hitTag);
}

async function clickSearch(page) {
  const search = page.locator('#new-work-form-slot [data-project-picker] input[type="search"]').first();
  await search.evaluate((el) => el.scrollIntoView({ block: 'center' }));
  await search.click({ force: true });
  await page.waitForTimeout(250);
  await page.evaluate(() => {
    const ul = document.querySelector('#new-work-form-slot [data-project-picker] ul.combobox-list');
    if (ul) ul.scrollIntoView({ block: 'nearest' });
  });
  await page.waitForTimeout(150);
  return search;
}

const browser = await launch();

async function withCookie(token, fn) {
  const page = await browser.newPage();
  await page.context().addCookies([cookie(token)]);
  await page.setExtraHTTPHeaders({ Cookie: 'token=' + token });
  await page.goto(base + '/workboard/board?view=day&mine=0&auto=0');
  await page.evaluate((t) => { document.cookie = 'token=' + t + '; path=/'; }, token);
  try {
    await fn(page);
  } finally {
    await page.close();
  }
}

await withCookie(techTok, async (page) => {
  const title = await page.title();
  const nw = await page.locator('[data-new-work]').count();
  console.log('board title=' + title + ' newWorkBtns=' + nw + ' url=' + page.url());
  if (!nw) fail('등록 버튼 없음', await page.locator('body').innerText().then((t) => t.slice(0, 400)));
  for (const kind of ['admin', 'sales', 'project_work']) {
    await openKind(page, kind);
    if (kind === 'admin') {
      const sup = page.locator('#new-work-form-slot input[name="work_type"][value="support"]');
      if (await sup.count()) await sup.click();
    }
    const search = await clickSearch(page);
    let m = await page.evaluate(measureFn);
    assertPos('T1 ' + kind + ' desktop', m);

    await search.fill('도서');
    await page.waitForTimeout(150);
    m = await page.evaluate(measureFn);
    assertPos('T2 filter ' + kind, m);
    if (m.filtered < 1) fail('T2 필터 0건', JSON.stringify(m));

    const pick = page.locator('#new-work-form-slot [data-project-picker] ul.combobox-list button').nth(1);
    const name = (await pick.innerText()).replace(/\s+/g, ' ').trim();
    await pick.click();
    await page.waitForTimeout(150);
    const after = await page.evaluate(() => {
      const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
      const d = Alpine.$data(pp);
      const ul = pp.querySelector('ul.combobox-list');
      return { id: d.selectedId, name: d.selectedName, open: d.open, q: d.q, hid: pp.querySelector('input[name="project_id"]').value, ulDisp: getComputedStyle(ul).display };
    });
    if (!after.id || after.open || after.q !== '' || after.hid !== after.id) {
      fail('T2 선택 반영 실패 ' + kind, JSON.stringify({ name, after }));
    }
    console.log('OK T2 pick', kind, after.name);

    await search.click({ force: true });
    await page.evaluate(() => {
      const inner = document.querySelector('[aria-labelledby="nw-title"] .overflow-y-auto')
        || document.querySelector('[data-new-work-dialog] .overflow-y-auto');
      if (inner) inner.scrollTop = inner.scrollHeight;
      const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
      const d = window.Alpine && Alpine.$data ? Alpine.$data(pp) : null;
      if (d && d.comboOpen) d.comboOpen();
      else if (d) d.open = true;
      const inp = pp && pp.querySelector('input[type="search"]');
      if (inp) inp.scrollIntoView({ block: 'center' });
      const ul = pp && pp.querySelector('ul.combobox-list');
      if (ul) ul.scrollIntoView({ block: 'nearest' });
    });
    await page.waitForTimeout(200);
    m = await page.evaluate(measureFn);
    assertPos('T1 scroll ' + kind, m);
  }

  await openKind(page, 'as');
  await page.waitForFunction(() => {
    const slot = document.getElementById('new-work-form-slot');
    if (!slot) return false;
    return !!(slot.querySelector('#as-register-source') || (slot.innerText || '').indexOf('AS') >= 0);
  }, null, { timeout: 8000 }).catch(() => {});
  const asPicker = await page.locator('#new-work-form-slot [data-project-picker]').count();
  if (asPicker) fail('AS 폼에 project_picker 가 생겼음');
  console.log('OK AS 폼 피커 없음');

  await page.setViewportSize({ width: 390, height: 844 });
  await openKind(page, 'admin');
  await clickSearch(page);
  const mob = await page.evaluate(measureFn);
  assertPos('T1 admin 390px', mob);

  await page.evaluate(() => {
    const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
    const b = [...pp.querySelectorAll('button')].find((el) => (el.textContent || '').includes('타 부서 사업 보기'));
    if (b) { b.scrollIntoView({ block: 'center' }); b.click(); }
  });
  await page.waitForTimeout(200);
  const denied = await page.evaluate(() => {
    const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
    const d = Alpine.$data(pp);
    return { denied: d.otherDenied, open: d.otherOpen, text: pp.innerText };
  });
  if (!denied.denied || denied.open) fail('T3 tech 403 안내 없음', JSON.stringify(denied));
  if (!denied.text.includes('타 부서 사업을 선택할 권한이 없습니다')) fail('T3 tech 문구 없음', denied.text.slice(0, 200));
  console.log('OK T3 tech deny');
});

for (const [label, tok] of [['oa', oaTok], ['sup', supTok]]) {
  await withCookie(tok, async (page) => {
    await openKind(page, 'admin');
    await clickSearch(page);
    const pos = await page.evaluate(measureFn);
    assertPos('T3 ' + label + ' own list', pos);
    await page.evaluate(() => {
      const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
      const b = [...pp.querySelectorAll('button')].find((el) => (el.textContent || '').includes('타 부서 사업 보기'));
      if (b) { b.scrollIntoView({ block: 'center' }); b.click(); }
    });
    await page.waitForTimeout(800);
    const other = await page.evaluate(() => {
      const pp = document.querySelector('#new-work-form-slot [data-project-picker]');
      const d = Alpine.$data(pp);
      const overlay = document.querySelector('[aria-labelledby="nw-title"]');
      const panel = overlay.querySelector(':scope > div');
      const box = pp.querySelector('[x-show="otherOpen"]') || [...pp.querySelectorAll('div')].find((el) => (el.innerText || '').includes('미디어사업'));
      const pr = panel.getBoundingClientRect();
      const br = box ? box.getBoundingClientRect() : null;
      const inPanel = br && br.top >= pr.top - 2 && br.bottom <= pr.bottom + 24;
      const hitName = '미디어사업';
      const b = [...pp.querySelectorAll('button')].find((el) => (el.textContent || '').includes(hitName));
      if (b) b.click();
      const d2 = Alpine.$data(pp);
      return {
        otherOpen: d.otherOpen, groups: (d.otherGroups || []).length, denied: d.otherDenied,
        inPanel, picked: d2.selectedName, id: d2.selectedId,
      };
    });
    if (other.denied || !other.otherOpen && other.groups === 0 && !other.id) {
      fail('T3 ' + label + ' 타 부서 목록 실패', JSON.stringify(other));
    }
    if (other.groups === 0 && !other.id) fail('T3 ' + label + ' groups 0', JSON.stringify(other));
    if (other.inPanel === false) fail('T3 ' + label + ' 타 부서 목록이 모달 밖', JSON.stringify(other));
    if (other.id !== 'WP-O02' && other.picked && !String(other.picked).includes('미디어')) {
      fail('T3 ' + label + ' 선택 실패', JSON.stringify(other));
    }
    console.log('OK T3', label, JSON.stringify(other));
  });
}

await browser.close();
console.log('95_117_picker_pos_browser ok');
