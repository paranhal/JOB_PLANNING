package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newProjectWorkServer(t *testing.T) (*echo.Echo, *repository.WBRepo, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "pw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	u3 := model.User{Username: "oa03", PasswordHash: HashPassword("password10"), FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(&u3); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP-O01','도서관공사','active',?,'construction',1),
		      ('WP-O03','SI구축','active','O03','build',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,region,status,org_id,is_active)
		VALUES('S1','WP-O01','계룡고','충남','in_progress',?,1),
		      ('S2','WP-O01','계룡중','충남','in_progress',?,1),
		      ('SX','WP-O01','제외교','충남','excluded',?,1),
		      ('S3','WP-O03','타조직교','대전','in_progress','O03',1)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/workboard/kanban", h.Workboard.Kanban)
	g.GET("/workboard/project-sites", h.Workboard.SearchProjectWorkSites)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	return e, repository.NewWBRepo(db), users
}

func TestProjectWorkModalFiveKindsAndSave(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "구축·공사 작업을 일일 업무로 등록합니다") {
		t.Fatal("작업 카드 설명 없음")
	}
	if !strings.Contains(body, "dailyKind='project_work'") {
		t.Fatal("작업 종류 카드 없음")
	}
	if !strings.Contains(body, "사업을 선택하면 대상 사이트를 고를 수 있습니다") {
		t.Fatal("사이트 비활성 안내 없음")
	}

	miss := doForm(t, e, "/workboard/tasks", url.Values{"work_type": {"project_work"}})
	if miss.Code != http.StatusBadRequest {
		t.Fatalf("필수 누락 status=%d", miss.Code)
	}

	ok := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":    {"project_work"},
		"title":        {"배선작업"},
		"work_subtype": {"construction"},
		"work_date":    {"2026-10-21"},
		"assignee":     {"관리자"},
		"start_time":   {"09:00"},
		"end_time":     {"09:30"},
		"redirect":     {"/workboard/register"},
	})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("사업 없이 저장 status=%d body=%s", ok.Code, ok.Body.String())
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range tasks {
		if tk.WorkType == model.WBWorkProjectWork && tk.Title == "배선작업" {
			found = true
			if tk.WorkSubtype != model.WBWorkSubtypeConstruction {
				t.Fatalf("subtype=%s", tk.WorkSubtype)
			}
			if strings.Contains(tk.Title, "[작업]") {
				t.Fatal("저장값에 [작업] 접두")
			}
		}
	}
	if !found {
		t.Fatal("저장된 작업 없음")
	}

	kanban := doGet(t, e, "/workboard/kanban")
	if !strings.Contains(kanban.Body.String(), "작업") {
		t.Fatal("칸반에 작업 열 없음")
	}

	reg := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	body = reg.Body.String()
	if !strings.Contains(body, "[작업]배선작업") {
		t.Fatal("일정표 배지 없음")
	}
	if !strings.Contains(body, "#ccfbf1") {
		t.Fatal("작업 카드 색(에메랄드) 없음")
	}

	dup := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"겹침"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
		"start_time": {"09:00"}, "end_time": {"09:30"},
	})
	if dup.Code != http.StatusSeeOther || !strings.Contains(dup.Header().Get("Location"), "err=") {
		t.Fatalf("겹침 loc=%s code=%d", dup.Header().Get("Location"), dup.Code)
	}

	pal := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"팔레트작업"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
	})
	if pal.Code != http.StatusSeeOther {
		t.Fatalf("팔레트 저장 %d", pal.Code)
	}
	pagePal := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(pagePal.Body.String(), "팔레트작업") {
		t.Fatal("팔레트 구역에 시간 미지정 작업 없음")
	}
	if !strings.Contains(pagePal.Body.String(), `data-kind="construction"`) {
		t.Fatal("사업 유형 제안 data-kind 없음")
	}
}

func TestProjectWorkSitesAndSearch(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":    {"project_work"},
		"title":        {"설치"},
		"work_subtype": {"build"},
		"work_date":    {"2026-10-21"},
		"assignee":     {"관리자"},
		"project_id":   {"WP-O01"},
		"site_id":      {"S1", "S2"},
		"redirect":     {"/workboard/register"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", rec.Code, rec.Body.String())
	}
	tasks, _ := repo.ListTasks()
	var id string
	for _, tk := range tasks {
		if tk.Title == "설치" {
			id = tk.TaskID
		}
	}
	if id == "" {
		t.Fatal("task")
	}
	sites, err := repo.ListTaskSites(id)
	if err != nil || len(sites) != 2 {
		t.Fatalf("sites=%d err=%v", len(sites), err)
	}

	ex := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"제외거부"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O01"}, "site_id": {"SX"},
	})
	if ex.Code != http.StatusBadRequest {
		t.Fatalf("excluded status=%d", ex.Code)
	}
	other := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"타사업"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O01"}, "site_id": {"S3"},
	})
	if other.Code != http.StatusBadRequest {
		t.Fatalf("타 사업 사이트 status=%d", other.Code)
	}
	cross := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"타조직"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O03"},
	})
	if cross.Code != http.StatusBadRequest {
		t.Fatalf("타 조직 사업 status=%d", cross.Code)
	}

	q1 := doGet(t, e, "/workboard/project-sites?project_id=WP-O01&q="+url.QueryEscape("계"))
	var empty []repository.WorkTaskSite
	_ = json.Unmarshal(q1.Body.Bytes(), &empty)
	if len(empty) != 0 {
		t.Fatalf("1글자 %d건", len(empty))
	}
	q2 := doGet(t, e, "/workboard/project-sites?project_id=WP-O01&q="+url.QueryEscape("계룡"))
	var got []repository.WorkTaskSite
	if err := json.Unmarshal(q2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("계룡 후보 %d", len(got))
	}

	upd := doForm(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"scope": {"register"}, "title": {"설치"}, "work_date": {"2026-10-21"},
		"due_date": {"2026-10-21"}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {"관리자"}, "status": {"complete"}, "work_subtype": {"construction"},
		"project_id": {"WP-O01"}, "site_id": {"S1"}, "complete_note": {"배선 완료"},
	})
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("수정 %d", upd.Code)
	}
	gotT, _ := repo.GetTask(id)
	if gotT.WorkSubtype != model.WBWorkSubtypeConstruction || gotT.CompleteNote != "배선 완료" || gotT.Status != model.WBTaskComplete {
		t.Fatalf("after subtype=%s note=%s status=%s", gotT.WorkSubtype, gotT.CompleteNote, gotT.Status)
	}
	left, _ := repo.ListTaskSites(id)
	if len(left) != 1 || left[0].SiteID != "S1" {
		t.Fatalf("사이트 제거 후 %+v", left)
	}
}

func TestProjectWorkOrgIsolationSearch(t *testing.T) {
	e, _, users := newProjectWorkServer(t)
	oa03, err := users.GetByUsername("oa03")
	if err != nil || oa03 == nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workboard/project-sites?project_id=WP-O01&q="+url.QueryEscape("계룡"), nil)
	req.AddCookie(jwtCookieUser(t, oa03))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("O03이 O01 사업 검색 status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectWorkACLAndCrossTask(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	ok := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"ACL작업"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
	})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", ok.Code)
	}
	var id string
	tasks, _ := repo.ListTasks()
	for _, tk := range tasks {
		if tk.Title == "ACL작업" {
			id = tk.TaskID
		}
	}
	if id == "" {
		t.Fatal("task")
	}
	oa03, err := users.GetByUsername("oa03")
	if err != nil || oa03 == nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workboard/tasks/"+id, nil)
	req.AddCookie(jwtCookieUser(t, oa03))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 상세 status=%d", rec.Code)
	}

	ro := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader(url.Values{
		"work_type": {"project_work"}, "title": {"RO"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
	}.Encode()))
	ro.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ro.AddCookie(jwtCookieReadOnly(t, "tech"))
	roRec := httptest.NewRecorder()
	e.ServeHTTP(roRec, ro)
	if roRec.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 등록 %d", roRec.Code)
	}
}
