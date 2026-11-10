package handler

import (
	"database/sql"
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

func setup95_164(t *testing.T) (*echo.Echo, *repository.ProjectRepo, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "p164.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','SI사업팀','business',1,30),('O03','O03','미디어사업팀','business',1,40)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Project.backup.DataDir = filepath.Join(dir, "data")
	_ = os.MkdirAll(h.Project.backup.DataDir, 0755)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id", h.Project.Update)
	g.GET("/projects/:id", h.Project.Show)
	return e, repository.NewProjectRepo(db), db
}

func create95_164Project(t *testing.T, repo *repository.ProjectRepo, org string) *model.WorkProject {
	t.Helper()
	p := &model.WorkProject{
		Name: "이관사유칸164", OrgID: org, Status: model.WBProjectActive,
		ProjectKind: model.ProjectKindBuild, ProjectKindSet: 1, IsPaid: true,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func vision164(t *testing.T) *http.Cookie {
	t.Helper()
	return jwtCookieOrg(t, model.RoleVisionAdmin, "O01")
}

func postProjectEdit(t *testing.T, e *echo.Echo, id string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doFormWith(t, e, "/projects/"+id, form, ck)
}

func Test95_164_MoveReasonKeptOnError(t *testing.T) {
	e, repo, _ := setup95_164(t)
	p := create95_164Project(t, repo, "O01")
	ck := vision164(t)
	form := url.Values{
		"name": {p.Name}, "org_id": {"O02"}, "project_kind": {model.ProjectKindBuild},
		"is_paid": {"1"}, "status": {model.WBProjectActive},
		"keep_prev_related": {"1"},
	}
	rec := postProjectEdit(t, e, p.ProjectID, form, ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("사유없음 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "이관 사유를 입력하세요") {
		t.Fatalf("오류 문구 %s", clipBody(body))
	}
	if !strings.Contains(body, `value="O02" selected`) {
		t.Fatal("대상 팀 유지")
	}
	if !strings.Contains(body, `name="org_move_reason"`) {
		t.Fatal("사유 칸 없음")
	}
	if strings.Contains(body, `name="move_open_tasks" value="1" checked`) {
		t.Fatal("미체크 미완료 이관이 다시 켜짐")
	}
	if !strings.Contains(body, `name="keep_prev_related"`) {
		t.Fatal("유관 유지 체크 없음")
	}

	got, _ := repo.Get(p.ProjectID)
	if got.OrgID != "O01" {
		t.Fatalf("실패 후 DB org=%s", got.OrgID)
	}

	if !strings.Contains(body, "orgTo") || strings.Contains(body, "querySelector('[name=org_id]')") {
		t.Fatal("이관 박스 반응형 상태 아님")
	}

	form.Set("org_move_reason", "팀 이관 사유164")
	form.Set("move_open_tasks", "1")
	form.Set("drop_foreign_assign", "1")
	ok := postProjectEdit(t, e, p.ProjectID, form, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("이관 %d %s", ok.Code, clipBody(ok.Body.String()))
	}
	got, _ = repo.Get(p.ProjectID)
	if got.OrgID != "O02" {
		t.Fatalf("이관 후 org=%s", got.OrgID)
	}
}

func Test95_164_TransferPermissionRegression(t *testing.T) {
	e, repo, _ := setup95_164(t)
	p := create95_164Project(t, repo, "O01")
	tech := jwtCookieOrg(t, model.RoleTech, "O01")
	edit := doGetWith(t, e, "/projects/"+p.ProjectID+"/edit", tech)
	if edit.Code != http.StatusOK {
		t.Fatalf("기술 GET %d", edit.Code)
	}
	if strings.Contains(edit.Body.String(), `name="org_move_reason"`) {
		t.Fatal("기술직에 이관 칸")
	}
	foreign := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	rec := postProjectEdit(t, e, p.ProjectID, url.Values{
		"name": {p.Name}, "org_id": {"O02"}, "org_move_reason": {"무단이관"},
		"project_kind": {model.ProjectKindBuild}, "is_paid": {"1"}, "status": {model.WBProjectActive},
	}, foreign)
	got, _ := repo.Get(p.ProjectID)
	if got.OrgID != "O01" {
		t.Fatalf("타팀 이관됨 org=%s code=%d", got.OrgID, rec.Code)
	}
	home := jwtCookieOrg(t, model.RoleOrgAdmin, "O01")
	homeEdit := doGetWith(t, e, "/projects/"+p.ProjectID+"/edit", home)
	if homeEdit.Code != http.StatusOK || !strings.Contains(homeEdit.Body.String(), `name="org_move_reason"`) {
		t.Fatalf("주관 팀장 이관 칸 %d", homeEdit.Code)
	}
}

func Test95_164_BrowserOrgMoveReason(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, repo, _ := setup95_164(t)
	p := create95_164Project(t, repo, "O01")
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-164")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_164_org_move_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+vision164(t).Value,
		"PROJECT_ID="+p.ProjectID,
		"FROM_ORG=O01",
		"TO_ORG=O02",
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}
