package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new95_111(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *model.User, *model.User, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "d111.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	tech := &model.User{Username: "choi111", PasswordHash: HashPassword("password10"), FullName: "최혜영", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	other := &model.User{Username: "yang111", PasswordHash: HashPassword("password10"), FullName: "양기헌", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	out := &model.User{Username: "out111", PasswordHash: HashPassword("password10"), FullName: "타조직지원", Role: model.RoleCLevel, IsActive: true, OrgID: "O02", ProfileDone: true}
	for _, u := range []*model.User{tech, other, out} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	tech, _ = users.GetByUsername("choi111")
	other, _ = users.GetByUsername("yang111")
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.POST("/maintenance/visits/:visit_id/action", h.Maintenance.UpdateVisitAction)
	g.GET("/as/:id/action", h.AS.Action)
	return e, db, h, tech, other, h.Workboard.repo
}

func Test95_111_RenderNoUSRAndOutsideAssignee(t *testing.T) {
	e, db, h, tech, _, repo := new95_111(t)
	day := "2026-11-18"
	seedVisit(t, db, "V111a", day, tech.FullName, "KLAS", "부여도서관")
	if _, err := db.Exec(`INSERT INTO maintenance_visit_assignees (visit_id, user_id, assignee_name, role, sort_order)
		VALUES ('V111a', ?, ?, 'owner', 0)`, tech.UserID, tech.FullName); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "T111a", "V111a", day, "08:00", "08:30", tech.FullName, "[점검]부여도서관")
	if _, err := db.Exec(`UPDATE work_tasks SET assignee_user_id=? WHERE task_id='T111a'`, tech.UserID); err != nil {
		t.Fatal(err)
	}

	outUID := ""
	if u, err := h.Workboard.userRepo.GetByUsername("out111"); err == nil && u != nil {
		outUID = u.UserID
	}
	if _, err := db.Exec(`UPDATE work_tasks SET assignee=?, assignee_user_id=? WHERE task_id='T111a'`, "타조직지원", outUID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE maintenance_visit_assignees SET user_id=?, assignee_name=? WHERE visit_id='V111a'`, outUID, "타조직지원"); err != nil {
		t.Fatal(err)
	}

	adminCK := jwtCookie(t)
	edit := doGetWith(t, e, "/workboard/tasks/T111a/edit", adminCK)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit %d %s", edit.Code, edit.Body.String())
	}
	assertNoUSRLabel(t, edit.Body.String(), "수정")
	if !strings.Contains(edit.Body.String(), "타조직지원") {
		t.Fatal("목록 밖 담당자 이름 없음")
	}
	if !strings.Contains(edit.Body.String(), `value="08:00"`) || !strings.Contains(edit.Body.String(), `value="08:30"`) {
		t.Fatal("수정 화면 시간이 달력과 다름")
	}

	vis := doGetWith(t, e, "/maintenance/visits/V111a/action", adminCK)
	if vis.Code != http.StatusOK {
		t.Fatalf("visit %d %s", vis.Code, vis.Body.String())
	}
	assertNoUSRLabel(t, vis.Body.String(), "방문 조치")
	if !strings.Contains(vis.Body.String(), "타조직지원") {
		t.Fatal("방문 조치 목록 밖 이름 없음")
	}
	if !strings.Contains(vis.Body.String(), "계획 08:00~08:30") {
		t.Fatal("계획 힌트 없음")
	}
	if !strings.Contains(vis.Body.String(), "계획 시각으로 채우기") {
		t.Fatal("채우기 버튼 없음")
	}

	as := &model.ASReceipt{
		CustomerID: "C_V111a", ReceiptChannel: "phone", Requester: "홍", Symptom: "게이트",
		Urgency: "normal", Priority: "normal", AssignedTo: "타조직지원", AssignedUserID: outUID,
		ReceiptDatetime: time.Now(), Status: "in_progress", OrgID: model.OrgIDLibrary,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	asPage := doGetWith(t, e, "/as/"+as.ASID+"/action", adminCK)
	if asPage.Code != http.StatusOK {
		t.Fatalf("as action %d %s", asPage.Code, asPage.Body.String())
	}
	assertNoUSRLabel(t, asPage.Body.String(), "AS 조치")

	pw := &model.WorkTask{
		Title: "작업111", WorkDate: day, DueDate: day, StartTime: "09:00", EndTime: "10:00",
		OrgID: model.OrgIDLibrary, Assignee: "타조직지원", AssigneeUserID: outUID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkProjectWork, WorkSubtype: "build",
	}
	if err := repo.CreateTask(pw); err != nil {
		t.Fatal(err)
	}
	pwPage := doGetWith(t, e, "/workboard/tasks/"+pw.TaskID+"/action", adminCK)
	if pwPage.Code != http.StatusOK {
		t.Fatalf("pw action %d %s", pwPage.Code, pwPage.Body.String())
	}
	assertNoUSRLabel(t, pwPage.Body.String(), "작업 조치")
}

func assertNoUSRLabel(t *testing.T, body, where string) {
	t.Helper()
	if strings.Contains(body, `data-name="USR-`) || strings.Contains(body, `>USR-`) {
		t.Fatalf("%s 화면에 USR- 라벨", where)
	}
}

func Test95_111_EditSaveAssigneeAndTime(t *testing.T) {
	e, db, _, tech, other, repo := new95_111(t)
	day := "2026-11-23"
	seedVisit(t, db, "V111b", day, tech.FullName, "KLAS", "유구도서관")
	seedVisitTask(t, db, "T111b", "V111b", day, "10:30", "11:00", tech.FullName, "[점검]유구도서관")
	if _, err := db.Exec(`UPDATE work_tasks SET assignee_user_id=? WHERE task_id='T111b'`, tech.UserID); err != nil {
		t.Fatal(err)
	}

	adminCK := jwtCookie(t)
	keep := doFormWith(t, e, "/workboard/tasks/T111b/update", url.Values{
		"scope": {"register"}, "title": {"[점검]유구도서관"}, "work_date": {day}, "due_date": {day},
		"start_time": {"10:30"}, "end_time": {"11:00"}, "assignee_multi": {"1"},
		"owner_user_id": {tech.UserID}, "assignee_user_id": {tech.UserID},
	}, adminCK)
	if keep.Code != http.StatusSeeOther || strings.Contains(keep.Header().Get("Location"), "err=") {
		t.Fatalf("유지 저장 loc=%s", keep.Header().Get("Location"))
	}
	got, _ := repo.GetTask("T111b")
	if got.AssigneeUserID != tech.UserID {
		t.Fatalf("assignee_user_id 불변 실패 %s", got.AssigneeUserID)
	}
	if model.ClockHHMM(got.StartTime) != "10:30" || model.ClockHHMM(got.EndTime) != "11:00" {
		t.Fatalf("시간 불변 실패 %s-%s", got.StartTime, got.EndTime)
	}

	chg := doFormWith(t, e, "/workboard/tasks/T111b/update", url.Values{
		"scope": {"register"}, "title": {"[점검]유구도서관"}, "work_date": {day}, "due_date": {day},
		"start_time": {"11:45"}, "end_time": {"12:15"}, "assignee_multi": {"1"},
		"owner_user_id": {other.UserID}, "assignee_user_id": {other.UserID},
	}, adminCK)
	if chg.Code != http.StatusSeeOther || strings.Contains(chg.Header().Get("Location"), "err=") {
		t.Fatalf("변경 저장 loc=%s", chg.Header().Get("Location"))
	}
	got, _ = repo.GetTask("T111b")
	if got.AssigneeUserID != other.UserID {
		t.Fatalf("담당자 변경 실패 %s", got.AssigneeUserID)
	}
	if model.ClockHHMM(got.StartTime) != "11:45" || model.ClockHHMM(got.EndTime) != "12:15" {
		t.Fatalf("업무 시간 변경 실패 %s-%s", got.StartTime, got.EndTime)
	}
	var vst, ven string
	_ = db.QueryRow(`SELECT COALESCE(start_time,''), COALESCE(end_time,'') FROM maintenance_visits WHERE visit_id='V111b'`).Scan(&vst, &ven)
	if model.ClockHHMM(vst) != "11:45" || model.ClockHHMM(ven) != "12:15" {
		t.Fatalf("방문 시간 동기화 실패 %s-%s", vst, ven)
	}

	seedVisit(t, db, "V111e", day, tech.FullName, "KLAS", "빈시간도서관")
	seedVisitTask(t, db, "T111e", "V111e", day, "", "", tech.FullName, "[점검]빈시간도서관")
	if _, err := db.Exec(`UPDATE work_tasks SET assignee_user_id=? WHERE task_id='T111e'`, tech.UserID); err != nil {
		t.Fatal(err)
	}
	blank := doFormWith(t, e, "/workboard/tasks/T111e/update", url.Values{
		"scope": {"register"}, "title": {"[점검]빈시간도서관"}, "work_date": {day}, "due_date": {day},
		"assignee_multi": {"1"}, "owner_user_id": {tech.UserID}, "assignee_user_id": {tech.UserID},
	}, adminCK)
	if blank.Code != http.StatusSeeOther || strings.Contains(blank.Header().Get("Location"), "err=time") {
		t.Fatalf("빈 시간 저장이 막힘 loc=%s", blank.Header().Get("Location"))
	}
}

func Test95_111_MultiSegReadOnlyAndVisitFillHint(t *testing.T) {
	e, db, _, tech, _, repo := new95_111(t)
	day := "2026-11-18"
	seedVisit(t, db, "V111c", day, tech.FullName, "KLAS", "부여도서관")
	seedVisitTask(t, db, "T111c", "V111c", day, "08:00", "12:00", tech.FullName, "[점검]부여도서관")
	if err := repo.ReplacePlanSegments("T111c", []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "08:00", EndTime: "09:00"},
		{Kind: model.SegKindPlan, Scope: model.SegScopeCommon, WorkDate: day, StartTime: "10:00", EndTime: "12:00"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE maintenance_visits SET start_time='', end_time='' WHERE visit_id='V111c'`); err != nil {
		t.Fatal(err)
	}
	adminCK := jwtCookie(t)
	edit := doGetWith(t, e, "/workboard/tasks/T111c/edit", adminCK)
	body := edit.Body.String()
	if !strings.Contains(body, "작업 구간 2개") {
		t.Fatal("구간 2개 읽기 전용 링크 없음")
	}
	if strings.Count(body, `name="start_time"`) < 1 {
		t.Fatal("숨은 시간 필드 없음")
	}

	vis := doGetWith(t, e, "/maintenance/visits/V111c/action", adminCK)
	if vis.Code != http.StatusOK {
		t.Fatalf("visit %d", vis.Code)
	}
	vb := vis.Body.String()
	i := strings.Index(vb, "mntVisitAction(")
	if i < 0 {
		t.Fatal("mntVisitAction 없음")
	}
	chunk := vb[i:]
	if n := 180; len(chunk) > n {
		chunk = chunk[:n]
	}
	if strings.Contains(chunk, `start:"08:00"`) {
		t.Fatalf("방문 실제 시작이 계획으로 채워짐 %s", chunk)
	}
	if !strings.Contains(vis.Body.String(), "계획 08:00~12:00") {
		t.Fatal("다구간 계획 힌트 없음")
	}
	save := doFormWith(t, e, "/maintenance/visits/V111c/action", url.Values{
		"visit_date": {day}, "assignee_multi": {"1"}, "owner_user_id": {tech.UserID},
		"assignee_user_id": {tech.UserID}, "status": {"open"},
	}, adminCK)
	if save.Code != http.StatusSeeOther && save.Code != http.StatusOK {
		t.Fatalf("빈 실제시각 저장 code=%d loc=%s body=%s", save.Code, save.Header().Get("Location"), save.Body.String())
	}
}
