# UI 업그레이드 설계 및 Cursor 작업지시서 (v0.3 — Linear 스타일, 라이트 전용, 결재 대비)

작성일: 2026-10-11 · 대상: `server/` (Go Echo + html/template + HTMX + Alpine + Tailwind 3.4.17)
역할 분담: **Figma = 시각 결정(토큰·컴포넌트·패턴 화면) / Claude = IA·디자인 시스템 명세·결과 리뷰 / Cursor = 규칙 파일 기준 구현 + 테스트(필수)**
관련 문서: `claude/ia-and-approval-plan.md` (메뉴 구조 IA + 결재 첫 적용 설계)

변경 이력
- v0.1 토큰·컴포넌트·ratchet 테스트
- v0.2 Linear 스타일, 결재 대비 컴포넌트
- **v0.3 다크모드 제거(라이트 고정), 회사색 미확정 대응(색 교체 지점 1곳 + 대비 테스트), 기존 Figma 프레임 무시하고 신규 제작, IA 동시 진행, 결재 첫 대상 = 견적서·휴가 신청서**

---

## 0. 한 줄 결론

**새로 만들지 않는다.** 현재 스택(서버 렌더링 + HTMX + Alpine + Tailwind) 위에서 **토큰 → 컴포넌트 → 셸(+IA) → 패턴 화면 → 나머지** 순으로 점진 업그레이드한다. Linear의 "조밀하고 조용한 UI, 키보드 중심, 사이드 패널"은 현 스택으로 구현 가능하다. 결재가 쓸 컴포넌트는 지금 디자인 시스템에 포함한다.

## 1. 확정 결정사항

| # | 결정 | 근거 |
|---|------|------|
| D1 | React/Vue/Bootstrap/번들러 도입 금지 | 폐쇄망·벤더 고정·권한 HTML 이중화 방지 |
| D2 | 디자인 값은 **토큰(CSS 변수 + `theme.extend`)** 한 곳에만 정의 | 현재 `theme.extend` 공백 |
| D3 | 반복 UI는 `@layer components` 클래스 + `ui/_ui_*.html` 부분 템플릿 | 버튼 하나가 20여 조합으로 복붙됨 |
| D4 | 템플릿에 원색 유틸·hex·인라인 style 신규 사용 금지 (T1 린트) | 일관성 |
| D5 | CDN·웹폰트 CDN 금지 유지. 폰트는 **Pretendard 자체 호스팅 woff2** | 폐쇄망 |
| D6 | Figma Variables 이름 = Tailwind 토큰 이름 (1:1) | 해석 여지 제거 |
| D7 | 기획서 v2와 Figma가 다르면 기획서 우선 | AGENTS.md |
| D8 | 스타일 방향: **Linear 풍** — 저채도 중립색 + 단일 강조색, 1px 경계, 작은 모서리, 조밀한 행, 키보드 중심 | 사용자 결정 |
| D9 | **라이트 전용. 다크모드 없음.** 기기(iPhone 등)의 다크 설정과 무관하게 항상 같은 화면 | 사용자 결정 (아래 §2.3) |
| D10 | **회사색 미확정**: 강조색 임시값으로 시작, 확정 시 **CSS 변수 4개 값만 교체** | §2.2 |
| D11 | 결재 대비: 결재선 스테퍼·상태 뱃지·사이드 패널·댓글 스레드 선행 정의 | 결재 첫 대상: 견적서·휴가 신청서 |
| D12 | 스타일 전환 PR에서 로직·권한·쿼리 변경 금지 | 회귀 원인 분리 |
| D13 | 인라인 `<script>`는 신규 화면부터 `web/static/js/*.js`로 분리 | 유지보수 |
| D14 | Figma는 **기존 프레임을 무시하고 신규 제작** | 사용자 결정 |
| D15 | IA(메뉴 구조)는 디자인과 **같은 흐름**에서 확정 (`ia-and-approval-plan.md`) | 사용자 결정 |

## 2. 디자인 토큰

### 2.1 토큰 정의 (라이트 단일 세트)

RGB 3값으로 선언해야 `bg-primary/50` 같은 투명도 유틸이 동작한다.

`web/css/input.css`
```css
@layer base {
  :root {
    color-scheme: light;            /* 기기 다크 설정이 있어도 라이트 고정 (§2.3) */
    --c-canvas: 252 252 253;        --c-surface: 255 255 255;
    --c-sidebar: 246 246 248;       --c-sidebar-text: 60 63 72;
    --c-hover: 243 243 245;         --c-selected: 238 240 253;
    --c-border: 228 229 233;        --c-border-strong: 208 210 217;
    --c-text: 28 29 33;             --c-text-muted: 108 111 122;  --c-text-subtle: 150 153 163;
    /* ── 회사색 확정 시 교체하는 곳: 아래 4줄 ── */
    --c-primary: 91 103 214;        --c-primary-hover: 76 88 196;
    --c-primary-soft: 238 240 253;  --c-on-primary: 255 255 255;
    /* ──────────────────────────────────────── */
    --c-success: 34 160 107;        --c-success-soft: 232 247 240;
    --c-warning: 214 140 20;        --c-warning-soft: 253 244 227;
    --c-danger: 224 66 66;          --c-danger-soft: 253 236 236;
    --c-info: 59 130 246;           --c-info-soft: 233 242 254;
    /* 업무 의미색 (화면설계서: 점검대상/지연) */
    --c-target-a: 34 160 107;  --c-target-b: 59 130 246;  --c-target-c: 139 92 246;  --c-overdue: 224 66 66;
    --radius-control: 6px;  --radius-card: 8px;
    --shadow-pop: 0 8px 24px rgb(0 0 0 / .12);
  }
}
```

`tailwind.config.js`
```js
const c = (n) => `rgb(var(--c-${n}) / <alpha-value>)`;
theme: { extend: {
  colors: {
    canvas: c('canvas'), surface: c('surface'), hover: c('hover'), selected: c('selected'),
    sidebar: { DEFAULT: c('sidebar'), text: c('sidebar-text') },
    line: { DEFAULT: c('border'), strong: c('border-strong') },
    ink: { DEFAULT: c('text'), muted: c('text-muted'), subtle: c('text-subtle') },
    primary: { DEFAULT: c('primary'), hover: c('primary-hover'), soft: c('primary-soft'), fg: c('on-primary') },
    success: { DEFAULT: c('success'), soft: c('success-soft') },
    warning: { DEFAULT: c('warning'), soft: c('warning-soft') },
    danger:  { DEFAULT: c('danger'),  soft: c('danger-soft') },
    info:    { DEFAULT: c('info'),    soft: c('info-soft') },
    target: { a: c('target-a'), b: c('target-b'), c: c('target-c') },
    overdue: c('overdue'),
  },
  borderRadius: { control: 'var(--radius-control)', card: 'var(--radius-card)' },
  boxShadow: { pop: 'var(--shadow-pop)' },
  fontFamily: { sans: ['Pretendard Variable', 'Pretendard', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Malgun Gothic', 'sans-serif'] },
  fontSize: { 'ui-xs': ['0.6875rem','1rem'], 'ui-sm': ['0.75rem','1.125rem'], ui: ['0.8125rem','1.25rem'], 'ui-lg': ['0.9375rem','1.375rem'], 'ui-xl': ['1.125rem','1.5rem'] },
}}
```
기본 본문은 **13px(`text-ui`)**. 기존 `text-sm`(14px)은 일괄 치환하지 말고 화면 전환 때 컴포넌트와 함께 바꾼다.

### 2.2 회사색이 확정되면 (교체 절차)

1. Figma Variables의 `primary`, `primary/hover`, `primary/soft`, `primary/on` 4개를 확정 색으로 교체
2. `input.css`의 위 **4줄 값만** 교체 (다른 파일 수정 없음)
3. `npm run build:css` → 커밋 → T6(대비 테스트) 통과 확인
4. 로고·파비콘은 별도 작업(토큰과 무관)

회사색이 **밝은 색(노랑 등)** 이면 `on-primary`를 흰색이 아닌 어두운 색으로 바꿔야 한다. 이 판단을 사람 눈이 아니라 **T6이 자동 검증**한다.

### 2.3 다크모드를 넣지 않는 이유와 주의점

"다크모드"는 화면 전체를 어두운 배경·밝은 글자로 바꾸는 기능이다(아이폰 설정의 '다크 모드'와 같은 것). 이 시스템은 **넣지 않는다**.
단, 브라우저나 기기가 **강제로** 어둡게 바꾸는 경우(자동 다크)가 있어 화면이 깨져 보일 수 있으므로 아래를 적용한다.
- `:root { color-scheme: light; }` (위 토큰에 포함)
- `base.html` `<head>`에 `<meta name="color-scheme" content="light">` 추가
- 인쇄(A4/견적서/결재문서)는 어차피 흰 배경 → 영향 없음

### 2.4 Linear 풍 규칙 (Figma 디자인 시 준수)
- 색은 중립 회색 + **강조색 1개**. 상태색(성공/경고/위험)은 뱃지·아이콘에만 작게. 큰 면적에 채도 높은 색 금지.
- 경계는 1px `border-line`, 그림자는 팝오버·메뉴(`shadow-pop`)에만. 카드에 그림자 금지.
- 목록은 **행 높이 32px(조밀)/36px(기본)**, hover `bg-hover`, 선택 `bg-selected`.
- 사이드바는 옅은 회색(`bg-sidebar`), 항목은 아이콘 16px + 라벨, 선택 항목은 `bg-hover` + `text-ink`.
- 상태는 색 점/아이콘 + 짧은 텍스트. 단축키는 `kbd` 칩.
- 빈 값·측정 불가는 `—` (기획서 3대 원칙).

### 2.5 기존 클래스 → 토큰 매핑 (치환 가이드)

| 기존 | 토큰 |
|------|------|
| `bg-gray-50` (페이지) | `bg-canvas` |
| `bg-white` (카드·모달) | `bg-surface` |
| `bg-gray-900` (사이드바) | `bg-sidebar` (**시각 변화 큼 — Phase 2에서만**) |
| `bg-blue-600 hover:bg-blue-700 text-white` | `.btn-primary` |
| `text-gray-500/600` | `text-ink-muted` |
| `text-gray-700/900` | `text-ink` |
| `border-gray-300` / `border-gray-200` | `border-line` (구분 필요 시 `-strong`) |
| `bg-green-50` / `bg-amber-50` / `bg-red-700` 플래시 | `.flash-success` / `.flash-warning` / `.flash-danger` |
| `rounded-lg` (입력/버튼) / `rounded-xl` (카드) | `rounded-control` / `rounded-card` |
| `text-sm` | `text-ui` (컴포넌트 안에서) |

## 3. 컴포넌트 명세

### 3.1 CSS 컴포넌트 클래스 (`@layer components`)

| 클래스 | 용도 |
|--------|------|
| `.btn` + `-primary/-secondary/-danger/-ghost`, `.btn-sm` | 버튼 (높이 28/32px) |
| `.input` `.select` `.textarea` `.is-invalid`, `.label` `.field-hint` `.field-error` | 폼 |
| `.card` `.card-header` `.card-body` | 경계선 카드 (그림자 없음) |
| `.badge` + `-success/-warning/-danger/-info/-neutral/-target-a/b/c/-overdue` | 상태 뱃지 |
| `.table-base`, `.list-row`, `.list-row-selected` | 표 / Linear식 목록 행 |
| `.flash` + 톤 | 상단 알림 (`?ok=` `?err=`) |
| `.tab` `.tab-active` | 탭 |
| `.kbd` | 단축키 칩 |
| `.avatar` `.avatar-sm` | 사용자 원형 이니셜 |
| `.menu` `.menu-item` | 드롭다운/팝오버 메뉴 (`shadow-pop`) |
| `.modal-backdrop` `.modal-panel`, `.peek-panel` | 모달 / 우측 사이드 패널 |
| `.stepper` `.stepper-step(-done/-current/-pending/-rejected)` | 단계 표시 (**결재선·업무 진행 공용**) |
| `.toast` | 토스트 |
| `.sidebar-section` `.sidebar-item` `.sidebar-item-active` `.sidebar-count` | 사이드바 (IA 문서 참고) |

예시:
```css
@layer components {
  .btn { @apply inline-flex items-center justify-center gap-1.5 rounded-control px-4 py-2 text-ui font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed; }
  .btn-primary { @apply btn bg-primary text-primary-fg hover:bg-primary-hover; }
  .btn-secondary { @apply btn border border-line bg-surface text-ink hover:bg-hover; }
  .btn-danger { @apply btn bg-danger text-white hover:opacity-90; }
  .btn-ghost { @apply btn text-ink-muted hover:bg-hover; }
  .btn-sm { @apply px-3 py-1.5 text-ui-sm; }
  .input { @apply w-full rounded-control border border-line bg-surface px-3 py-2 text-ui text-ink placeholder:text-ink-subtle focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary; }
  .badge { @apply inline-flex items-center rounded-full px-2 py-0.5 text-ui-sm font-medium; }
  .badge-success { @apply badge bg-success-soft text-success; }
}
```

### 3.2 부분 템플릿 (`web/templates/ui/_ui_*.html`)

`_ui_page_header`, `_ui_flash`, `_ui_field`, `_ui_empty`, `_ui_modal`, `_ui_peek`(우측 패널 껍데기), `_ui_badge`(상태→톤 한 곳), `_ui_stepper`, `_ui_comment_thread`, `_ui_avatar`, `_ui_icons`(SVG를 `{{define "icon_xxx"}}`로 모음), `_ui_kbd_help`(단축키 도움말).
규칙: 부분 템플릿은 **권한 판단을 하지 않는다**(서버가 `CanWrite` 등을 전달). 상태→톤 매핑과 상태 판정은 **서버 공유 함수** 한 곳.

### 3.3 Linear 패턴을 현 스택에서 구현하는 방법

| Linear 패턴 | 구현 방식 (신규 도구 없음) |
|-------------|----------------------------|
| 조밀한 목록 + 행 hover/선택 | `.list-row` + 서버 정렬/필터 (`sort/_th.html` 재사용) |
| **사이드 피크 패널** | `hx-get`으로 상세 content 조각을 `#peek`에 swap, `hx-push-url`로 `?peek=ID`. **직접 접근/새로고침 시 풀페이지로 폴백**(서버가 HX-Request 유무로 분기 — `renderer.go`에 구조 있음) |
| **커맨드 팔레트 (Ctrl/Cmd+K)** | Alpine 컴포넌트 + 서버 JSON `/api/palette?q=` (기존 자동완성 fetch 패턴). **결과 필터링은 서버가 권한·조직 기준으로** |
| 키보드 단축키 (J/K 이동, Enter 열기, C 신규, `?` 도움말) | Alpine `@keydown.window`, 입력 필드 포커스 중에는 무시 |
| 필터 칩 / 저장된 뷰 | 서버 쿼리 파라미터 + 칩 UI (뷰 저장은 후속) |
| 인라인 상태 변경 | 기존 칸반 상태 저장 fetch 패턴 재사용 + 팝오버 `.menu` |
| 토스트 | Alpine store + 서버 `HX-Trigger` 헤더로 발행 |

**한계(솔직히)**: Linear의 "즉시 반응"은 클라이언트 동기화·낙관적 UI에서 나온다. 서버 렌더링 구조에서는 재현하지 않는다. 대신 빠른 서버 응답, 부분 교체(HTMX), 인디케이터, 키보드 조작으로 **체감 근사**한다.

## 4. Figma 쪽 규칙 (신규 제작)

기존 프레임은 참고하지 않는다. 아래 순서로 새로 만든다.

1. **Variables**: 컬렉션 `color`(모드 1개: Light), `radius`, `space`. 변수명 = 코드 토큰명(`primary`, `ink/muted`, `line/strong` …). 회사색 미확정이므로 `primary` 계열은 임시값으로 두고 확정 시 교체.
2. **컴포넌트 이름 = 클래스 이름**: `Button/Primary`, `Badge/Success`, `ListRow`, `Stepper`, `PeekPanel`, `Menu`, `Kbd`, `SidebarItem` …
3. **그릴 패턴 9개**(화면 156개가 아님): ① 앱 셸(사이드바·헤더·조직 스위처·커맨드 팔레트) ② 목록(+필터 칩·정렬·모바일) ③ 폼 ④ 상세(+탭) ⑤ 사이드 피크 패널 ⑥ 대시보드/지표 ⑦ 모달·확인 ⑧ 칸반(§35 해당 화면만) ⑨ **결재 문서 화면(상세+결재선+의견)**
4. 반응형 프레임 2종: **1280(lg+)**, **390(모바일)**. 브레이크포인트 유지(sm 640 / md 768 / lg 1024).
5. 사이드바 메뉴 구성은 `ia-and-approval-plan.md` §1을 따른다.
6. Code Connect는 요금제 지원 확인 후 사용, 불가하면 §2.5 매핑표 + 이름 규칙으로 대체.

## 5. 단계별 계획과 완료 기준(DoD)

**공통 DoD**:
1. `go build ./...` · `go vet ./...` · `go test ./...` 통과
2. `npm run build:css` 후 `web/static/vendor/tailwind.css` 커밋
3. §6 테스트 통과 (T1~T3, 해당 단계의 T6·T7)
4. 변경 화면 수동 검수 + 전/후 스크린샷
5. Alpine Expression Error 0건, 콘솔 오류 0건

| Phase | 내용 | 시각 변화 | 추가 완료 기준 |
|-------|------|-----------|----------------|
| **0 기준선** | 테스트 하네스, baseline 고정, 대표 화면 스크린샷 | 없음 | baseline 커밋, green |
| **1 토큰·컴포넌트** | §2 토큰, §3.1 CSS, Pretendard 자체 호스팅, `color-scheme` 메타. **템플릿 미수정** | 폰트 외 없음 | T2, T6 |
| **2 셸 + IA** | `base.html` 사이드바(옅은 회색)·헤더·조직 스위처·배너·플래시. IA 문서 §1 적용 (섹션 접기, 결재/휴가 자리) | 전 화면 외곽 | 전 화면 스모크 200, 모바일 사이드바, 역할별 메뉴 노출 테스트 |
| **3 패턴 대표 5개** | 목록(고객), 폼(AS 접수), 상세(AS 상세), 대시보드, 모달(workboard) | 5개 화면 | Figma 대조, baseline 감소 |
| **3b 인터랙션 층** | 사이드 피크 패널, 커맨드 팔레트, 단축키, 토스트 (대표 목록 1~2곳) | 해당 화면 | 직접 URL 폴백 테스트, T7 |
| **3c 결재 UI 키트** | 스테퍼·의견 스레드·승인/반려 패턴을 관리자 전용 `/admin/ui-kit` 샘플로 검증 (결재 기능은 구현하지 않음) | 없음 | UI 키트 스모크 |
| **4 전체 전환** | 사용 빈도 순, 한 PR = 한 도메인 폴더 | 점진 | baseline → 0 |
| **5 JS 정리(선택)** | 인라인 script 분리, fetch→HTMX 전환 검토 | 없음 | 도메인별 PR |
| **(별도) 결재 구현** | `ia-and-approval-plan.md` 순서대로: 기획 § → DB → 구현 | — | 해당 문서 DoD |

## 6. 필수 테스트 (Go 테스트, 폐쇄망·추가 라이브러리 불필요)

- **T1 UI 린트 ratchet** (`ui_lint_test.go`): 템플릿의 원색 유틸·hex·인라인 style·`<style>`·CDN을 파일별로 센다. baseline JSON 대비 **늘면 실패, 줄었는데 baseline 미갱신이어도 실패**. 신규 파일은 0. 최종 목표: 전체 0.
  - 원색 정규식: `\b(bg|text|border|ring|from|to|divide)-(gray|slate|zinc|blue|red|green|amber|yellow|violet|purple|indigo|orange|emerald|teal|sky|rose|pink)-\d{2,3}\b`
- **T2 컴포넌트 클래스 존재** (`static_css_test.go`): 템플릿에서 쓴 컴포넌트·토큰 클래스가 빌드된 `tailwind.css`에 존재.
- **T3 렌더 스모크** (`render_smoke_test.go`): 모든 페이지 템플릿 파싱·렌더·200(권한별: 관리자/일반/옵저버/시점 보기). 옵저버·시점 보기에서 저장 UI 부재. `HX-Request: true`이면 `<html` 없이 content만. 피크 패널 URL 직접 접근 시 풀페이지. 응답에 `color-scheme` 메타 포함.
- **T4 (선택, 개발기)**: Playwright 스크린샷 비교(1280/390). `scripts/visual/`에만 두고 배포 산출물 제외.
- **T5 CSS 최신성**: `scripts/check_css.sh` = `npm run build:css` 후 `git diff --exit-code`. `검수_배포_탭.md`에 단계 추가.
- **T6 토큰 대비 검증** (`token_contrast_test.go`): `input.css`의 `--c-*` 값을 파싱해 WCAG 대비 계산.
  - `text-on-primary` vs `primary` ≥ 4.5:1, `text` vs `surface/canvas` ≥ 7:1, `text-muted` vs `surface` ≥ 4.5:1, 상태색 vs `*-soft` ≥ 4.5:1
  - 목적: 회사색 확정 후 색만 바꿔도 가독성이 자동으로 지켜지게 함
- **T7 커맨드 팔레트 권한**: `/api/palette`가 로그인 사용자의 권한/조직 범위 밖 항목을 반환하지 않음(조직 전환·시점 보기 포함).

## 7. Cursor 규칙 개정안 — `.cursor/rules/figma-ui.mdc`

```
---
description: Figma 화면·디자인 토큰·UI 컴포넌트 작업, 화면 스타일 개선 요청에 적용
alwaysApply: false
---

# Figma → 이 프로젝트 (서버 렌더링 + Tailwind, Linear 풍 스타일, 라이트 전용)

구현은 React/Vue가 아니다. 새 UI 프레임워크·번들러·CDN·웹폰트 CDN 금지. 다크모드 없음(라이트 고정).

## 구조
- 화면: server/web/templates/ (base.html + {{define "content"}}), 부분 템플릿은 `_` 접두
- 공통 UI: server/web/templates/ui/_ui_*.html
- 스타일 소스: server/web/css/input.css (@layer base 토큰 :root, @layer components)
- 토큰: input.css CSS 변수 → tailwind.config.js theme.extend (이름은 Figma Variables와 동일)
- 회사색 교체 지점: input.css의 --c-primary / -hover / -soft / --c-on-primary 4줄뿐
- 인터랙션: Alpine / HTMX RenderPartial / 같은 오리진 fetch JSON. 피크 패널은 HX-Request 분기 + 직접 접근 시 풀페이지 폴백

## 금지 (T1 린트가 막는다)
- 템플릿에 원색 유틸, hex 색, 인라인 style, <style> 신규 추가
- 화면마다 버튼/입력/뱃지 클래스를 새로 조합 → 컴포넌트 클래스 사용
- 스타일 전환 PR에서 로직·권한·쿼리 변경
- 토큰 값을 바꾸고 T6(대비) 확인 생략

## 스타일 원칙
- 저채도 중립색 + 강조색 1개, 1px 경계, 카드 그림자 금지(팝오버만), 목록 행 32/36px, 본문 13px(text-ui)
- 상태 판정·상태→톤 매핑은 서버 공유 함수 한 곳. 부분 템플릿은 권한 판단 금지

## 작업 순서
1. Figma 프레임/컴포넌트를 읽고 토큰·컴포넌트 이름을 매핑표와 대조
2. 토큰/컴포넌트에 없는 값이 필요하면 먼저 토큰·컴포넌트를 추가(임의 값 [#...] 금지)
3. 기존 템플릿을 수정 (별도 프런트 앱 금지). URL은 기획서 §6.3에 따라 유지, 신설 화면만 새 URL
4. 새 클래스 사용 시 cd server && npm run build:css → vendor/tailwind.css 함께 커밋
5. 테스트 필수: go test ./... (T1·T2·T3·T6, 해당 시 T7) 통과 + 전/후 스크린샷

## 기준
- 기획 기준은 기획_업무일지_프로그램_요구사항_v2.md. Figma가 기획과 다르면 기획을 따른다
- 칸반은 §35: 상태 축 없는 목록에 붙이지 않는다
- 측정 불가 지표는 0%가 아니라 "—"
- 화면 구현 사실 기준은 코드·VERSIONS.txt·static_vendor_test.go (화면설계서_요약.md §4의 CDN 서술은 폐기된 내용)
```

## 8. 문서 정리 항목 (Cursor에게 함께 요청)

- `화면설계서_요약.md` §4: "CDN 로드" → "로컬 vendor + Tailwind 빌드본"으로 정정
- `server/README.md` 기술 스택의 "Tailwind CSS (CDN)" → 정정
- `검수_배포_탭.md`에 T5 단계 추가

## 9. Cursor 지시 프롬프트 (Phase 0~1)

```
@agent-implementation @figma-ui
docs의 "UI 업그레이드 설계 및 Cursor 작업지시서 v0.3" Phase 0과 Phase 1만 수행해 줘.
- 템플릿(화면)은 수정하지 말 것. 폰트 외 시각 변화 0. 다크모드는 만들지 말 것(라이트 전용).
- Phase 0: §6 T1(ratchet)·T2·T3 테스트 작성, 현재 위반 수로 baseline 생성.
- Phase 1: §2 토큰을 input.css·tailwind.config.js에 추가(:root에 color-scheme: light 포함),
  §3.1 컴포넌트 클래스 추가, base.html에 <meta name="color-scheme" content="light"> 추가,
  Pretendard Variable woff2를 web/static/fonts에 자체 호스팅(@font-face, OFL 라이선스 고지 포함),
  T6(토큰 대비 테스트) 작성, npm run build:css 후 vendor CSS 커밋.
- 완료 조건: go build/vet/test green, T1·T2·T3·T6 통과, 변경 파일 목록과 baseline 요약 보고.
- 막히면 임의 판단하지 말고 질문할 것.
```

## 10. 사용자 확인 대기 항목

1. **회사색**: 확정 시 알려 주세요 (§2.2 절차로 4줄만 교체). 그 전까지 임시 인디고로 진행.
2. **휴가 신청서 양식**: 업로드 대기 중 → `ia-and-approval-plan.md` §3에서 필드·결재선 확정에 사용.
3. 그 외 질문은 `ia-and-approval-plan.md` §5 참고.
