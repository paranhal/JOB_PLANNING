# 커서 작업지시서 — 93단계: 업무 영역 조직 격리 + C-Level 등급 + 조직별 통합 대시보드

> 선행: 92단계(조직별 데이터 격리 점검) 구현 후 진행. 92단계의 원칙(92-A)·정적 감사 테스트(`TestOrgScopeStaticAudit`)를 그대로 쓴다.
> 결정: D-99 (결정로그 참조)

## 0. 용어 정리 (사용자 확정)

| 등급 | 의미 | 비고 |
|---|---|---|
| 비젼관리자 `vision_admin` (vsadmin) | **시스템 관리자**. 사용자·권한·코드·설정·조직 전환 | 기존 그대로 |
| **C-Level `c_level` (신규)** | **회사 대표이사 및 중역**. 전 조직의 업무·실적을 열람하는 경영진 | 시스템 관리 권한 없음 |
| 조직관리자 `org_admin` | 팀장. 자기 조직만 | 기존 그대로 |
| 영업/기술/지원/옵저버/테스터 | 기존 그대로 | |

C-Level은 비젼관리자의 하위/상위가 아니라 **별개의 등급**이다. 시스템 설정·사용자 관리를 못 하고, 대신 전 조직 데이터를 한 화면에서 본다.

## A. 업무 영역 조직 격리 (92단계 추가분)

코드 확인 결과(읽기 기준) 아래 업무 경로는 org 조건이 없다. 92단계의 방식(조직을 repository 인자로 받는다, 타 조직 ID 직접 접근은 404)으로 고친다.

| # | 위치 | 문제 | 조치 |
|---|---|---|---|
| A1 | `repository/wb_repo.go` `ListTasks`, `GetTask`, `ListChildren`, `ListTasksBetween`, `ListTasksDatedBetween`, `ListTasksByDate`, `ListTasksOnDate`, `ListTasksNeedingAutoPlace`, `GetTaskBySource`, `SummaryOnDate` | `work_tasks.org_id` 조건 없음 | 첫 인자로 `orgID` 추가, `AppendOrgSQL`/`appendOrg` 적용. 호출부 전부 수정(컴파일 에러로 누락 방지) |
| A2 | `handler/workboard.go` 156행(일정표), 1306행(`boardData`), 1874행(하위업무) | 위 함수를 org 없이 호출. 상세/수정/삭제/상태변경/이관 POST도 task_id만으로 접근 | `currentOrg(c)` 전달. task_id 기반 핸들러는 조회 후 `VisibleInOrg` 확인, 아니면 404 |
| A3 | `repository/work_board_repo.go` `ListScheduledOn`, `ListCompletedOn`, `ListUnplanned`, `collectByDate`, `collectCompletedOn` | 담당자(userID) 기준만 있고 org 조건 없음 | `orgID` 인자 추가. `CollectUnassigned`가 쓰는 `applyOrgWhere`를 재사용 |
| A4 | `handler/meeting.go`, `meeting_weekly.go` | 위 A3 호출. 담당자 목록 `userRepo.ListAssignable()`이 타 조직 사용자를 포함할 수 있음 | org 전달. 담당자 선택 목록은 `currentOrg` 소속만(전체 보기 시 비젼관리자/C-Level만 전 조직) |
| A5 | 휴가·결재(`staff_leaves`, approvals), 업무 협조(87단계), 알림(`work_assign_notices`), 담당자 지정(`work_task_members`), 첨부 다운로드, 업무 엑셀 내보내기 | 92단계 표에 일부 포함. 아직 미확인분 | 각각 목록 + ID 직접 접근 + POST 3가지를 확인하고 미적용이면 같은 방식으로 수정. **협조(87)는 의도된 조직 간 예외**이므로 allowlist에 사유와 함께 기록(요청자/수신자 조직만 열람) |
| A6 | 자동 배치 `autoPlacePlanned`, `syncMaintenanceTasks` | 조직 구분 없이 전 조직 업무를 일괄 처리할 가능성 | 조직 루프로 처리하거나 `orgID` 인자. 한 조직의 동기화가 타 조직 데이터를 만들거나 바꾸지 않음 |
| A7 | 담당자·사용자 목록(`UserRepo.ListAssignable`, `FindAssignable`) 약 40곳 | 조직 조건 없음 → 일일 업무 등록 화면 등에 타 조직 사용자가 나타남 | **93-1단계 지시서**(`커서_작업지시서_93-1_담당자목록_조직격리.md`)로 처리. 93단계와 함께 진행 |

### A-테스트 (필수)
- `TestOrgScopeStaticAudit` allowlist 갱신: A1~A6 함수가 org 인자 없이 호출되면 실패.
- O01/O02/O03에 각각 업무·회의·휴가·결재 데이터를 시드하고, 각 조직의 조직관리자·일반 사용자로 `/workboard`, `/work`, `/meeting`, `/meeting/weekly`, `/meeting/weekly/me.xlsx`, 업무 상세, 업무 수정 POST, 첨부 다운로드를 호출 → 타 조직 데이터 0건, 타 조직 ID는 404.
- 일반 사용자도 **같은 조직 팀원 자료**는 보이고(89단계 결정), 타 조직은 안 보임.
- 회귀: 기존 `TestOrgCrossNoLeak`·`sales_visibility_test` 통과.

## B. C-Level 등급 신설

### B-1. 권한 원칙 (기본값 — 사용자 확정 전까지 안전 쪽으로)
1. **조직 범위**: 전 조직(O01~O03) 열람. 특정 조직 소속이 아님(`org_id`는 비워두거나 본사 조직 값; 어떤 경우에도 `currentOrg`가 단일 조직으로 고정되지 않음). `repository.OrgAll`로 조회.
2. **읽기 전용**: 모든 업무 데이터는 열람만. 등록/수정/삭제/상태변경 POST는 `isReadOnly`와 같은 경로로 막는다(옵저버의 읽기 전용 구현 재사용).
3. **금액**: 계약금액·매출·원가 등 금액 항목 **열람 가능**(경영진). 단 개인정보성 항목(주민번호 등)은 기존 마스킹 규칙 그대로.
4. **시스템 관리 불가**: 사용자 관리, 권한표, 코드 관리, 조직 관리, 서명/설정, 데이터 가져오기·내보내기(전사 백업) 모두 불가 → 메뉴에 노출하지 않고 URL 직접 접근도 403.
5. **조직 전환**: `canSwitchOrg`에 C-Level 추가 → `view_org_id`로 한 조직만 보는 것도 가능(읽기 전용). 단 기본 진입은 **통합 대시보드**.
6. **결재**: C-Level이 결재선에 지정되면 결재 처리는 가능(지정된 건에 한해). 그 외 업무 편집은 불가. (확정 필요 시 D-99에서 변경)
7. **뷰-애즈**: 비젼관리자의 `sim_role`에 `c_level` 추가(화면 확인용). `sim_org_id`는 무시.

### B-2. 구현 위치
- `model/role.go`: `RoleCLevel = "c_level"`, `NormalizeRole` 별칭(`"C-Level"`, `"c-level"`, `"경영진"`, `"대표"`, `"임원"`), `IsKnownRole`, `RoleLabel` → `"C-Level(경영진)"`, `AdminGrade`는 `"exec"`, `JobRole`은 `""`, `RoleFromAdminGrade`에 `"exec"`/`c_level` 추가. `IsAdminGrade`에는 **넣지 않는다**(관리자 기능 혼입 방지).
- `DefaultPermissions(RoleCLevel)`: 모든 조회 권한 true, 쓰기/관리 권한 false. `role_permissions` 시드 + 권한표 테스트(role_matrix_test 패턴)에 행 추가.
- `handler/org.go`: `canSwitchOrg`, `currentOrg` — C-Level은 `view_org_id` 없으면 `OrgAll`. `handler/rbac.go`: `isReadOnly` 포함, `hasPerm` 반영.
- 사용자 관리 화면(`user-admin`): 등급 선택에 "C-Level(경영진)" 추가. **지정은 비젼관리자만** 가능. 조직관리자는 C-Level 지정·수정·삭제 불가(자기 조직 사용자 목록에도 C-Level이 보이지 않게).
- `nav_menu.go`: C-Level 메뉴는 읽기 전용 항목만(통합 대시보드, 사업관리, 영업 파이프라인, AS·점검 현황, 주간 업무 회의 팀 자료 등). 설정/관리 그룹 숨김.
- DB: `users.role` 값만 추가되는 경우 마이그레이션 불필요. CHECK 제약이 있으면 별도 커밋으로 마이그레이션.

### B-테스트
- 권한표 테스트: C-Level 행(조회 전부 true, 쓰기·관리 false).
- HTTP: C-Level로 GET 목록은 200 + 3개 조직 데이터 모두 포함, 모든 변경성 POST/PUT/DELETE는 403, `/admin/*`·사용자 관리·권한표는 403.
- 조직관리자로 사용자 목록·수정 API 호출 시 C-Level 계정이 목록에 없고 수정은 404/403.
- 비젼관리자 `sim_role=c_level` 확인.

## C. 경영 통합 대시보드·업무 관리 화면

**94단계 지시서로 이동했다** (`커서_작업지시서_94단계_경영통합대시보드_업무관리.md`). 93단계는 A(업무 격리)와 B(C-Level 등급)까지만 구현하고, C는 94단계에서 구현한다.

## D. 순서와 완료 조건
1. A(업무 격리) 먼저 → 2. B(등급). 대시보드는 94단계.
2. 각 단계 `go test ./...` 신규 실패 0(기준 `docs/리뷰/gotest-baseline-72A2.md`, `TestPolicyCRequiresDeputy` 별도 보고), `npm run build:css` 같은 커밋, `ui_lint` 0, 마이그레이션 별도 커밋.
3. 운영/개발 데이터 변경 금지. 브라우저 확인용 TEST 데이터는 확인 후 원복. `server/data` 커밋 금지.
4. 완료 보고: A 항목별 수정 여부 표, 새 테스트 목록, C-Level 권한 체크 결과, `/exec` 스크린샷.

## E. 확정 필요(기본값으로 진행, 변경 시 알려주세요)
- C-Level의 결재 처리 허용 범위(기본: 결재선 지정 건만).
- C-Level이 업무 코멘트/지시 남기기(읽기 전용 예외) 허용 여부(기본: 불가).
- 대시보드 지표 행 추가·삭제(기본: C-1 구성).
