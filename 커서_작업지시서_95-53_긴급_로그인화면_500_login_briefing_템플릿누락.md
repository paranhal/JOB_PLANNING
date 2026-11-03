# 커서 작업지시서 95-53 (긴급) — 운영 `/login` 이 `{"message":"Internal Server Error"}` (95-50 이후 로그인 화면 템플릿 누락)

> 작성: 2026-10-26 / 대상 버전 v95-50~v95-52 / **테스트 필수**, `git add .` 금지.
> **이번 건은 운영 로그인이 막힌 장애라서 배포 이미지(`deploy/server-app.tar`)까지 만들어 달라고 요청한다(사용자가 즉시 배포).** 이미지 빌드 전 `npm run build:css` 최신 여부 확인, 이미지에 포함되는 `VERSION` 은 `95-53`.
> 사용자 보고: 「업데이트 후에 lib.visionit.co.kr/login 에 `{"message":"Internal Server Error"}`」 — 로그인 화면 자체가 열리지 않음.

## 1. 원인 (코드 확인 — 재현 테스트로 확정할 것)
1. 95-50(`ff0fa58`)에서 `server/web/templates/layout/base.html` 맨 아래에 `{{template "login_briefing" .}}` 가 추가되었고, 정의(`{{define "login_briefing"}}`)는 `web/templates/_partials/login_briefing.html` 에 있다. 이 파일은 `renderer.go` `templateFiles()` 의 **공통 목록 끝**(`appendExisting(... "_partials/login_briefing.html")`, 약 142행)에만 추가되었다.
2. 그런데 `templateFiles()` 는 **`auth/login.html` 이면 앞쪽(`base.html`, `_nav_tree.html`, `login.html` 3개)에서 곧바로 `return files`**(47~49행) 한다. 즉 **로그인 페이지는 `login_briefing` 정의가 없는 템플릿 세트**로 만들어지고, `base.html` 이 그 템플릿을 호출하는 순간 실행 오류(`no such template "login_briefing"`)가 나 Echo 가 500 JSON 을 돌려준다.
3. 95-50 테스트는 로그인 **후** 화면만 확인했고, `/login` 화면 렌더링 테스트가 없어 통과했다(로컬 개발 서버도 로그인 상태 세션이 남아 있으면 발견하기 어렵다).
4. 같은 구조(`login.html` 조기 반환)가 다른 로그인 전 화면(예: 로그인 도움, 계정 신청 등 `base.html` 을 쓰는데 조기 반환하는 템플릿)에도 있는지 전수 점검한다.

## 2. 수정
1. **근본 수정**: `base.html` 이 호출하는 **모든 하위 템플릿**(`nav_tree`, `login_briefing`, 이후 추가될 것)이 **어떤 화면의 템플릿 세트에도 항상 포함**되도록 한다. 구체적으로 `templateFiles()` 의 `auth/login.html` 조기 반환 목록에 `web/templates/_partials/login_briefing.html` 을 추가한다(로그인 화면은 `.LoginBriefing` 이 없으므로 `{{if .LoginBriefing}}` 에서 빈 출력).
2. **구조 개선(재발 방지, 권장)**: `base.html` 에서 `{{template "login_briefing" .}}` 를 직접 호출하지 말고 `{{if .LoginBriefing}}{{template "login_briefing" .}}{{end}}` 로 감싸 **데이터가 없는 화면에서는 호출 자체가 일어나지 않게** 하고, 가능하면 base 가 의존하는 공통 조각 목록을 **한 곳(`baseTemplateFiles()` 같은 상수)** 에 모아 `auth/login.html` 포함 모든 이름이 같은 목록을 쓰게 한다(조기 반환 분기에서 누락되지 않도록). 95-51 에서 추가된 `_ui_assignee_picker.html` 도 base 가 호출하는지 확인(현재는 base 에서 호출하지 않고 `<script>` 만 추가되어 영향 없음 — 호출한다면 같은 방식으로 포함).
3. 로그인 화면 외 **로그인 전에 접근 가능한 모든 경로**(`/login`, 로그인 도움 요청, 계정 신청 화면, `/version` 등) 응답 코드와 본문 확인.
4. (선택) 템플릿 실행 오류 시 `{"message":"Internal Server Error"}` 대신 로그에 **원인(에러 메시지)** 이 남도록 렌더러의 에러 로그를 확인한다(현재 원인을 운영 로그에서 바로 보기 어려움). 사용자에게 보이는 화면은 바뀌지 않아도 된다.

## 3. 테스트 (필수)
1. **재현 테스트(먼저 실패해야 함)**: 실제 `TemplateRenderer` 로 `GET /login`(비로그인) → 200, 본문에 로그인 폼(`name="username"` 등) 포함, `no such template` 없음. 수정 전 코드에서 500 이 되는지 확인 후 수정(리뷰 문서에 로그 첨부).
2. **전수 렌더 스모크**: `render_smoke_test.go`(이미 있음, 138행에서 `auth/login.html` 은 `content` 검사 예외)를 확장해 **`web/templates` 의 모든 페이지 템플릿 이름 × `templateFiles(name)`** 으로 템플릿 세트를 만들고 `base.html` 이 호출하는 모든 `{{template "x"}}` 가 해당 세트에 **정의되어 있는지**(`Lookup`) 검사. `auth/login.html` 을 예외 없이 포함. 앞으로 base 에 호출이 추가되면 이 테스트가 즉시 실패해야 한다(샘플 위반으로 실패 확인).
3. 로그인 성공 흐름 회귀: 로그인 → 홈에 95-50 브리핑 팝업(직원/조직관리자) 정상, 로그아웃 → `/login` 200.
4. 기존 95-50 테스트(`Briefing`), `AssignNoticeModal`, 95-51·95-52 관련 테스트, `go vet`, `go test ./...` 신규 실패 0.

## 4. 산출물 / 배포
- 코드·테스트, 리뷰 문서 `docs/리뷰/95-53단계_로그인500_login_briefing.md`(원인 재현 로그, 변경 파일, 전수 점검 결과, 테스트 결과)
- 커밋 메시지 `95-53: 로그인 화면 500 수정 — login.html 템플릿 세트에 login_briefing 포함, base 호출 전수 렌더 테스트`
- **배포 이미지 생성**(`deploy/server-app.tar`). 이번 이미지에는 95-50~95-53 이 모두 들어간다.

## 5. 참고 (사용자 안내용, 구현 대상 아님)
- 마이그레이션 091(담당자 선택기)·092(영업 대분류)는 이미 운영 DB 에 적용되었을 수 있다. 두 마이그레이션은 컬럼·테이블 **추가**이므로 이전 이미지로 되돌려도 DB 는 호환될 가능성이 높지만, 되돌리기 전 DB 백업은 필수.
