package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_113_T1_RenderPickerAndClick(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113t1.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1),
		       ('WP-O02','미디어사업','active','O02',0,1)`); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	rec := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("form %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-project-picker`) {
		t.Fatal("피커 없음")
	}
	if !strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 접힘")
	}
	if !strings.Contains(body, "사업 필수 아님") {
		t.Fatal("나중에 매칭")
	}
	if !strings.Contains(body, `data-can-other="0"`) {
		t.Fatal("기술은 타 부서 불가")
	}
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	oaBody := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", jwtCookieUser(t, oa)).Body.String()
	if !strings.Contains(oaBody, `data-can-other="1"`) {
		t.Fatal("조직관리자 타 부서")
	}
	if !strings.Contains(oaBody, "도서관사업") {
		t.Fatal("내 부서 사업")
	}
	other := doGetWith(t, e, "/workboard/projects/picker-other", ck)
	if other.Code != http.StatusForbidden {
		t.Fatalf("기술 other %d", other.Code)
	}
	ok := doGetWith(t, e, "/workboard/projects/picker-other", jwtCookieUser(t, oa))
	if ok.Code != http.StatusOK {
		t.Fatalf("관리자 other %d %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), "미디어사업") {
		t.Fatal("타 부서 목록")
	}
}

func Test95_113_T2_PendingAndStartBlocked(t *testing.T) {
	e, _, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113t2.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	rec := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type":             {"support"},
		"title":                 {"매칭대기"},
		"due_date":              {day},
		"work_date":             {day},
		"start_time":            {"09:00"},
		"end_time":              {"09:30"},
		"assignee":              {me.FullName},
		"project_match_pending": {"1"},
	}, ck)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("pending 저장 loc=%s code=%d", loc, rec.Code)
	}
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var tk *model.WorkTask
	for i := range tasks {
		if tasks[i].Title == "매칭대기" {
			tk = &tasks[i]
			break
		}
	}
	if tk == nil || !tk.ProjectMatchPending || tk.ProjectID != "" {
		t.Fatalf("pending 저장값 %+v", tk)
	}
	show := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID, ck).Body.String()
	if !strings.Contains(show, "사업명 매칭 필요") {
		t.Fatal("배지 없음")
	}
	start := doFormJSONWith(t, e, "/workboard/tasks/"+tk.TaskID+"/live/start", url.Values{"live_json": {"1"}, "live_at": {day + " 09:00"}}, ck)
	if start.Code != http.StatusUnprocessableEntity {
		t.Fatalf("시작 거절 %d %s", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "사업명 매칭") {
		t.Fatalf("시작 메시지 %s", start.Body.String())
	}
}

func Test95_113_T3_SaveVisibleOnly(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113t3.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1),
		       ('WP-O02','미디어사업','active','O02',0,1)`); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	day := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	deny := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"타부서거절"},
		"due_date":   {day},
		"work_date":  {day},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
		"assignee":   {me.FullName},
		"project_id": {"WP-O02"},
	}, ck)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("기술 타 부서 저장 %d loc=%s body=%s", deny.Code, deny.Header().Get("Location"), deny.Body.String())
	}
	oa, _ := repository.NewUserRepo(db).GetByUsername("oa01")
	ok := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"타부서허용"},
		"due_date":   {day},
		"work_date":  {day},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
		"assignee":   {oa.FullName},
		"project_id": {"WP-O02"},
	}, jwtCookieUser(t, oa))
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("조직관리자 저장 loc=%s code=%d", ok.Header().Get("Location"), ok.Code)
	}
	tasks, _ := repo.ListTasks()
	found := false
	for _, tk := range tasks {
		if tk.Title == "타부서허용" && tk.ProjectID == "WP-O02" {
			found = true
		}
	}
	if !found {
		t.Fatal("허용 저장 없음")
	}
}

func Test95_113_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "project_picker_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_113_BrowserPickClick(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p113br.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1)`); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_113_picker_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, me).Value,
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
