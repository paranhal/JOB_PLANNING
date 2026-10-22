package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type projectOrgCookies struct {
	vs, cl, oa01, oa02, oa03 *http.Cookie
}

func newProjectOrgFilterHTTP(t *testing.T) (*echo.Echo, *sql.DB, *repository.ProjectRepo, projectOrgCookies) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "p95-2d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	vs := &model.User{Username: "vs-2d", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa01 := &model.User{Username: "oa01-2d", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	oa02 := &model.User{Username: "oa02-2d", PasswordHash: "x", FullName: "미디어장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	oa03 := &model.User{Username: "oa03-2d", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl-2d", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{vs, oa01, oa02, oa03, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	h.Project.backup.DataDir = filepath.Join(dir, "data")
	_ = os.MkdirAll(h.Project.backup.DataDir, 0755)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/registry", h.Project.Registry)
	g.GET("/projects/kind-review", h.Project.KindReview)
	g.GET("/projects/dashboard", h.Project.Dashboard)
	g.GET("/contracts/:id", h.Project.ContractShow)
	g.GET("/projects/:id/years", h.Project.Years)
	g.GET("/projects/:id/sites", h.Project.Sites)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id", h.Project.Update)
	g.POST("/projects/:id/archive", h.Project.Archive)
	g.POST("/projects/:id/delete", h.Project.Delete)
	g.GET("/workboard/register", h.Workboard.Register)
	repo := repository.NewProjectRepo(db)
	ck := projectOrgCookies{
		vs: jwtCookieUserClaims(t, vs), cl: jwtCookieUserClaims(t, cl),
		oa01: jwtCookieUserClaims(t, oa01), oa02: jwtCookieUserClaims(t, oa02), oa03: jwtCookieUserClaims(t, oa03),
	}
	return e, db, repo, ck
}

func seedOrgContractProject(t *testing.T, db *sql.DB, repo *repository.ProjectRepo, name, org string) string {
	t.Helper()
	p := &model.WorkProject{
		Name: name, PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1_000_000,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET org_id=? WHERE project_id=?`, org, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	return p.ProjectID
}

func getHTMLWith(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.Header.Set(echo.HeaderAccept, "text/html,application/xhtml+xml")
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func postFormWith(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie, accept, referer string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if accept != "" {
		req.Header.Set(echo.HeaderAccept, accept)
	} else {
		req.Header.Set(echo.HeaderAccept, "text/html,application/xhtml+xml")
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestRegistryOrgFilterAdminsAndCLevel(t *testing.T) {
	e, db, repo, ck := newProjectOrgFilterHTTP(t)
	seedOrgContractProject(t, db, repo, "O01고유A-95-2D", "O01")
	seedOrgContractProject(t, db, repo, "O01고유B-95-2D", "O01")
	seedOrgContractProject(t, db, repo, "O02고유A-95-2D", "O02")
	seedOrgContractProject(t, db, repo, "O02고유B-95-2D", "O02")
	seedOrgContractProject(t, db, repo, "O03고유A-95-2D", "O03")
	seedOrgContractProject(t, db, repo, "O03고유B-95-2D", "O03")

	oa03 := getHTMLWith(t, e, "/projects/registry", ck.oa03)
	if oa03.Code != http.StatusOK {
		t.Fatalf("oa03 %d", oa03.Code)
	}
	b := oa03.Body.String()
	if !strings.Contains(b, "O03고유A-95-2D") || !strings.Contains(b, "O03고유B-95-2D") {
		t.Fatal("SI 자기 팀 사업 없음")
	}
	if !strings.Contains(b, "O01고유A-95-2D") || !strings.Contains(b, "O02고유A-95-2D") {
		t.Fatal("95-2E SI 목록에 타 조직 사업 없음")
	}
	if !strings.Contains(b, "소속 조직") {
		t.Fatal("사업 목록 소속 조직 컬럼 없음")
	}

	oa01 := getHTMLWith(t, e, "/projects/registry", ck.oa01)
	if !strings.Contains(oa01.Body.String(), "O03고유A-95-2D") {
		t.Fatal("95-2E 도서관 목록에 SI 사업 없음")
	}
	oa02 := getHTMLWith(t, e, "/projects/registry", ck.oa02)
	if !strings.Contains(oa02.Body.String(), "O01고유A-95-2D") {
		t.Fatal("95-2E 미디어 목록에 도서관 사업 없음")
	}

	vs := getHTMLWith(t, e, "/projects/registry", ck.vs)
	if !strings.Contains(vs.Body.String(), "O01고유A-95-2D") || !strings.Contains(vs.Body.String(), "O03고유B-95-2D") {
		t.Fatal("시스템 관리자 전체 없음")
	}
	cl := getHTMLWith(t, e, "/projects/registry", ck.cl)
	if !strings.Contains(cl.Body.String(), "O02고유B-95-2D") || !strings.Contains(cl.Body.String(), "O01고유B-95-2D") {
		t.Fatal("C-Level 전체 없음")
	}
}

func TestSharedProjectViewOnlyNotOtherTeam(t *testing.T) {
	e, db, repo, ck := newProjectOrgFilterHTTP(t)
	sharedID := seedOrgContractProject(t, db, repo, "O01공개-95-2D", "O01")
	hiddenID := seedOrgContractProject(t, db, repo, "O01비공개-95-2D", "O01")
	_ = hiddenID
	if _, err := db.Exec(`UPDATE work_projects SET is_shared=1, share_from=?, share_to=? WHERE project_id=?`,
		"O01", "O02", sharedID); err != nil {
		t.Fatal(err)
	}

	oa02 := getHTMLWith(t, e, "/projects/registry", ck.oa02)
	if !strings.Contains(oa02.Body.String(), "O01공개-95-2D") {
		t.Fatal("공개 대상 팀에 안 보임")
	}
	if !strings.Contains(oa02.Body.String(), "O01비공개-95-2D") {
		t.Fatal("95-2E 전 직원 조회 — 비공개도 대상 팀에 보여야 한다")
	}
	show := getHTMLWith(t, e, "/projects/"+sharedID, ck.oa02)
	if show.Code != http.StatusOK {
		t.Fatalf("공개 상세 %d", show.Code)
	}
	edit := getHTMLWith(t, e, "/projects/"+sharedID+"/edit", ck.oa02)
	if edit.Code != http.StatusForbidden && edit.Code != http.StatusSeeOther {
		t.Fatalf("공개 수정 GET %d", edit.Code)
	}
	upd := postFormWith(t, e, "/projects/"+sharedID, url.Values{"name": {"해킹"}, "project_kind": {"maintenance"}}, ck.oa02, "text/html", "/projects/"+sharedID+"/edit")
	if upd.Code != http.StatusSeeOther && upd.Code != http.StatusForbidden {
		t.Fatalf("공개 수정 POST %d", upd.Code)
	}
	got, err := repo.Get(sharedID)
	if err != nil || got.Name != "O01공개-95-2D" {
		t.Fatalf("쓰기가 먹음 %+v %v", got, err)
	}

	oa03 := getHTMLWith(t, e, "/projects/registry", ck.oa03)
	if !strings.Contains(oa03.Body.String(), "O01공개-95-2D") {
		t.Fatal("95-2E 비대상 팀도 조회")
	}
	miss := getHTMLWith(t, e, "/projects/"+sharedID, ck.oa03)
	if miss.Code != http.StatusOK {
		t.Fatalf("95-2E 타 조직 상세 %d", miss.Code)
	}
	if strings.Contains(miss.Body.String(), `href="/projects/`+sharedID+`/edit"`) {
		t.Fatal("타 조직 상세에 수정 버튼")
	}
}

func TestOtherOrgProjectURLDenied95_12(t *testing.T) {
	e, db, repo, ck := newProjectOrgFilterHTTP(t)
	id := seedOrgContractProject(t, db, repo, "O01직접URL-95-2D", "O01")

	show := getHTMLWith(t, e, "/projects/"+id, ck.oa03)
	if show.Code != http.StatusOK {
		t.Fatalf("95-2E 상세 조회 %d", show.Code)
	}
	if strings.Contains(show.Body.String(), `href="/projects/`+id+`/edit"`) {
		t.Fatal("타 조직 수정 버튼")
	}
	years := getHTMLWith(t, e, "/projects/"+id+"/years", ck.oa03)
	if years.Code != http.StatusOK {
		t.Fatalf("연차 조회 %d", years.Code)
	}

	htmlPost := postFormWith(t, e, "/projects/"+id+"/archive", url.Values{}, ck.oa03, "text/html", "/projects/registry")
	if htmlPost.Code != http.StatusSeeOther {
		t.Fatalf("폼 POST %d", htmlPost.Code)
	}
	var flash *http.Cookie
	for _, c := range htmlPost.Result().Cookies() {
		if c.Name == "jp_flash_err" {
			flash = c
		}
	}
	if flash == nil || flash.Value == "" {
		t.Fatal("폼 오류 flash 없음")
	}

	js := postFormWith(t, e, "/projects/"+id+"/archive", url.Values{}, ck.oa03, echo.MIMEApplicationJSON, "")
	if js.Code != http.StatusForbidden && js.Code != http.StatusNotFound {
		t.Fatalf("JSON %d %s", js.Code, js.Body.String())
	}
	if loc := js.Header().Get("Location"); loc != "" {
		t.Fatal("JSON이 리다이렉트")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil && !strings.Contains(js.Body.String(), "message") {
		t.Fatalf("JSON 본문 %s", js.Body.String())
	}
}

func TestWorkPickProjectsStaysOrgIndependent(t *testing.T) {
	e, db, repo, ck := newProjectOrgFilterHTTP(t)
	seedOrgContractProject(t, db, repo, "O01픽목록-95-2D", "O01")
	reg := getHTMLWith(t, e, "/projects/registry", ck.oa03)
	if !strings.Contains(reg.Body.String(), "O01픽목록-95-2D") {
		t.Fatal("95-2E 등록 관리에 타 조직이 안 보인다")
	}
	pick := getHTMLWith(t, e, "/workboard/register?view=day", ck.oa03)
	if pick.Code != http.StatusOK {
		t.Fatalf("register %d", pick.Code)
	}
	if !strings.Contains(pick.Body.String(), "O01픽목록-95-2D") {
		t.Fatal("작업 선택 목록에서 타 조직 계약 사업이 빠졌다")
	}
}
