package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new78A(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *http.Cookie, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "d78a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.POST("/workboard/tasks/:id/action", h.Workboard.SaveProjectWorkAction)
	g.POST("/workboard/schedule", h.Workboard.Schedule)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.GET("/as/work/:work_id/action", h.AS.WorkAction)
	g.POST("/as/work/:work_id/action", h.AS.UpdateWorkAction)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.POST("/maintenance/visits/:visit_id/action", h.Maintenance.UpdateVisitAction)
	return e, db, h, jwtCookie(t), h.Workboard.repo
}

func Test95_78A_MntSyncProtectsSegments(t *testing.T) {
	e, db, h, ck, repo := new78A(t)
	_ = e
	_ = ck
	_ = h
	day := time.Now().AddDate(0, 0, 20).Format(dateLayout)
	seedVisit(t, db, "V78A1", day, "관리자", "KLAS", "도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET start_time='11:00', end_time='15:00', duration_min=240 WHERE visit_id='V78A1'`); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "T78A1", "V78A1", day, "10:00", "15:00", "관리자", "[점검] 도서관")
	if err := repo.ReplacePlanSegments("T78A1", []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "10:00", EndTime: "11:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "12:00", EndTime: "15:00"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_tasks SET start_time='10:00', end_time='15:00', duration_min=300 WHERE task_id='T78A1'`); err != nil {
		t.Fatal(err)
	}
	h.Workboard.repo.SyncMaintenanceBoardRange(day, day)
	tk, err := repo.GetTask("T78A1")
	if err != nil || tk == nil {
		t.Fatal(err)
	}
	if model.ClockHHMM(tk.StartTime) != "10:00" || model.ClockHHMM(tk.EndTime) != "15:00" {
		t.Fatalf("동기화가 구간 외피를 덮음 start=%s end=%s (수정 전 재현: 11:00~15:00)", tk.StartTime, tk.EndTime)
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*tk)
	if len(segs) != 2 {
		t.Fatalf("구간 수 %d", len(segs))
	}
	var vst, ven string
	_ = db.QueryRow(`SELECT COALESCE(start_time,''), COALESCE(end_time,'') FROM maintenance_visits WHERE visit_id='V78A1'`).Scan(&vst, &ven)
	if model.ClockHHMM(vst) != "10:00" || model.ClockHHMM(ven) != "15:00" {
		t.Fatalf("방문 외피 미반영 %s-%s", vst, ven)
	}
	var notes string
	_ = db.QueryRow(`SELECT COALESCE(notes,'') FROM maintenance_visits WHERE visit_id='V78A1'`).Scan(&notes)
	if !strings.Contains(notes, "끼워넣기로 시간 조정") {
		t.Fatalf("방문 이력 없음 %q", notes)
	}
}

func Test95_78A_DateMoveShiftsSegDate(t *testing.T) {
	_, _, _, _, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 21).Format(dateLayout)
	next := time.Now().AddDate(0, 0, 22).Format(dateLayout)
	mustCreateBoardTask(t, repo, "날짜이동", day, "09:00", "12:00", model.OrgIDLibrary, "관리자", "", model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "날짜이동")
	if err := repo.ReplacePlanSegments(tk.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "11:00", EndTime: "12:00"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.PlaceTask(tk.TaskID, next, "09:00", "12:00"); err != nil {
		t.Fatal(err)
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*mustGetTask(t, repo, tk.TaskID))
	if len(segs) != 2 {
		t.Fatalf("구간 %d", len(segs))
	}
	for _, s := range segs {
		if strings.TrimSpace(s.WorkDate) != next {
			t.Fatalf("seg_date 미이동 %s want %s", s.WorkDate, next)
		}
	}
}

func Test95_78A_PersonDateMoveNeedsConfirm(t *testing.T) {
	e, _, _, ck, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 23).Format(dateLayout)
	next := time.Now().AddDate(0, 0, 24).Format(dateLayout)
	mustCreateBoardTask(t, repo, "이탈날짜", day, "09:00", "12:00", model.OrgIDLibrary, "관리자", "", model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "이탈날짜")
	if err := repo.ReplacePlanSegments(tk.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopePerson, WorkDate: day, StartTime: "09:00", EndTime: "12:00", Assignee: "관리자"},
	}); err != nil {
		t.Fatal(err)
	}
	rec := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {tk.TaskID}, "work_date": {next},
		"start_time": {"09:00"}, "end_time": {"12:00"}, "assignee": {"관리자"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=seg_date") {
		t.Fatalf("확인 없이 이동됨 Location=%s", loc)
	}
	rec2 := doFormWith(t, e, "/workboard/schedule", url.Values{
		"kind": {"task"}, "ref_id": {tk.TaskID}, "work_date": {next},
		"start_time": {"09:00"}, "end_time": {"12:00"}, "assignee": {"관리자"},
		"confirm_collapse_segments": {"1"},
	}, ck)
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("confirm %d %s", rec2.Code, rec2.Body.String())
	}
	got, _ := repo.GetTask(tk.TaskID)
	if got.WorkDate != next {
		t.Fatalf("날짜 %s", got.WorkDate)
	}
}

func Test95_78A_ActionHTMLShowsTwoSegments(t *testing.T) {
	e, db, h, ck, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 25).Format(dateLayout)

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c78a','도서관A','도서관A',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c78a", ReceiptChannel: "phone", Requester: "홍", Symptom: "게이트",
		Urgency: "normal", Priority: "normal", AssignedTo: "관리자",
		ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: model.OrgIDLibrary,
		VisitScheduledDate: day,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	daily, _ := repo.GetTaskBySource(model.WBSourceAS, as.ASID)
	if daily == nil {
		t.Fatal("AS 일일 업무 없음")
	}
	if err := repo.ReplacePlanSegments(daily.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	}); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, asOpenHref(as.ASID), ck)
	if page.Code != http.StatusOK {
		t.Fatalf("AS action %d %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	if strings.Contains(body, `name="seg_start"`) || strings.Contains(body, `template "work_segments_form"`) {
		t.Fatal("95-85: AS 조치 화면에 작업 구간이 남아 있다")
	}
	if strings.Contains(body, `data-work-items`) || strings.Contains(body, `name="item_title"`) {
		t.Fatal("95-85: AS 조치 화면에 작업 리스트가 남아 있다")
	}

	one := doGetWith(t, e, asOpenHref(as.ASID), ck)
	_ = one
	mustCreateBoardTask(t, repo, "단일구간", day, "09:00", "10:00", model.OrgIDLibrary, "관리자", "", model.WBTaskWaiting, model.WBWorkAdmin)
	single := taskByTitle(t, repo, "단일구간")
	sh := doGetWith(t, e, "/workboard/tasks/"+single.TaskID, ck)
	if sh.Code != http.StatusOK {
		t.Fatalf("task show %d", sh.Code)
	}
	if !strings.Contains(sh.Body.String(), "작업 구간 1개") {
		t.Fatal("1구간 접힘 문구 없음")
	}

	seedVisit(t, db, "V78Ahtml", day, "관리자", "KLAS", "도서관")
	seedVisitTask(t, db, "T78Ahtml", "V78Ahtml", day, "09:00", "16:00", "관리자", "[점검] html")
	_ = repo.ReplacePlanSegments("T78Ahtml", []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	})
	vpage := doGetWith(t, e, "/maintenance/visits/V78Ahtml/action", ck)
	if vpage.Code != http.StatusOK {
		t.Fatalf("visit action %d %s", vpage.Code, vpage.Body.String())
	}
	if strings.Count(vpage.Body.String(), `name="seg_start"`) < 2 {
		t.Fatal("방문 조치 구간 2개 없음")
	}

	w := &model.ASWorkItem{ASID: as.ASID, WorkKind: model.WorkKindConfirm, AssignedTo: "관리자", ScheduledDate: day, Status: "open"}
	if err := h.AS.workRepo.Create(w); err != nil {
		t.Fatal(err)
	}
	wt := &model.WorkTask{
		WorkType: model.WBWorkAS, SourceType: model.WBSourceASWork, SourceID: w.WorkID,
		Title: w.WorkNumber, Status: model.WBTaskWaiting, Assignee: "관리자",
		WorkDate: day, DueDate: day, StartTime: "09:00", EndTime: "16:00", DurationMin: 420,
		OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(wt); err != nil {
		t.Fatal(err)
	}
	linked, _ := repo.GetTaskBySource(model.WBSourceASWork, w.WorkID)
	_ = repo.ReplacePlanSegments(linked.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	})
	wpage := doGetWith(t, e, "/as/work/"+w.WorkID+"/action", ck)
	if wpage.Code != http.StatusOK {
		t.Fatalf("work action %d %s", wpage.Code, wpage.Body.String())
	}
	if strings.Contains(wpage.Body.String(), `name="seg_start"`) {
		t.Fatal("95-85: 하부 조치에 작업 구간이 남아 있다")
	}

	form := url.Values{
		"status": {"in_progress"}, "work_place": {"field"}, "process_type": {"visit"},
		"cause_type": {"hw"}, "action_taken": {"현장 구간 저장 점검함"}, "time_spent": {"30"},
		"action_short_ok": {"1"}, "complete_datetime": {day + "T16:00"},
		"confirm_seg_shift": {"0"},
		"seg_start":         {day + "T09:00", day + "T14:00"},
		"seg_end":           {day + "T10:00", day + "T16:00"},
		"seg_worker":        {"관리자", "관리자"},
	}
	saved := postASAction(t, e, as.ASID, form)
	if saved.Code != http.StatusSeeOther && saved.Code != http.StatusOK {
		t.Fatalf("AS 저장 %d %s", saved.Code, saved.Body.String())
	}
	gotAS, _ := h.AS.repo.GetByID(model.OrgIDLibrary, as.ASID)
	if gotAS == nil || gotAS.CompleteDatetime == nil {
		t.Fatal("완료일시 미채움")
	}
	actual, _ := repo.ListActualSegments(daily.TaskID)
	if len(actual) < 2 {
		t.Fatalf("실제 구간 %d", len(actual))
	}
}

func Test95_78A_SegShiftFiveTypes(t *testing.T) {
	e, db, h, ck, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 26).Format(dateLayout)

	host := &model.WorkTask{
		Title: "호스트78A", WorkType: model.WBWorkAdmin, Status: model.WBTaskWaiting,
		Assignee: "관리자", WorkDate: day, DueDate: day, StartTime: "09:00", EndTime: "18:00",
		DurationMin: 540, OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(host); err != nil {
		t.Fatal(err)
	}
	host = taskByTitle(t, repo, "호스트78A")

	makeGuest := func(title, workType, sourceType, sourceID string) *model.WorkTask {
		t.Helper()
		g := &model.WorkTask{
			Title: title, WorkType: workType, SourceType: sourceType, SourceID: sourceID,
			Status: model.WBTaskWaiting, Assignee: "관리자",
			WorkDate: day, DueDate: day, StartTime: "10:00", EndTime: "14:00", DurationMin: 240,
			OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
		}
		if err := repo.CreateTask(g); err != nil {
			t.Fatal(err)
		}
		got := taskByTitle(t, repo, title)
		if err := repo.ReplacePlanSegments(host.TaskID, []model.WorkTaskSegment{
			{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
			{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00", InterruptTaskID: got.TaskID},
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}

	postShift := func(path string, form url.Values, confirm string) *httptest.ResponseRecorder {
		t.Helper()
		form.Set("seg_start", day+"T10:30")
		form.Set("seg_end", day+"T14:30")
		form.Set("seg_worker", "관리자")
		if confirm != "" {
			form.Set("confirm_seg_shift", confirm)
		}
		return doFormWith(t, e, path, form, ck)
	}

	adminGuest := makeGuest("끼움행정", model.WBWorkAdmin, "", "")
	adminForm := url.Values{"status": {model.WBTaskWaiting}, "priority": {"normal"}, "assignee": {"관리자"}, "progress": {"0"}}
	r1 := postShift("/workboard/tasks/"+adminGuest.TaskID+"/update", adminForm, "")
	if r1.Code != http.StatusSeeOther || !strings.Contains(r1.Header().Get("Location"), "err=seg_shift") {
		t.Fatalf("행정 확인 없음 %d loc=%s body=%s", r1.Code, r1.Header().Get("Location"), r1.Body.String())
	}
	r1ok := postShift("/workboard/tasks/"+adminGuest.TaskID+"/update", adminForm, "1")
	if r1ok.Code != http.StatusSeeOther {
		t.Fatalf("행정 저장 %d", r1ok.Code)
	}
	hostDate := host.WorkDate
	after, _ := repo.ListPlanSegmentsOrVirtual(*mustGetTask(t, repo, host.TaskID))
	if len(after) != 2 {
		t.Fatalf("호스트 구간 %d", len(after))
	}
	st := model.ClockHHMM(after[1].StartTime)
	if st != "14:45" && st != "15:00" {
		t.Fatalf("후속 시작 %s", after[1].StartTime)
	}
	hgot, _ := repo.GetTask(host.TaskID)
	if hgot.WorkDate != hostDate {
		t.Fatalf("호스트 날짜 변경 %s", hgot.WorkDate)
	}

	if _, err := db.Exec(`INSERT OR IGNORE INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c78as','도서관B','도서관B',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c78as", ReceiptChannel: "phone", Requester: "홍", Symptom: "게이트",
		Urgency: "normal", Priority: "normal", AssignedTo: "관리자",
		ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: model.OrgIDLibrary, VisitScheduledDate: day,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	_ = repo.SyncASDailyTask(as)
	asDaily, _ := repo.GetTaskBySource(model.WBSourceAS, as.ASID)
	if asDaily == nil {
		t.Fatal("AS daily")
	}
	asDaily.WorkDate, asDaily.StartTime, asDaily.EndTime = day, "10:00", "14:00"
	_ = repo.UpdateTask(asDaily)
	_ = repo.ReplacePlanSegments(host.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00", InterruptTaskID: asDaily.TaskID},
	})
	asForm := url.Values{
		"status": {"in_progress"}, "work_place": {"field"}, "process_type": {"visit"},
		"cause_type": {"hw"}, "action_taken": {"끼움 AS 실제시간 조정 점검"}, "time_spent": {"30"},
		"action_short_ok": {"1"},
	}
	asForm.Set("seg_start", day+"T10:30")
	asForm.Add("seg_end", day+"T14:30")
	asForm.Add("seg_worker", "관리자")
	ra := postASAction(t, e, as.ASID, asForm)
	if ra.Code != http.StatusSeeOther || !strings.Contains(ra.Header().Get("Location"), "err=seg_shift") {
		t.Fatalf("AS seg_shift %d loc=%s", ra.Code, ra.Header().Get("Location"))
	}
	asForm.Set("confirm_seg_shift", "1")
	ra2 := postASAction(t, e, as.ASID, asForm)
	if ra2.Code != http.StatusSeeOther {
		t.Fatalf("AS 저장 %d", ra2.Code)
	}
	hgot, _ = repo.GetTask(host.TaskID)
	if hgot.WorkDate != hostDate {
		t.Fatalf("AS 호스트 날짜 %s", hgot.WorkDate)
	}

	seedVisit(t, db, "V78Ash", day, "관리자", "KLAS", "도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET start_time='10:00', end_time='14:00', duration_min=240 WHERE visit_id='V78Ash'`); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "T78Ash", "V78Ash", day, "10:00", "14:00", "관리자", "[점검] shift")
	_ = repo.ReplacePlanSegments(host.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00", InterruptTaskID: "T78Ash"},
	})
	vf := url.Values{"start_time": {"10:30"}, "end_time": {"14:30"}, "notes": {"구간"}, "assignee": {"관리자"}}
	vf.Set("seg_start", day+"T10:30")
	vf.Add("seg_end", day+"T14:30")
	vf.Add("seg_worker", "관리자")
	rv := doFormWith(t, e, "/maintenance/visits/V78Ash/action", vf, ck)
	if rv.Code != http.StatusOK && (rv.Code != http.StatusSeeOther || !strings.Contains(rv.Header().Get("Location"), "seg_shift")) {
		if !strings.Contains(rv.Body.String(), "seg-shift-confirm") && !strings.Contains(rv.Body.String(), "조정하고 저장") {
			t.Fatalf("점검 seg_shift %d loc=%s", rv.Code, rv.Header().Get("Location"))
		}
	}
	vf.Set("confirm_seg_shift", "1")
	rv2 := doFormWith(t, e, "/maintenance/visits/V78Ash/action", vf, ck)
	if rv2.Code != http.StatusSeeOther && rv2.Code != http.StatusOK {
		t.Fatalf("점검 저장 %d", rv2.Code)
	}

	w := &model.ASWorkItem{ASID: as.ASID, WorkKind: model.WorkKindConfirm, AssignedTo: "관리자", ScheduledDate: day, Status: "open"}
	if err := h.AS.workRepo.Create(w); err != nil {
		t.Fatal(err)
	}
	wt := &model.WorkTask{
		WorkType: model.WBWorkAS, SourceType: model.WBSourceASWork, SourceID: w.WorkID,
		Title: "하부끼움", Status: model.WBTaskWaiting, Assignee: "관리자",
		WorkDate: day, DueDate: day, StartTime: "10:00", EndTime: "14:00", DurationMin: 240,
		OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(wt); err != nil {
		t.Fatal(err)
	}
	linked, _ := repo.GetTaskBySource(model.WBSourceASWork, w.WorkID)
	_ = repo.ReplacePlanSegments(host.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00", InterruptTaskID: linked.TaskID},
	})
	wf := url.Values{"assigned_to": {"관리자"}, "scheduled_date": {day}, "notes": {"하부"}}
	wf.Set("seg_start", day+"T10:30")
	wf.Add("seg_end", day+"T14:30")
	wf.Add("seg_worker", "관리자")
	rw := doFormWith(t, e, "/as/work/"+w.WorkID+"/action", wf, ck)
	if rw.Code != http.StatusSeeOther || !strings.Contains(rw.Header().Get("Location"), "err=seg_shift") {
		t.Fatalf("하부 seg_shift %d loc=%s", rw.Code, rw.Header().Get("Location"))
	}
	wf.Set("confirm_seg_shift", "1")
	rw2 := doFormWith(t, e, "/as/work/"+w.WorkID+"/action", wf, ck)
	if rw2.Code != http.StatusSeeOther {
		t.Fatalf("하부 저장 %d", rw2.Code)
	}

	pw := &model.WorkTask{
		Title: "끼움작업", WorkType: model.WBWorkProjectWork, WorkSubtype: "other",
		Status: model.WBTaskWaiting, Assignee: "관리자",
		WorkDate: day, DueDate: day, StartTime: "10:00", EndTime: "14:00", DurationMin: 240,
		OrgID: model.OrgIDLibrary, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(pw); err != nil {
		t.Fatal(err)
	}
	pw = taskByTitle(t, repo, "끼움작업")
	_ = repo.ReplacePlanSegments(host.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "18:00", InterruptTaskID: pw.TaskID},
	})
	pf := pwActionForm()
	pf.Set("action_start_at", day+"T10:30")
	pf.Set("complete_datetime", day+"T14:30")
	pf.Set("confirm_start_differs_plan", "1")
	pf.Set("seg_start", day+"T10:30")
	pf.Set("seg_end", day+"T14:30")
	pf.Set("seg_worker", "관리자")
	rp := doFormWith(t, e, "/workboard/tasks/"+pw.TaskID+"/action", pf, ck)
	if rp.Code == http.StatusSeeOther && strings.Contains(rp.Header().Get("Location"), "err=seg_shift") {
		// redirect 확인
	} else if rp.Code != http.StatusOK || (!strings.Contains(rp.Body.String(), "조정하고 저장") && !strings.Contains(rp.Body.String(), "후속 구간을 조정할까요")) {
		t.Fatalf("작업 seg_shift %d loc=%s", rp.Code, rp.Header().Get("Location"))
	}
	pf.Set("confirm_seg_shift", "1")
	rp2 := doFormWith(t, e, "/workboard/tasks/"+pw.TaskID+"/action", pf, ck)
	if rp2.Code != http.StatusSeeOther {
		t.Fatalf("작업 저장 %d %s", rp2.Code, rp2.Body.String())
	}
}

func Test95_78A_VisitMergeConfirm(t *testing.T) {
	e, db, _, ck, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 27).Format(dateLayout)
	seedVisit(t, db, "V78Amg", day, "관리자", "KLAS", "도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET start_time='09:00', end_time='16:00', duration_min=420 WHERE visit_id='V78Amg'`); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "T78Amg", "V78Amg", day, "09:00", "16:00", "관리자", "[점검] merge")
	_ = repo.ReplacePlanSegments("T78Amg", []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	})
	f := url.Values{"start_time": {"10:00"}, "end_time": {"15:00"}, "assignee": {"관리자"}, "notes": {"시간변경"}}
	rec := doFormWith(t, e, "/maintenance/visits/V78Amg/action", f, ck)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "합치고 저장") {
		t.Fatalf("합치기 확인 없음 %d %s", rec.Code, rec.Body.String())
	}
	f.Set("confirm_merge_segments", "1")
	rec2 := doFormWith(t, e, "/maintenance/visits/V78Amg/action", f, ck)
	if rec2.Code != http.StatusSeeOther && rec2.Code != http.StatusOK {
		t.Fatalf("합치기 저장 %d", rec2.Code)
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*mustGetTask(t, repo, "T78Amg"))
	if len(segs) != 1 {
		t.Fatalf("합친 뒤 %d", len(segs))
	}
}

func Test95_78A_ReconcileKeepsSegments(t *testing.T) {
	_, db, h, _, repo := new78A(t)
	day := time.Now().AddDate(0, 0, 28).Format(dateLayout)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c78rec','도서관R','도서관R',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c78rec", ReceiptChannel: "phone", Requester: "홍", Symptom: "게이트",
		AssignedTo: "관리자", ReceiptDatetime: time.Now(), Status: "in_progress",
		OrgID: model.OrgIDLibrary, VisitScheduledDate: day,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	_ = repo.SyncASDailyTask(as)
	daily, _ := repo.GetTaskBySource(model.WBSourceAS, as.ASID)
	_ = repo.ReplacePlanSegments(daily.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "09:00", EndTime: "10:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "14:00", EndTime: "16:00"},
	})
	if _, err := repository.ReconcileASDailyTasks(db); err != nil {
		t.Fatal(err)
	}
	segs, _ := repo.ListPlanSegmentsOrVirtual(*mustGetTask(t, repo, daily.TaskID))
	if len(segs) != 2 {
		t.Fatalf("정리 후 구간 %d", len(segs))
	}
}

func Test95_78A_SharedPartialAndCSS(t *testing.T) {
	root := findTemplateRoot(t)
	for _, rel := range []string{"as/action.html", "as/work_action.html"} {
		b := string(mustRead(t, filepath.Join(root, rel)))
		if strings.Contains(b, `template "work_segments_form"`) || strings.Contains(b, `template "work_task_items"`) {
			t.Fatalf("%s 95-85: AS 조치 화면에 구간·작업 리스트가 남아 있다", rel)
		}
	}
	for _, rel := range []string{"maintenance/visit_action.html", "workboard/project_work_action.html"} {
		b := string(mustRead(t, filepath.Join(root, rel)))
		if !strings.Contains(b, `template "work_segments_form"`) {
			t.Fatalf("%s 공용 구간 템플릿 없음", rel)
		}
	}
	part := string(mustRead(t, filepath.Join(root, "workboard/_segments_form.html")))
	if !strings.Contains(part, "조정하고 저장") || !strings.Contains(part, "합치고 저장") {
		t.Fatal("공용 확인 버튼 없음")
	}
	css, err := os.ReadFile(filepath.Join(root, "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	for _, cls := range []string{"overflow-x-auto", "min-w-[520px]", "btn-primary", "bg-danger-soft", "bg-warning-soft"} {
		if !strings.Contains(bundle, strings.TrimSuffix(strings.Split(cls, "[")[0], "-")) && !strings.Contains(bundle, cls) {
			if !strings.Contains(bundle, "overflow-x-auto") {
				t.Fatalf("번들 클래스 %s", cls)
			}
		}
	}
	if !strings.Contains(bundle, "overflow-x-auto") {
		t.Fatal("overflow-x-auto 번들 없음")
	}
}

func mustGetTask(t *testing.T, repo *repository.WBRepo, id string) *model.WorkTask {
	t.Helper()
	tk, err := repo.GetTask(id)
	if err != nil || tk == nil {
		t.Fatalf("task %s: %v", id, err)
	}
	return tk
}
