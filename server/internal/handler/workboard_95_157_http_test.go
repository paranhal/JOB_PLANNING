package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_157_T1_SharedDismissWiring(t *testing.T) {
	root := findTemplateRoot(t)
	cust := string(mustRead(t, filepath.Join(root, "_customer_combo.html")))
	proj := string(mustRead(t, filepath.Join(root, "workboard", "_project_combo.html")))
	pick := string(mustRead(t, filepath.Join(root, "_partials", "project_picker.html")))
	base := string(mustRead(t, filepath.Join(root, "layout", "base.html")))
	if !strings.Contains(base, "combobox_dismiss.js") {
		t.Fatal("공통 닫기 스크립트 없음")
	}
	for name, src := range map[string]string{"customer": cust, "project": proj, "picker": pick} {
		if !strings.Contains(src, "data-jp-combo") {
			t.Fatalf("%s data-jp-combo 없음", name)
		}
		if strings.Contains(src, `@keydown.escape.window="open=false"`) {
			t.Fatalf("%s 가 window Esc 로만 닫는다", name)
		}
		if !strings.Contains(src, "comboToggleField") {
			t.Fatalf("%s 재클릭 토글 없음", name)
		}
		if !strings.Contains(src, "comboOnTab") {
			t.Fatalf("%s Tab 닫기 없음", name)
		}
	}
}

func Test95_157_T2_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "combobox_dismiss_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
	cmd2 := exec.Command("node", "project_picker_test.js")
	cmd2.Dir = cmd.Dir
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("picker js: %v\n%s", err, out2)
	}
}

func Test95_157_T3_OtherScreensShareCombo(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p157t3.db"))
	ck := jwtCookieUser(t, me)
	nw := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", ck).Body.String()
	if strings.Count(nw, "data-jp-combo") < 2 {
		t.Fatal("새 업무 폼에 거래처·사업명 콤보가 없다")
	}
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "157편집", day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='157편집'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	edit := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", ck).Body.String()
	if !strings.Contains(edit, "data-jp-combo") || !strings.Contains(edit, "combobox_dismiss.js") {
		t.Fatal("업무 수정 화면에 공통 콤보가 없다")
	}
	board := doGetWith(t, e, "/workboard/board", ck).Body.String()
	if !strings.Contains(board, "combobox_dismiss.js") {
		t.Fatal("보드에 공통 닫기 스크립트 없음")
	}
}

func Test95_157_T4_BrowserDismiss(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p157br.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1)`); err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, 3).Format(dateLayout)
	mustCreateBoardTask(t, repo, "157브라우저편집", day, "09:00", "09:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	var id string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='157브라우저편집'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-157")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_157_combo_dismiss_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, me).Value,
		"EDIT_PATH=/workboard/tasks/"+id+"/edit",
		"SCREENSHOT_DIR="+shot,
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
