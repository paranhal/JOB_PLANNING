package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_117_PickerDropdownPosition(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p117.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1),
		       ('WP-O02','미디어사업','active','O02',0,1)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	sup := &model.User{
		Username: "sup01", PasswordHash: HashPassword("password10"), FullName: "지원일",
		Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(sup); err != nil {
		t.Fatal(err)
	}
	gotSup, err := users.GetByUsername("sup01")
	if err != nil || gotSup == nil {
		t.Fatal(err)
	}
	oa, err := users.GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_117_picker_pos_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, me).Value,
		"AUTH_COOKIE_OA="+jwtCookieVerified(t, oa).Value,
		"AUTH_COOKIE_SUP="+jwtCookieVerified(t, gotSup).Value,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}
