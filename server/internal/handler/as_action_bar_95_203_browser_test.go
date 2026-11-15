package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-203: 하단 고정 액션 영역이 모바일 탭바에 가리는지 본다(0단계 측정·재현·수정 확인 공용 환경).
type env203 struct {
	env201
	srvURL string
}

func setup203(t *testing.T) env203 {
	t.Helper()
	v := setup201(t)
	g := v.e.Group("")
	g.Use(v.h.Auth.AuthMiddleware)
	g.GET("/as/new", v.h.AS.New)
	g.GET("/api/assets/:customer_id", v.h.Asset.APIAssetsByCustomer)
	g.POST("/as/:id/action-sets", v.h.AS.SaveActionSet)
	g.GET("/stats/company-weekly", v.h.Stats.ShowWeeklyReport)
	g.POST("/stats/company-weekly/save", v.h.Stats.SaveWeeklyReport)
	g.GET("/workboard/tasks/:id/action", v.h.Workboard.ShowProjectWorkAction)
	// 주간보고 편집 화면을 위한 행(조직 O01)
	if _, err := v.db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O01','O01','도서관사업부','도서관',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := v.h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O01", DisplayName: "시험 사업", Kind: "manual", SortOrder: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	// 작업(프로젝트 업무) 조치 화면
	if _, err := v.db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP203','측정 사업','active','O01')`); err != nil {
		t.Fatal(err)
	}
	tk := &model.WorkTask{WorkType: model.WBWorkProjectWork, Title: "작업 조치 측정", Status: model.WBTaskInProgress, OrgID: "O01", ProjectID: "WP203",
		WorkDate: "2026-11-12", DueDate: "2026-11-12", StartTime: "09:00", EndTime: "09:30", DurationMin: 30, Assignee: "도서관장"}
	if err := v.h.Workboard.repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	_, _ = v.db.Exec(`UPDATE work_tasks SET task_id='TP203' WHERE task_id=?`, tk.TaskID)
	return env203{env201: v}
}

type screen203 struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Sel  string `json:"sel"`
	Dump bool   `json:"dump,omitempty"`
}

func screens203() []screen203 {
	return []screen203{
		{"AS 조치 저장", "/as/R201/action", `#as-action-bar button[type=submit]`, true},
		{"작업 조치 저장", "/workboard/tasks/TP203/action", `#pw-action-submit`, false},
		{"AS 접수 폼(접수 등록)", "/as/new", `#as-submit-btn`, false},
		{"주간보고 저장 막대", "/stats/company-weekly?week=2026-10-19&org=O01&edit=1", `[data-weekly-save]`, false},
	}
}

// Test95_203_Measure 0단계 측정(읽기 전용). `RUN_203_MEASURE=1 go test -run Test95_203_Measure -v` 로만 돈다.
func Test95_203_Measure(t *testing.T) {
	if os.Getenv("RUN_203_MEASURE") != "1" {
		t.Skip("측정 전용(RUN_203_MEASURE=1)")
	}
	v := setup203(t)
	staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	raw, _ := json.Marshal(screens203())
	cmd := exec.Command("node", "95_203_measure.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+jwtCookieVerified(t, mustUser203(t, v)).Value, "SCREENS="+string(raw))
	out, err := cmd.CombinedOutput()
	s := string(out)
	if i := strings.Index(s, "MEASURE_JSON "); i >= 0 {
		_ = os.WriteFile(os.Getenv("MEASURE_OUT"), []byte(s[i+len("MEASURE_JSON "):]), 0o644)
	}
	if err != nil {
		t.Fatalf("measure: %v\n%s", err, s)
	}
}

func mustUser203(t *testing.T, v env203) *model.User {
	t.Helper()
	u, err := repository.NewUserRepo(v.db).GetByUsername("oa151")
	if err != nil || u == nil {
		t.Fatal("oa151")
	}
	return u
}

var _ = echo.New

type case203 struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Sel      string `json:"sel"`
	Widths   []int  `json:"widths"`
	ShotName string `json:"shotName,omitempty"`
	LastSel  string `json:"lastSel,omitempty"`
	BarSel   string `json:"barSel,omitempty"`
	// ClickProbe true 면 저장 버튼 중앙을 실제 마우스로 누르는 시험을 더한다(제출은 막는다).
	ClickProbe bool `json:"clickProbe,omitempty"`
	// lg 이상(1024·1440, 뷰포트 높이 900)에서 수정 전과 같아야 하는 버튼 좌표(0단계 측정값)
	ExpTop    float64 `json:"expTop,omitempty"`
	ExpBottom float64 `json:"expBottom,omitempty"`
}

func cases203() []case203 {
	narrow := []int{390, 820, 1023}
	all := []int{390, 820, 1023, 1024, 1440}
	return []case203{
		{"AS 조치 저장", "/as/R201/action", `#as-action-bar button[type=submit]`, all, "as-action", ".as-action-right", "#as-action-bar", true, 852, 888},
		{"작업 조치 저장", "/workboard/tasks/TP203/action", `#pw-action-submit`, all, "work-action", "#as-action-main", "#as-action-bar", true, 852, 888},
		{"AS 접수 폼", "/as/new", `#as-submit-btn`, all, "as-form", "#as-form-root", "[data-as-form-bar]", true, 850, 888},
		// 가려지지 않던 화면: 회귀 없음 확인
		{"주간보고 저장 막대", "/stats/company-weekly?week=2026-10-19&org=O01&edit=1", `[data-weekly-save]`, narrow, "", "", "", false, 0, 0},
	}
}

func run203(t *testing.T, mode string, cs []case203) (string, error) {
	t.Helper()
	v := setup203(t)
	staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	raw, _ := json.Marshal(cs)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-203"))
	cmd := exec.Command("node", "95_203_tabbar_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "MODE="+mode,
		"AUTH_COOKIE="+jwtCookieVerified(t, mustUser203(t, v)).Value, "CASES="+string(raw), "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// 재현(수정 전 실패): 390·820·1023 에서 실제 마우스 클릭이 탭바 때문에 가로막힌다. 1024 이상은 막히지 않는다.
// 수정 뒤에는 이 재현이 더 이상 성립하지 않으므로 RUN_203_REPRO=1 일 때만 돈다(수정 전 상태 기록용).
func Test95_203_ReproBlockedByTabBar(t *testing.T) {
	if os.Getenv("RUN_203_REPRO") != "1" || os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("재현 전용(RUN_203_REPRO=1)")
	}
	out, err := run203(t, "repro", cases203()[:3])
	t.Log(out)
	if err != nil {
		t.Fatalf("repro: %v", err)
	}
	i := strings.Index(out, "RESULTS ")
	if i < 0 {
		t.Fatal("결과 없음")
	}
	var res []struct {
		Screen  string `json:"screen"`
		W       int    `json:"w"`
		Blocked bool   `json:"blocked"`
	}
	line := out[i+len("RESULTS "):]
	if j := strings.Index(line, "\n"); j >= 0 {
		line = line[:j]
	}
	if e := json.Unmarshal([]byte(strings.SplitN(line, "\n", 2)[0]), &res); e != nil {
		t.Fatal(e)
	}
	for _, r := range res {
		want := r.W < 1024
		if r.Blocked != want {
			t.Fatalf("%s %d: 가로막힘 %v (기대 %v)", r.Screen, r.W, r.Blocked, want)
		}
	}
}

// Test95_203_BrowserSaveBarAboveTabBar 수정 뒤: 390·820·1023 에서 저장 버튼이 탭바 위에 완전히 보이고(스크롤 맨 위·중간·맨 아래),
// 실제 마우스 클릭이 닿으며, 1024·1440 은 탭바 없이 기존 위치 그대로다. 콘텐츠 마지막 항목은 하단 바에 가려지지 않고,
// safe-area 를 가정해도 가려지지 않으며, 더보기 시트·탭바 이동·z-index 순서는 그대로다.
func Test95_203_BrowserSaveBarAboveTabBar(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup203(t)
	staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	raw, _ := json.Marshal(cases203())
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-203"))
	cmd := exec.Command("node", "95_203_tabbar_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "MODE=verify", "TABBAR_URL=/as/R201/action",
		"AUTH_COOKIE="+jwtCookieVerified(t, mustUser203(t, v)).Value, "CASES="+string(raw), "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	if !strings.Contains(string(out), "TABBAR_OK") {
		t.Fatal("탭바 회귀 검사가 돌지 않았다")
	}
}

// Test95_203_BrowserSaveByRealMouseClick 390·820·1023 에서 조치를 입력하고 실제 마우스 클릭으로 저장하면 접수가 저장된다.
func Test95_203_BrowserSaveByRealMouseClick(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup203(t)
	cases := map[string]string{}
	for _, w := range []string{"390", "820", "1023"} {
		id := "R203-" + w
		insert151AS(t, v.db, id, "C151", "in_progress", "", "", false)
		if _, err := v.db.Exec(`UPDATE as_receipts SET asset_id='A201-MAIN' WHERE as_id=?`, id); err != nil {
			t.Fatal(err)
		}
		cases[w] = id
	}
	staticDir, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	raw, _ := json.Marshal(cases)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-203"))
	cmd := exec.Command("node", "95_203_save_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, mustUser203(t, v)).Value, "CASES="+string(raw), "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	for _, id := range cases {
		var st, act string
		_ = v.db.QueryRow(`SELECT COALESCE(status,''), COALESCE(action_taken,'') FROM as_receipts WHERE as_id=?`, id).Scan(&st, &act)
		if st != "completed" {
			t.Fatalf("%s: 접수 상태 %q (저장되지 않았다)", id, st)
		}
		_ = act
	}
}
