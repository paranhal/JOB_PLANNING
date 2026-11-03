# 95-25 공용 담당자 선택기 `AssigneePicker` — 체크박스 복수 선택 + 업무 그룹 선택을 모든 업무에 적용, AS 처리담당자 복수

작성: 2026-10-24 · 결정 D-167
선행: 95-14A(정기점검 복수 담당 컴포넌트) 완료분을 **일반화**한다. 95-22(CustomerLookup)와 같은 방식(규칙·가드 테스트·Cursor rule)을 따른다.
마이그레이션: 084 (아래 §4). 모든 담당자 지정 입력은 이 지시서 이후 **이 컴포넌트·함수 하나만** 쓴다.

## 1. 요구 (사용자)

1. AS 처리를 **그룹**으로 하는 경우가 있어 **처리담당자를 복수 선택**할 수 있어야 한다.
2. 「체크박스로 여러 명 선택」과 「업무 그룹 선택(그룹 체크 → 구성원 자동 선택)」을 **모든 업무**에 적용한다.
3. 담당자 선정 기능이 있는 **모든 곳**이 같은 기능을 쓰고, **함수·클래스로 만들어 같은 로직을 계속 호출**한다(화면마다 따로 만들지 않는다). 새 화면이 생겨도 이것만 쓴다.

## 2. 현황 (코드 확인 완료)

- 이미 있는 것: 정기점검 전용 `web/templates/maintenance/_assignee_multi.html` + Alpine `mntAssigneeMulti` + `handler/mnt_assignee_form.go`(`workGroupPicksForVisit`, `mergeVisitAssigneeIDs`, `formVisitGroupIDs`) + 테이블 `maintenance_visit_assignees`(082). → **이것이 원형**이다. 이름·위치만 일반화한다.
- 담당자 입력이 아직 **단일 `<select>`** 인 곳(전수): `workboard/_modals.html`(AS·점검·행정/지원·작업 4곳), `workboard/new_work_form.html`(95-20A 새 모달 폼 조각), `workboard/task_show.html:601`, `workboard/_subtask_modal.html`, `admin_work/form.html`, `admin_work/_subtask_modal.html`, `work/list.html:137`(재배정), `plan/unplanned.html:68`, `meeting/_item_edit.html`, `as/show.html:451`(배정), `as/form.html:181`(접수), `as/action.html:245`(**처리담당자**), `as/work_action.html:63`, `workboard/project_work_action.html:100`, `maintenance/plan_manage.html:190,281`, `maintenance/visit_action.html:46`, `layout/base.html:312·333·366`(헤더 빠른 등록 — 용도 확인 후 처리). 핸들러 쪽은 `assigned_to_code`/`assigned_to_custom`(as.go, as_support.go, as_work_action.go, workboard_project_work_action.go)·`assignee` 폼 값을 각각 파싱.
- 목록 필터용 `select name="assignee"`(work/list, meeting/show·weekly, work_status/timeline, register 상단 등)는 **지정 입력이 아니라 조회 필터**다 → 이번에는 UI 를 바꾸지 않되 **사용자 목록 공급은 §3-4 의 같은 함수**를 쓴다.
- 담당자 목록 공급은 `listAssignableForPicker`/`assignableOrgID`(93-1, 조직 격리)로 이미 하나다. 이것도 새 함수 안으로 흡수한다.
- 데이터: AS `as_receipts.assigned_to/assigned_user_id` 단일, `as_work_items.assigned_to/assigned_user_id` 단일, `work_tasks.assignee/assignee_user_id` 단일 + `work_task_members(task_id, assignee(이름), member_role owner|support)`(이름 기준, user_id 없음), 점검 `maintenance_visit_assignees`(user_id).

## 3. 설계

### 3-1 Go: 한 곳의 함수/타입 (`server/internal/handler/assignee_picker.go`, 필요하면 `internal/assignee` 패키지로 분리)

```go
type PickerMode string // PickerMulti | PickerSingle

type PickerSpec struct {
    Field         string     // 폼 필드 접두어(기본 "assignee")
    Mode          PickerMode
    Label         string
    Required      bool
    AllowGroups   bool       // 업무 그룹 섹션(기본 true)
    AllowExternal bool       // 협력사·직접입력(AS 처리담당자 등, 이름만 저장)
    AllowCrossOrg bool       // 타팀 지원자 섹션(95-26에서 켬. 이 단계는 false)
    Selected      Selection  // 수정 화면의 현재값
}

type Selection struct {
    PrimaryUserID string
    UserIDs       []string   // 주담당 포함, 중복 제거
    GroupIDs      []string
    ExcludeIDs    []string   // 그룹 구성원 중 제외한 사람
    External      string     // 직접입력 이름(AllowExternal)
}

func (h *Handlers) BuildPicker(c echo.Context, spec PickerSpec) PickerView // 템플릿 데이터(사용자·그룹 JSON 포함)
func ParseSelection(c echo.Context, deps PickerDeps, spec PickerSpec) (Selection, error) // 서버 검증 포함
func (s Selection) Primary() (userID string) // 주담당 규칙: 그룹장 우선 → owner_user_id → 첫 체크
```

- **서버 검증은 `ParseSelection` 한 곳**: 각 user_id 는 `assignableOrgID(c)` 의 활성 사용자여야 함(타 조직·비활성·협력사·읽기전용·C-Level 400, 95-26 전까지 예외 없음), 그룹은 `ListVisibleActiveForWork` 가시 범위, 그룹 구성원 펼침(`mergeVisitAssigneeIDs` 이동·재사용), 제외 반영, `Required` 이고 비면 400 + 폼 오류 팝업(95-12 방식).
- 저장 헬퍼도 하나: `SaveAssignees(target AssigneeTarget, sel Selection, actor)` — `AssigneeTarget{Kind: as_receipt|as_work_item|work_task|maintenance_visit, ID}`. 종류별 테이블에 (user_id, assignee_name, role owner|support, work_group_id, sort_order) 행을 **교체 저장(트랜잭션)**하고, 기존 단일 컬럼(`assigned_to`, `assigned_user_id` / `assignee`, `assignee_user_id`)은 **주담당 미러**로 항상 같이 갱신(기존 화면·통계 호환, 95-14 와 동일 방식). 변경 이력에 `담당자 변경: 이전 목록 → 새 목록` 한 줄(기존 이력 기능 재사용).
- 조회 헬퍼도 하나: `AssigneesOf(kind, ids...)` 일괄 조회(N+1 금지, 95-18 기준), `SQLAssignedToUser(kind, alias, keys)` — **「이 사용자가 담당자(주·보조 모두)인가」를 SQL 로 판정하는 단일 조각**. 개인 화면·`ListWorkToday`·대시보드·회의·AS 권한 판정(D-74 「배정 담당자」)이 모두 이 조각을 쓴다(각자 `assigned_to = ?` 비교 금지).

### 3-2 템플릿·JS (한 번만)

- `web/templates/_ui_assignee_picker.html` 에 `{{template "assignee_picker" (picker …)}}` — 정기점검 `_assignee_multi.html` 을 이동·일반화(마크업·접근성 유지: `role`, 키보드, Esc, 검색, 그룹 `구성원` 펼침, 요약 라벨 `김현우 외 2명 · 그룹 A`).
- 정적 JS `web/static/js/assignee_picker.js` 의 Alpine `assigneePicker({mode, field})` (기존 `mntAssigneeMulti` 는 이 파일로 옮기고 한 단계 동안 별칭만 남긴 뒤 제거).
- 폼 필드 이름 표준: `assignee_user_id`(반복), `assignee_group_id`(반복), `assignee_exclude_user_id`(반복), `owner_user_id`(주담당), `assignee_external`, `assignee_multi=1`. `Mode=single` 은 같은 컴포넌트에서 체크 하나만 허용(라디오처럼 동작)하고 같은 필드명으로 제출.
- `AllowExternal` 이면 목록 끝에 「직접입력(협력사·외부)」 입력칸(AS 처리담당자의 기존 `custom` 동작 유지, 이름만 저장).
- 모달 안·HTMX 조각 안에서도 동작(주입 후 `Alpine.initTree`, 95-20C 의 바인딩 중복 방지 방식 참고). 모달 `overflow` 에 잘리지 않게 팝오버는 body 위 레이어 또는 `position:fixed`.

### 3-3 AS 처리담당자 복수 (요구 1)

- 접수(`as/form.html`), 배정(`as/show.html`), **조치(`as/action.html` 처리담당자)**, 하부업무(`as/work_action.html`, `as_support.go`) 모두 picker(`Multi`, `AllowExternal`).
- 조치 화면 헤더·목록·상세·엑셀·보고서의 「담당자」 표기: `주담당 외 N명`(툴팁에 전체), 이력에 전원 기록.
- **통계의 「담당자별 건수」는 주담당 1명 기준 유지**(중복 집계 방지, 임시 가정 — 사용자 확인 시 변경). 개인 화면·내 업무·알림·권한은 **보조 담당 포함**.
- AS 완료·수정 권한(D-65/D-74): 담당자 N명 모두 「배정 담당자」로 취급.

### 3-4 목록 필터·보조 화면

- 필터용 담당자 `<select>` 는 UI 유지, 옵션 공급은 `assigneeFilterOptions(c)` 한 함수(내부는 `BuildPicker` 와 같은 조직 규칙)로 통일. 주담당뿐 아니라 보조 담당으로도 검색되도록 필터 SQL 은 `SQLAssignedToUser` 사용(기존 단일 비교 교체).

## 4. 마이그레이션 084 (데이터는 코드로 채우지 않음, 백필은 무손실 규칙만)

- `as_receipt_assignees(as_id, user_id, assignee_name, role CHECK owner|support, work_group_id NULL, sort_order, created_at)` + 유니크(as_id, user_id WHERE user_id≠''), 인덱스(user_id).
- `as_work_item_assignees` 동일 구조(`work_id`).
- `work_task_members` 에 `user_id TEXT NOT NULL DEFAULT ''`, `work_group_id TEXT NULL` 컬럼 추가(기존 이름 기준 PK 유지).
- 백필: 기존 단일 담당자 → 각 테이블에 owner 1행 복제(**user_id 가 있을 때만 user_id 로, 이름만 있으면 이름 행**). 이름→user_id 추정 매칭 금지. `.down.sql` 포함, `AppSchemaVersion` 갱신 별도 커밋.

## 5. 전수 치환 목록 (보고서에 체크표로)

§2 의 모든 지정 화면을 picker 로 교체하고 핸들러는 `ParseSelection`+`SaveAssignees` 로 통일한다. 새 모달(95-20A `new_work_form`)·예전 모달(`_modals.html`)·AS·점검·작업·행정/지원·영업·하부업무·재배정·미계획 업무함 날짜 지정·회의 항목 수정·사업 업무 조치. 각 행에 `기존 필드 → 새 필드`, `핸들러`, `테스트` 를 적는다.
단일 담당만 의미 있는 곳(예: 미계획 업무함 「담당자 1명 지정」 등)은 `Mode=single` 로 같은 컴포넌트 사용 — **다른 구현 금지**.

## 6. 재발 방지 (95-22 방식)

1. `docs/규칙/담당자-선택기.md`, `.cursor/rules/assignee-picker.mdc`: 「담당자 지정 입력은 `assignee_picker` 만, 서버는 `ParseSelection`/`SaveAssignees` 만」.
2. 가드 테스트: 템플릿에서 지정 폼의 `<select name="assignee"`·`assigned_to_code`·`assignee_name` 직접 입력이 나오면 실패(필터용은 허용 목록 파일에 사유와 함께 등록), 핸들러에서 `ListAssignable(`·`listAssignableForPicker(` 직접 호출 및 `c.FormValue("assignee")` 로 담당자를 저장하는 코드가 `assignee_picker.go` 밖에 있으면 실패(허용 목록 방식).
3. `ui_lint` 규칙 추가. 새 지시서에는 항상 이 컴포넌트를 인용.

## 7. 테스트 (필수)

1. `ParseSelection`: 단일/복수, 그룹 펼침(그룹장 주담당), 그룹 구성원 일부 제외, 중복 제거, 타 조직·비활성·협력사·읽기전용·C-Level 사용자 400, 비가시 그룹 400, 필수 누락 400, 직접입력(AllowExternal), 구 폼 값(`assignee`, `assigned_to_code`) 입력 시 호환 처리 또는 명확한 400(선택하고 보고서에 명시).
2. `SaveAssignees` 4종 대상 각각: 행 교체·주담당 미러 컬럼·이력 한 줄·트랜잭션(일부 실패 시 롤백).
3. AS 시나리오: 접수 시 2명+그룹 1개 → 행 수·주담당·`주담당 외 N명` 표기 / 조치 화면에서 처리담당자 3명 저장·재오픈 시 복원 / 협력사 직접입력 + 직원 혼합.
4. 모든 소비자가 보조 담당자도 포함: 업무 관리 개인 화면(95-20/23), `ListWorkToday`, 대시보드 버킷, 일일·주간 회의, 업무 처리 현황, 엑셀 담당자 열, 알림, 권한(D-74 수정 가능), 필터 `assignee=` 검색. (보조 담당자 사용자로 로그인해 각 화면에 건이 보임을 단언.)
5. 통계 담당자별 집계는 주담당 1명 기준 유지(중복 집계 없음).
6. 템플릿 렌더 테스트: §5 의 모든 화면에 picker 마크업이 있고 구 `<select>` 지정 입력이 없음. 모달/조각 주입 후 동작(Playwright 스모크 또는 DOM 테스트): 열기·검색·그룹 체크→구성원 자동·구성원 해제·요약 라벨·키보드·Esc.
7. 조직 격리: O03 사용자가 O01 사용자 ID 를 폼에 넣어 제출 → 400, 그룹도 동일. 로그인 조직별 목록이 다름(O01/O03).
8. 마이그레이션 084 up/down, 백필(user_id 있는 단일 담당 → owner 1행, 이름만 → 이름 행, 추정 매칭 없음), 기존 데이터 화면 동작 불변.
9. 가드 테스트가 실제로 위반을 잡는지(샘플 위반 템플릿/핸들러로 실패 확인).
10. 회귀: 95-14A 정기점검 복수 담당(`mnt_95_14a_test.go`) 통과, 95-20A/20C 모달, 94-8·93-1 담당자 격리 테스트 통과.

공통: `go test ./...` 신규 실패 0(baseline 이름 대조), `npm run build:css` 같은 커밋, `ui_lint` 0.

## 8. 기존 기능 연동 점검표 (보고서에 채워 제출)

양식은 95-23 §0 과 같다. 최소 행: 개인 화면(95-20/23), 일일 업무 등록(예전), 업무 확인(`/work`, `/work/all`), 미계획 업무함(95-18), 정기점검(95-14/14A/16), AS 조치(95-8/95-11), 작업 조치(95-11A), 업무 그룹(95-13), 협조(87), 대시보드, 회의(94-3/94-4), 엑셀(94-5/94-6), 알림, 통계, 권한(D-65/D-74), **CustomerLookup(95-22) 와 같은 폼 안에서 같이 쓰일 때의 동작**.

## 9. 단계·커밋

① picker 컴포넌트·Go 함수 신설 + 정기점검을 새 컴포넌트로 이전(동작 동일, 테스트 통과) ② 마이그레이션 084 + `SaveAssignees`/`AssigneesOf`/`SQLAssignedToUser` ③ AS 4개 화면 복수화 ④ 업무(work_tasks) 계열 전수 치환(새 모달·예전 모달·상세·하부·재배정·사업 업무) ⑤ 필터 공급 통일·소비자(개인 화면·대시보드·회의) 보조 담당 반영 ⑥ 가드·규칙 문서. 단계마다 `go test` 확인, `git add .` 금지. 보고서 `docs/리뷰/95-25단계_공용담당자선택기.md`(치환 체크표, 소비자 점검, 테스트 대조표, 커밋 해시). 배포는 사용자가 한다(DB 백업 후).

## 10. 범위 밖

- 타 조직 지원자 지정(95-26, `AllowCrossOrg`). 이 단계에서는 항상 false.
- 담당자별 부하·투입시간 계산, 담당자 변경 알림 개편.
