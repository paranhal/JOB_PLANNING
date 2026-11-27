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

// 완료된 AS 의 사업 연결: 미연결 경고, 후보마다 한 번 클릭 [연결], 담당자 이력에서 읽은 「추천」, 연결 해제, 업무 구분 지정.
func Test95_228_ASProjectConnect(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	ck := as151OA(t, db)
	// 후보 2건: 이 사이트(C151)가 두 사업의 사이트로 등록돼 있다
	for _, pid := range []string{"WP-A", "WP-B"} {
		if r := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{"confirm": {"1"}, "project_id": {pid}, "customer_id": {"C151"}}, ck); r.Code != http.StatusOK {
			t.Fatalf("link %s %d %s", pid, r.Code, r.Body.String())
		}
	}
	// 같은 담당자(도서관장)가 이 기관 접수 2건을 WP-A 에 연결한 이력
	insert151AS(t, db, "R228-h1", "C151", "completed", "점검", "WP-A", true)
	insert151AS(t, db, "R228-h2", "C151", "completed", "점검", "WP-A", true)
	insert151AS(t, db, "R228-x", "C151", "completed", "점검", "", true)
	wb := repository.NewWBRepo(db)
	tk := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "[AS]성환도서관_R228-x", Status: model.WBTaskComplete, Priority: model.WBPriorityNormal,
		DueDate: "2026-10-21", WorkDate: "2026-10-21", OrgID: model.OrgIDLibrary, SourceType: model.WBSourceAS, SourceID: "R228-x"}
	if err := wb.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	page := doAS151Get(t, e, "/as/R228-x/action", ck).Body.String()
	for _, want := range []string{"data-as-project-warn", `data-as-project-cand="WP-A"`, `data-as-project-cand="WP-B"`, "data-as-project-connect", "data-as-category-form"} {
		if !strings.Contains(page, want) {
			t.Fatalf("미연결 완료 건 화면에 %s 가 없다", want)
		}
	}
	// 사이드 패널은 모바일·데스크톱용으로 두 번 그려지므로 추천 표시 수는 패널 수와 같아야 한다(후보 1건만 추천)
	if blocks := strings.Count(page, "data-as-project-block"); blocks == 0 || strings.Count(page, "data-as-project-rec") != blocks {
		t.Fatalf("추천 표시 %d개 (패널 %d개)", strings.Count(page, "data-as-project-rec"), blocks)
	}
	if !strings.Contains(page, "접수 2건을 이 사업에 연결했습니다") {
		t.Fatal("추천 근거 문구가 없다")
	}
	// 구분 지정: 사업이 없을 때만, 활성 구분만
	if r := doAS151Form(t, e, "/as/R228-x/category", url.Values{"activity_kind": {"nope"}}, ck); !strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("잘못된 구분이 통과했다 %s", r.Header().Get("Location"))
	}
	// 한 번 클릭으로 연결 → 접수에서 만든 업무에도 반영(95-227)
	if r := doAS151Form(t, e, "/as/R228-x/project", url.Values{"project_id": {"WP-A"}}, ck); r.Code != http.StatusSeeOther {
		t.Fatalf("연결 %d %s", r.Code, r.Body.String())
	}
	var pid, tpid string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='R228-x'`).Scan(&pid)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, tk.TaskID).Scan(&tpid)
	if pid != "WP-A" || tpid != "WP-A" {
		t.Fatalf("연결 결과 접수=%q 업무=%q", pid, tpid)
	}
	linked := doAS151Get(t, e, "/as/R228-x/action", ck).Body.String()
	if !strings.Contains(linked, "data-as-project-linked") || !strings.Contains(linked, "data-as-project-unlink") || strings.Contains(linked, "data-as-project-warn") {
		t.Fatal("연결된 뒤 화면(연결 사업명·해제 버튼, 경고 없음)이 맞지 않다")
	}
	// 사업이 연결된 접수에는 구분을 지정하지 않는다
	if r := doAS151Form(t, e, "/as/R228-x/category", url.Values{"activity_kind": {"service"}}, ck); !strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatal("사업이 있는데 구분 지정이 통과했다")
	}
	// 연결 해제 → 업무의 사업도 비워진다
	if r := doAS151Form(t, e, "/as/R228-x/project", url.Values{"project_id": {""}}, ck); r.Code != http.StatusSeeOther {
		t.Fatalf("해제 %d", r.Code)
	}
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, tk.TaskID).Scan(&tpid)
	if tpid != "" {
		t.Fatalf("해제 뒤 업무 사업 %q", tpid)
	}
	// 사업 무관 AS: 업무 구분으로 지정 → 업무에 구분이 붙는다
	if r := doAS151Form(t, e, "/as/R228-x/category", url.Values{"activity_kind": {"service"}}, ck); strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("구분 지정 %s", r.Header().Get("Location"))
	}
	if k := wb.TaskActivityKind(tk.TaskID); k != "service" {
		t.Fatalf("업무 구분 %q want service", k)
	}
	if !strings.Contains(doAS151Get(t, e, "/as/R228-x/action", ck).Body.String(), "업무 구분 사업 부문") {
		t.Fatal("구분 지정 뒤 화면에 구분이 안 보인다")
	}
}

// 추천 근거가 없거나 동점이면 추천하지 않는다(임의로 고르지 않는다).
func Test95_228_RecommendNoGuess(t *testing.T) {
	e, db, _ := newAS151Server(t)
	_ = e
	seed151Projects(t, db)
	pr := repository.NewProjectRepo(db)
	if id, _ := pr.RecommendASProject("C151", "도서관장", []string{"WP-A", "WP-B"}); id != "" {
		t.Fatalf("이력이 없는데 추천 %q", id)
	}
	insert151AS(t, db, "R228-t1", "C151", "completed", "a", "WP-A", true)
	insert151AS(t, db, "R228-t2", "C151", "completed", "a", "WP-B", true)
	if id, _ := pr.RecommendASProject("C151", "도서관장", []string{"WP-A", "WP-B"}); id != "" {
		t.Fatalf("동점인데 추천 %q", id)
	}
	insert151AS(t, db, "R228-t3", "C151", "completed", "a", "WP-B", true)
	if id, why := pr.RecommendASProject("C151", "도서관장", []string{"WP-A", "WP-B"}); id != "WP-B" || !strings.Contains(why, "2건") {
		t.Fatalf("추천 %q %q", id, why)
	}
	if id, _ := pr.RecommendASProject("C151", "다른사람", []string{"WP-A", "WP-B"}); id != "" {
		t.Fatalf("다른 담당자 이력으로 추천했다 %q", id)
	}
}

// 브라우저: 완료된 AS 의 사업 연결(1440/390) — 경고·후보·추천·한 번 클릭 연결·해제.
func Test95_228_BrowserASProjectConnect(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	ck := as151OA(t, db)
	for _, pid := range []string{"WP-A", "WP-B"} {
		if r := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{"confirm": {"1"}, "project_id": {pid}, "customer_id": {"C151"}}, ck); r.Code != http.StatusOK {
			t.Fatalf("link %s %d", pid, r.Code)
		}
	}
	insert151AS(t, db, "R228-h1", "C151", "completed", "점검", "WP-A", true)
	insert151AS(t, db, "R228-h2", "C151", "completed", "점검", "WP-A", true)
	insert151AS(t, db, "R228-d", "C151", "completed", "점검", "", true)
	insert151AS(t, db, "R228-m", "C151", "completed", "점검", "", true)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-228"))
	cmd := exec.Command("node", "95_228_as_project_connect_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+ck.Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
}
