package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func jwtCookieVision(t *testing.T) *http.Cookie {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "vision-id", "username": "vision", "role": model.RoleVisionAdmin,
		"name": "비전", "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func newPG64Server(t *testing.T) (*echo.Echo, *repository.ProductGroupRepo, *http.Cookie) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/product-groups", h.ProductGroup.Page)
	g.POST("/product-groups", h.ProductGroup.Create)
	g.POST("/product-groups/:id/delete", h.ProductGroup.Delete)
	g.POST("/product-groups/:id/deactivate", h.ProductGroup.Deactivate)
	g.POST("/product-groups/bulk-delete", h.ProductGroup.BulkDelete)
	g.GET("/product-groups/import", h.ProductGroup.ImportForm)
	g.POST("/product-groups/import-preview", h.ProductGroup.ImportPreview)
	g.POST("/product-groups/import-apply", h.ProductGroup.ImportApply)
	g.GET("/product-groups/template.xlsx", h.ProductGroup.TemplateXLSX)
	g.GET("/assets/new", h.Asset.New)
	g.POST("/assets", h.Asset.Create)
	return e, repository.NewProductGroupRepo(db), jwtCookieVision(t)
}

func Test95_64_HTTPPageSeedNoPNGExcelAddAnd전선스위치(t *testing.T) {
	e, _, ck := newPG64Server(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/product-groups", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "제품군 관리") || !strings.Contains(body, "PG-0003") || !strings.Contains(body, "전선스위치") {
		t.Fatal("시안 시드 없음")
	}
	if strings.Contains(body, "PNG 다운로드") {
		t.Fatal("PNG 단추가 있다")
	}
	if !strings.Contains(body, "엑셀 업로드") || !strings.Contains(body, "제품군 추가") {
		t.Fatal("우상단 단추 없음")
	}
	if !strings.Contains(body, "열 너비 초기화") || !strings.Contains(body, "col-resize-handle-wide") || !strings.Contains(body, `data-table-cols="product_groups"`) {
		t.Fatal("열 너비 조절 없음")
	}
	if !strings.Contains(body, "table-scroll") {
		t.Fatal("가로 스크롤 래퍼 없음")
	}
}

func Test95_64_HTTPUnmappedBannerAndAssetFormCascade(t *testing.T) {
	e, _, ck := newPG64Server(t)
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "/assets/new", nil)
	preq.AddCookie(ck)
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK {
		t.Fatalf("form %d", page.Code)
	}
	b := page.Body.String()
	if !strings.Contains(b, "세부제품군") || !strings.Contains(b, "name=\"product_group_id\"") || !strings.Contains(b, "pgGroups") {
		t.Fatal("3단 연동 없음")
	}
}

func Test95_64_HTTPCreateAndImportPreview(t *testing.T) {
	e, repo, ck := newPG64Server(t)
	form := url.Values{"class_id": {"hw"}, "type_name": {"카메라"}, "detail_name": {"돔"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/product-groups", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d", rec.Code)
	}
	g, _ := repo.Get("PG-0017")
	if g == nil || g.TypeName != "카메라" {
		t.Fatalf("등록 %+v", g)
	}

	imp := httptest.NewRecorder()
	ireq := httptest.NewRequest(http.MethodGet, "/product-groups/import", nil)
	ireq.AddCookie(ck)
	e.ServeHTTP(imp, ireq)
	if imp.Code != http.StatusOK || !strings.Contains(imp.Body.String(), "양식 내려받기") {
		t.Fatalf("import form %d", imp.Code)
	}
	tpl := httptest.NewRecorder()
	treq := httptest.NewRequest(http.MethodGet, "/product-groups/template.xlsx", nil)
	treq.AddCookie(ck)
	e.ServeHTTP(tpl, treq)
	if tpl.Code != http.StatusOK || !bytes.Contains(tpl.Body.Bytes(), []byte("PK")) {
		t.Fatalf("template %d", tpl.Code)
	}
}
