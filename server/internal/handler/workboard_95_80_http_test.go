package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func new80(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *http.Cookie, *repository.WBRepo, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "d80.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("adminadmin"))
	tech := &model.User{Username: "tech80", PasswordHash: HashPassword("password10"), FullName: "기술80", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(tech); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("tech80")
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.POST("/workboard/tasks/:id/carry", h.Workboard.CarryTask)
	g.POST("/workboard/tasks/:id/delay-reason", h.Workboard.SaveDelayReason)
	g.POST("/workboard/schedule", h.Workboard.Schedule)
	g.GET("/work", h.Work.List)
	g.GET("/meeting/board", h.Meeting.Board)
	return e, db, h, jwtCookieUser(t, got), h.Workboard.repo, got
}

func Test95_80_DelayStateAndCallers(t *testing.T) {
	today := time.Now().Format(dateLayout)
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	files := []string{
		filepath.Join(dir, "workboard_board.go"),
		filepath.Join(dir, "work.go"),
		filepath.Join(dir, "meeting_weekly.go"),
		filepath.Join(dir, "handler.go"),
		filepath.Join(dir, "..", "model", "work_board.go"),
		filepath.Join(dir, "..", "repository", "work_board_repo.go"),
	}
	need := []string{"TaskDelayState", "WorkListTaskDelayState"}
	found := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		if strings.Contains(s, need[0]) || strings.Contains(s, need[1]) {
			found++
		}
	}
	if found < 3 {
		t.Fatalf("지연 판정 함수 소비 화면이 부족 %d", found)
	}
	if boardStatus(model.WorkTask{DueDate: today, Status: model.WBTaskWaiting}, today).Key == "late" {
		t.Fatal("종료일 오늘")
	}
}

func Test95_80_ListDelayedWorkTasksHTML(t *testing.T) {
	e, _, h, ck, repo, me := new80(t)
	today := time.Now().Format(dateLayout)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	tom := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	mustCreateBoardTask(t, repo, "어제종료작업", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "내일종료진행", time.Now().AddDate(0, 0, -2).Format(dateLayout), "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	fut := taskByTitle(t, repo, "내일종료진행")
	fut.DueDate = tom
	if err := repo.UpdateTask(fut); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "종료일없음과거", time.Now().AddDate(0, 0, -3).Format(dateLayout), "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	nodue := taskByTitle(t, repo, "종료일없음과거")
	if _, err := h.Workboard.repo.DB().Exec(`UPDATE work_tasks SET due_date='' WHERE task_id=?`, nodue.TaskID); err != nil {
		t.Fatal(err)
	}

	rec := doGetWith(t, e, "/workboard/board?view=kanban&date="+today+"&auto=0", ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("kanban %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "어제종료작업") {
		t.Fatal("칸반에 어제 종료 작업 없음")
	}
	if !strings.Contains(body, "지연") {
		t.Fatal("지연 열 없음")
	}
	items, err := h.workBoard.ListBucketOn(model.WorkBucketDelayed, today, me.UserID, []string{me.FullName}, 50)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Title)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "어제종료작업") {
		t.Fatalf("지연 묶음에 어제 종료 없음 %v", names)
	}
	if strings.Contains(joined, "내일종료진행") {
		t.Fatalf("종료 내일은 지연 묶음에 있으면 안 됨 %v", names)
	}
	if strings.Contains(joined, "종료일없음과거") {
		t.Fatalf("종료일 없음은 지연 아님 %v", names)
	}
}

func Test95_80_SpanListAndMonthChip(t *testing.T) {
	e, _, _, ck, repo, me := new80(t)
	start := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	end := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	mid := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "기간업무80", start, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "기간업무80")
	tk.DueDate = end
	if err := repo.UpdateTask(tk); err != nil {
		t.Fatal(err)
	}
	list := doGetWith(t, e, "/workboard/board?view=day&date="+mid+"&auto=0", ck)
	if !strings.Contains(list.Body.String(), "기간업무80") {
		t.Fatal("중간 날 리스트에 기간 업무 없음")
	}
	if strings.Count(list.Body.String(), "기간업무80") < 1 {
		t.Fatal("기간 업무 제목 없음")
	}
	month := doGetWith(t, e, "/workboard/board?view=month&date="+start+"&auto=0", ck)
	if !strings.Contains(month.Body.String(), "~") {
		t.Fatal("월 칩 종료일 ~ 없음")
	}
}

func Test95_80_CarryModalTomorrowAndReason(t *testing.T) {
	e, _, _, ck, repo, me := new80(t)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	tom, _ := service.NextWorkday(time.Now(), nil, "", false)
	if tom == "" {
		tom = time.Now().AddDate(0, 0, 1).Format(dateLayout)
	}
	mustCreateBoardTask(t, repo, "이월대상", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "이월대상")
	page := doGetWith(t, e, "/workboard/board?view=day&date="+time.Now().Format(dateLayout)+"&auto=0", ck)
	body := page.Body.String()
	if !strings.Contains(body, `id="carry-new-date"`) {
		t.Fatal("이월 모달 없음")
	}
	if !strings.Contains(body, `value="`+tom+`"`) {
		t.Fatal("모달 초기값이 다음 영업일이 아님")
	}
	if !strings.Contains(body, `min="`+time.Now().Format(dateLayout)+`"`) && !strings.Contains(body, `min="`+tom+`"`) {
		t.Fatal("오늘 이전 선택 min 없음")
	}
	empty := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/carry", url.Values{
		"new_date": {tom}, "delay_reason": {""},
	}, ck)
	if empty.Code != http.StatusBadRequest && empty.Code != http.StatusSeeOther {
		t.Fatalf("빈 사유 code=%d", empty.Code)
	}
	if empty.Code == http.StatusBadRequest && !strings.Contains(empty.Body.String(), "사유") {
		t.Fatal("빈 사유 메시지 없음")
	}
	ok := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/carry", url.Values{
		"new_date": {tom}, "delay_reason": {"자재 대기"},
	}, ck)
	if ok.Code != http.StatusOK && ok.Code != http.StatusSeeOther {
		t.Fatalf("이월 실패 %d %s", ok.Code, ok.Body.String())
	}
	got, _ := repo.GetTask(tk.TaskID)
	if got.WorkDate != tom || got.DueDate != tom {
		t.Fatalf("단일일 이동 work=%s due=%s", got.WorkDate, got.DueDate)
	}
	if strings.TrimSpace(got.OrigWorkDate) == "" {
		t.Fatal("orig_work_date 미보존")
	}
	hist, _ := repo.ListDelaysByTask(tk.TaskID)
	if len(hist) == 0 || hist[0].Reason != "자재 대기" {
		t.Fatalf("이력 %+v", hist)
	}
}

func Test95_80_CarryPermissionsAndSpan(t *testing.T) {
	e, db, _, ck, repo, me := new80(t)
	yest := time.Now().AddDate(0, 0, -2).Format(dateLayout)
	end := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	tom := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "기간이월", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "기간이월")
	tk.DueDate = end
	_ = repo.UpdateTask(tk)
	rec := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/carry", url.Values{
		"new_due_date": {tom}, "delay_reason": {"기간연장"},
	}, ck)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("기간 이월 %d %s", rec.Code, rec.Body.String())
	}
	got, _ := repo.GetTask(tk.TaskID)
	if got.WorkDate != yest {
		t.Fatalf("시작일 유지 실패 %s", got.WorkDate)
	}
	if got.DueDate != tom {
		t.Fatalf("종료일만 이동 실패 %s", got.DueDate)
	}

	other := &model.User{Username: "oth80", PasswordHash: HashPassword("password10"), FullName: "타인80", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	_ = repository.NewUserRepo(db).Create(other)
	gotO, _ := repository.NewUserRepo(db).GetByUsername("oth80")
	deny := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/carry", url.Values{
		"new_due_date": {tom}, "delay_reason": {"x"},
	}, jwtCookieUser(t, gotO))
	if deny.Code != http.StatusForbidden && deny.Code != http.StatusNotFound {
		t.Fatalf("타인 이월 %d", deny.Code)
	}
}

func Test95_80_ReasonOnlyAndUpdateRequiresReason(t *testing.T) {
	e, _, _, ck, repo, me := new80(t)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	tom := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	mustCreateBoardTask(t, repo, "사유만", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "사유만")
	r1 := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/delay-reason", url.Values{"delay_reason": {"1차"}}, ck)
	if r1.Code != http.StatusOK && r1.Code != http.StatusSeeOther {
		t.Fatalf("사유1 %d", r1.Code)
	}
	r2 := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/delay-reason", url.Values{"delay_reason": {"2차대표"}}, ck)
	if r2.Code != http.StatusOK && r2.Code != http.StatusSeeOther {
		t.Fatalf("사유2 %d", r2.Code)
	}
	m, _ := repo.LatestDelayReasons([]string{tk.TaskID})
	if m[tk.TaskID] != "2차대표" {
		t.Fatalf("최신 사유 %q", m[tk.TaskID])
	}
	upd := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/update", url.Values{
		"scope": {"register"}, "title": {"사유만"}, "work_date": {tom}, "due_date": {tom},
		"start_time": {"09:00"}, "end_time": {"10:00"}, "delay_reason": {""},
	}, ck)
	if upd.Code == http.StatusOK && strings.Contains(upd.Body.String(), `"ok"`) {
		t.Fatal("빈 사유로 날짜 변경되면 안 됨")
	}
	ok := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/update", url.Values{
		"scope": {"register"}, "title": {"사유만"}, "work_date": {tom}, "due_date": {tom},
		"start_time": {"09:00"}, "end_time": {"10:00"}, "delay_reason": {"계획수정"},
	}, ck)
	if ok.Code != http.StatusSeeOther && ok.Code != http.StatusOK {
		t.Fatalf("계획수정 %d %s", ok.Code, ok.Body.String())
	}
	show := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID, ck)
	if !strings.Contains(show.Body.String(), "지연·이월 이력") {
		t.Fatal("상세 이력 없음")
	}
	meet := doGetWith(t, e, "/meeting/board", ck)
	if meet.Code != http.StatusOK {
		t.Fatalf("회의 %d", meet.Code)
	}
	work := doGetWith(t, e, "/work", ck)
	if work.Code != http.StatusOK {
		t.Fatalf("업무관리 %d", work.Code)
	}
}

func Test95_80_OverlapAndSegmentsAndActionHTML(t *testing.T) {
	e, _, _, ck, repo, me := new80(t)
	tom, _ := service.NextWorkday(time.Now(), nil, "", false)
	if tom == "" {
		tom = time.Now().AddDate(0, 0, 1).Format(dateLayout)
	}
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	mustCreateBoardTask(t, repo, "겹침기존", tom, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "겹침이월", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	tk := taskByTitle(t, repo, "겹침이월")
	_ = repo.ReplacePlanSegments(tk.TaskID, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: yest, StartTime: "09:00", EndTime: "09:30"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: yest, StartTime: "09:30", EndTime: "10:00"},
	})
	ov := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/carry", url.Values{
		"new_date": {tom}, "delay_reason": {"겹침시험"},
	}, ck)
	if ov.Code != http.StatusConflict && !strings.Contains(ov.Body.String(), "겹") && ov.Code != http.StatusSeeOther {
		// 겹침 모달 JSON 또는 리다이렉트
		if ov.Code == http.StatusOK {
			t.Fatal("겹침인데 바로 성공")
		}
	}
	pw := &model.WorkTask{
		Title: "작업이월화면", WorkDate: yest, DueDate: yest, StartTime: "13:00", EndTime: "14:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkProjectWork, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(pw); err != nil {
		t.Fatal(err)
	}
	act := doGetWith(t, e, "/workboard/tasks/"+pw.TaskID+"/action", ck)
	if !strings.Contains(act.Body.String(), "이월") {
		t.Fatal("조치 화면 이월 없음")
	}
	if !strings.Contains(act.Body.String(), `name="next_date"`) {
		t.Fatal("후속 날짜 없음")
	}
	if !strings.Contains(act.Body.String(), tom) {
		t.Fatal("후속 작업일 기본 내일 없음")
	}

	asID := "AS80CARRY"
	if _, err := repo.DB().Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C80','이월고객','이월고객',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	nows := time.Now().Format("2006-01-02 15:04:05")
	if _, err := repo.DB().Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, visit_scheduled_date, schedule_confirmed, org_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,1,?,?,?)`, asID, "R80-1", "C80", nows, "이월시험", "assigned", me.FullName, yest, model.OrgIDLibrary, nows, nows); err != nil {
		t.Fatal(err)
	}
	asTask := &model.WorkTask{
		Title: "AS이월원본", WorkDate: yest, DueDate: yest, StartTime: "15:00", EndTime: "16:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAS, SourceType: model.WBSourceAS, SourceID: asID,
		Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(asTask); err != nil {
		t.Fatal(err)
	}
	asCarry := doFormJSONWith(t, e, "/workboard/tasks/"+asTask.TaskID+"/carry", url.Values{
		"new_date": {tom}, "delay_reason": {"AS방문이월"},
	}, ck)
	if asCarry.Code != http.StatusOK && asCarry.Code != http.StatusSeeOther {
		t.Fatalf("AS 이월 %d %s", asCarry.Code, asCarry.Body.String())
	}
	var vis string
	if err := repo.DB().QueryRow(`SELECT COALESCE(visit_scheduled_date,'') FROM as_receipts WHERE as_id=?`, asID).Scan(&vis); err != nil {
		t.Fatal(err)
	}
	if vis != tom {
		t.Fatalf("AS 방문일 동기화 %s", vis)
	}

	mntID := "MV80DONE"
	if _, err := repo.DB().Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('p80',2026,'t','draft')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed) VALUES (?,?,?,?,1)`, mntID, "p80", yest, "C80"); err != nil {
		t.Fatal(err)
	}
	mntTask := &model.WorkTask{
		Title: "점검완료이월", WorkDate: yest, DueDate: yest, StartTime: "16:00", EndTime: "17:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkMaintenance, SourceType: model.WBSourceMaintenance, SourceID: mntID,
		Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(mntTask); err != nil {
		t.Fatal(err)
	}
	mntCarry := doFormJSONWith(t, e, "/workboard/tasks/"+mntTask.TaskID+"/carry", url.Values{
		"new_date": {tom}, "delay_reason": {"점검완료거절"},
	}, ck)
	if mntCarry.Code == http.StatusOK {
		t.Fatal("완료 점검은 이월되면 안 됨")
	}
}

func Test95_80_MigrationAndCSSAndMobile(t *testing.T) {
	root := findTemplateRoot(t)
	css, err := os.ReadFile(filepath.Join(root, "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cls := range []string{".modal-backdrop", ".btn-primary", ".text-danger", ".z-\\[90\\]", ".min-h-\\[5rem\\]"} {
		if !strings.Contains(string(css), cls) {
			t.Errorf("번들에 %s 없음", cls)
		}
	}
	e, _, _, ck, repo, me := new80(t)
	yest := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	mustCreateBoardTask(t, repo, "모바일이월", yest, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	if err := repo.DB().QueryRow(`SELECT task_id FROM work_tasks WHERE title=?`, "모바일이월").Scan(&id); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/workboard/tasks/"+id, ck)
	if !strings.Contains(page.Body.String(), "data-carry-open") {
		t.Fatal("상세 이월 버튼")
	}
}

func Test95_80_HolidayNoticeDefaultTomorrow(t *testing.T) {
	e, db, _, ck, _, _ := new80(t)
	tom := time.Now().AddDate(0, 0, 1)
	_, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES (?,?,?,?)`,
		tom.Format(dateLayout), tom.Year(), "테스트휴무", "company")
	if err != nil {
		t.Fatal(err)
	}
	service.SetHolidayCalendar(service.NewCalendar(repository.NewHolidayRepo(db)))
	page := doGetWith(t, e, "/workboard/board?view=day&auto=0", ck)
	next, _ := service.NextWorkday(time.Now(), nil, "", false)
	if !strings.Contains(page.Body.String(), "휴무") && !strings.Contains(page.Body.String(), "바꾸기") {
		t.Fatal("휴무 안내 없음")
	}
	if !strings.Contains(page.Body.String(), `value="`+next+`"`) {
		t.Fatalf("초기값이 다음 영업일이 아님 want %s", next)
	}
}
