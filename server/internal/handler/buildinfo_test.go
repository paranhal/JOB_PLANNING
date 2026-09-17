package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestVersionJSONUnauthenticated(t *testing.T) {
	started := time.Date(2026, 9, 13, 14, 25, 0, 0, time.FixedZone("KST", 9*3600))
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 14:22", started)

	e := echo.New()
	e.GET("/version", VersionJSON)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "v2.37" || got["commit"] != "a1b2c3d" || got["built"] != "2026-09-13 14:22" {
		t.Fatalf("json=%v", got)
	}
	if got["started"] != "2026-09-13 14:25" {
		t.Fatalf("started=%q", got["started"])
	}
	if _, ok := got["history"]; ok {
		t.Fatal("/version 이 이력을 주면 안 된다")
	}
}

func TestVersionSkipsAuthMiddleware(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ver.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	SetBuildInfo("v2.37", "nogit", "2026-09-13 14:22", time.Now())
	h := New(db)
	e := echo.New()
	e.GET("/version", VersionJSON)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/", h.Dashboard)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("로그인 없이 /version 실패 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "v2.37" || got["commit"] != "nogit" {
		t.Fatalf("json=%v", got)
	}
}

func TestBuildBadgeCopyAndCacheBust(t *testing.T) {
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 14:22", time.Now())
	if got := badgeShort("v2.37", "2026-09-13 14:22"); got != "v2.37 · 09-13 14:22" {
		t.Fatalf("short=%q", got)
	}
	if got := badgeFull("v2.37", "a1b2c3d", "2026-09-13 14:22"); got != "v2.37 · 커밋 a1b2c3d · 빌드 2026-09-13 14:22 KST" {
		t.Fatalf("full=%q", got)
	}
	q1 := AssetQuery()
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 15:00", time.Now())
	q2 := AssetQuery()
	if q1 == q2 {
		t.Fatal("빌드 시각이 바뀌었는데 ?v= 가 같다")
	}
}

func TestBaseHTMLUsesInjectedVersion(t *testing.T) {
	body, err := os.ReadFile("web/templates/layout/base.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, ">v2.0<") {
		t.Fatal("사이드바에 v2.0 이 글자로 박혀 있다")
	}
	if !strings.Contains(s, "{{.BuildVersion}}") {
		t.Fatal("사이드바가 BuildVersion 을 쓰지 않는다")
	}
	if !strings.Contains(s, "{{.BuildBadgeShort}}") || !strings.Contains(s, "{{.BuildBadgeFull}}") {
		t.Fatal("배지가 빌드 값을 쓰지 않는다")
	}
	if !strings.Contains(s, `id="build-badge"`) {
		t.Fatal("오른쪽 아래 배지가 없다")
	}
	if !strings.Contains(s, "@media print") {
		t.Fatal("인쇄 시 배지 숨김이 없다")
	}
	if !strings.Contains(s, "/static/vendor/tailwind.css?v={{.AssetQuery}}") {
		t.Fatal("정적 파일에 빌드 값이 안 붙는다")
	}
}

func TestLoginHTMLHasCacheBustAndBadge(t *testing.T) {
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 14:22", time.Now())
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	e.GET("/login", h.Auth.LoginPage)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d", rec.Code)
	}
	body := rec.Body.String()
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control=%q", rec.Header().Get("Cache-Control"))
	}
	if strings.Contains(body, ">v2.0<") {
		t.Fatal("로그인 HTML 에 v2.0 이 남아 있다")
	}
	if !strings.Contains(body, "v2.37 · 09-13 14:22") {
		t.Fatal("배지 짧은 표시가 없다")
	}
	if !strings.Contains(body, "커밋 a1b2c3d") {
		t.Fatal("배지 전체(title)에 커밋이 없다")
	}
	q := AssetQuery()
	if !strings.Contains(body, "/static/vendor/tailwind.css?v="+q) {
		t.Fatalf("정적 주소에 ?v=%s 없음", q)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("두 번째 login status=%d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "고객지원시스템") {
		t.Fatal("두 번째 로그인 화면이 비었다")
	}
}

func TestLoginTemplateDoesNotPullKanbanPartials(t *testing.T) {
	files := templateFiles("auth/login.html")
	for _, f := range files {
		if strings.Contains(filepath.ToSlash(f), "/kanban/") {
			t.Fatalf("로그인 템플릿에 칸반이 붙으면 안 된다: %s", f)
		}
	}
}

func TestMainListensBeforeHousekeeping(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "go repository.RunWorkListHousekeeping(db)") {
		t.Fatal("기동 정리가 리슨을 막으면 로그인부터 느려진다")
	}
	idxHK := strings.Index(s, "go repository.RunWorkListHousekeeping(db)")
	idxStart := strings.Index(s, "e.Start")
	if idxHK < 0 || idxStart < 0 || idxHK > idxStart {
		t.Fatal("일일업무 정리는 e.Start 앞에서 백그라운드로 돌려야 한다")
	}
	if strings.Contains(s, "repository.RunWorkListHousekeeping(db)\n") &&
		!strings.Contains(s, "go repository.RunWorkListHousekeeping(db)") {
		t.Fatal("정리가 동기 호출로 남아 있다")
	}
	if !strings.Contains(s, "syscall.SIGTERM") || !strings.Contains(s, "wal_checkpoint(TRUNCATE)") {
		t.Fatal("SIGTERM 종료 시 WAL TRUNCATE 가 있어야 한다")
	}
	if strings.Contains(s, "defer db.Close()") {
		t.Fatal("db.Close 는 종료 경로에서 한 번만 호출한다")
	}
}

func TestDockerfileInjectsLdflags(t *testing.T) {
	b, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, need := range []string{
		"ARG VERSION=dev",
		"ARG COMMIT=unknown",
		"ARG BUILD_TIME=unknown",
		"-X 'main.buildVersion=${VERSION}'",
		"-X 'main.buildCommit=${COMMIT}'",
		"-X 'main.buildTime=${BUILD_TIME}'",
	} {
		if !strings.Contains(s, need) {
			t.Errorf("Dockerfile 에 %q 없음", need)
		}
	}
}

func TestDeployBuildScriptGitOptional(t *testing.T) {
	root := ".."
	sh, err := os.ReadFile(filepath.Join(root, "deploy", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(sh)
	if !strings.Contains(s, "nogit") {
		t.Fatal("build.sh 가 git 없을 때 nogit 을 쓰지 않는다")
	}
	if !strings.Contains(s, "--build-arg VERSION=") {
		t.Fatal("build.sh 가 VERSION 을 안 넣는다")
	}
	ver, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(ver)) != "v2.37" {
		t.Fatalf("VERSION=%q", ver)
	}
}

func TestSidebarAndBadgeShareOneValue(t *testing.T) {
	SetBuildInfo("v2.37", "deadbee", "2026-09-13 14:22", time.Now())
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "nav.db"))
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
	g.GET("/work", h.Work.List)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Count(body, "v2.37") < 2 {
		t.Fatal("사이드바와 배지가 같은 버전을 쓰지 않는다")
	}
	if strings.Contains(body, ">v2.0<") {
		t.Fatal("사이드바 v2.0 이 남아 있다")
	}
}
