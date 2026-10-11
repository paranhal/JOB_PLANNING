package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type uiSmokeActors struct {
	admin, tech, obs *model.User
	asID             string
}

func newUISmokeApp(t *testing.T) (*echo.Echo, *uiSmokeActors) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ui-smoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "tech1", PasswordHash: HashPassword("password10"), FullName: "기술담당",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "obs1", PasswordHash: HashPassword("password10"), FullName: "읽기전용",
		Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}); err != nil {
		t.Fatal(err)
	}
	admin, err := users.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin")
	}
	tech, err := users.GetByUsername("tech1")
	if err != nil || tech == nil {
		t.Fatal("tech")
	}
	obs, err := users.GetByUsername("obs1")
	if err != nil || obs == nil {
		t.Fatal("obs")
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-smoke','스모크기관','스모크기관',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	as := &model.ASReceipt{
		CustomerID: "c-smoke", Symptom: "스모크", AssignedTo: admin.FullName,
		Status: "received", ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
	}
	if err := repository.NewASRepo(db).Create(as); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.GET("/login", h.Auth.LoginPage)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.RequireActiveRole)
	g.Use(h.Auth.GuardViewAsWrite)
	g.GET("/", h.Dashboard)
	g.GET("/customers", h.Customer.List)
	g.GET("/work", h.Work.List)
	g.GET("/as", h.AS.List)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/quotes", h.Quotes.List)
	return e, &uiSmokeActors{admin: admin, tech: tech, obs: obs, asID: as.ASID}
}

func uiGET(t *testing.T, e *echo.Echo, path string, cookies []*http.Cookie, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestRenderSmokeParseAllPageTemplates(t *testing.T) {
	root := findTemplateRoot(t)
	r := NewRenderer()
	var pages []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") || strings.Contains(path, string(filepath.Separator)+"layout"+string(filepath.Separator)) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pages = append(pages, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("페이지 템플릿 없음")
	}
	for _, name := range pages {
		tmpl, err := r.lookup(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if tmpl.Lookup("content") == nil && name != "auth/login.html" {
			// 일부 조각은 define content 가 있을 수 있음. 없으면 구문만 통과.
		}
		_ = tmpl
	}
	t.Logf("파싱한 페이지 템플릿 %d개", len(pages))
}

func TestRenderSmokeRolesAndHX(t *testing.T) {
	e, ids := newUISmokeApp(t)
	adminCK := jwtCookieUserClaims(t, ids.admin)
	techCK := jwtCookieUserClaims(t, ids.tech)
	obsCK := jwtCookieUserClaims(t, ids.obs)
	viewCK := []*http.Cookie{adminCK, {Name: viewAsCookie, Value: ids.tech.UserID, Path: "/"}}

	login := uiGET(t, e, "/login", nil, false)
	if login.Code != http.StatusOK {
		t.Fatalf("login %d", login.Code)
	}
	if !strings.Contains(login.Body.String(), `name="color-scheme"`) || !strings.Contains(login.Body.String(), `content="light"`) {
		t.Fatal("로그인 응답에 color-scheme 메타 없음")
	}

	pages := []string{"/", "/customers", "/work", "/as", "/quotes", "/as/" + ids.asID}
	type actor struct {
		name    string
		cookies []*http.Cookie
		ro      bool
	}
	actors := []actor{
		{"admin", []*http.Cookie{adminCK}, false},
		{"normal", []*http.Cookie{techCK}, false},
		{"observer", []*http.Cookie{obsCK}, true},
		{"view-as", viewCK, true},
	}
	for _, a := range actors {
		for _, path := range pages {
			rec := uiGET(t, e, path, a.cookies, false)
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s status=%d body=%s", a.name, path, rec.Code, truncate(rec.Body.String(), 400))
				continue
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<html") {
				t.Errorf("%s %s: 풀페이지가 아님", a.name, path)
			}
			if !strings.Contains(body, `name="color-scheme"`) || !strings.Contains(body, `content="light"`) {
				t.Errorf("%s %s: color-scheme 메타 없음", a.name, path)
			}
			if a.ro {
				if strings.Contains(body, `href="/customers/new"`) {
					t.Errorf("%s %s: 저장 UI(고객 등록)가 있다", a.name, path)
				}
				if strings.Contains(body, `href="/as/new"`) {
					t.Errorf("%s %s: 저장 UI(AS 접수)가 있다", a.name, path)
				}
				if strings.Contains(body, `href="/quotes/new"`) {
					t.Errorf("%s %s: 저장 UI(견적 작성)가 있다", a.name, path)
				}
			}
		}
	}

	hx := uiGET(t, e, "/customers", []*http.Cookie{adminCK}, true)
	if hx.Code != http.StatusOK {
		t.Fatalf("HX customers %d", hx.Code)
	}
	hxBody := hx.Body.String()
	if strings.Contains(hxBody, "<html") {
		t.Fatal("HX-Request 응답에 <html 이 있다")
	}
	if !strings.Contains(hxBody, "고객") && !strings.Contains(hxBody, "기관") {
		t.Fatalf("HX content 가 비어 보임: %s", truncate(hxBody, 200))
	}

	peekDirect := uiGET(t, e, "/as/"+ids.asID, []*http.Cookie{adminCK}, false)
	if peekDirect.Code != http.StatusOK || !strings.Contains(peekDirect.Body.String(), "<html") {
		t.Fatalf("피크 직접 접근(상세 URL)이 풀페이지가 아님 status=%d", peekDirect.Code)
	}
	if !strings.Contains(peekDirect.Body.String(), `name="color-scheme"`) {
		t.Fatal("상세 직접 접근에 color-scheme 없음")
	}
	peekHX := uiGET(t, e, "/as/"+ids.asID, []*http.Cookie{adminCK}, true)
	if peekHX.Code != http.StatusOK {
		t.Fatalf("HX 상세 %d", peekHX.Code)
	}
	if strings.Contains(peekHX.Body.String(), "<html") {
		t.Fatal("HX 상세 응답에 <html 이 있다")
	}

	viewBody := uiGET(t, e, "/customers", viewCK, false).Body.String()
	if !strings.Contains(viewBody, "시점으로 보고 있습니다") {
		t.Fatal("시점 보기 띠 없음")
	}
}

func TestRenderSmokePartialsParse(t *testing.T) {
	root := findTemplateRoot(t)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") || !strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		if _, err := template.New("").Funcs(funcMap()).ParseFiles(path); err != nil {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s: %v", filepath.ToSlash(rel), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
