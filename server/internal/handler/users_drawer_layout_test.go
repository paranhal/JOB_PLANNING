package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

func TestUsersDrawerFormsShrinkZero(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("web", "templates", "auth", "users.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, `id="edit-form"`) || !strings.Contains(src, `id="del-form"`) {
		t.Fatal("edit-form/del-form 없음")
	}
	if !strings.Contains(src, `id="edit-form" method="POST" action="/users" novalidate class="flex flex-col min-h-full shrink-0"`) {
		t.Fatal("edit-form 에 shrink-0 이 없다")
	}
	if !strings.Contains(src, `id="del-form"`) || !strings.Contains(src, `shrink-0`) {
		t.Fatal("del-form shrink-0")
	}
	delAt := strings.Index(src, `id="del-form"`)
	if delAt < 0 || !strings.Contains(src[delAt:delAt+180], "shrink-0") {
		t.Fatal("del-form 클래스에 shrink-0 이 없다")
	}
}

func TestUsersPageVisionHasDeleteDrawer(t *testing.T) {
	e, users := newUserProfileApp(t)
	ck := profileVisionCookie(t, users)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="edit-form"`) || !strings.Contains(body, `id="del-form"`) {
		t.Fatal("비전 관리자 화면에 수정/삭제 서랍이 없다")
	}
	if !strings.Contains(body, "min-h-full shrink-0") {
		t.Fatal("렌더된 edit-form 에 shrink-0 이 없다")
	}
}
