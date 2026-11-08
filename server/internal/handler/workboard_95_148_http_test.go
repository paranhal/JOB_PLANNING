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

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seed148Task(t *testing.T, repo *repository.WBRepo, title, cid, pid, due string) *model.WorkTask {
	t.Helper()
	tk := &model.WorkTask{
		Title: title, WorkDate: "2026-11-08", DueDate: due,
		StartTime: "09:00", EndTime: "10:00",
		OrgID: model.OrgIDLibrary, Assignee: "최혜영",
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin,
		Priority: model.WBPriorityNormal, CustomerID: cid, ProjectID: pid,
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	return tk
}

func Test95_148_T1_KeepCustomerOnEditSave(t *testing.T) {
	e, repo := newWorkboardServer(t, "d148t1.db")
	db := repo.DB()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind, has_parent)
		VALUES ('C-MISS','숨은거래처','숨은거래처',1,?,'partner',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	tk := seed148Task(t, repo, "148거래처유지", "C-MISS", "", "2026-11-08")
	ck := jwtCookie(t)
	body := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID+"/edit", ck).Body.String()
	if !strings.Contains(body, "숨은거래처") {
		t.Fatal("수정 화면에 기존 거래처 이름 없음")
	}
	if !strings.Contains(body, "true") || !strings.Contains(body, "C-MISS") {
		t.Fatal("KeepMiss·고객 ID가 콤보에 없음")
	}
	rec := doFormWith(t, e, "/workboard/tasks/"+tk.TaskID+"/update", url.Values{
		"scope": {"register"}, "title": {"148거래처유지"}, "work_type": {"admin"},
		"work_date": {"2026-11-08"}, "due_date": {"2026-11-08"},
		"start_time": {"09:00"}, "end_time": {"10:00"},
		"assignee": {"최혜영"}, "status": {model.WBTaskWaiting},
		"customer_id": {"C-MISS"},
	}, ck)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 loc=%s code=%d", rec.Header().Get("Location"), rec.Code)
	}
	got, err := repo.GetTask(tk.TaskID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.CustomerID != "C-MISS" {
		t.Fatalf("customer_id=%q", got.CustomerID)
	}
	if got.CustomerLabel() != "숨은거래처" {
		t.Fatalf("표시명=%q", got.CustomerLabel())
	}
}

func Test95_148_T2_SiteProjectCandidates(t *testing.T) {
	e, repo := newWorkboardServer(t, "d148t2.db")
	db := repo.DB()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C148','서산고','서산고',1,?,'customer')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026)`, org, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','C148','서산고','in_progress',?,1),
		       ('PS-B','WP-B','C148','서산고','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	tk := seed148Task(t, repo, "148사업후보", "C148", "", "2026-11-08")
	body := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID+"/edit", jwtCookie(t)).Body.String()
	at := strings.Index(body, `data-project-picker`)
	if at < 0 {
		t.Fatal("사업 피커 없음")
	}
	chunk := body[at:]
	if nxt := strings.Index(chunk[len(`data-project-picker`):], `data-project-picker`); nxt >= 0 {
		chunk = chunk[:len(`data-project-picker`)+nxt]
	}
	if !strings.Contains(chunk, "사업A") || !strings.Contains(chunk, "사업B") {
		t.Fatal("등록 사업 A·B가 후보에 없음")
	}
	if strings.Contains(chunk, "사업C") {
		t.Fatal("미등록 사업 C가 후보에 있음")
	}
	if strings.Contains(chunk, "타 부서 사업 보기") {
		t.Fatal("타 부서 전체 목록이 있다")
	}
	if !strings.Contains(chunk, `"SiteCount":2`) && !strings.Contains(chunk, "SiteCount&#34;:2") && !strings.Contains(chunk, `\u0022SiteCount\u0022:2`) {
		t.Fatal("사이트 등록 2건이 아님")
	}
	if strings.Contains(chunk, `"Selected":"WP-A"`) || strings.Contains(chunk, `"Selected":"WP-B"`) {
		t.Fatal("1개 이상이어도 자동 선택되면 안 됨")
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C0','빈사이트','빈사이트',1,?,'customer')`, org); err != nil {
		t.Fatal(err)
	}
	empty := seed148Task(t, repo, "148사업0건", "C0", "", "2026-11-08")
	z := doGetWith(t, e, "/workboard/tasks/"+empty.TaskID+"/edit", jwtCookie(t)).Body.String()
	zat := strings.Index(z, `data-project-picker`)
	if zat < 0 {
		t.Fatal("0건 피커 없음")
	}
	zchunk := z[zat:]
	if nxt := strings.Index(zchunk[len(`data-project-picker`):], `data-project-picker`); nxt >= 0 {
		zchunk = zchunk[:len(`data-project-picker`)+nxt]
	}
	if !strings.Contains(zchunk, "연결된 사업이 없습니다") {
		t.Fatal("0건 검색 안내 없음")
	}
	if strings.Contains(zchunk, "타 부서 사업 보기") {
		t.Fatal("0건에 타 부서 목록")
	}
}

func Test95_148_T3_LinkSiteConfirm(t *testing.T) {
	e, repo := newWorkboardServer(t, "d148t3.db")
	db := repo.DB()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C148L','연결고','연결고',1,?,'customer')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-L','연결사업','active',?,0,1,2026)`, org); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookie(t)
	deny := doFormWith(t, e, "/workboard/projects/link-site", url.Values{
		"project_id": {"WP-L"}, "customer_id": {"C148L"},
	}, ck)
	if deny.Code != http.StatusBadRequest {
		t.Fatalf("확인 없이 %d %s", deny.Code, deny.Body.String())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-L' AND customer_id='C148L'`).Scan(&n)
	if n != 0 {
		t.Fatalf("확인 없이 등록됨 %d", n)
	}
	ok := doFormWith(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-L"}, "customer_id": {"C148L"},
	}, ck)
	if ok.Code != http.StatusOK {
		t.Fatalf("등록 %d %s", ok.Code, ok.Body.String())
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-L' AND customer_id='C148L' AND is_active=1`).Scan(&n)
	if n != 1 {
		t.Fatalf("등록 건수 %d", n)
	}
	again := doFormWith(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-L"}, "customer_id": {"C148L"},
	}, ck)
	if again.Code != http.StatusOK {
		t.Fatalf("중복 %d %s", again.Code, again.Body.String())
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-L' AND customer_id='C148L'`).Scan(&n)
	if n != 1 {
		t.Fatalf("중복 등록 %d", n)
	}
}

func Test95_148_T4_DueDateMessage(t *testing.T) {
	e, repo := newWorkboardServer(t, "d148t4.db")
	tk := seed148Task(t, repo, "148종료일", "", "", "2026-11-08")
	ck := jwtCookie(t)
	rec := doFormWith(t, e, "/workboard/tasks/"+tk.TaskID+"/update", url.Values{
		"scope": {"register"}, "title": {"148종료일"}, "work_type": {"admin"},
		"work_date": {"2026-11-08"}, "due_date": {""},
		"start_time": {"09:00"}, "end_time": {"10:00"},
		"assignee": {"최혜영"}, "status": {model.WBTaskWaiting},
	}, ck)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=due_date") {
		t.Fatalf("종료일 거절 loc=%s code=%d", loc, rec.Code)
	}
	if strings.Contains(loc, "err=task") {
		t.Fatal("err=task 코드 노출")
	}
	shown := doGetWith(t, e, "/workboard/tasks/"+tk.TaskID+"/edit?err=due_date", ck).Body.String()
	if !strings.Contains(shown, "업무 종료일을 입력해 주세요") {
		t.Fatal("원인 문장 없음")
	}
	if strings.Contains(shown, "is-invalid") == false || !strings.Contains(shown, "시작일과 같게") {
		t.Fatal("종료일 칸 강조 또는 시작일과 같게 없음")
	}
	flash := ""
	if i := strings.Index(shown, "bg-danger-soft"); i >= 0 {
		flash = shown[i:]
		if j := strings.Index(flash, "</div>"); j > 0 {
			flash = flash[:j]
		}
	}
	if flash == "" || strings.Contains(flash, ">task<") || strings.TrimSpace(flash) == "task" {
		t.Fatalf("배너에 코드값 %s", flash)
	}
	if !strings.Contains(shown, "미정 + 사유") {
		t.Fatal("날짜 미정 없음")
	}
	und := doFormWith(t, e, "/workboard/tasks/"+tk.TaskID+"/update", url.Values{
		"scope": {"register"}, "title": {"148종료일"}, "work_type": {"admin"},
		"work_date": {"2026-11-08"}, "due_date": {""},
		"due_undetermined": {"1"}, "no_date_reason": {"customer"},
		"start_time": {"09:00"}, "end_time": {"10:00"},
		"assignee": {"최혜영"}, "status": {model.WBTaskWaiting},
	}, ck)
	if und.Code != http.StatusSeeOther || strings.Contains(und.Header().Get("Location"), "err=") {
		t.Fatalf("미정 저장 loc=%s", und.Header().Get("Location"))
	}
	got, _ := repo.GetTask(tk.TaskID)
	if got.DueDate != "" {
		t.Fatalf("미정이면 종료일 비어야 함 %q", got.DueDate)
	}
}

func Test95_148_T5_BrowserTaskEdit(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, repo := newWorkboardServer(t, "d148t5.db")
	db := repo.DB()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C148','서산고','서산고',1,?,'customer')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026), ('WP-B','사업B','active',?,0,1,2025)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','C148','서산고','in_progress',?,1), ('PS-B','WP-B','C148','서산고','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	tk := seed148Task(t, repo, "148브라우저", "C148", "", "2026-11-08")
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_148_task_edit_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookie(t).Value,
		"SCREENSHOT_DIR="+shot,
		"EDIT_PATH=/workboard/tasks/"+tk.TaskID+"/edit",
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
