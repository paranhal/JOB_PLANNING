package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestOrgAdminListCreateUpdateAndSwitch(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "org-http.db"))
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
	g.GET("/admin/orgs", h.Org.List, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs", h.Org.Create, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs/update", h.Org.Update, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs/switch", h.Org.Switch, h.Auth.RequireVisionOnly)

	ck := jwtCookie(t)
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}
	list := get()
	if list.Code != http.StatusOK {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "도서관사업팀") || !strings.Contains(body, "계정") {
		t.Fatalf("목록에 건수가 없다: %s", body)
	}
	if !strings.Contains(body, "조직 관리") {
		t.Fatal("사이드바 조직 관리가 없다")
	}

	form := url.Values{"org_name": {"다른팀"}, "short_name": {"이팀"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/orgs", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	list = get()
	if !strings.Contains(list.Body.String(), "O02") || !strings.Contains(list.Body.String(), "다른팀") {
		t.Fatalf("생성 후 목록: %s", list.Body.String())
	}

	upd := url.Values{"org_id": {"O02"}, "org_name": {"바꾼이름"}, "is_active": {"1"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/orgs/update", strings.NewReader(upd.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	list = get()
	if !strings.Contains(list.Body.String(), "바꾼이름") {
		t.Fatalf("수정 후: %s", list.Body.String())
	}

	sw := url.Values{"view_org_id": {"*"}, "return": {"/admin/orgs"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/orgs/switch", strings.NewReader(sw.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("switch %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "token" && c.Value != "" {
			ck = c
		}
	}
	list = get()
	if !strings.Contains(list.Body.String(), "전 조직") || !strings.Contains(list.Body.String(), "bg-amber-600") {
		t.Fatalf("전 조직 띠가 없다: %s", list.Body.String())
	}
}

func TestOrgAdminForbiddenForTech(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "org-tech.db"))
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
	g.GET("/admin/orgs", h.Org.List, h.Auth.RequireVisionOnly)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "조직 만들기") {
		t.Fatal("기술 계정이 조직 관리에 들어갔다")
	}
}

func mountOrgAdmin(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	v := h.Auth.RequireVisionOnly
	g.GET("/admin/orgs", h.Org.List, v)
	g.POST("/admin/orgs", h.Org.Create, v)
	g.GET("/admin/orgs/restore", h.Org.RestoreForm, v)
	g.POST("/admin/orgs/restore", h.Org.Restore, v)
	g.GET("/admin/orgs/:id/delete", h.Org.DeleteForm, v)
	g.POST("/admin/orgs/:id/delete", h.Org.Delete, v)
	g.GET("/admin/orgs/:id/purge", h.Org.PurgeForm, v)
	g.POST("/admin/orgs/:id/purge", h.Org.Purge, v)
	g.GET("/admin/orgs/:id/split", h.Org.SplitForm, v)
	g.POST("/admin/orgs/:id/split/preview", h.Org.SplitPreview, v)
	g.POST("/admin/orgs/:id/split", h.Org.Split, v)
}

func TestOrgDeleteRestoreBlockedAndSplit(t *testing.T) {
	root := t.TempDir()
	db, err := repository.InitDB(filepath.Join(root, "org-danger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, org_id) VALUES ('C1','고객','고객','O01')`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Org.dataDir = root
	mountOrgAdmin(e, h)
	ck := jwtCookie(t)

	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}

	if rec := post("/admin/orgs", url.Values{"org_name": {"분리팀"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	del := get("/admin/orgs/O02/delete")
	if del.Code != 200 || !strings.Contains(del.Body.String(), "계정") {
		t.Fatalf("delete form %d %s", del.Code, del.Body.String())
	}
	if rec := post("/admin/orgs/O02/delete", url.Values{
		"org_id": {"O02"}, "confirm_name": {"분리팀"}, "vision_password": {"admin"},
	}); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("hide %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	entries, _ := os.ReadDir(filepath.Join(root, "backups"))
	found := false
	var folder string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "조직삭제_") {
			found = true
			folder = e.Name()
			if _, err := os.Stat(filepath.Join(root, "backups", e.Name(), "org.db")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("조직 백업 폴더가 없다")
	}
	org, _ := repository.NewOrgRepo(db).Get("O02")
	if org == nil || org.IsActive {
		t.Fatal("숨기지 못했다")
	}
	rec := post("/admin/orgs/restore", url.Values{"folder": {folder}, "vision_password": {"admin"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("살아 있는 복구가 막히지 않음 loc=%s", rec.Header().Get("Location"))
	}

	if rec := post("/admin/orgs/O02/purge", url.Values{
		"org_id": {"O02"}, "confirm_name": {"분리팀"}, "vision_password": {"admin"},
	}); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("purge %s", rec.Header().Get("Location"))
	}
	if rec := post("/admin/orgs/restore", url.Values{"folder": {folder}, "vision_password": {"admin"}}); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("restore %s", rec.Header().Get("Location"))
	}

	if rec := post("/admin/orgs", url.Values{"org_name": {"세번째"}}); rec.Code != http.StatusSeeOther {
		t.Fatal("O03")
	}
	prev := post("/admin/orgs/O01/split/preview", url.Values{
		"org_id": {"O01"}, "to_org_id": {"O03"}, "customer_id": {"C1"},
	})
	if prev.Code != 200 || !strings.Contains(prev.Body.String(), "고객 1") {
		t.Fatalf("preview %d %s", prev.Code, prev.Body.String())
	}
	if rec := post("/admin/orgs/O01/split", url.Values{
		"org_id": {"O01"}, "to_org_id": {"O03"}, "customer_id": {"C1"}, "vision_password": {"admin"},
	}); rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("split %s", rec.Header().Get("Location"))
	}
	var orgID string
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE customer_id='C1'`).Scan(&orgID); err != nil || orgID != "O03" {
		t.Fatalf("split org=%s err=%v", orgID, err)
	}
}
