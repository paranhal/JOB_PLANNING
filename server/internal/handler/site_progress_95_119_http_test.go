package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func mount119(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/new", h.Project.New)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/sites/pick", h.Project.SitesPick)
	g.GET("/projects/:id/sites-panel", h.Project.SitesPanel)
	g.POST("/projects/:id/sites/add-schools", h.Project.SitesAddSchools)
	g.POST("/workboard/tasks/:id/site-check", h.Workboard.SaveSiteCheck)
}

func Test95_119_DutyPMAndSitePickPager(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	page := doGet(t, e, "/projects/new")
	if page.Code != http.StatusOK {
		t.Fatalf("new %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), `assign_pm`) || !strings.Contains(page.Body.String(), "PM") {
		t.Fatal("PM 담당 칸 없음")
	}
	for i := 0; i < 101; i++ {
		id := fmt.Sprintf("C-pg%03d", i)
		name := fmt.Sprintf("페이징교%03d", i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,has_parent,parent_customer_id,party_kind)
			VALUES(?,?,?,1,?,1,'C-edu','customer')`, id, name, name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	p1 := doGet(t, e, "/projects/WP-O01/sites/pick?parent_id=C-edu")
	if p1.Code != http.StatusOK {
		t.Fatalf("pick %d %s", p1.Code, p1.Body.String())
	}
	body := p1.Body.String()
	if !strings.Contains(body, `data-sites-pick-pager`) || !strings.Contains(body, "page=2") {
		t.Fatal("1쪽에 다음 페이지 링크가 없다")
	}
	if !strings.Contains(body, "선택한 학교 추가") {
		t.Fatal("관리자 등록 버튼 없음")
	}
	p2 := doGet(t, e, "/projects/WP-O01/sites/pick?parent_id=C-edu&page=2")
	if p2.Code != http.StatusOK {
		t.Fatalf("page2 %d", p2.Code)
	}
	if !strings.Contains(p2.Body.String(), "page=1") || !strings.Contains(p2.Body.String(), "이전") {
		t.Fatal("2쪽에 이전 링크가 없다")
	}
}

func Test95_119_SiteRegisterPMOnly(t *testing.T) {
	e, _, users, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	execU := model.User{Username: "exec119", PasswordHash: HashPassword("password10"), FullName: "수행원",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	pmU := model.User{Username: "pm119", PasswordHash: HashPassword("password10"), FullName: "피엠",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(&execU); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&pmU); err != nil {
		t.Fatal(err)
	}
	ex, _ := users.GetByUsername("exec119")
	pm, _ := users.GetByUsername("pm119")
	if _, err := db.Exec(`INSERT INTO project_assignments(project_id,user_id,duty) VALUES('WP-O01',?,'execution'),('WP-O01',?,'pm')`,
		ex.UserID, pm.UserID); err != nil {
		t.Fatal(err)
	}
	deny := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieUser(t, ex))
	if deny.Code != http.StatusOK {
		t.Fatalf("수행 pick %d", deny.Code)
	}
	if strings.Contains(deny.Body.String(), "선택한 학교 추가") {
		t.Fatal("수행 담당이 사이트 등록 가능")
	}
	add := doFormUser(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-pct"}}, jwtCookieUser(t, ex))
	if add.Code != http.StatusForbidden && add.Code != http.StatusSeeOther {
		t.Fatalf("수행 추가 status=%d", add.Code)
	}
	if add.Code == http.StatusSeeOther {
		t.Fatal("수행 담당 추가가 통과함")
	}
	ok := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieUser(t, pm))
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), "선택한 학교 추가") {
		t.Fatalf("PM pick %d", ok.Code)
	}
	panel := httptest.NewRequest(http.MethodGet, "/projects/WP-O01/sites-panel", nil)
	panel.Header.Set("HX-Request", "true")
	panel.AddCookie(jwtCookieUser(t, pm))
	prec := httptest.NewRecorder()
	e.ServeHTTP(prec, panel)
	if prec.Code != http.StatusOK || !strings.Contains(prec.Body.String(), "data-project-sites-panel") {
		t.Fatalf("panel %d %s", prec.Code, prec.Body.String())
	}
	if !strings.Contains(prec.Body.String(), "사이트 등록") {
		t.Fatal("패널에 등록 링크 없음")
	}
}

func Test95_119_PoolSearchAndSiteCheck(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	q := doGet(t, e, "/workboard/work-sites/search?project_id=WP-O01&q="+url.QueryEscape("계룡"))
	var got []repository.WorkTaskSite
	if err := json.Unmarshal(q.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("풀 검색 %d", len(got))
	}
	save := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"진행체크"}, "work_subtype": {"other"},
		"work_date": {"2026-11-01"}, "due_date": {"2026-11-02"}, "assignee": {"관리자"},
		"site_id": {"C-high", "C-mid"}, "project_id": {"WP-O01"},
	})
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", save.Code)
	}
	tasks, err := repo.ListTasksBetweenForOrg("2026-11-01", "2026-11-02", model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, tk := range tasks {
		if tk.Title == "진행체크" {
			id = tk.TaskID
			break
		}
	}
	if id == "" {
		t.Fatal("업무 없음")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 %d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "data-site-check-grid") || !strings.Contains(body, "0%") {
		t.Fatalf("그리드/0%% 없음: 달성률 구간 없음")
	}
	if !strings.Contains(body, "2026-11-02") {
		t.Fatal("여러 날 열 없음")
	}
	chk := doForm(t, e, "/workboard/tasks/"+id+"/site-check", url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-01"}, "status": {"done"},
	})
	if chk.Code != http.StatusSeeOther {
		t.Fatalf("체크 %d", chk.Code)
	}
	show2 := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show2.Body.String(), "25%") {
		t.Fatal("1/4 달성률 아님")
	}
	asID := "WT-AS119"
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,work_date,due_date,org_id,source_type)
		VALUES(?,'as','접수원본','waiting','2026-11-01','2026-11-01',?,'as')`, asID, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	asChk := doForm(t, e, "/workboard/tasks/"+asID+"/site-check", url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-01"}, "status": {"done"},
	})
	if asChk.Code != http.StatusForbidden {
		t.Fatalf("AS 체크 %d", asChk.Code)
	}
	proj := doGet(t, e, "/projects/WP-O01")
	if proj.Code != http.StatusOK || !strings.Contains(proj.Body.String(), "data-site-progress-tab") {
		t.Fatalf("사업 상세 탭 %d", proj.Code)
	}
}

func Test95_119_SiteNotRequired(t *testing.T) {
	e, _, _ := newProjectWorkServer(t)
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"사이트없음"}, "work_subtype": {"other"},
		"work_date": {"2026-11-04"}, "assignee": {"관리자"}, "project_id": {"WP-O01"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사이트 없이 저장 %d", rec.Code)
	}
}
