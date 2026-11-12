# 배포 전 점검: C-Level 경영 통합 대시보드 (95-90)

작성: 2026-11-04. 코드 미수정. 운영 DB·이미지 빌드·push 없음. 이 문서는 커밋하지 않는다.

## 시작 조건

| 항목 | 값 |
|------|-----|
| 브랜치 | `wip/95-정리-백업` |
| HEAD | `582630a` `chore: rebuild tailwind.css (95-113 피커 whitespace-normal 반영)` |
| 최근 6 | `582630a` `a19757c` `e9f4409` `bdebdb9` `d81ec6e` `a4681f6` |
| status | 예외만. `M docs/deploy/docker-compose.yml`, `M docs/리뷰/95-98단계_*.md`, 미추적 `배포이미지_*.md` `배포전점검_*.md` `클로드_전달용_*.md` |

예외 밖 미커밋 없음 → 점검 진행.

기준선: `server/testdata/known_failures.txt` 10건.

## 1. 포함 여부

| 커밋 | `merge-base --is-ancestor HEAD` |
|------|----------------------------------|
| `4629b65` (93 C-Level 메뉴·/exec 골격) | 조상 |
| `54df612` (95-90 조직 카드) | 조상 |
| `b26dbe1` (113D) | 조상 |

| 위치 | HEAD 내용 |
|------|-----------|
| `server/internal/repository/exec_dash.go` 48–55 | `ExecMeetingHref` → `/meeting?` + `view_org_id` + `date` + `from=exec` |
| `server/web/templates/exec/_org_cards.html` 5–44 | 조직 박스 **하나의 `<a href="{{.Href}}">`** |
| `server/internal/handler/nav_exec.go` 122–136 | **`cLevelHiddenPath("/meeting")` 는 여전히 true** (사이드바 숨김 목록 유지, 101–117행 테스트도 그렇게 단언) |
| `server/internal/handler/exec.go` 37–47 | `GuardCLevelHidden` 이 `cLevelHiddenPath` 가 아니라 **`cLevelWriteBlocked`** |
| `server/internal/handler/access_policy.go` 192–202 | GET/HEAD 는 쓰기 화면만 막음 → **GET `/meeting` 허용**, POST는 막음 |
| `server/web/templates/meeting/show.html` 7–8 | `FromExec` 이면 `id="exec-dash-back"` 「← 대시보드로」 `href="/exec"` |

`/meeting` 허용은 hidden 목록에서 빼는 방식이 아니다.

## 2. 자동 테스트

| 명령 | 결과 |
|------|------|
| `go test ./... -run 'Exec\|CLevel\|Meeting\|Nav\|ReadOnly' -count=1` | 패키지 ok (`handler` 31s, `repository` 4s, `model` ok) |
| `go test ./... -count=1` | `handler` FAIL 10건, 그 외 패키지 ok |

실패 10건 = known_failures 와 **동일**:

`TestShowPlanQuarterDisplayHTMLByMonth`, `TestNewTestFilesDoNotCallTimeNow`, `Test95_68_Survey94Templates`, `TestWorkSelfReviewHeaderAverage`, `TestWorkSelfReviewThreeForks`, `TestWorkSelfReviewBlockedBecomesWaiting`, `TestWorkTodayAdminSeesOwnOnly`, `TestWorkTodayAdminAssigneePicker`, `TestWorkTodayDefaultIsListAndKanbanCountsMatch`, `TestPersonalBoardQueryCountBound`.

신규 실패 없음.

## 3. 로컬 실점검 (복사본 DB · :8090)

원본 `server/data/app.db` 를 `tmp/jp-clevel-check/app.db` 에 복사. 점검 계정만 복사본에 추가. 운영 DB 미사용. 점검 후 서버·복사본 삭제.

날짜: **2026-11-04**. C-Level `clchk` 로그인 후 렌더된 HTML·실제 HTTP.

### 3a /exec 렌더 — **통과**

| 확인 | 근거 |
|------|------|
| GET `/exec` | **200** |
| 스텁 문구 | 「집계 화면은 이어서 채웁니다」 **없음** |
| 전사 요약 5칸 | `data-exec-totals`: 전사 오늘 업무 **4** · 완료 **0** · 진행 **0** · 지연 **0** · 오늘 휴가자 **0** |
| 조직 박스 | `data-exec-card` **2**. 활성 조직 O01·O02·O03·(점검용 O99) 중 **인원 0인 O02·O03는 `exec_dash.go` 156–158에서 생략**. 인원 있는 조직 수 = 박스 수 |

### 3b 박스 → /meeting — **통과**

O01 카드는 **단일 `<a>`**, 안쪽 `<a>` 0.

실제 href: `/meeting?date=2026-11-04&from=exec&view_org_id=O01` (키 세 개 모두 있음. 순서만 `url.Values.Encode` 정렬).

GET 그 URL → **200** (403 아님). `id="exec-dash-back"` 「← 대시보드로」 `href="/exec"`. 클릭(GET `/exec`) → **200**, 대시보드 제목 있음.

### 3c 숫자 일치 — **실패**

같은 조직 O01 · 같은 날.

| 항목 | `/exec` 박스 | `/meeting` 화면 |
|------|--------------|-----------------|
| 오늘 업무 | **4** | 예정 **4** (일치) |
| 완료 | **0** | 칸반 「오늘 완료」 **0** (일치) |
| 진행 | **0** | 칸반 「진행중」 **2** (**불일치**) |
| 지연 | **0** | 일일 회의 칸반에 **지연 열 없음** (비교 칸 없음) |
| 휴가자 | **0명** | `/meeting` 일일 화면에 **휴가자 집계 없음** |

원인(코드 읽기, 이번 미수정): `/exec` 오늘 업무 = 회의 PlannedN(오늘 완료∪오늘 예정). 예정일이 오늘이 아닌 진행중 2건은 회의 칸반 「진행중」에만 있다.

### 3d 읽기 전용 — **통과**

C-Level POST `/workboard/tasks`, `/workboard/tasks/WT-NOPE/update`, `/workboard/tasks/WT-NOPE/delete`, `/meeting/weekly/item` → 모두 **403**. GET `/admin/users`, `/admin/orgs`, `/admin/system` → **403**.

### 3e 권한 — **통과**

| 주체 | GET `/exec` |
|------|-------------|
| 조직관리자 `oachk` | **403** |
| 일반 `techchk` | **403** |
| 비로그인 | 로그인으로 이동 (`/login` 200) |
| 비전관리자 `vischk` | **200** |

### 3f 0건 조직 — **통과**

점검용 O99(오늘 업무 0): 완료율 **0%**, 오류 없음. `/meeting?view_org_id=O99&date=2026-11-04&from=exec` 200, 예정 0건.

### 3g 메뉴 — **통과**

사이드바에 「경영 통합」, `href="/exec"` (렌더 HTML에서 확인). `TestCLevelMenuHrefsOrder` 등 Nav 테스트는 2항 필터 실행에 포함되어 통과.

## 4. 94단계 미구현 · 준비 중 문구

KPI 6카드, 억원, 우선 확인 표, 결재·휴가 카드, 경영지원/기획 보조 카드는 **이번 `/exec`에 없음** (범위 외).

| 경로 | 사용자에게 보이는 「준비 중」「이어서 채웁니다」 |
|------|--------------------------------------------------|
| GET `/exec` | **없음** |
| GET `/exec/work` | **없음** (「읽기 전용」「조회만」만 있음) |

`exec/org_soon.html` 의 「지원 조직 대시보드는 준비 중입니다.」 는 지원 조직 `/exec/org/:id` 용. 이번 복사본에 지원 조직 없음 → `/exec`·`/exec/work` 에는 안 나옴.

## 결론

**조건부(O01 진행 0 vs 회의 칸반 진행중 2. 오늘 업무·완료는 일치. GET /meeting·읽기전용·권한·메뉴는 통과. 94 KPI는 없음).**
