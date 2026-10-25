package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestCompanyWeekly95_34DebugVisionOnly(t *testing.T) {
	_ = initTestAccessDB(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw34-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)

	week := parseWeekMonday("", time.Now()).AddDate(0, 0, -7).Format("2006-01-02")
	path := "/stats/company-weekly?week=" + week + "&org=O03&debug=1"

	orgRec := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, path, nil)
	oreq.AddCookie(jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	e.ServeHTTP(orgRec, oreq)
	if orgRec.Code != http.StatusOK && orgRec.Code != http.StatusForbidden {
		t.Fatalf("조직관리자 debug code=%d", orgRec.Code)
	}
	if strings.Contains(orgRec.Body.String(), `data-weekly-debug`) {
		t.Fatal("조직관리자에게 진단 표가 보인다")
	}

	visRec := httptest.NewRecorder()
	vreq := httptest.NewRequest(http.MethodGet, path, nil)
	vreq.AddCookie(jwtCookieOrg(t, model.RoleVisionAdmin, "O03"))
	e.ServeHTTP(visRec, vreq)
	if visRec.Code != http.StatusOK {
		t.Fatalf("비젼관리자 %d %s", visRec.Code, visRec.Body.String()[:min(400, visRec.Body.Len())])
	}
	if !strings.Contains(visRec.Body.String(), `data-weekly-debug`) && !strings.Contains(visRec.Body.String(), "제외 사유") {
		t.Fatalf("진단 표 없음: %s", visRec.Body.String()[:min(800, visRec.Body.Len())])
	}
}
