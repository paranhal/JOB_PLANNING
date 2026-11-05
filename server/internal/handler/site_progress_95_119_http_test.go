package handler

import (
	"encoding/json"
	"fmt"
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

func mount119(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/new", h.Project.New)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/sites", h.Project.Sites)
	g.GET("/projects/:id/sites/candidates", h.Project.SitesCandidates)
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
	if p1.Code != http.StatusSeeOther {
		t.Fatalf("pick redirect %d %s", p1.Code, p1.Body.String())
	}
	if loc := p1.Header().Get("Location"); !strings.Contains(loc, "/projects/WP-O01/sites") {
		t.Fatalf("pick loc %s", loc)
	}
	sitesPage := doGet(t, e, "/projects/WP-O01/sites")
	if sitesPage.Code != http.StatusOK {
		t.Fatalf("sites %d", sitesPage.Code)
	}
	body := sitesPage.Body.String()
	if strings.Contains(body, `data-sites-pick-pager`) {
		t.Fatal("페이징 UI가 남음")
	}
	if !strings.Contains(body, "등록 (") && !strings.Contains(body, `id="btn-site-reg"`) {
		t.Fatal("관리자 등록 버튼 없음")
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
	denyPick := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieUser(t, ex))
	if denyPick.Code != http.StatusSeeOther {
		t.Fatalf("수행 pick %d", denyPick.Code)
	}
	deny := doGetWith(t, e, "/projects/WP-O01/sites", jwtCookieUser(t, ex))
	if deny.Code != http.StatusOK {
		t.Fatalf("수행 sites %d", deny.Code)
	}
	if !strings.Contains(deny.Body.String(), "이 사업의 사이트를 등록할 권한이 없습니다") {
		t.Fatal("수행 담당 안내 없음")
	}
	if !strings.Contains(deny.Body.String(), "data-site-register-denied") {
		t.Fatal("수행 담당이 사이트 등록 가능")
	}
	add := doFormUser(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-pct"}}, jwtCookieUser(t, ex))
	if add.Code != http.StatusForbidden && add.Code != http.StatusSeeOther {
		t.Fatalf("수행 추가 status=%d", add.Code)
	}
	if add.Code == http.StatusSeeOther {
		t.Fatal("수행 담당 추가가 통과함")
	}
	okPick := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieUser(t, pm))
	if okPick.Code != http.StatusSeeOther {
		t.Fatalf("PM pick %d", okPick.Code)
	}
	ok := doGetWith(t, e, "/projects/WP-O01/sites", jwtCookieUser(t, pm))
	if ok.Code != http.StatusOK || strings.Contains(ok.Body.String(), "이 사업의 사이트를 등록할 권한이 없습니다") {
		t.Fatalf("PM sites %d", ok.Code)
	}
	if !strings.Contains(ok.Body.String(), `id="btn-site-reg"`) {
		t.Fatal("PM 등록 버튼")
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
	if !strings.Contains(show2.Body.String(), "50%") {
		t.Fatal("사이트당 1회 1/2 달성률 아님")
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

func Test95_119A_TrackProgressCheckbox(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "달성률 계산") || !strings.Contains(body, `name="track_progress"`) || !strings.Contains(body, "checked") {
		t.Fatal("여러 날 폼에 달성률 계산 기본 체크 없음")
	}
	if !strings.Contains(body, "data-track-progress") {
		t.Fatal("data-track-progress 없음")
	}
	off := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"집계제외"}, "work_subtype": {"other"},
		"work_date": {"2026-11-01"}, "due_date": {"2026-11-02"}, "assignee": {"관리자"},
		"site_id": {"C-high", "C-mid"}, "project_id": {"WP-O01"}, "track_progress": {"0"},
	})
	if off.Code != http.StatusSeeOther {
		t.Fatalf("해제 저장 %d", off.Code)
	}
	id := lastTaskID(t, repo, "집계제외")
	if repo.TrackProgress(id) {
		t.Fatal("해제했는데 집계 대상")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show.Body.String(), "—") {
		t.Fatal("집계 제외 시 달성률이 — 가 아님")
	}
	if !strings.Contains(show.Body.String(), "data-site-check-grid") {
		t.Fatal("체크 그리드가 사라져서는 안 됨")
	}
	on := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"집계포함"}, "work_subtype": {"other"},
		"work_date": {"2026-11-01"}, "due_date": {"2026-11-02"}, "assignee": {"관리자"},
		"site_id": {"C-high"}, "project_id": {"WP-O01"}, "track_progress": {"1"},
	})
	if on.Code != http.StatusSeeOther {
		t.Fatalf("체크 저장 %d", on.Code)
	}
	id2 := lastTaskID(t, repo, "집계포함")
	if !repo.TrackProgress(id2) {
		t.Fatal("기본 체크 저장이 집계에서 빠짐")
	}
}

func Test95_119A_SiteCap100(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	okForm := url.Values{
		"work_type": {"project_work"}, "title": {"100곳"}, "work_subtype": {"other"},
		"work_date": {"2026-11-04"}, "assignee": {"관리자"}, "project_id": {"WP-O01"},
	}
	var ids []string
	for i := 1; i <= 100; i++ {
		id := fmt.Sprintf("C-ok%03d", i)
		name := fmt.Sprintf("허용교%03d", i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
			VALUES(?,?,?,1,?,'customer')`, id, name, name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
		okForm.Add("site_id", id)
		ids = append(ids, id)
	}
	ok := doForm(t, e, "/workboard/tasks", okForm)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("100곳 %d %s", ok.Code, ok.Body.String())
	}
	tid := lastTaskID(t, repo, "100곳")
	sites, err := repo.ListTaskSites(tid)
	if err != nil || len(sites) != 100 {
		t.Fatalf("저장 사이트 %d err=%v", len(sites), err)
	}
	_ = ids
}

func Test95_119A_RecurrenceOncePerSite(t *testing.T) {
	e, repo, _, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	save := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"반복상위"}, "work_subtype": {"other"},
		"work_date": {"2026-11-01"}, "due_date": {"2026-11-10"}, "assignee": {"관리자"},
		"site_id": {"C-high"}, "project_id": {"WP-O01"}, "track_progress": {"1"},
	})
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", save.Code)
	}
	parentID := lastTaskID(t, repo, "반복상위")
	if _, err := db.Exec(`UPDATE work_tasks SET recurrence_role='parent' WHERE task_id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,work_date,due_date,org_id,parent_task_id,recurrence_role,project_id)
		VALUES('WT-occ1','project_work','회차1','waiting','2026-11-03','2026-11-03',?,?,'occurrence','WP-O01'),
		      ('WT-occ2','project_work','회차2','waiting','2026-11-05','2026-11-05',?,?,'occurrence','WP-O01')`,
		model.OrgIDLibrary, parentID, model.OrgIDLibrary, parentID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteCheck("WT-occ1", "C-high", "2026-11-03", model.SiteCheckDone, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteCheck("WT-occ2", "C-high", "2026-11-05", model.SiteCheckDone, "admin"); err != nil {
		t.Fatal(err)
	}
	sites, err := repo.ListTaskSites(parentID)
	if err != nil {
		t.Fatal(err)
	}
	done, total, lab := repo.TaskSiteCheckRate(parentID, "2026-11-01", "2026-11-10", sites)
	if done != 1 || total != 1 || lab != "100%" {
		t.Fatalf("이중 집계 done=%d total=%d lab=%s", done, total, lab)
	}
	rows, err := repo.ListProjectSiteWorkProgress("WP-O01")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.CustomerID == "C-high" && row.Done > 1 {
			t.Fatalf("사업 탭 사이트 완료가 %d회", row.Done)
		}
	}
}

func Test95_119A_MigrationReapplyEmptyTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reapply.db")
	db, err := repository.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = repository.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('work_task_site_progress','work_task_progress_flag')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("105 표 n=%d err=%v", n, err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP-empty','빈사이트','active',?,'build',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/:id", h.Project.Show)
	page := doGet(t, e, "/projects/WP-empty")
	if page.Code != http.StatusOK {
		t.Fatalf("상세 %d %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "data-site-progress-tab") || !strings.Contains(page.Body.String(), "등록된 사이트가 없습니다") {
		t.Fatal("사이트 0건 탭 안내 없음")
	}
}

func Test95_119A_HTMXAndPermissionMatrix(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	h := New(db)
	mount119(e, h)
	save := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"권한행렬"}, "work_subtype": {"other"},
		"work_date": {"2026-11-01"}, "due_date": {"2026-11-02"}, "assignee": {"관리자"},
		"site_id": {"C-high"}, "project_id": {"WP-O01"}, "track_progress": {"1"},
	})
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", save.Code)
	}
	id := lastTaskID(t, repo, "권한행렬")
	hx := httptest.NewRequest(http.MethodPost, "/workboard/tasks/"+id+"/site-check", strings.NewReader(url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-01"}, "status": {"done"},
	}.Encode()))
	hx.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	hx.Header.Set("HX-Request", "true")
	hx.AddCookie(jwtCookie(t))
	hrec := httptest.NewRecorder()
	e.ServeHTTP(hrec, hx)
	if hrec.Code != http.StatusOK {
		t.Fatalf("HX 체크 %d %s", hrec.Code, hrec.Body.String())
	}
	if !strings.Contains(hrec.Body.String(), "id=\"site-check-grid\"") || !strings.Contains(hrec.Body.String(), "100%") {
		t.Fatalf("HX 조각 아님: %s", hrec.Body.String())
	}

	clPick := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieRole(t, model.RoleCLevel))
	if clPick.Code != http.StatusSeeOther && clPick.Code != http.StatusForbidden {
		t.Fatalf("C-Level pick %d", clPick.Code)
	}
	clSites := doGetWith(t, e, "/projects/WP-O01/sites", jwtCookieRole(t, model.RoleCLevel))
	if clSites.Code == http.StatusOK && !strings.Contains(clSites.Body.String(), "이 사업의 사이트를 등록할 권한이 없습니다") {
		t.Fatal("C-Level이 사이트 등록 가능")
	}
	clAdd := doFormUser(t, e, "/projects/WP-O01/sites/add-schools", url.Values{"customer_id": {"C-pct"}}, jwtCookieRole(t, model.RoleCLevel))
	if clAdd.Code != http.StatusForbidden && clAdd.Code != http.StatusSeeOther {
		t.Fatalf("C-Level 추가 %d", clAdd.Code)
	}
	if clAdd.Code == http.StatusSeeOther {
		t.Fatal("C-Level 사이트 추가가 통과함")
	}
	clChk := doFormUser(t, e, "/workboard/tasks/"+id+"/site-check", url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-02"}, "status": {"done"},
	}, jwtCookieRole(t, model.RoleCLevel))
	if clChk.Code != http.StatusForbidden {
		t.Fatalf("C-Level 체크 %d", clChk.Code)
	}
	roChk := doFormUser(t, e, "/workboard/tasks/"+id+"/site-check", url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-02"}, "status": {"done"},
	}, jwtCookieReadOnly(t, "tech"))
	if roChk.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 체크 %d", roChk.Code)
	}
	oa03, err := users.GetByUsername("oa03")
	if err != nil {
		t.Fatal(err)
	}
	other := doFormUser(t, e, "/workboard/tasks/"+id+"/site-check", url.Values{
		"customer_id": {"C-high"}, "work_date": {"2026-11-02"}, "status": {"done"},
	}, jwtCookieUser(t, oa03))
	if other.Code != http.StatusForbidden && other.Code != http.StatusNotFound {
		t.Fatalf("타 조직 체크 %d", other.Code)
	}
	vs := model.User{Username: "vs119a", PasswordHash: HashPassword("password10"), FullName: "비젼119",
		Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	if err := users.Create(&vs); err != nil {
		t.Fatal(err)
	}
	vu, _ := users.GetByUsername("vs119a")
	vPick := doGetWith(t, e, "/projects/WP-O01/sites/pick", jwtCookieUser(t, vu))
	if vPick.Code != http.StatusSeeOther {
		t.Fatalf("비젼 pick %d", vPick.Code)
	}
	vSites := doGetWith(t, e, "/projects/WP-O01/sites", jwtCookieUser(t, vu))
	if vSites.Code != http.StatusOK || strings.Contains(vSites.Body.String(), "이 사업의 사이트를 등록할 권한이 없습니다") {
		t.Fatalf("비젼 sites %d", vSites.Code)
	}
}

func lastTaskID(t *testing.T, repo *repository.WBRepo, title string) string {
	t.Helper()
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for i := len(tasks) - 1; i >= 0; i-- {
		if tasks[i].Title == title {
			return tasks[i].TaskID
		}
	}
	t.Fatalf("업무 없음: %s", title)
	return ""
}
