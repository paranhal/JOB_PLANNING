# 95-90 C-Level 경영 통합 대시보드(업무)

## 변경 파일
- `server/internal/model/exec_today.go` — 오늘 업무 분류·합집합
- `server/internal/repository/meeting_today.go` — `MeetingTodaySets`
- `server/internal/repository/exec_dash.go` — 조직별 오늘 집계
- `server/internal/handler/exec.go`, `meeting.go`, `handler.go`, `renderer.go`
- `server/cmd/server/main.go` — `GET /exec/cards`
- `server/web/templates/exec/dashboard.html`, `exec/_org_cards.html`, `meeting/show.html`, `layout/base.html`
- `server/web/static/js/exec_dash_nav.js`
- 테스트: `exec_today_test.go`, `exec_dash_test.go`, `exec_95_90_http_test.go`, `exec_dash_nav_test.js`, `95_90_exec_back_browser.mjs`

## 집계 기준
| 항목 | 회의 `/meeting` | 대시보드 |
|------|-----------------|----------|
| 오늘 업무 건수 | `MeetingKanbanScheduledSum` = 오늘 완료 ∪ 오늘 예정 (`PlannedN`) | 동일 집합을 조직 `org_id` 로 나눔 |
| 진행중만 있고 오늘 예정이 아닌 건 | 칸반 「진행중」 열에는 있으나 `PlannedN` 에는 없음 | **넣지 않음** (지시서 3원 합집합과 다름. 회의 요약 숫자에 맞춤) |
| 상태 4칸 | 회의 카드는 예정 합만 표시 | 완료 / 진행 / 미착수 / 지연(95-80). 한 건 한 칸 |
| 조직 | `filterMeetingOrg`: `org_id` 있으면 그 값, 없으면 담당자 | 대시보드는 `org_id` 있는 건만. `org_id` 빈 건은 카드에 안 넣음 |
| 휴가 | `staff_leaves`(승인분) | 동일. 최대 5명 + 외 N명. 라벨은 `LeaveKindLabel`(오전반차, 띄어쓰기 없음) |
| 인원 0 | — | 숨김(Q-1) |
| 조직 범위 | 기존 `currentOrg` | **DataScopeFor 만**. C-Level은 All |

## `/meeting` GET
95-99 유지. `GuardCLevelHidden` 은 `cLevelWriteBlocked`(쓰기만 403). `cLevelHiddenPath("/meeting")` 는 메뉴 숨김용으로 그대로. `cLevelWriteExempt` 미변경. `view_org_id` 없는 GET 도 200.

## 이동·복귀
카드 `<a href="/meeting?…view_org_id=&date=&from=exec">`. `sessionStorage['exec.dash.state']`. `from=exec` 일 때만 「← 대시보드로」. 이전 문서가 `/exec` 이면 `history.back()`.

## 지시서와 다른 점
- Q-5 기본(view_org_id 없는 `/meeting` 403)을 쓰지 않음 — 사용자: 95-99 이후 403 없이 연다.
- 「오늘 업무」는 3원 합집합이 아니라 회의 `PlannedN`.
- 휴가 문구는 기존 `오전반차`/`오후반차`(95-85). 지시서 예시 `오전 반차` 와 띄어쓰기만 다름.
- Figma 담당자 열 회의 화면은 기존 `/meeting` 레이아웃 유지.
- `VERSION` 은 95-106 유지(이후 패치 번호가 더 큼). 운영 배포는 하지 않음.

## 테스트
통과: `TestClassifyExecToday`, `Test95_90_ExecDashTodayCounts`, `Test95_90_ExecDashQueryBound`, `Test95_90_AccessAndMeetingGET`, `Test95_90_RenderAndMeetingTotal`, `Test95_90_StaticAndJS`, `Test95_90_BrowserBackRestoresScroll`(Playwright: 카드→회의→대시보드로/뒤로가기 스크롤), `exec_dash_nav_test.js`, `TestCLevelHiddenGET403`, `TestNavHrefsHaveGETRoutesInMain`. `go build ./cmd/server` 성공. 운영 배포 없음.
