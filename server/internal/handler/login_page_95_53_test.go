package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

var baseTemplateCallRe = regexp.MustCompile(`\{\{\s*template\s+"([^"]+)"`)

func TestLoginPageRendersFormUnauthenticated(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "login53.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login/help", h.Auth.LoginHelp)
	e.GET("/version", VersionJSON)
	e.GET("/csrf", CSRFJSON)

	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(login, req)
	if login.Code != http.StatusOK {
		t.Fatalf("GET /login status=%d body=%s", login.Code, truncate(login.Body.String(), 400))
	}
	body := login.Body.String()
	if strings.Contains(body, `"message":"Internal Server Error"`) || strings.Contains(body, "no such template") {
		t.Fatalf("로그인 화면에 템플릿 오류: %s", truncate(body, 400))
	}
	if !strings.Contains(body, `name="username"`) || !strings.Contains(body, `action="/login"`) {
		t.Fatal("로그인 폼이 없다")
	}

	ver := httptest.NewRecorder()
	e.ServeHTTP(ver, httptest.NewRequest(http.MethodGet, "/version", nil))
	if ver.Code != http.StatusOK {
		t.Fatalf("GET /version %d", ver.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(ver.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["version"]; !ok {
		t.Fatalf("version JSON %s", ver.Body.String())
	}

	csrf := httptest.NewRecorder()
	e.ServeHTTP(csrf, httptest.NewRequest(http.MethodGet, "/csrf", nil))
	if csrf.Code != http.StatusOK {
		t.Fatalf("GET /csrf %d", csrf.Code)
	}

	help := httptest.NewRecorder()
	form := url.Values{"name": {"홍길동"}, "mobile": {"01012345678"}, "kind": {"unknown_id"}}
	hreq := httptest.NewRequest(http.MethodPost, "/login/help", strings.NewReader(form.Encode()))
	hreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(help, hreq)
	if help.Code != http.StatusOK {
		t.Fatalf("POST /login/help %d body=%s", help.Code, truncate(help.Body.String(), 400))
	}
	if !strings.Contains(help.Body.String(), `name="username"`) {
		t.Fatal("도움 요청 후 로그인 화면이 아니다")
	}
}

func TestLoginTemplateSetIncludesBasePartials(t *testing.T) {
	files := templateFiles("auth/login.html")
	joined := strings.Join(files, "\n")
	if !strings.Contains(filepath.ToSlash(joined), "_partials/login_briefing.html") {
		t.Fatalf("로그인 세트에 login_briefing 없음: %v", files)
	}
	if strings.Contains(joined, "/kanban/") {
		t.Fatalf("로그인 템플릿에 칸반이 붙으면 안 된다: %v", files)
	}
	tmpl, err := NewRenderer().lookup("auth/login.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base", "content", "nav_tree", "login_briefing"} {
		if tmpl.Lookup(name) == nil {
			t.Errorf("정의 없음: %s", name)
		}
	}
}

func TestBaseLayoutTemplateCallsAreDefinedForEveryPage(t *testing.T) {
	root := findTemplateRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "layout", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	seen := map[string]bool{}
	for _, m := range baseTemplateCallRe.FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		calls = append(calls, name)
	}
	if !seen["login_briefing"] || !seen["nav_tree"] || !seen["content"] {
		t.Fatalf("base 호출 목록이 부족하다: %v", calls)
	}

	r := NewRenderer()
	var pages []string
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") || strings.HasPrefix(info.Name(), "tab_") || info.Name() == "modal.html" {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+"layout"+string(filepath.Separator)) ||
			strings.Contains(path, string(filepath.Separator)+"_partials"+string(filepath.Separator)) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pages = append(pages, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	loginSeen := false
	for _, name := range pages {
		if name == "auth/login.html" {
			loginSeen = true
		}
		tmpl, err := r.lookup(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, call := range calls {
			if tmpl.Lookup(call) == nil {
				t.Errorf("%s: base 가 호출하는 %q 정의 없음", name, call)
			}
		}
	}
	if !loginSeen {
		t.Fatal("auth/login.html 이 전수 검사에서 빠졌다")
	}
}

func TestOmittingLoginBriefingLeavesTemplateUndefined(t *testing.T) {
	files := templateFiles("auth/login.html")
	var slim []string
	for _, f := range files {
		if strings.Contains(filepath.ToSlash(f), "login_briefing") {
			continue
		}
		slim = append(slim, f)
	}
	if len(slim) == len(files) {
		t.Fatal("login_briefing 파일이 세트에 없어 누락 재현을 못한다")
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(slim...)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Lookup("login_briefing") != nil {
		t.Fatal("login_briefing 파일을 뺐는데 정의가 남아 있다")
	}
}
