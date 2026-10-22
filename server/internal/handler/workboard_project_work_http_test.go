package handler

import (
	"database/sql"
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

func newProjectWorkServer(t *testing.T) (*echo.Echo, *repository.WBRepo, *repository.UserRepo) {
	t.Helper()
	e, repo, users, _ := newProjectWorkHarness(t)
	return e, repo, users
}

func newProjectWorkHarness(t *testing.T) (*echo.Echo, *repository.WBRepo, *repository.UserRepo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "pw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30),
		('O04','O04','빈사업팀','business',1,40)`); err != nil {
		t.Fatal(err)
	}
	u3 := model.User{Username: "oa03", PasswordHash: HashPassword("password10"), FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(&u3); err != nil {
		t.Fatal(err)
	}
	u4 := model.User{Username: "oa04", PasswordHash: HashPassword("password10"), FullName: "빈관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O04", ProfileDone: true}
	if err := users.Create(&u4); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP-O01','도서관공사','active',?,'construction',1),
		      ('WP-O03','SI구축','active','O03','build',1),
		      ('WP-hold','보류사업','hold','O03','build',1),
		      ('WP-done','종료사업','complete','O03','build',1),
		      ('WP-arch','보관사업','archived','O03','build',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,has_parent,org_code,addr_sigungu,party_kind)
		VALUES('C-edu','충남교육청','충남교육청',1,?,0,'','충남','customer'),
		      ('C-high','계룡고','계룡고등학교',1,?,1,'GYE001','충남','customer'),
		      ('C-mid','계룡중','계룡중학교',1,?,1,'GYE002','충남','customer'),
		      ('C-oth','타조직교','타조직교',1,'O03',0,'OTH1','대전','customer'),
		      ('C-off','비활성교','비활성교',0,?,0,'','충남','customer'),
		      ('C-pct','퍼센트%고','퍼센트%고',1,?,0,'','충남','customer'),
		      ('C-und','밑줄_고','밑줄_고',1,?,0,'','충남','customer')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET parent_customer_id='C-edu' WHERE customer_id IN ('C-high','C-mid')`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 25; i++ {
		id := fmt.Sprintf("C-sr%02d", i)
		name := fmt.Sprintf("검색학교%02d", i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
			VALUES(?,?,?,1,?,'customer')`, id, name, name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,region,status,org_id,is_active)
		VALUES('S1','WP-O01','C-high','계룡고','충남','in_progress',?,1),
		      ('S2','WP-O01','C-mid','계룡중','충남','in_progress',?,1),
		      ('SX','WP-O01','C-off','제외교','충남','excluded',?,1),
		      ('S3','WP-O03','C-oth','타조직교','대전','in_progress','O03',1)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/workboard/kanban", h.Workboard.Kanban)
	g.GET("/workboard/tasks", h.Workboard.List)
	g.GET("/workboard/project-sites", h.Workboard.SearchProjectWorkSites)
	g.GET("/workboard/work-sites/search", h.Workboard.SearchWorkTaskSites)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/workboard/tasks/:id/edit", h.Workboard.EditTask)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	g.POST("/workboard/tasks/:id/refill-group", h.Workboard.RefillTaskGroup)
	g.GET("/workboard/tasks/:id/action", h.Workboard.ShowProjectWorkAction)
	g.POST("/workboard/tasks/:id/action", h.Workboard.SaveProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/hold", h.Workboard.HoldProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/transfer", h.Workboard.TransferProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/cancel", h.Workboard.CancelProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/hold/release", h.Workboard.ReleaseProjectWorkHold)
	g.POST("/workboard/tasks/:id/action/unlock-edit", h.Workboard.UnlockProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/project", h.Workboard.LinkProjectWorkAction)
	g.POST("/workboard/tasks/:id/action/sites", h.Workboard.ImportProjectWorkSites)
	g.POST("/attachments", h.Attachment.Upload)
	g.GET("/attachments/:id", h.Attachment.Download)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)
	g.GET("/projects/:id/sites", h.Project.Sites)
	h.Attachment.uploadDir = filepath.Join(t.TempDir(), "uploads")
	return e, repository.NewWBRepo(db), users, db
}

func doFormUser(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
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
	if !strings.Contains(body, "학교·기관명을 2글자 이상 입력") {
		t.Fatal("사이트 검색 안내 없음")
	}
	if strings.Contains(body, "등록된 사업이 없습니다") {
		t.Fatal("O01 사업 목록이 있는데 빈 안내가 나옴")
	}
	if !strings.Contains(body, `data-kind="construction"`) {
		t.Fatal("사업 유형 제안 data-kind 없음")
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
		"site_id":      {"C-high", "C-mid"},
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
	if !sites[0].OnProject || !sites[1].OnProject {
		t.Fatalf("사업 대상 배지 %+v", sites)
	}
	cal := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(cal.Body.String(), "계룡고") {
		t.Fatal("일정표 카드에 사이트 없음")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show.Body.String(), "대상 사이트") || !strings.Contains(show.Body.String(), "계룡고") {
		t.Fatal("상세에 대상 사이트 없음")
	}

	dead := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"비활성"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "site_id": {"C-off"},
	})
	if dead.Code != http.StatusBadRequest {
		t.Fatalf("비활성 status=%d", dead.Code)
	}
	missing := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"없음"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "site_id": {"NO-SUCH"},
	})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("없는 고객 status=%d", missing.Code)
	}
	dupID := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"중복"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "site_id": {"C-high", "C-high"},
	})
	if dupID.Code != http.StatusBadRequest {
		t.Fatalf("중복 status=%d", dupID.Code)
	}
	tooMany := url.Values{
		"work_type": {"project_work"}, "title": {"21곳"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
	}
	for i := 1; i <= 21; i++ {
		tooMany.Add("site_id", fmt.Sprintf("C-sr%02d", i))
	}
	if got := doForm(t, e, "/workboard/tasks", tooMany); got.Code != http.StatusBadRequest {
		t.Fatalf("21곳 status=%d", got.Code)
	}
	cross := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"타조직"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O03"},
	})
	if cross.Code != http.StatusSeeOther {
		t.Fatalf("타 조직 사업 status=%d", cross.Code)
	}

	q1 := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("계"))
	var empty []repository.WorkTaskSite
	_ = json.Unmarshal(q1.Body.Bytes(), &empty)
	if len(empty) != 0 {
		t.Fatalf("1글자 %d건", len(empty))
	}
	q2 := doGet(t, e, "/workboard/work-sites/search?project_id=WP-O01&q="+url.QueryEscape("계룡"))
	raw := q2.Body.Bytes()
	if strings.Contains(string(raw), "phone") || strings.Contains(string(raw), "amount") || strings.Contains(string(raw), "main_phone") || strings.Contains(string(raw), "customer_id") {
		t.Fatal("검색 응답에 연락처·금액·customer_id")
	}
	var got []repository.WorkTaskSite
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("계룡 후보 %d", len(got))
	}
	byID := map[string]repository.WorkTaskSite{}
	for _, s := range got {
		byID[s.SiteID] = s
		if s.ParentName != "충남교육청" || s.Region != "충남" {
			t.Fatalf("최소정보 %+v", s)
		}
	}
	if !byID["C-high"].OnProject || !byID["C-mid"].OnProject {
		t.Fatalf("배지 %+v", got)
	}
	codeQ := doGet(t, e, "/workboard/work-sites/search?q=GYE001")
	var byCode []repository.WorkTaskSite
	_ = json.Unmarshal(codeQ.Body.Bytes(), &byCode)
	if len(byCode) != 1 || byCode[0].SiteID != "C-high" {
		t.Fatalf("코드 검색 %+v", byCode)
	}
	parentQ := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("충남교육"))
	var byParent []repository.WorkTaskSite
	_ = json.Unmarshal(parentQ.Body.Bytes(), &byParent)
	if len(byParent) < 2 {
		t.Fatalf("상위기관 검색 %d", len(byParent))
	}
	regionQ := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("대전"))
	var byRegion []repository.WorkTaskSite
	_ = json.Unmarshal(regionQ.Body.Bytes(), &byRegion)
	if len(byRegion) != 1 || byRegion[0].SiteID != "C-oth" {
		t.Fatalf("지역 검색 %+v", byRegion)
	}
	esc := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("퍼센트%"))
	var escHits []repository.WorkTaskSite
	_ = json.Unmarshal(esc.Body.Bytes(), &escHits)
	if len(escHits) != 1 || escHits[0].SiteID != "C-pct" {
		t.Fatalf("%% 이스케이프 %+v", escHits)
	}
	und := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("밑줄_"))
	var undHits []repository.WorkTaskSite
	_ = json.Unmarshal(und.Body.Bytes(), &undHits)
	if len(undHits) != 1 || undHits[0].SiteID != "C-und" {
		t.Fatalf("_ 이스케이프 %+v", undHits)
	}
	lim := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("검색학교"))
	var many []repository.WorkTaskSite
	_ = json.Unmarshal(lim.Body.Bytes(), &many)
	if len(many) != 20 {
		t.Fatalf("20건 상한 %d", len(many))
	}
	offQ := doGet(t, e, "/workboard/work-sites/search?q="+url.QueryEscape("비활성"))
	var offHits []repository.WorkTaskSite
	_ = json.Unmarshal(offQ.Body.Bytes(), &offHits)
	if len(offHits) != 0 {
		t.Fatalf("비활성 검색 %+v", offHits)
	}

	unlinked := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"외부기관"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O01"}, "site_id": {"C-oth"},
	})
	if unlinked.Code != http.StatusSeeOther {
		t.Fatalf("사업 미대상 기관 저장 %d", unlinked.Code)
	}

	upd := doForm(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"scope": {"register"}, "title": {"설치"}, "work_date": {"2026-10-21"},
		"due_date": {"2026-10-21"}, "start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee": {"관리자"}, "status": {"complete"}, "work_subtype": {"construction"},
		"project_id": {"WP-O01"}, "site_id": {"C-high"}, "complete_note": {"배선 완료"},
	})
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("수정 %d", upd.Code)
	}
	gotT, _ := repo.GetTask(id)
	if gotT.WorkSubtype != model.WBWorkSubtypeConstruction || gotT.CompleteNote != "배선 완료" || gotT.Status != model.WBTaskComplete {
		t.Fatalf("after subtype=%s note=%s status=%s", gotT.WorkSubtype, gotT.CompleteNote, gotT.Status)
	}
	left, _ := repo.ListTaskSites(id)
	if len(left) != 1 || left[0].SiteID != "C-high" {
		t.Fatalf("사이트 제거 후 %+v", left)
	}
	hist, err := repo.ListSiteWorksByProject("WP-O01")
	if err != nil || len(hist["S1"]) != 1 || hist["S1"][0].Line() != "공사 · 설치 · 완료" {
		t.Fatalf("이력 %+v err=%v", hist["S1"], err)
	}
	if len(hist["S2"]) != 0 {
		t.Fatalf("제거한 S2 이력 %+v", hist["S2"])
	}
	sitesPage := doGet(t, e, "/projects/WP-O01/sites")
	if sitesPage.Code != http.StatusOK {
		t.Fatalf("사이트 화면 %d", sitesPage.Code)
	}
	body := sitesPage.Body.String()
	if !strings.Contains(body, "작업 이력") || !strings.Contains(body, "공사 · 설치 · 완료") || !strings.Contains(body, "/workboard/tasks/"+id) {
		t.Fatal("사이트 행 작업 이력 없음")
	}
	if !strings.Contains(body, "설치완료") {
		t.Fatal("완료 연동 설치완료 없음")
	}
}

func TestProjectWorkOrgIsolationSearch(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	oa03, err := users.GetByUsername("oa03")
	if err != nil || oa03 == nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/workboard/work-sites/search?q="+url.QueryEscape("계룡"), nil)
	req.AddCookie(jwtCookieUser(t, oa03))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("O03 전체 검색 status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got []repository.WorkTaskSite
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("O03이 계룡 검색 %d", len(got))
	}
	save := doFormUser(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"교차사이트"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"SI관리자"}, "site_id": {"C-high", "C-oth"},
	}, jwtCookieUser(t, oa03))
	if save.Code != http.StatusSeeOther {
		t.Fatalf("O03 사업없이 저장 %d %s", save.Code, save.Body.String())
	}
	tasks, _ := repo.ListTasks()
	var id, pid string
	for _, tk := range tasks {
		if tk.Title == "교차사이트" {
			id, pid = tk.TaskID, tk.ProjectID
		}
	}
	if id == "" || pid != "" {
		t.Fatalf("task=%s project=%s", id, pid)
	}
	sites, err := repo.ListTaskSites(id)
	if err != nil || len(sites) != 2 {
		t.Fatalf("sites=%d err=%v", len(sites), err)
	}

	oa04, err := users.GetByUsername("oa04")
	if err != nil || oa04 == nil {
		t.Fatal(err)
	}
	empty := httptest.NewRequest(http.MethodGet, "/workboard/register?view=day", nil)
	empty.AddCookie(jwtCookieUser(t, oa04))
	emptyRec := httptest.NewRecorder()
	e.ServeHTTP(emptyRec, empty)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("O04 register status=%d", emptyRec.Code)
	}
	if strings.Contains(emptyRec.Body.String(), "등록된 사업이 없습니다") {
		t.Fatal("조직 무관 목록인데 빈 안내")
	}

	cl := httptest.NewRequest(http.MethodGet, "/workboard/work-sites/search?q="+url.QueryEscape("계룡"), nil)
	cl.AddCookie(jwtCookieRole(t, model.RoleCLevel))
	clRec := httptest.NewRecorder()
	e.ServeHTTP(clRec, cl)
	if clRec.Code != http.StatusForbidden {
		t.Fatalf("C-Level 검색 %d", clRec.Code)
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

	roSearch := httptest.NewRequest(http.MethodGet, "/workboard/work-sites/search?q="+url.QueryEscape("계룡"), nil)
	roSearch.AddCookie(jwtCookieReadOnly(t, "tech"))
	roSRec := httptest.NewRecorder()
	e.ServeHTTP(roSRec, roSearch)
	if roSRec.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 검색 %d", roSRec.Code)
	}
}
