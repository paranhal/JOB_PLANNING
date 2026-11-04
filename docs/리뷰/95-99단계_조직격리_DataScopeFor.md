# 95-99 — 전체 조회 권한 / 조직 격리 (`DataScopeFor`)

작성: 2026-11-03. 지시서 `커서_작업지시서_95-99_…`. Q-1~Q-4 기본값.

## 0단계 (수정 전)

| 파일·함수 | 현재 동작 |
|-----------|-----------|
| `currentOrg` (`org.go`) | 비전·C-Level은 `*` 또는 `view_org_id`. 일반은 소속 조직. **삭제하지 않음.** |
| `canSwitchOrg` | 비전·C-Level만 조직 전환 |
| `isReadOnly` | C-Level·계정 깃발 |
| `OrgScopeIDs` | **없음** (95-77 미구현, `parent_org_id` 컬럼 없음) |
| `GuardCLevelHidden` (`exec.go`, `main.go` 적용) | GET에도 `cLevelHiddenPath` → `/meeting`, `/workboard`, `/work` **403** |
| `cLevelHiddenPath` | 경로 목록 유지. GET에 쓰이지 않게 바꿈 |
| AS `Show`/`Action` | `GetByID(currentOrg)` — C-Level이 조직 선택 중이면 타 조직 상세 404 |
| 첨부 `Download` | 업무·영업만 가드. **AS 첨부는 조직 검사 없음** |
| 고객 검색 | 95-2E 전사 공용 — 이번 격리 대상 아님 |

전수 표(자동 매트릭스 T2): 비전/C-Level은 타 조직 AS 상세·첨부 열람. 조직관리자·영업·기술·지원·옵저버는 타 조직 상세 404(또는 403). C-Level 쓰기 POST 403. 고객 목록은 전사(95-2E).

## 구현

- `access_policy.go`: `DataScopeFor` · `OrgScopeIDs` · `CanViewOrg` · `CanWriteOrg` · `CanViewRecord`. 부문 하위는 컨텍스트 `org_child_ids`(95-77 컬럼 전 시험용).
- `cLevelWriteBlocked`: GET은 `/new` `/edit` `/register` `/form` `/admin`(switch 제외)만 차단. POST 등은 기존처럼 차단.
- AS 조회 `scopeOrgForRead`(전체 조회자면 `*`) + `denyUnlessCanView`.
- AS 첨부 다운로드 조직 검사.
- 레이아웃 「조회 전용」 띠.

기존 `currentOrg`/`canSwitchOrg`/`isReadOnly`/`orgAdminInHome`/`cLevelHiddenPath` 시그니처 유지.

## 테스트

- T1 `Test95_99_T1_*` (단위, DB 없음)
- T2 매트릭스·라우트 누락 감지
- T3 C-Level GET 열림 + POST 403 + 조회 전용 HTML
- T4 기술 목록에 타 조직 0건, 직접 주소 404
- T5 `DataScopeFor` 단일 정의
- T6 `/meeting` 403 아님
- T7 브라우저 스모크: HTTP 렌더로 대체(운영 컨테이너 95-93, 로컬 바이너리 95-98). Playwright S6는 95-90 대시보드와 함께 두는 편이 맞음.

## 보류

- 95-77 마이그레이션(`org_level`/`parent_org_id`)·조직 관리 화면. `OrgScopeIDs`만 자리 잡음.
- 목록 쿼리의 `currentOrg` 전수 교체(지시서: 기존 목록 조건 유지, 없는 상세만 보강).
- 휴가·결재(Q-3).
- 95-90 C-Level 대시보드 화면(이번은 /meeting 403만 해제).
- 95-102 담당자 초기값.

통과 N / 실패 N (기존 K · 신규 M · 해결 R)
