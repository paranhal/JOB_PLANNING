package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func Test95_63_HTTPProductCatTabCreateDupAndInUse(t *testing.T) {
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
	h.Backup.cfg.DataDir = dir
	h.Backup.cfg.DB = db
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/data", h.Backup.Page)
	g.POST("/admin/data/product-categories", h.Backup.ProductCatCreate)
	g.POST("/admin/data/product-categories/:id", h.Backup.ProductCatUpdate)
	g.POST("/admin/data/product-categories/:id/deactivate", h.Backup.ProductCatDeactivate)
	g.POST("/admin/data/product-categories/:id/delete", h.Backup.ProductCatDelete)
	g.GET("/codes", h.Code.List)
	ck := jwtCookie(t)

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/data?tab=product_cat", nil)
	req.AddCookie(ck)
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("탭 %d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "SPC007") || !strings.Contains(body, "방화벽") || !strings.Contains(body, "고유번호") {
		t.Fatalf("시드 표시 없음 %s", body[:min(800, len(body))])
	}

	form := url.Values{"code_value": {"cam"}, "code_name": {"카메라"}}
	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/admin/data/product-categories", strings.NewReader(form.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(ck)
	e.ServeHTTP(post, preq)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d", post.Code)
	}
	loc := post.Header().Get("Location")
	if !strings.Contains(loc, "SPC013") {
		t.Fatalf("고유번호 안내 %s", loc)
	}

	dup := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "/admin/data/product-categories", strings.NewReader(form.Encode()))
	dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	dreq.AddCookie(ck)
	e.ServeHTTP(dup, dreq)
	if !strings.Contains(dup.Header().Get("Location"), url.QueryEscape("같은 분류명이 있습니다")) &&
		!strings.Contains(dup.Header().Get("Location"), "같은") {
		t.Fatalf("동명 %s", dup.Header().Get("Location"))
	}

	codes := httptest.NewRecorder()
	creq := httptest.NewRequest(http.MethodGet, "/codes?group="+repository.ProductCategoryGroup, nil)
	creq.AddCookie(ck)
	e.ServeHTTP(codes, creq)
	if !strings.Contains(codes.Body.String(), "SPC013") {
		t.Fatal("코드관리 고유번호 없음")
	}

	lock := httptest.NewRecorder()
	lform := url.Values{"code_value": {"changed"}, "code_name": {"카메라"}, "sort_order": {"20"}, "is_active": {"1"}}
	lreq := httptest.NewRequest(http.MethodPost, "/admin/data/product-categories/SPC013", strings.NewReader(lform.Encode()))
	lreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	lreq.AddCookie(ck)
	e.ServeHTTP(lock, lreq)
	ldec, _ := url.QueryUnescape(lock.Header().Get("Location"))
	if !strings.Contains(ldec, "키는") {
		t.Fatalf("키변경 %s", ldec)
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_category) VALUES ('A1','C1','캠','cam')`); err != nil {
		t.Fatal(err)
	}
	del := httptest.NewRecorder()
	delreq := httptest.NewRequest(http.MethodPost, "/admin/data/product-categories/SPC013/delete", nil)
	delreq.AddCookie(ck)
	e.ServeHTTP(del, delreq)
	ddec, _ := url.QueryUnescape(del.Header().Get("Location"))
	if !strings.Contains(ddec, "사용 중") {
		t.Fatalf("사용중 삭제 %s", ddec)
	}
}
