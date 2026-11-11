package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// Test95_170_RevisitConfirmedNotOnCalendar 재방문 하위업무는 AS에 생기지만
// 수정 전 월·주·일 일정표에는 카드가 없다.
func Test95_170_RevisitConfirmedNotOnCalendar(t *testing.T) {
	e, db, h, ck, _ := new78A(t)
	e.GET("/workboard/register", h.Workboard.Register)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-kdi','KDI국제정책대학원','KDI국제정책대학원',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	as := &model.ASReceipt{
		CustomerID: "c-kdi", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "재방문 일정표", AssignedTo: "관리자", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	rec := postASAction(t, e, as.ASID, url.Values{
		"work_place":             {"office"},
		"process_type":           {"remote"},
		"cause_type":             {"sw"},
		"action_taken":           {"재방문 일정을 확정했다"},
		"result_code":            {model.ResultPartial},
		"revisit_scheduled_date": {day},
		"revisit_reason":         {"추가 확인"},
		"schedule_confirmed":     {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 저장 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("조치가 거절됨 %s", rec.Header().Get("Location"))
	}
	var workNo, workID, confirmed string
	if err := db.QueryRow(`SELECT work_id, work_number, COALESCE(schedule_confirmed,0) FROM as_work_items WHERE as_id=? AND work_kind=?`,
		as.ASID, model.WorkKindRevisit).Scan(&workID, &workNo, &confirmed); err != nil {
		t.Fatalf("재방문 하위업무가 안 생김: %v", err)
	}
	if confirmed != "1" {
		t.Fatalf("일정확정=%s", confirmed)
	}
	var taskN int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=?`,
		model.WBSourceASWork, workID).Scan(&taskN); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"month", "week", "day"} {
		body := doGetWith(t, e, "/workboard/board?view="+view+"&date="+day+"&mine=0&auto=0", ck).Body.String()
		if !strings.Contains(body, workNo) {
			t.Errorf("일정표 %s 에 재방문 %s 없음 (work_tasks as_work=%d)", view, workNo, taskN)
		}
		reg := doGetWith(t, e, "/workboard/register?view="+view+"&date="+day+"&auto=0", ck).Body.String()
		if !strings.Contains(reg, workNo) {
			t.Errorf("등록표 %s 에 재방문 %s 없음", view, workNo)
		}
	}
	var parentTitle string
	_ = db.QueryRow(`SELECT title FROM work_tasks WHERE source_type=? AND source_id=?`, model.WBSourceAS, as.ASID).Scan(&parentTitle)
	month := doGetWith(t, e, "/workboard/board?view=month&date="+day+"&mine=0&auto=0", ck).Body.String()
	if parentTitle != "" && monthChipHas(month, parentTitle) {
		t.Fatalf("같은 날 접수 본체가 하위업무와 같이 보인다: %s", parentTitle)
	}
	if !monthChipHas(month, workNo) {
		t.Fatal("월간 칸에 재방문 칩이 없다")
	}
}

func Test95_170_TentativeConfirmSupportAndFollowups(t *testing.T) {
	e, db, h, ck := new170(t)
	day := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	next := time.Now().AddDate(0, 0, 11).Format("2006-01-02")
	as := seed170AS(t, db, h, "가배정")
	rec := postASAction(t, e, as.ASID, action170(day, "0"))
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("가배정 저장 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	workNo := workNo170(t, db, as.ASID, model.WorkKindRevisit)
	month := doGetWith(t, e, "/workboard/board?view=month&date="+day+"&mine=0&auto=0", ck).Body.String()
	if !monthChipHas(month, workNo) || !strings.Contains(month, "가배정") {
		t.Fatal("가배정 재방문이 월간에 없다")
	}

	confirmAS := seed170AS(t, db, h, "확인")
	w := &model.ASWorkItem{
		ASID: confirmAS.ASID, WorkKind: model.WorkKindConfirm, ScheduledDate: day,
		ScheduleConfirmed: true, AssignedTo: "관리자", Status: "open", Notes: "회신확인",
	}
	if err := h.AS.workRepo.Create(w); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReconcileASWorkDailyTasks(db); err != nil {
		t.Fatal(err)
	}
	confirmNo := workNo170(t, db, confirmAS.ASID, model.WorkKindConfirm)
	if !monthChipHas(doGetWith(t, e, "/workboard/board?view=month&date="+day+"&mine=0&auto=0", ck).Body.String(), confirmNo) {
		t.Fatal("확인 하위업무가 월간에 없다")
	}

	supAS := seed170AS(t, db, h, "지원")
	sup := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+supAS.ASID+"/support", strings.NewReader(url.Values{
		"support_type": {model.SupportOther}, "memo": {"현장 지원"}, "due_date": {day}, "schedule_confirmed": {"1"},
	}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(sup, req)
	if sup.Code != http.StatusSeeOther || strings.Contains(sup.Header().Get("Location"), "err=") {
		t.Fatalf("지원 저장 %d %s", sup.Code, sup.Header().Get("Location"))
	}
	supNo := workNo170(t, db, supAS.ASID, model.WorkKindSupport)
	if !monthChipHas(doGetWith(t, e, "/workboard/board?view=month&date="+day+"&mine=0&auto=0", ck).Body.String(), supNo) {
		t.Fatal("지원 하위업무가 월간에 없다")
	}

	moveAS := seed170AS(t, db, h, "변경")
	if rec := postASAction(t, e, moveAS.ASID, action170(day, "1")); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("변경용 저장 %s", rec.Header().Get("Location"))
	}
	moveNo := workNo170(t, db, moveAS.ASID, model.WorkKindRevisit)
	var workID string
	if err := db.QueryRow(`SELECT work_id FROM as_work_items WHERE as_id=? AND work_kind=?`, moveAS.ASID, model.WorkKindRevisit).Scan(&workID); err != nil {
		t.Fatal(err)
	}
	upd := doForm(t, e, "/as/work/"+workID+"/action", url.Values{
		"scheduled_date": {next}, "schedule_confirmed": {"1"}, "assigned_to": {"관리자"},
	})
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("일정 변경 %d", upd.Code)
	}
	if monthChipHas(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", ck).Body.String(), moveNo) {
		t.Fatal("이전 날짜에 칩이 남았다")
	}
	moved := doGetWith(t, e, "/workboard/board?view=day&date="+next+"&mine=0&auto=0", ck).Body.String()
	if !strings.Contains(moved, moveNo) {
		t.Fatal("바꾼 날짜에 칩이 없다")
	}
	done := doForm(t, e, "/as/work/"+workID+"/action", url.Values{
		"scheduled_date": {next}, "schedule_confirmed": {"1"}, "mark_done": {"1"},
	})
	if done.Code != http.StatusSeeOther {
		t.Fatalf("완료 %d", done.Code)
	}
	doneBody := doGetWith(t, e, "/workboard/board?view=day&date="+next+"&mine=0&auto=0", ck).Body.String()
	if !strings.Contains(doneBody, moveNo) || !strings.Contains(doneBody, "완료") {
		t.Fatal("완료 배지가 없다")
	}

	blank := seed170AS(t, db, h, "날짜없음")
	if rec := postASAction(t, e, blank.ASID, action170(day, "1")); rec.Code != http.StatusSeeOther {
		t.Fatal(rec.Code)
	}
	var blankID, blankNo string
	if err := db.QueryRow(`SELECT work_id, work_number FROM as_work_items WHERE as_id=?`, blank.ASID).Scan(&blankID, &blankNo); err != nil {
		t.Fatal(err)
	}
	cleared := doForm(t, e, "/as/work/"+blankID+"/action", url.Values{"scheduled_date": {""}})
	if cleared.Code != http.StatusSeeOther {
		t.Fatalf("날짜 삭제 %d", cleared.Code)
	}
	panel := unplacedPanelHTML(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", ck).Body.String())
	if !strings.Contains(panel, blankNo) {
		t.Fatal("날짜 없는 하위업무가 미배치에 없다")
	}
}

func Test95_170_ScopeAndNoDoubleUnplaced(t *testing.T) {
	e, db, h, ck := new170(t)
	day := time.Now().AddDate(0, 0, 12).Format("2006-01-02")
	as := seed170AS(t, db, h, "범위")
	if rec := postASAction(t, e, as.ASID, action170(day, "1")); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %s", rec.Header().Get("Location"))
	}
	workNo := workNo170(t, db, as.ASID, model.WorkKindRevisit)
	users := repository.NewUserRepo(db)
	other := &model.User{Username: "o170", PasswordHash: HashPassword("password10"), FullName: "타조직", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	tech := &model.User{Username: "t170", PasswordHash: HashPassword("password10"), FullName: "김기술", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	ro := &model.User{Username: "r170", PasswordHash: HashPassword("password10"), FullName: "읽기", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01", Email: "r170@ex.com", ProfileDone: true}
	cl := &model.User{Username: "c170", PasswordHash: HashPassword("password10"), FullName: "경영", Role: model.RoleCLevel, IsActive: true, OrgID: "O01", Email: "c170@ex.com", ProfileDone: true}
	for _, u := range []*model.User{other, tech, ro, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0", jwtCookieUser(t, other)).Body.String(), workNo) {
		t.Fatal("타 조직에 하위업무가 보인다")
	}
	if !strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", jwtCookieUser(t, tech)).Body.String(), workNo) {
		t.Fatal("팀 전체 보기에 없다")
	}
	if strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0", jwtCookieUser(t, tech)).Body.String(), workNo) {
		t.Fatal("기술 담당 기본 보기에 남의 하위업무가 있다")
	}
	if strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0&assignee="+url.QueryEscape("없는사람"), ck).Body.String(), workNo) {
		t.Fatal("다른 담당자 필터에 보인다")
	}
	if !strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0&assignee="+url.QueryEscape("관리자"), ck).Body.String(), workNo) {
		t.Fatal("담당자 필터에 없다")
	}
	if !strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", jwtCookieUser(t, ro)).Body.String(), workNo) {
		t.Fatal("읽기 전용에 안 보인다")
	}
	if !strings.Contains(doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", jwtCookieUser(t, cl)).Body.String(), workNo) {
		t.Fatal("C-Level에 안 보인다")
	}

	undated := seed170AS(t, db, h, "미배치키")
	undated.VisitScheduledDate = ""
	undated.AssignedTo = "관리자"
	if err := h.Workboard.repo.SyncASDailyTask(undated); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_tasks SET work_date='', due_date='' WHERE source_type=? AND source_id=?`, model.WBSourceAS, undated.ASID); err != nil {
		t.Fatal(err)
	}
	child := &model.ASWorkItem{ASID: undated.ASID, WorkKind: model.WorkKindRevisit, Status: "open", AssignedTo: "관리자"}
	if err := h.AS.workRepo.Create(child); err != nil {
		t.Fatal(err)
	}
	if err := h.Workboard.repo.SyncASWorkDailyTask(child); err != nil {
		t.Fatal(err)
	}
	tk, err := h.Workboard.repo.GetTaskBySource(model.WBSourceASWork, child.WorkID)
	if err != nil || tk == nil {
		t.Fatal(err)
	}
	panel := unplacedPanelHTML(doGetWith(t, e, "/workboard/board?view=day&mine=0&auto=0", ck).Body.String())
	if strings.Count(panel, `data-pid="as:`+undated.ASID+`"`) != 1 {
		t.Fatalf("접수 미배치 키 %d", strings.Count(panel, `data-pid="as:`+undated.ASID+`"`))
	}
	if strings.Count(panel, `data-pid="task:`+tk.TaskID+`"`) != 1 {
		t.Fatalf("하위업무 미배치 키 %d", strings.Count(panel, `data-pid="task:`+tk.TaskID+`"`))
	}
}

func new170(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *http.Cookie) {
	t.Helper()
	e, db, h, ck, _ := new78A(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/register", h.Workboard.Register)
	g.POST("/as/:id/support", h.AS.CreateSupport)
	return e, db, h, ck
}

func seed170AS(t *testing.T, db *sql.DB, h *Handler, symptom string) *model.ASReceipt {
	t.Helper()
	id := "c170-" + symptom
	_, _ = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`,
		id, "KDI국제정책대학원", "KDI국제정책대학원")
	as := &model.ASReceipt{
		CustomerID: id, ReceiptChannel: "phone", Requester: "학교",
		Symptom: symptom, AssignedTo: "관리자", Status: "in_progress",
		ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	return as
}

func action170(day, confirmed string) url.Values {
	return url.Values{
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"sw"},
		"action_taken": {"재방문 일정을 적었다"}, "result_code": {model.ResultPartial},
		"revisit_scheduled_date": {day}, "revisit_reason": {"추가 확인"},
		"schedule_confirmed": {confirmed},
	}
}

func workNo170(t *testing.T, db *sql.DB, asID, kind string) string {
	t.Helper()
	var no string
	if err := db.QueryRow(`SELECT work_number FROM as_work_items WHERE as_id=? AND work_kind=?`, asID, kind).Scan(&no); err != nil {
		t.Fatal(err)
	}
	return no
}

func monthChipHas(body, needle string) bool {
	parts := strings.Split(body, `data-month-chip="1"`)
	for _, part := range parts[1:] {
		end := strings.Index(part, "</a>")
		if end < 0 {
			end = len(part)
		}
		if strings.Contains(part[:end], needle) {
			return true
		}
	}
	return false
}
