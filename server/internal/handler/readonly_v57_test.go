package handler

import (
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

func TestReadOnlyAllowsGetBlocksPostExceptPassword(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	u := &model.User{
		Username: "ro1", PasswordHash: HashPassword("password10"), FullName: "읽기",
		Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, Mobile: "010-1111-2222",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/", h.Dashboard)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	ck := jwtCookieUserClaims(t, u)

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/account", nil)
	greq.AddCookie(ck)
	e.ServeHTTP(get, greq)
	if get.Code != http.StatusOK && get.Code != http.StatusSeeOther {
		t.Fatalf("GET %d", get.Code)
	}

	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader(url.Values{"title": {"x"}}.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(ck)
	e.ServeHTTP(post, preq)
	if post.Code == http.StatusOK {
		t.Fatal("읽기전용 POST 가 통과했다")
	}

	pw := httptest.NewRecorder()
	pwreq := httptest.NewRequest(http.MethodPost, "/account/password", strings.NewReader(url.Values{
		"current_password": {"password10"}, "new_password": {"password11"}, "confirm_password": {"password11"},
	}.Encode()))
	pwreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	pwreq.AddCookie(ck)
	e.ServeHTTP(pw, pwreq)
	if pw.Code == http.StatusForbidden {
		t.Fatalf("자기 비밀번호 변경이 막혔다 %d", pw.Code)
	}
}

func TestTechReadOnlyHidesWriteButtonsAndBlocksPost(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "tech-ro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	u := &model.User{
		Username: "techro", PasswordHash: HashPassword("password10"), FullName: "기술읽기",
		Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-ro','읽기기관','읽기기관',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/work", h.Work.List)
	g.GET("/as", h.AS.List)
	g.GET("/as/new", h.AS.New)
	g.POST("/as", h.AS.Create)
	ck := jwtCookieReadOnly(t, model.RoleTech)
	ckUser := jwtCookieUserClaims(t, u)

	for _, cookie := range []*http.Cookie{ck, ckUser} {
		asList := httptest.NewRecorder()
		areq := httptest.NewRequest(http.MethodGet, "/as", nil)
		areq.AddCookie(cookie)
		e.ServeHTTP(asList, areq)
		if asList.Code != http.StatusOK {
			t.Fatalf("GET /as %d", asList.Code)
		}
		if strings.Contains(asList.Body.String(), `href="/as/new"`) {
			t.Fatal("기술+읽기전용에 AS 접수 단추가 있다")
		}

		work := httptest.NewRecorder()
		wreq := httptest.NewRequest(http.MethodGet, "/work", nil)
		wreq.AddCookie(cookie)
		e.ServeHTTP(work, wreq)
		if work.Code != http.StatusOK {
			t.Fatalf("GET /work %d", work.Code)
		}

		asNew := httptest.NewRecorder()
		nreq := httptest.NewRequest(http.MethodGet, "/as/new", nil)
		nreq.AddCookie(cookie)
		e.ServeHTTP(asNew, nreq)
		if asNew.Code != http.StatusForbidden {
			t.Fatalf("GET /as/new %d", asNew.Code)
		}

		post := httptest.NewRecorder()
		preq := httptest.NewRequest(http.MethodPost, "/as", strings.NewReader(url.Values{
			"customer_id": {"c-ro"}, "symptom": {"읽기전용 접수"},
		}.Encode()))
		preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		preq.AddCookie(cookie)
		e.ServeHTTP(post, preq)
		loc := post.Header().Get("Location")
		if post.Code == http.StatusOK || strings.HasPrefix(loc, "/as/R") {
			t.Fatalf("기술+읽기전용 POST /as 가 열렸다 %d loc=%s", post.Code, loc)
		}
	}
}

func TestLegacyObserverJWTWithoutReadOnlyClaimStaysReadOnly(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "legacy-obs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_ = repository.NewUserRepo(db).EnsureAdmin(HashPassword("adminadmin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	ck := jwtCookieLegacyObserver(t)

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/account", nil)
	greq.AddCookie(ck)
	e.ServeHTTP(get, greq)
	if get.Code != http.StatusOK && get.Code != http.StatusSeeOther {
		t.Fatalf("GET %d loc=%s", get.Code, get.Header().Get("Location"))
	}
	if loc := get.Header().Get("Location"); strings.Contains(loc, "/login") {
		t.Fatal("옛 observer 토큰이 로그인으로 떨어졌다")
	}

	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader(url.Values{"title": {"x"}}.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(ck)
	e.ServeHTTP(post, preq)
	if post.Code == http.StatusOK {
		t.Fatal("is_readonly 없는 옛 observer JWT 가 POST 를 열었다")
	}
}
