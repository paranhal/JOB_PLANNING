package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-210: 실제 서버 바이너리(임시 DB·TEST 데이터)로 대표 화면을 열어 변환한 링크 버튼의 높이·줄바꿈·겹침·가림·이동을 1440/820/390 에서 확인한다.
// SHOT_TAG=before 로 돌리면(변경 전 트리) 같은 화면의 「전」 스크린샷만 만든다.
func Test95_210_BrowserLinkButtons(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := repository.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs210", FullName: "비젼210", Role: model.RoleVisionAdmin, Email: "vs210@example.test", Mobile: "010-2100-0000",
		PasswordHash: HashPassword("password10"), IsActive: true, ProfileDone: true}
	if err := users.Create(vs); err != nil {
		t.Fatal(err)
	}
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C210','TEST도서관','TEST도서관',1,'O01')`)
	ex(`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,status,org_id,symptom,data_origin) VALUES ('AS210','R210','C210','2026-11-10 09:00:00','in_progress','O01','TEST 증상','app')`)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES ('WT210','admin','TEST 업무','open','O01')`)
	db.Close()

	// 서버 바이너리 빌드·기동
	bin := filepath.Join(t.TempDir(), "server")
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/server").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	srv := exec.Command(bin)
	srv.Dir = root
	srv.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port), "DB_PATH="+dbPath, "JWT_SECRET=cs-system-jwt-secret-2026", "VISION_ADMIN_INIT_PASSWORD=", "VISION_ADMIN_RESET_PASSWORD=")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _, _ = srv.Process.Wait() })
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	up := false
	for i := 0; i < 60; i++ {
		if r, err := http.Get(base + "/login"); err == nil {
			r.Body.Close()
			up = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !up {
		t.Fatal("서버가 뜨지 않았다")
	}

	type pg struct {
		Name    string   `json:"name,omitempty"`
		Path    string   `json:"path"`
		Texts   []string `json:"texts"`
		Click   bool     `json:"click,omitempty"`
		Shot820 bool     `json:"shot820,omitempty"`
		Pre     string   `json:"pre,omitempty"`
	}
	pages := []pg{
		{Name: "dashboard", Path: "/", Texts: []string{"영업 대시보드로", "통계에서 보기", "전체 →", "팀 전체 →"}, Click: true, Shot820: true},
		{Name: "as-stats", Path: "/as/stats", Texts: []string{"AS 처리 목록"}, Click: true},
		{Name: "as-knowledge", Path: "/as/knowledge", Texts: []string{"수정 이력 페이지", "원본 열기"}},
		{Name: "as-show", Path: "/as/AS210", Texts: []string{"조치 화면", "조치완료보고서", "보고서 재발급"}, Click: true, Shot820: true},
		{Name: "as-action", Path: "/as/AS210/action", Texts: []string{"예전 화면으로", "대기 목록", "접수취소 목록"}},
		{Name: "meeting-weekly", Path: "/meeting/weekly", Texts: []string{"휴가 화면", "결재함"}, Click: true},
		{Name: "meeting-board", Path: "/meeting/board", Texts: []string{"(예전)"}},
		{Name: "sales-dashboard", Path: "/sales/dashboard", Texts: []string{"기술 대시보드 보기", "전체 보기", "계약으로 전환"}, Click: true},
		{Name: "sales-new", Path: "/sales/new", Texts: []string{"빠른 견적"}},
		{Name: "stats-reports", Path: "/stats/reports", Texts: []string{"행 관리"}},
		{Name: "weekly-report", Path: "/stats/company-weekly?week=2026-10-19&org=O01", Texts: []string{"행 관리", "다른 출력물", "예전 화면으로"}, Shot820: true},
		{Name: "customer-tab-as", Path: "/customers/C210", Pre: "AS 이력", Texts: []string{"AS 접수하기"}},
		{Name: "customer-tab-assets", Path: "/customers/C210", Pre: "설치자산", Texts: []string{"첫 자산 등록하기"}},
		{Name: "customer-tab-contacts", Path: "/customers/C210", Pre: "담당자", Texts: []string{"첫 담당자 등록하기"}},
		{Name: "admin-orgs", Path: "/admin/orgs", Texts: []string{"분리·정리·복구"}},
		{Name: "workboard-task", Path: "/workboard/tasks/WT210", Texts: []string{"조치 화면 열기", "조치 완료 화면 열기", "수정"}, Shot820: true},
	}
	if os.Getenv("SHOT_TAG") == "before" {
		// 전(前) 트리에서는 버튼 클래스가 없어 단언하지 않는다(스크린샷만). 같은 스크립트가 strict=false 로 통과한다.
	}
	pj, _ := json.Marshal(pages)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join("..", "docs", "리뷰", "screenshots", "95-210"))
	}
	tag := os.Getenv("SHOT_TAG")
	if tag == "" {
		tag = "after"
	}
	cmd := exec.Command("node", "95_210_link_buttons_browser.mjs")
	cmd.Dir = filepath.Join(root, "web", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+base, "AUTH_COOKIE="+visionCookie(t, vs).Value,
		"SCREENSHOT_DIR="+shot, "SHOT_TAG="+tag, "PAGES="+string(pj))
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
