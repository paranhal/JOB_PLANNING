# 94-2단계 보고서 — 사용자 관리 초기화·C-Level 역할 (D-107)

기준: `커서_작업지시서_94-2_사용자관리_초기화_C-Level역할.md`

## 원인

- 「＋ 아이디 생성」은 템플릿에 이미 있었다. `CanCreateUser`/`UserCreate`가 `currentRole`(보기 전환 `sim_role`)을 봐서, 비젼관리자가 다른 등급으로 보는 중이면 버튼이 사라지고 생성도 403이 됐다.
- `/users/:id/reset-permissions`는 권한 초기화다. 비밀번호 초기화 경로가 없었다.
- 역할 및 권한 등급 탭 열이 `vision_admin / org_admin / support`로 고정돼 C-Level이 없었다.

## 동작

- 생성·수정·비밀번호 변경·초기화 권한은 `loginRole` 기준. 보기 전환 중에도 비젼관리자 쓰기(`/users*`)는 `GuardViewAsWrite`에서 허용.
- `POST /users/:id/reset-password`: 12자 임시 비밀번호, 응답 화면에 1회 표시, `must_change_password=1`. 본인·마지막 비젼관리자 초기화 거부. 조직관리자는 자기 조직만, C-Level/비젼관리자는 403.
- 직접 변경에 「다음 로그인 때 변경하게 하기」 체크. 해시만 바꿀 때는 `must_change_password`를 건드리지 않는다.
- 역할 화면 등급 탭: 비젼관리자 / C-Level(경영진) / 조직관리자 / 일반. C-Level은 없음·조회만, 관리 권한은 없음 고정. 서버에서도 `ClampCLevelAccess`.
- 세션 무효화: **미적용**(세션 버전 장치 없음).

## 테스트

`go test` `users_94_2_http_test.go`, `TestCLevelAccessNeverAboveView`.
