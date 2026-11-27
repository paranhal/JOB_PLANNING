package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-229 ②: AS 접수 화면의 사업 입력란, 접수 때 고른 사업의 검증(이 기관이 등록된 사업만).
func Test95_229_ASFormProjectField(t *testing.T) {
	e, db, h := newAS151Server(t)
	seed151Projects(t, db)
	ck := as151OA(t, db)
	// 접수 신규 화면에 사업 입력란이 있다(기관을 고르면 후보를 불러오는 컴포넌트)
	e.GET("/as/new", h.AS.New, h.Auth.AuthMiddleware)
	body := doAS151Get(t, e, "/as/new", ck).Body.String()
	if !strings.Contains(body, "data-as-form-project") || !strings.Contains(body, `name="project_id"`) {
		t.Fatal("접수 화면에 사업 입력란이 없다")
	}
	// 기관이 사이트로 등록된 사업만 접수에서 고를 수 있다
	if r := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"C151"}}, ck); r.Code != http.StatusOK {
		t.Fatalf("link %d", r.Code)
	}
	if err := h.AS.checkReceiptProject(nil, "C151", "WP-A"); err != nil {
		t.Fatalf("등록된 사업이 거절됐다: %v", err)
	}
	if err := h.AS.checkReceiptProject(nil, "C151", "WP-B"); err == nil {
		t.Fatal("이 기관이 등록되지 않은 사업이 통과했다")
	}
	if err := h.AS.checkReceiptProject(nil, "C151", ""); err != nil {
		t.Fatalf("사업을 안 고른 경우는 통과해야 한다: %v", err)
	}
	// 접수 때 고른 사업은 규칙 판정보다 우선해 저장된다
	as := &model.ASReceipt{CustomerID: "C151", Symptom: "게이트", Status: "received", ProjectID: "WP-A", OrgID: model.OrgIDLibrary,
		AssignedTo: "도서관장", Urgency: "normal", Priority: "normal"}
	if err := repository.NewASRepo(db).Create(as); err != nil {
		t.Fatal(err)
	}
	var pid string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&pid)
	if pid != "WP-A" {
		t.Fatalf("접수 때 고른 사업 저장 %q", pid)
	}
}

// 95-229 ②: 정기점검 방문 조치 화면 — 후보마다 한 번 클릭 [연결], 추천, 연결 해제(방문에 먼저 쓰고 점검 업무에 복사).
func Test95_229_VisitProjectConnect(t *testing.T) {
	e, db, _ := newAS151Server(t)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.POST("/maintenance/visits/:visit_id/project", h.Maintenance.VisitProjectLink)
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES ('O03','O03','미디어사업팀','business',1,40)`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','가도서관','가도서관',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-M','점검 사업 A','active','O03','project',0),('WP-M2','점검 사업 B','active','O03','project',0)`,
		`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES ('S1','WP-M','c1','가도서관','planned',1,1,'O03'),('S2','WP-M2','c1','가도서관','planned',2,1,'O03')`,
		`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MPX',2026,'p','approved','O03')`,
		`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,entry_category,assignee,product_type,completed) VALUES ('V1','MPX','2026-12-02','c1',0,'normal','양기헌','KLAS',0),('V0','MPX','2026-11-02','c1',0,'normal','양기헌','KLAS',1),('V00','MPX','2026-11-09','c1',0,'normal','양기헌','KLAS',1)`,
		`UPDATE maintenance_visits SET project_id='WP-M' WHERE visit_id IN ('V0','V00')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	wb := repository.NewWBRepo(db)
	taskID, err := wb.EnsureMaintenanceTaskForVisit("V1")
	if err != nil || taskID == "" {
		t.Fatalf("점검 업무 %q %v", taskID, err)
	}
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	get := func(path string) string {
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(oa)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	post := func(path string, f url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(f.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(oa)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	page := get("/maintenance/visits/V1/action")
	for _, want := range []string{"data-visit-project-warn", `data-visit-project-cand="WP-M"`, `data-visit-project-cand="WP-M2"`, "data-visit-project-connect"} {
		if !strings.Contains(page, want) {
			t.Fatalf("방문 조치 화면에 %s 가 없다", want)
		}
	}
	if strings.Count(page, "data-visit-project-rec") != 1 || !strings.Contains(page, "점검 2건을 이 사업에 연결했습니다") {
		t.Fatalf("같은 담당자 이력(WP-M 2건) 추천이 없다: rec=%d", strings.Count(page, "data-visit-project-rec"))
	}
	// 후보가 아닌 사업은 연결되지 않는다
	if r := post("/maintenance/visits/V1/project", url.Values{"project_id": {"WP-NOPE"}}); !strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("없는 사업이 통과 %s", r.Header().Get("Location"))
	}
	// 한 번 클릭 연결 → 방문과 점검 업무에 같이 반영
	if r := post("/maintenance/visits/V1/project", url.Values{"project_id": {"WP-M"}}); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("연결 %s", r.Header().Get("Location"))
	}
	var vp, tp string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V1'`).Scan(&vp)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&tp)
	if vp != "WP-M" || tp != "WP-M" {
		t.Fatalf("연결 결과 방문=%q 업무=%q", vp, tp)
	}
	// 해제
	if r := post("/maintenance/visits/V1/project", url.Values{"project_id": {""}}); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("해제 %s", r.Header().Get("Location"))
	}
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V1'`).Scan(&vp)
	if vp != "" {
		t.Fatalf("해제 뒤 방문 사업 %q", vp)
	}
}

// 95-229 ④: 작업 등록 화면(project_required=1)은 사업 없이 저장할 수 없다. 이 값이 없는 옛 요청은 종전대로.
func Test95_229_ProjectWorkRequiresProject(t *testing.T) {
	v := setup191(t)
	v.wb.DB().Exec(`INSERT OR IGNORE INTO work_projects(project_id,name,status,org_id) VALUES ('WP-229','작업 사업','active','O01')`)
	slot229 := 0
	mk := func(title, project, required string) int {
		slot229++
		st := time198(slot229 + 3)
		f := url.Values{"work_type": {"project_work"}, "title": {title}, "work_subtype": {"other"}, "work_date": {"2026-11-12"}, "due_date": {"2026-11-12"},
			"start_time": {st}, "end_time": {st[:3] + "30"}, "duration_min": {"30"}, "assignee": {v.owner.FullName}}
		if project != "" {
			f.Set("project_id", project)
		}
		if required != "" {
			f.Set("project_required", required)
		}
		return v.do("POST", "/workboard/tasks", f, v.admin).Code
	}
	if c := mk("사업없음-화면", "", "1"); c != http.StatusBadRequest {
		t.Fatalf("등록 화면에서 사업 없이 %d want 400", c)
	}
	if taskIDByTitle198(t, v, "사업없음-화면") != "" {
		t.Fatal("거절됐는데 업무가 만들어졌다")
	}
	if c := mk("사업있음-화면", "WP-229", "1"); c != http.StatusSeeOther {
		t.Fatalf("사업 있는 등록 %d want 303", c)
	}
	if c := mk("사업없음-옛요청", "", ""); c != http.StatusSeeOther {
		t.Fatalf("옛 요청(필수 표시 없음) %d want 303(종전대로)", c)
	}
}

// 브라우저: AS 접수 화면 사업 입력란(1440/390) — 기관 선택 → 후보 불러오기, 1개면 자동 선택, 여러 개면 직접.
func Test95_229_BrowserASFormProject(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, h := newAS151Server(t)
	seed151Projects(t, db)
	ck := as151OA(t, db)
	e.GET("/as/new", h.AS.New, h.Auth.AuthMiddleware)
	e.GET("/workboard/projects/for-site", h.Workboard.ListProjectsForSite, h.Auth.AuthMiddleware)
	// C151: 사업 1개(WP-A), C151b: 사업 2개(WP-A, WP-B)
	for _, q := range [][]string{{"WP-A", "C151"}, {"WP-A", "C151b"}, {"WP-B", "C151b"}} {
		if r := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{"confirm": {"1"}, "project_id": {q[0]}, "customer_id": {q[1]}}, ck); r.Code != http.StatusOK {
			t.Fatalf("link %v %d", q, r.Code)
		}
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-229"))
	cmd := exec.Command("node", "95_229_as_form_project_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+ck.Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
}
