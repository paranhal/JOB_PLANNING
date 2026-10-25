# 95-53단계 — 로그인 화면 500 (`login_briefing` 템플릿 누락)

기준: `커서_작업지시서_95-53_긴급_로그인화면_500_login_briefing_템플릿누락.md`  
선행: 95-50~95-52. 실행일: 2026-10-26. 배포 이미지 **요청됨**.

## 원인 재현

95-50이 `base.html` 맨 아래에 `{{template "login_briefing" .}}` 를 넣었고, 정의 파일은 `templateFiles()` **공통 목록 끝**에만 있었다. `auth/login.html` 은 앞쪽 3개(`base`, `_nav_tree`, `login.html`)에서 바로 `return` 해서 **정의가 없는 세트**로 `base` 를 실행했다.

수정 전 `TestLoginHTMLHasCacheBustAndBadge`:

```
buildinfo_test.go:243: login status=500
--- FAIL: TestLoginHTMLHasCacheBustAndBadge
```

운영 `/login` 의 `{"message":"Internal Server Error"}` 와 같다. `no such template "login_briefing"` 이 Execute 에서 난다. 95-50 테스트는 로그인 **후** 홈만 봐서 통과했다.

`templateFiles()` 조기 반환은 **로그인 화면만**. 로그인 도움은 같은 `auth/login.html`, 계정 신청 전용 페이지는 없음. `/version`·`/csrf` 는 JSON.

## 수정

- `baseTemplateFiles()` 에 `base.html` · `_nav_tree.html` · `login_briefing.html` 을 모아 `auth/login.html` 포함 모든 화면이 쓴다.
- `base.html` 은 `{{if .LoginBriefing}}{{template "login_briefing" .}}{{end}}` — 데이터 없는 화면은 호출하지 않음.
- `TemplateRenderer.Render` 실행 실패를 `템플릿 실행 실패 name=…` 로 로그. 사용자 JSON 본문은 그대로.
- `_ui_assignee_picker.html` 은 base 가 호출하지 않음(스크립트만). 로그인 세트에 넣지 않음.

## 전수 점검

| 경로 | 결과 |
|------|------|
| GET `/login` | 200, `name="username"` |
| POST `/login/help` | 200, 같은 로그인 폼 |
| GET `/version` | 200 JSON |
| GET `/csrf` | 200 |
| 로그인 → 홈 브리핑 | 기존 95-50 테스트 |
| 로그아웃 → `/login` | 200 |

`TestBaseLayoutTemplateCallsAreDefinedForEveryPage`: 페이지 템플릿마다 `content`·`nav_tree`·`login_briefing` Lookup. `TestOmittingLoginBriefingLeavesTemplateUndefined` 로 파일을 빼면 Lookup 실패.

## 변경 파일

- `server/internal/handler/renderer.go`, `login_page_95_53_test.go`, `render_smoke_test.go`, `login_briefing_test.go`
- `server/web/templates/layout/base.html`
- `VERSION` / `server/VERSION` = `95-53`

## 테스트

수정 전: `TestLoginHTMLHasCacheBustAndBadge` → `login status=500`.  
수정 후: `TestLoginPageRendersFormUnauthenticated`, `TestBaseLayoutTemplateCallsAreDefinedForEveryPage`, `TestOmittingLoginBriefingLeavesTemplateUndefined`, 95-50 브리핑·로그아웃 후 `/login` PASS.

직접 `ParseFiles` 하던 등록·점검 화면 테스트는 95-51 선택기·사이트 피커 파일을 같은 세트에 넣었다. `go vet ./internal/handler` 통과.

`go test ./...`: `work_tasks.org_id 필요` 등 기존 패키지 실패가 다수. 이번 단계 신규 테스트는 그 목록에 없음.
