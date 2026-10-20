package handler

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newEquipCatHTTP(t *testing.T) (*echo.Echo, *sql.DB, *http.Cookie, *http.Cookie, *http.Cookie) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "cathttp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs95c", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa95c", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	cl := &model.User{Username: "cl95c", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{vs, oa, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Project.backup.DataDir = filepath.Join(dir, "data")
	_ = os.MkdirAll(h.Project.backup.DataDir, 0755)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/equipment-categories", h.Project.EquipCats)
	g.GET("/projects/equipment-categories/template.xlsx", h.Project.EquipCatTemplateXLSX)
	g.GET("/projects/equipment-categories/export.xlsx", h.Project.EquipCatExportXLSX)
	g.POST("/projects/equipment-categories/import-preview", h.Project.EquipCatImportPreview)
	g.POST("/projects/equipment-categories/import-apply", h.Project.EquipCatImportApply)
	g.POST("/projects/equipment-categories/import-errors.xlsx", h.Project.EquipCatImportErrors)
	return e, db, jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, cl)
}

func TestEquipCatHTTPUploadDownloadPerm(t *testing.T) {
	e, db, vs, oa, cl := newEquipCatHTTP(t)
	page := doGetWith(t, e, "/projects/equipment-categories", vs)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "양식 다운로드") {
		t.Fatalf("page %d", page.Code)
	}
	tpl := doGetWith(t, e, "/projects/equipment-categories/template.xlsx", vs)
	if tpl.Code != http.StatusOK {
		t.Fatalf("tpl %d", tpl.Code)
	}
	if !strings.Contains(tpl.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("filename")
	}
	if tpl.Header().Get("Content-Length") == "" || tpl.Header().Get("Content-Length") == "0" {
		t.Fatal("length")
	}
	assertExcelizeRGB8(t, tpl.Body.Bytes())

	clGet := doGetWith(t, e, "/projects/equipment-categories/template.xlsx", cl)
	if clGet.Code != http.StatusOK {
		t.Fatalf("clevel download %d", clGet.Code)
	}
	raw, err := repository.BuildEquipCatWorkbook(repository.EquipCatGuideLines(),
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "디스플레이", "", "1", "Y"},
			{"O03", "NET", "네트워크", "", "1", "Y"},
		},
		[][]string{append([]string{}, []string{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"}...)},
		repository.EquipCatSampleRows())
	if err != nil {
		t.Fatal(err)
	}
	clPost := postEquipFile(t, e, "/projects/equipment-categories/import-preview", "file", "t.xlsx", raw, cl)
	if clPost.Code != http.StatusForbidden {
		t.Fatalf("clevel preview %d", clPost.Code)
	}
	oaPrev := postEquipFile(t, e, "/projects/equipment-categories/import-preview", "file", "t.xlsx", raw, oa)
	if oaPrev.Code != http.StatusOK || !strings.Contains(oaPrev.Body.String(), "쓸 수 없는 조직") {
		t.Fatalf("oa preview %d %s", oaPrev.Code, clipBody(oaPrev.Body.String()))
	}
	vsPrev := postEquipFile(t, e, "/projects/equipment-categories/import-preview", "file", "t.xlsx", raw, vs)
	if vsPrev.Code != http.StatusOK || !strings.Contains(vsPrev.Body.String(), "미리보기") {
		t.Fatalf("vs preview %d", vsPrev.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM equipment_categories`).Scan(&n)
	if n != 0 {
		t.Fatal("미리보기 쓰기")
	}
	app := postEquipFile(t, e, "/projects/equipment-categories/import-apply", "file", "t.xlsx", raw, vs)
	if app.Code != http.StatusOK {
		t.Fatalf("apply %d", app.Code)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM equipment_categories`).Scan(&n)
	if n != 2 {
		t.Fatalf("applied %d", n)
	}
	exp := doGetWith(t, e, "/projects/equipment-categories/export.xlsx", vs)
	if exp.Code != http.StatusOK {
		t.Fatalf("export %d", exp.Code)
	}
	assertExcelizeRGB8(t, exp.Body.Bytes())
	sales := jwtCookieUserClaims(t, &model.User{UserID: "USR-S95C", Username: "s95c", Role: model.RoleSales, IsActive: true, OrgID: model.OrgIDLibrary})
	if doGetWith(t, e, "/projects/equipment-categories/template.xlsx", sales).Code != http.StatusForbidden {
		t.Fatal("sales download")
	}
}
