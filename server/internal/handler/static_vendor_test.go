package handler

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

var vendorFiles = []struct {
	name string
	min  int64
}{
	{"tailwind.css", 10_000},
	{"htmx.min.js", 10_000},
	{"alpine.min.js", 10_000},
	{"chart.umd.min.js", 10_000},
}

func TestVendorAssetsPresentAndPinned(t *testing.T) {
	dir := filepath.Join("web", "static", "vendor")
	for _, f := range vendorFiles {
		p := filepath.Join(dir, f.name)
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("로컬 vendor 없음: %s (%v)", p, err)
		}
		if st.Size() < f.min {
			t.Fatalf("%s 크기가 너무 작음: %d", p, st.Size())
		}
	}
	ver, err := os.ReadFile(filepath.Join(dir, "VERSIONS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(ver)
	for _, need := range []string{"htmx.org", "2.0.3", "alpinejs", "3.14.8", "tailwindcss", "3.4.17"} {
		if !strings.Contains(s, need) {
			t.Errorf("VERSIONS.txt에 %q 없음", need)
		}
	}
	htmx, err := os.ReadFile(filepath.Join(dir, "htmx.min.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmx), `version:"2.0.3"`) {
		t.Error("htmx.min.js 가 2.0.3 이 아님")
	}
	alp, err := os.ReadFile(filepath.Join(dir, "alpine.min.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(alp), `version:"3.14.8"`) {
		t.Error("alpine.min.js 가 3.14.8 이 아님")
	}
	css, err := os.ReadFile(filepath.Join(dir, "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	cs := string(css)
	if !strings.Contains(cs, "tailwindcss v3.4.17") {
		t.Error("tailwind.css 가 빌드된 3.4.17 이 아님")
	}
	if strings.Contains(cs, "cdn.tailwindcss.com") {
		t.Error("Play CDN 흔적이 CSS에 남아 있음")
	}
}

func TestDockerfileCopiesWebStatic(t *testing.T) {
	b, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "COPY web/ web/") && !strings.Contains(s, "COPY web web") {
		t.Fatal("Dockerfile 이 web/ 을 이미지에 넣지 않음")
	}
	if strings.Contains(s, "cdn.tailwindcss.com") || strings.Contains(s, "unpkg.com") {
		t.Fatal("Dockerfile 이 CDN 을 받음")
	}
}

func TestTemplatesHaveNoExternalCDN(t *testing.T) {
	root := findTemplateRoot(t)
	banned := []string{
		"cdn.tailwindcss.com",
		"unpkg.com",
		"cdn.jsdelivr.net",
		"cdnjs.cloudflare.com",
		"googleapis.com",
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(b)
		for _, needle := range banned {
			if strings.Contains(body, needle) {
				t.Errorf("%s: 외부 CDN %q", path, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticVendorServedLocally(t *testing.T) {
	e := echo.New()
	e.Static("/static", "web/static")
	for _, f := range vendorFiles {
		req := httptest.NewRequest(http.MethodGet, "/static/vendor/"+f.name, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status=%d", f.name, rec.Code)
		}
		if rec.Body.Len() < int(f.min) {
			t.Errorf("%s: body=%d", f.name, rec.Body.Len())
		}
	}
}

func TestLoginHTMLUsesLocalVendor(t *testing.T) {
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
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, need := range []string{
		`/static/vendor/tailwind.css`,
		`/static/vendor/htmx.min.js`,
		`/static/vendor/alpine.min.js`,
	} {
		if !strings.Contains(body, need) {
			t.Errorf("로그인 HTML에 %s 없음", need)
		}
	}
	for _, ban := range []string{"cdn.tailwindcss.com", "unpkg.com", "cdn.jsdelivr.net"} {
		if strings.Contains(body, ban) {
			t.Errorf("로그인 HTML에 CDN %s", ban)
		}
	}
}

func TestTemplatesScriptAndStylesAreLocal(t *testing.T) {
	root := findTemplateRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			trim := strings.TrimSpace(line)
			if !(strings.Contains(trim, "<script") || strings.Contains(trim, "<link")) {
				continue
			}
			if strings.Contains(trim, "https://") || strings.Contains(trim, "http://") {
				t.Errorf("%s: 원격 자산 %s", path, trim)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
