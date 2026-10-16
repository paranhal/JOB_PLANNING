# go test 실패 목록 기준 (72-A2 마무리)

기준일: 2026-10-16. `go test ./... -count=1` (server/). 이후 비교는 **개수가 아니라 아래 이름**으로 한다.

72-A 시작 전 로그(`server/gotest-58.log`) 실패 **33개**. 72-A2 필터 수정 후 **16개**.

## 현재 실패 16개 (이후 비교 기준)

패키지 `internal/handler` (13)

1. `TestAdminWorkAssetLinkAndASMoveHTTP`
2. `TestASActionCauseCatsReplaceCauseTypeSelect`
3. `TestASVisitDateCreatesDailyTaskAndRegisterPalette`
4. `TestProcessConflictsExcelAndAdminTab`
5. `TestMainListensBeforeHousekeeping`
6. `TestDeliveriesHTTP_GoodsSplitSerialConsumableSkippedUnplanned`
7. `TestItemsHTTP_ManufacturerSearchAndPartner`
8. `TestLoginHelpHandleCompletesTask`
9. `TestMeetingHTTP_YesterdayDoneTodayScheduled`
10. `TestQuotesHTTP_FormB2GroupLaborRevisePurposeReverse`
11. `TestQuotesHTTP_RecipientSearchAndInlineNewSales`
12. `TestSalesHTTP_SupplyDealTypeListAndAutoStage`
13. `TestUILintRatchet`

패키지 `internal/repository` (2)

14. `TestNoCompleteDatetimeUpdatedAtFallback`
15. `TestListWorkToday_AppDBTimingAndCount`

패키지 `internal/service/leaveapp` (1)

16. `TestPolicyCRequiresDeputy` — `SQLITE_BUSY`. 72-A 시작 전 33개 목록에는 없음. **불안정**.

## 33개 중 지금은 안 뜨는 18개

이름 집합 차이는 18개다(33−현재 16 중 33에 있던 15개=18, 그리고 16번에 `TestPolicyCRequiresDeputy`가 새로 들어감). 오늘 이 18개를 따로 돌리면 **모두 통과**. gotest-58 당시 메시지는  pleaky HTTP가 아니라 단언·SQL·CSS 실패였다. **수정된 테스트**로 본다.

| 테스트 | 당시 실패 | 구분 |
|--------|-----------|------|
| `TestASUnassignedStaysInUnplannedNotDaily` | 미배정 건이 일일 화면에 보임 | 수정 |
| `TestSalesHTTP_AdminDeleteReasonLogAndPromotedBlock` | 삭제 status=303 | 수정 |
| `TestSalesHTTP_ChangeHistoryAndPartyReplace` | 금액 이전 값이 활동 로그에 없음 | 수정 |
| `TestComponentClassesExistInBuiltCSS` | `.btn-primary` 등 빌드 CSS 없음 | 수정 |
| `TestMissingCompleteDateBanner` | `/work-status` 500 | 수정 |
| `TestTokenContrastWCAG` | 대비 비율 단언 실패 | 수정 |
| `TestWorkStatusPlannedRedirectsToAction` | 조치 탭에 완료 점검 없음 | 수정 |
| `TestWorkStatusPaletteShowsCounts` | 단위 업무별 현황 제목 없음 | 수정 |
| `TestWorkStatusHTTP_TimelineSmoke` | 500 | 수정 |
| `TestWorkStatusMonthDefaultAndDayLink` | 500 | 수정 |
| `TestWorkStatusMissingBanners` | 완료일 미기록 배너 없음 | 수정 |
| `TestWorkStatusTechScopeToggle` | mine 500 | 수정 |
| `TestWorkStatusHTTP_SalesMonthBanners` | 500 | 수정 |
| `TestOccurrenceOverdueOnShowAndNextKept` | 과거=scheduled | 수정 |
| `TestSalesAmountChangeAndPartyReplaceKeepOld` | 금액 변경 이력 없음 | 수정 |
| `TestGenerateOccurrencesCopiesParentAndDates` | FOREIGN KEY | 수정 |
| `TestMarkPastOccurrencesOverdueLeavesFuture` | SQL `near ")"` | 수정 |
| `TestMeetingAndAS_AppDBTiming` | `sql: no rows` | 수정 |

남아 있는 HTTP 12개(1~12)와 `TestUILintRatchet`(베이스라인 파일 없음), `TestNoCompleteDatetimeUpdatedAtFallback`, `TestListWorkToday_AppDBTimingAndCount`는 이번 72-A2에서 손대지 않았다.
