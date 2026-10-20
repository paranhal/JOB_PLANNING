package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newAssigneeOrgServer(t *testing.T) (*echo.Echo, *repository.UserRepo, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "assignee-org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	seed := []model.User{
		{Username: "oa01", PasswordHash: HashPassword("password10"), FullName: "도서관관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true},
		{Username: "oa02", PasswordHash: HashPassword("password10"), FullName: "미디어관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true},
		{Username: "u03", PasswordHash: HashPassword("password10"), FullName: "SI담당C", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true},
		{Username: "libA", PasswordHash: HashPassword("password10"), FullName: "도서관담당A", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true},
		{Username: "medB", PasswordHash: HashPassword("password10"), FullName: "미디어담당B", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true},
		{Username: "vs1", PasswordHash: HashPassword("password10"), FullName: "비젼관리자", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true},
	}
	for i := range seed {
		if err := users.Create(&seed[i]); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/work", h.Work.List)
	g.GET("/work/all", h.Work.ListAll)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	g.GET("/as/new", h.AS.New)
	g.GET("/maintenance", h.Maintenance.ListPlans)
	g.GET("/sales", h.Sales.List)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.POST("/workboard/unschedule", h.Workboard.Unschedule)
	g.POST("/workboard/register/kanban-move", h.Workboard.RegisterKanbanMove)
	return e, users, h.Workboard.repo
}

func jwtCookieViewOrg(t *testing.T, u *model.User, view string) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	claims := jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "org_id": u.OrgID, "is_readonly": u.IsReadOnly,
		"exp": 9999999999,
	}
	if view != "" {
		claims["view_org_id"] = view
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func mustUserByName(t *testing.T, users *repository.UserRepo, username string) *model.User {
	t.Helper()
	u, err := users.GetByUsername(username)
	if err != nil || u == nil {
		t.Fatal(username)
	}
	return u
}

func TestRegisterChipsOwnOrgOnly(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "미등록업무", Assignee: "이효준",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", DurationMin: 30,
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		user, want, hide string
	}{
		{"oa01", "도서관담당A", "미디어담당B"},
		{"oa02", "미디어담당B", "도서관담당A"},
		{"u03", "SI담당C", "도서관담당A"},
	}
	for _, tc := range cases {
		u := mustUserByName(t, users, tc.user)
		rec := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19", jwtCookieUser(t, u))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s register %d", tc.user, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s 화면에 %s 없음", tc.user, tc.want)
		}
		if strings.Contains(body, tc.hide) {
			t.Errorf("%s 화면에 타 조직 %s", tc.user, tc.hide)
		}
		if tc.user == "oa01" {
			if !strings.Contains(body, "이효준") || !strings.Contains(body, "사용자 미등록") {
				t.Error("미등록 칩 없음")
			}
		}
	}
}

func TestAssigneePagesOwnOrgOnly(t *testing.T) {
	e, users, _ := newAssigneeOrgServer(t)
	oa := mustUserByName(t, users, "oa01")
	ck := jwtCookieUser(t, oa)
	for _, path := range []string{"/work", "/work/all", "/meeting", "/meeting/weekly", "/as/new", "/sales"} {
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %d", path, rec.Code)
			continue
		}
		if strings.Contains(rec.Body.String(), "미디어담당B") {
			t.Errorf("%s 에 미디어담당B", path)
		}
	}
}

func TestUpdateTaskRejectsOtherOrgAssignee(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	oa02 := mustUserByName(t, users, "oa02")
	libA := mustUserByName(t, users, "libA")
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "미디어행정", Assignee: "미디어담당B",
		OrgID: "O02", Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "10:00", EndTime: "10:30", DurationMin: 30,
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"assignee": {libA.FullName}, "status": {model.WBTaskWaiting},
		"work_date": {task.WorkDate}, "due_date": {task.DueDate},
		"start_time": {task.StartTime}, "end_time": {task.EndTime}, "duration_min": {"30"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/"+task.TaskID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUser(t, oa02))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errAssigneeSameOrg) {
		t.Fatalf("msg %s", rec.Body.String())
	}
	got, _ := wb.GetTask(task.TaskID)
	if got == nil || got.Assignee != "미디어담당B" {
		t.Fatalf("changed %+v", got)
	}

	oa01 := mustUserByName(t, users, "oa01")
	form2 := url.Values{"assignee": {"미디어담당B"}}
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/"+task.TaskID+"/update", strings.NewReader(form2.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.AddCookie(jwtCookieUser(t, oa01))
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("O01가 O02 업무 수정 %d", rec3.Code)
	}
}

func TestRejectNewUnregisteredAssignee(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	oa := mustUserByName(t, users, "oa01")
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "기존미등록", Assignee: "이효준",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "11:00", EndTime: "11:30", DurationMin: 30,
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"assignee": {"없는사람"}, "status": {model.WBTaskWaiting},
		"work_date": {task.WorkDate}, "due_date": {task.DueDate},
		"start_time": {task.StartTime}, "end_time": {task.EndTime},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/"+task.TaskID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUser(t, oa))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("new unregistered %d %s", rec.Code, rec.Body.String())
	}
	keep := url.Values{
		"assignee": {"이효준"}, "status": {model.WBTaskWaiting},
		"work_date": {task.WorkDate}, "due_date": {task.DueDate},
		"start_time": {task.StartTime}, "end_time": {task.EndTime}, "duration_min": {"30"},
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks/"+task.TaskID+"/update", strings.NewReader(keep.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(jwtCookieUser(t, oa))
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("keep unregistered %d %s", rec2.Code, rec2.Body.String())
	}
}

func TestVisionAssignablePicker(t *testing.T) {
	e, users, _ := newAssigneeOrgServer(t)
	vs := mustUserByName(t, users, "vs1")
	all := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19", jwtCookieViewOrg(t, vs, repository.OrgAll))
	if all.Code != http.StatusOK {
		t.Fatalf("all %d", all.Code)
	}
	if strings.Contains(all.Body.String(), `id="reg-assignee"`) {
		t.Fatal("전 조직에서 담당자 드롭다운이 있음")
	}
	o02 := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19", jwtCookieViewOrg(t, vs, "O02"))
	if o02.Code != http.StatusOK {
		t.Fatalf("o02 %d", o02.Code)
	}
	body := o02.Body.String()
	if !strings.Contains(body, "미디어담당B") {
		t.Fatal("O02 전환 후 미디어 사용자 없음")
	}
	if strings.Contains(body, "도서관담당A") {
		t.Fatal("O02 전환 후 도서관 사용자")
	}
}
