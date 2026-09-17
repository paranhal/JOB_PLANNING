package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestEnsureTimelineDisplayTimesFillsMissing(t *testing.T) {
	tasks := []model.WorkTask{
		{TaskID: "a", WorkDate: "2026-08-11", StartTime: "10:00", EndTime: "10:30", DurationMin: 30, Title: "scheduled"},
		{TaskID: "b", DueDate: "2026-08-11", Title: "due-only", DurationMin: 30},
		{TaskID: "c", WorkDate: "2026-08-11", Title: "date-only", DurationMin: 30},
	}
	got := ensureTimelineDisplayTimes(tasks)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	for _, tsk := range got {
		if tsk.WorkDate != "2026-08-11" {
			t.Fatalf("work_date=%q id=%s", tsk.WorkDate, tsk.TaskID)
		}
		if tsk.StartTime == "" || tsk.EndTime == "" {
			t.Fatalf("missing time: %+v", tsk)
		}
	}
	if got[0].StartTime != "10:00" {
		t.Fatalf("kept start=%s", got[0].StartTime)
	}
	if got[1].StartTime == "10:00" || got[2].StartTime == "10:00" {
		t.Fatalf("display slots should avoid occupied 10:00: %s %s", got[1].StartTime, got[2].StartTime)
	}
}

func TestIsCompletedWorkTask(t *testing.T) {
	asOK := map[string]string{"a1": "completed", "a2": "partial_complete", "a3": "in_progress"}
	mnt := map[string]bool{"v1": true, "v2": false}

	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a1"}, asOK, mnt) {
		t.Fatal("AS completed")
	}
	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a2"}, asOK, mnt) {
		t.Fatal("AS partial")
	}
	if isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a3"}, asOK, mnt) {
		t.Fatal("AS open should exclude")
	}
	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceMaintenance, SourceID: "v1"}, asOK, mnt) {
		t.Fatal("mnt done")
	}
	if isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceMaintenance, SourceID: "v2"}, asOK, mnt) {
		t.Fatal("mnt open")
	}
	if !isCompletedWorkTask(model.WorkTask{Status: model.WBTaskComplete}, asOK, mnt) {
		t.Fatal("admin complete")
	}
	if isCompletedWorkTask(model.WorkTask{Status: model.WBTaskWaiting}, asOK, mnt) {
		t.Fatal("admin waiting")
	}
}

func TestWorkStatusHTTP_TimelineSmoke(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "ws.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	day := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','현황도서관','현황도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom
	) VALUES ('as1','R2608-W01','c1',?,?,1,'completed','양기헌',?,'완료증상')`,
		day+" 09:00:00", day, day+" 11:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, start_time, end_time, duration_min,
		status, assignee, source_type, source_id
	) VALUES ('WT-ws1','as','[AS]현황도서관',?,'10:00','10:30',30,'complete','양기헌','as','as1')`, day); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, start_time, end_time, duration_min,
		status, assignee
	) VALUES ('WT-ws2','admin','미완료 행정',?,'11:00','11:30',30,'waiting','양기헌')`, day); err != nil {
		t.Fatal(err)
	}
	// 완료됐지만 시각 미기재 → 좌·우 모두 보여야 함
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, duration_min, status, assignee, complete_date
	) VALUES ('WT-ws3','admin','시각없는완료',?,30,'complete','양기헌',?)`, day, day); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work-status", h.WorkStatus.Calendar)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work-status?view=day&date="+day, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		t.Fatalf("status=%d body=%s", rec.Code, snippet)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"일일 업무처리현황", "완료", "[AS]현황도서관", "팀전체", "사업 전체", "시각없는완료",
		"목록 · 전체 기간 · 이관 데이터 포함 (통계 집계와 다름)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(body, "미완료 행정") {
		t.Fatal("미완료 업무가 실적에 포함되면 안 됨")
	}
	if !strings.Contains(body, "시각없는완료") {
		t.Fatal("완료 행정 건이 목록에 있어야 함")
	}
	if strings.Contains(body, "일일 업무 수정") || strings.Contains(body, "title=\"조치 화면\"") {
		t.Fatal("조치/수정 버튼이 보이면 안 됨")
	}
	if !strings.Contains(body, "kind=receipt") || !strings.Contains(body, ">접수<") {
		t.Fatal("접수/조치 전환 버튼 없음")
	}
	if strings.Contains(body, "예정업무") || strings.Contains(body, "kind=planned") {
		t.Fatal("예정업무 탭이 남아 있음")
	}
	if strings.Contains(body, "07:00") || strings.Contains(body, "시간별 일일업무처리") {
		t.Fatal("시간표가 업무처리현황에 남아 있음")
	}
}

func TestWorkStatusFilterQueryIncludesKind(t *testing.T) {
	q := workStatusFilterQuery("양기헌", "WP1", wsKindReceipt, "")
	if !strings.Contains(q, "kind=receipt") || !strings.Contains(q, "assignee=") {
		t.Fatalf("q=%s", q)
	}
	if strings.Contains(workStatusFilterQuery("", "", wsKindAction, ""), "kind=") {
		t.Fatal("action(default) should omit kind")
	}
	if !strings.Contains(workStatusFilterQuery("", "", wsKindAction, "0"), "mine=0") {
		t.Fatal("mine=0 유지 실패")
	}
}

func TestReceiptCardLabel(t *testing.T) {
	card := receiptCardFromWork(model.WorkTask{
		TaskID: "rcpt:a1", WorkType: model.WBWorkAS, SourceType: model.WBSourceAS,
		SourceID: "a1", Title: "[AS]도서관", Tags: "R2608-001", Status: "received",
		WorkDate: "2026-08-11", StartTime: "10:00", EndTime: "10:30",
		BoardHref: "/as/a1",
	})
	if card.StatusLabel != "접수" || card.SourceNumber != "R2608-001" {
		t.Fatalf("%+v", card)
	}
}

func TestWorkStatusPlannedURLPreservesFilters(t *testing.T) {
	got := registerURLWithProject("week", "2026-08-11", "양기헌", "WPSEED01")
	if !strings.Contains(got, "/workboard/register?") {
		t.Fatalf("base: %s", got)
	}
	if !strings.Contains(got, "view=week") || !strings.Contains(got, "date=2026-08-11") {
		t.Fatalf("view/date: %s", got)
	}
	if !strings.Contains(got, "assignee=") || !strings.Contains(got, "project=WPSEED01") {
		t.Fatalf("filters: %s", got)
	}
}

func TestWorkStatusMonthDefaultAndDayLink(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_month.db")
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, work_date, due_date, duration_min, status, assignee, complete_date)
		VALUES ('WT-sep','admin','9월완료행정','2026-08-15','2026-08-15',30,'complete','관리자','2026-09-05')`); err != nil {
		t.Fatal(err)
	}

	def := doGet(t, e, "/work-status?date=2026-09-05")
	if def.Code != http.StatusOK {
		t.Fatalf("status=%d", def.Code)
	}
	body := def.Body.String()
	if !strings.Contains(body, "월간 업무처리현황") {
		t.Fatal("기본 보기가 월간이 아니다")
	}
	if !strings.Contains(body, "9월완료행정") {
		t.Fatal("8월 배정·9월 완료 행정이 9월 캘린더에 없다")
	}
	if !strings.Contains(body, "view=day&amp;date=2026-09-05") && !strings.Contains(body, "view=day&date=2026-09-05") {
		t.Fatal("날짜 칸이 일간 보기로 가지 않는다")
	}
	if strings.Contains(body, "07:00") || strings.Contains(body, "시간별") {
		t.Fatal("월간에 시간표가 남아 있다")
	}

	aug := doGet(t, e, "/work-status?view=month&date=2026-08-01").Body.String()
	if strings.Contains(aug, "9월완료행정") {
		t.Fatal("완료일이 9월인 건이 8월 캘린더에 보인다")
	}
}

func TestWorkStatusMissingBanners(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_miss.db")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-miss','누락도서관','누락도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, updated_at
	) VALUES ('as-miss','R-MISS','c-miss','2026-08-10 09:00:00','completed','최혜영','2026-09-01 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, complete_datetime
	) VALUES ('as-noasg','R-NOA','c-miss','2026-08-11 09:00:00','completed','2026-08-11 15:00:00')`); err != nil {
		t.Fatal(err)
	}

	body := doGet(t, e, "/work-status?view=month&date=2026-08-11").Body.String()
	if !strings.Contains(body, "완료일 미기록") {
		t.Fatal("완료일 미기록 배너가 없다")
	}
	if !strings.Contains(body, "담당자 미배정") {
		t.Fatal("담당자 미배정 배너가 없다")
	}
	future := doGet(t, e, "/work-status?view=month&date=2030-01-01").Body.String()
	if !strings.Contains(future, "휴무일 미등록") {
		t.Fatal("휴무일 미등록 배너가 없다")
	}

	list := doGet(t, e, "/work-status?view=month&date=2026-08-11&missing=complete").Body.String()
	if !strings.Contains(list, "누락도서관") {
		t.Fatal("완료일 미기록 목록이 없다")
	}
	asg := doGet(t, e, "/work-status?view=month&date=2026-08-11&missing=assignee").Body.String()
	if !strings.Contains(asg, "누락도서관") {
		t.Fatal("담당자 미배정 목록이 없다")
	}
}

func TestWorkStatusTechScopeToggle(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_scope.db")
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, work_date, duration_min, status, assignee, complete_date)
		VALUES
		('WT-me','admin','내완료','2026-09-02',30,'complete','tech','2026-09-02'),
		('WT-other','admin','남완료','2026-09-02',30,'complete','관리자','2026-09-02')`); err != nil {
		t.Fatal(err)
	}

	reqMine := httptest.NewRequest(http.MethodGet, "http://localhost/work-status?view=day&date=2026-09-02", nil)
	reqMine.AddCookie(jwtCookieRole(t, "tech"))
	recMine := httptest.NewRecorder()
	e.ServeHTTP(recMine, reqMine)
	if recMine.Code != http.StatusOK {
		t.Fatalf("mine status=%d", recMine.Code)
	}
	mine := recMine.Body.String()
	if !strings.Contains(mine, "내완료") || strings.Contains(mine, "남완료") {
		t.Fatal("기술담당 기본이 본인 건이 아니다")
	}
	if !strings.Contains(mine, "팀 전체 보기") {
		t.Fatal("팀 전체 토글이 없다")
	}

	reqTeam := httptest.NewRequest(http.MethodGet, "http://localhost/work-status?view=day&date=2026-09-02&mine=0", nil)
	reqTeam.AddCookie(jwtCookieRole(t, "tech"))
	recTeam := httptest.NewRecorder()
	e.ServeHTTP(recTeam, reqTeam)
	team := recTeam.Body.String()
	if !strings.Contains(team, "내완료") || !strings.Contains(team, "남완료") {
		t.Fatal("기술담당이 팀 전체를 못 본다")
	}
	if !strings.Contains(team, "내 배정만") {
		t.Fatal("본인 보기 토글이 없다")
	}
}

func TestStatusCardShowsLabelNotActions(t *testing.T) {
	card := statusCardFromWork(model.WorkTask{
		TaskID: "WT1", WorkType: model.WBWorkAS, SourceType: model.WBSourceAS,
		SourceID: "as1", Title: "[AS]테스트", Status: "partial_complete",
		WorkDate: "2026-08-11", StartTime: "09:00", EndTime: "09:30",
	})
	if card.StatusLabel != "부분완료" {
		t.Fatalf("label=%q", card.StatusLabel)
	}
	if card.DetailHref() == "" {
		t.Fatal("detail href empty")
	}

	mnt := statusCardFromWork(model.WorkTask{
		TaskID: "done:v1", WorkType: model.WBWorkMaintenance, SourceType: model.WBSourceMaintenance,
		SourceID: "v1", Title: "[점검]나성동", Status: model.WBTaskComplete,
		BoardHref: "/maintenance/visits/v1/action",
	})
	if mnt.StatusLabel != "방문완료" {
		t.Fatalf("mnt label=%q", mnt.StatusLabel)
	}
	if mnt.DetailHref() != "/maintenance/visits/v1/action" {
		t.Fatalf("mnt href=%s", mnt.DetailHref())
	}
}

func TestAttachStatusDayCounts(t *testing.T) {
	weeks := [][]RegisterMonthDay{{{Date: "2026-09-05", InMonth: true}}}
	attachStatusDayCounts(weeks, []model.WorkTask{
		{WorkDate: "2026-09-05"}, {WorkDate: "2026-09-05"},
	}, []model.WorkTask{{WorkDate: "2026-09-05"}})
	if weeks[0][0].ActionCount != 2 || weeks[0][0].ReceiptCount != 1 {
		t.Fatalf("%+v", weeks[0][0])
	}
}

func TestWorkStatusHTTP_SalesMonthBanners(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_sales_banner.db")
	repo := repository.NewSalesRepo(db)

	monthP := &model.SalesProject{
		Name: "부여군도서관 제안", IsTentativeName: true,
		ExpectedYM: "2026-09", ExpectedPrecision: model.SalesPrecisionMonth,
	}
	if err := repo.Create(monthP); err != nil {
		t.Fatal(err)
	}
	qP := &model.SalesProject{
		Name: "분기제안사업", IsTentativeName: true,
		ExpectedYM: "2026-11", ExpectedPrecision: model.SalesPrecisionQuarter,
	}
	if err := repo.Create(qP); err != nil {
		t.Fatal(err)
	}
	confP := &model.SalesProject{
		Name: "확정월사업", IsTentativeName: true,
		ExpectedYM: "2026-09", ExpectedPrecision: model.SalesPrecisionMonth,
		ExpectedYMConfirmed: true,
	}
	if err := repo.Create(confP); err != nil {
		t.Fatal(err)
	}
	actP := &model.SalesProject{Name: "다음활동사업", IsTentativeName: true}
	if err := repo.Create(actP); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateActivity(&model.SalesActivity{
		SalesID: actP.SalesID, ActivityDate: "2026-08-03", ActivityType: "visit",
		Title: "방문", NextAction: "제안서 초안", NextActionDate: "2026-09",
	}, "관리자", nil); err != nil {
		t.Fatal(err)
	}

	sep := doGet(t, e, "/work-status?view=month&date=2026-09-15")
	if sep.Code != http.StatusOK {
		t.Fatalf("status=%d", sep.Code)
	}
	body := sep.Body.String()
	wantTitle := "9월 · 부여군도서관 제안 (월 미정)"
	if !strings.Contains(body, wantTitle) {
		t.Fatalf("월 띠 없음: %s", clipBody(body))
	}
	if !strings.Contains(body, `data-kind="month-banner"`) || !strings.Contains(body, `data-testid="sales-month-banners"`) {
		t.Fatal("월 단위 띠 마크가 없다")
	}
	if !strings.Contains(body, `href="/sales/`+monthP.SalesID) && !strings.Contains(body, `href="/sales/`+monthP.SalesID+`"`) {
		t.Fatalf("사업 상세 링크 없음 id=%s", monthP.SalesID)
	}
	if !strings.Contains(body, "확정") || !strings.Contains(body, "확정월사업") {
		t.Fatal("확정 뱃지가 없다")
	}
	if !strings.Contains(body, "9월 · 제안서 초안 (월 미정)") {
		t.Fatal("next_action 월 띠가 없다")
	}
	gridAt := strings.Index(body, "divide-y divide-slate-100")
	if gridAt < 0 {
		t.Fatal("날짜 칸 그리드가 없다")
	}
	grid := body[gridAt:]
	for _, id := range []string{monthP.SalesID, qP.SalesID, confP.SalesID, actP.SalesID} {
		if strings.Contains(grid, "/sales/"+id) {
			t.Fatalf("날짜 칸에 영업 띠가 들어갔다: %s", id)
		}
	}
	if strings.Contains(body, "4분기 중") {
		t.Fatal("4분기 띠가 9월에 보인다")
	}

	oct := doGet(t, e, "/work-status?view=month&date=2026-10-05").Body.String()
	if !strings.Contains(oct, "4분기 중") || !strings.Contains(oct, "10월 · 분기제안사업") {
		t.Fatalf("10월에 4분기 띠가 없다: %s", clipBody(oct))
	}
	if strings.Contains(oct, wantTitle) {
		t.Fatal("9월 사업이 10월 띠에 있다")
	}
	octGrid := oct
	if i := strings.Index(oct, "divide-y divide-slate-100"); i >= 0 {
		octGrid = oct[i:]
	}
	if strings.Contains(octGrid, "/sales/"+qP.SalesID) {
		t.Fatal("분기 띠가 날짜 칸에 들어갔다")
	}

	nov := doGet(t, e, "/work-status?view=month&date=2026-11-01").Body.String()
	if strings.Contains(nov, "4분기 중") {
		t.Fatal("분기 첫 달이 아닌 11월에 띠가 보인다")
	}

	detail := doGet(t, e, "/sales/"+monthP.SalesID)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "부여군도서관 제안") {
		t.Fatalf("띠 링크 사업 상세 status=%d", detail.Code)
	}
}
