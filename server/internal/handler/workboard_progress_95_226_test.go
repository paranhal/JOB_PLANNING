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

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type env226 struct {
	e        *echo.Echo
	h        *Handler
	oa, same *model.User
	exec     func(q string, a ...interface{})
	do       func(method, path string, form url.Values, u *model.User) *httptest.ResponseRecorder
	scalar   func(q string, a ...interface{}) string
	dbHandle interface{}
}

func setup226(t *testing.T) env226 {
	t.Helper()
	e, db, same, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p226.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("oa01")
	}
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C1','가도서관','가도서관',1,'O01'),('C2','나도서관','나도서관',1,'O01')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','충남 구축 사업','active','O01')`,
		// 여러 날 단일 업무(일자별) · 하위 업무가 있는 업무 · 하루 업무
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,work_date,due_date,start_time,end_time,created_by_user_id) VALUES ('TD','admin','3일 업무','waiting','O01','WP1','2026-12-01','2026-12-03','09:00','10:00','` + oa.UserID + `')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,work_date,due_date,start_time,end_time,created_by_user_id) VALUES ('TP','admin','상위 업무','waiting','O01','WP1','2026-12-01','2026-12-02','09:00','10:00','` + oa.UserID + `')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,parent_task_id,work_date,due_date,created_by_user_id) VALUES ('TK1','admin','하위1','complete','O01','WP1','TP','2026-12-01','2026-12-01','` + oa.UserID + `'),('TK2','admin','하위2','waiting','O01','WP1','TP','2026-12-02','2026-12-02','` + oa.UserID + `')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,work_date,due_date,start_time,end_time,created_by_user_id) VALUES ('T1','admin','하루 업무','waiting','O01','WP1','2026-12-01','2026-12-01','09:00','10:00','` + oa.UserID + `')`,
	} {
		ex(q)
	}
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.POST("/workboard/tasks/:id/progress", h.Workboard.ProgressManual)
	g.POST("/workboard/tasks/:id/day-progress", h.Workboard.ProgressDay)
	g.POST("/workboard/tasks/:id/complete-multi", h.Workboard.ProgressCompleteMulti)
	g.POST("/workboard/tasks/:id/loads", h.Workboard.ProgressLoads)
	g.POST("/workboard/tasks/:id/defer", h.Workboard.TaskDefer)
	g.POST("/workboard/tasks/:id/undefer", h.Workboard.TaskUndefer)
	do := func(method, path string, form url.Values, u *model.User) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			req = httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		} else {
			req = httptest.NewRequest(method, "http://localhost"+path, nil)
		}
		req.AddCookie(jwtCookieVerified(t, u))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	return env226{e: e, h: h, oa: oa, same: same, exec: ex, do: do,
		scalar: func(q string, a ...interface{}) string {
			var s string
			_ = db.QueryRow(q, a...).Scan(&s)
			return s
		}}
}

func loc(r *httptest.ResponseRecorder) string { return r.Header().Get("Location") }

// 진행률 패널: 일자별 모드, 수동 우선 표시, 일자별 부분 완료 → 마지막 부분 완료 때 완료와 완료일.
func TestHTTP95_226_ProgressPanelAndDays(t *testing.T) {
	e := setup226(t)
	page := e.do("GET", "/workboard/tasks/TD", nil, e.oa).Body.String()
	for _, want := range []string{"data-progress-panel", "data-progress-days", "data-complete-choice", "data-defer-btn", "일자 0/3일"} {
		if !strings.Contains(page, want) {
			t.Fatalf("진행률 패널에 %q 가 없다", want)
		}
	}
	save := func(date, load, done string) *httptest.ResponseRecorder {
		return e.do("POST", "/workboard/tasks/TD/day-progress", url.Values{"day_date": {date}, "load_pct": {load}, "done_pct": {done}}, e.oa)
	}
	if r := save("2026-12-01", "", "100"); r.Code != 303 || strings.Contains(loc(r), "err=") {
		t.Fatalf("1일차 %d %s", r.Code, loc(r))
	}
	if !strings.Contains(e.do("GET", "/workboard/tasks/TD", nil, e.oa).Body.String(), "33%") {
		t.Fatal("1/3일 완료인데 자동 진행률 33%가 안 보인다")
	}
	// 수동이 있으면 수동을 먼저 보이고 자동값도 병기한다
	if r := e.do("POST", "/workboard/tasks/TD/progress", url.Values{"pct": {"70"}}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("수동 진행률 %s", loc(r))
	}
	body := e.do("GET", "/workboard/tasks/TD", nil, e.oa).Body.String()
	if !strings.Contains(body, "70%") || !strings.Contains(body, "자동 33%") || !strings.Contains(body, "수동") {
		t.Fatal("수동 우선 표시·자동 병기가 맞지 않다")
	}
	if r := e.do("POST", "/workboard/tasks/TD/progress", url.Values{"pct": {"130"}}, e.oa); !strings.Contains(loc(r), "err=") {
		t.Fatal("범위 밖 수동값이 통과했다")
	}
	// 입력 검증: 완료 비율 범위·비숫자, 기간 밖 날짜
	if r := save("2026-12-02", "", "abc"); !strings.Contains(loc(r), "err=") {
		t.Fatal("비숫자 완료 비율이 통과했다")
	}
	if r := save("2026-12-30", "", "50"); !strings.Contains(loc(r), "err=") {
		t.Fatal("기간 밖 날짜가 통과했다")
	}
	// 마지막 부분 완료 → 업무 완료 + 완료일
	save("2026-12-02", "", "100")
	if r := save("2026-12-03", "", "100"); !strings.Contains(loc(r), "ok=day_complete") {
		t.Fatalf("마지막 부분 완료 %s", loc(r))
	}
	if got := e.scalar(`SELECT status||'|'||complete_date FROM work_tasks WHERE task_id='TD'`); got != "complete|2026-12-03" {
		t.Fatalf("마지막 부분 완료 뒤 %s", got)
	}
	// 권한: 담당이 아닌 일반 조직원은 저장 불가
	if r := e.do("POST", "/workboard/tasks/TP/progress", url.Values{"pct": {"10"}}, e.same); r.Code != http.StatusForbidden {
		t.Fatalf("권한 없는 수동 진행률 %d want 403", r.Code)
	}
}

// 하위 업무 부하량: 합 100 초과 거부, 저장하면 진행률에 반영.
func TestHTTP95_226_KidLoads(t *testing.T) {
	e := setup226(t)
	bad := e.do("POST", "/workboard/tasks/TP/loads", url.Values{"load_TK1": {"70"}, "load_TK2": {"60"}}, e.oa)
	if !strings.Contains(loc(bad), "err=") {
		t.Fatalf("합 130 이 통과했다 %s", loc(bad))
	}
	if n := e.scalar(`SELECT COUNT(*) FROM work_tasks WHERE load_pct IS NOT NULL`); n != "0" {
		t.Fatal("거부했는데 저장됐다")
	}
	if r := e.do("POST", "/workboard/tasks/TP/loads", url.Values{"load_TK1": {"-3"}}, e.oa); !strings.Contains(loc(r), "err=") {
		t.Fatal("음수가 통과했다")
	}
	if r := e.do("POST", "/workboard/tasks/TP/loads", url.Values{"load_TK1": {"abc"}}, e.oa); !strings.Contains(loc(r), "err=") {
		t.Fatal("비숫자가 통과했다")
	}
	if r := e.do("POST", "/workboard/tasks/TP/loads", url.Values{"load_TK1": {"30"}, "load_TK2": {""}}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("정상 저장 %s", loc(r))
	}
	if body := e.do("GET", "/workboard/tasks/TP", nil, e.oa).Body.String(); !strings.Contains(body, "data-kid-loads") || !strings.Contains(body, "하위 업무 1/2건") || !strings.Contains(body, "30%") {
		t.Fatal("부하량 반영 진행률(하위 1/2건 · 30%)이 안 보인다")
	}
}

// 연기 → 대기 → 오늘 업무 → 완료 안 하면 대기로 복귀. 사유 입력이 없고 지연 이력이 없다.
func TestHTTP95_226_DeferFlow(t *testing.T) {
	e := setup226(t)
	if r := e.do("POST", "/workboard/tasks/T1/defer", url.Values{}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("연기 %s", loc(r))
	}
	if got := e.scalar(`SELECT COALESCE(work_date,'')||'|'||status||'|'||no_date_reason FROM work_tasks WHERE task_id='T1'`); got != "|waiting|연기(대기)" {
		t.Fatalf("연기 뒤 %q", got)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM work_task_delays WHERE task_id='T1'`); n != "0" {
		t.Fatal("연기가 지연 이력에 들어갔다")
	}
	if r := e.do("POST", "/workboard/tasks/T1/defer", url.Values{}, e.oa); !strings.Contains(loc(r), "err=") {
		t.Fatal("대기 중인 업무를 다시 연기할 수 있다")
	}
	if !strings.Contains(e.do("GET", "/workboard/tasks/T1", nil, e.oa).Body.String(), "data-task-deferred") {
		t.Fatal("대기 중 표시가 없다")
	}
	if r := e.do("POST", "/workboard/tasks/T1/undefer", url.Values{"date": {"2026-12-05"}}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("오늘 업무로 등록 %s", loc(r))
	}
	if got := e.scalar(`SELECT work_date||'|'||from_deferred FROM work_tasks WHERE task_id='T1'`); got != "2026-12-05|1" {
		t.Fatalf("등록 뒤 %q", got)
	}
	if n := e.h.Workboard.repo.ReturnUnfinishedDeferred("2026-12-06"); n != 1 {
		t.Fatalf("미완료 복귀 %d", n)
	}
	// 대기 그룹: 사업별 그룹핑 건수
	e.exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,work_date,due_date,created_by_user_id) VALUES ('X1','admin','추가1','waiting','O01','WP1','2026-12-01','2026-12-01','` + e.oa.UserID + `'),('X2','admin','추가2','waiting','O01',NULL,'2026-12-01','2026-12-01','` + e.oa.UserID + `')`)
	e.h.Workboard.repo.DeferTask("X1", "u")
	e.h.Workboard.repo.DeferTask("X2", "u")
	list := e.h.Workboard.repo.ListDeferred("O01", e.oa.UserID, true)
	groups := map[string]int{}
	for _, it := range list {
		groups[it.Heading()]++
	}
	if groups["충남 구축 사업"] != 2 || groups["(사업·구분 없음)"] != 1 {
		t.Fatalf("그룹핑 %v", groups)
	}
	// 범위 밖(담당·등록자 아님) 일반 조직원에게는 대기 업무가 안 보인다
	if other := e.h.Workboard.repo.ListDeferred("O01", e.same.UserID, false); len(other) != 0 {
		t.Fatalf("범위 밖 사용자에게 대기 업무가 보인다 %d건", len(other))
	}
}

// 95-223 #3: 여러 날 업무 완료 선택. 오늘까지만 → 남은 날 계속, 전체 완료 → 이후 일정 제외.
func TestHTTP95_226_CompleteChoice(t *testing.T) {
	e := setup226(t)
	if r := e.do("POST", "/workboard/tasks/TD/complete-multi", url.Values{"mode": {"today"}, "date": {"2026-12-01"}}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("오늘까지만 %s", loc(r))
	}
	if got := e.scalar(`SELECT status FROM work_tasks WHERE task_id='TD'`); got == "complete" {
		t.Fatal("오늘까지만인데 업무가 완료됐다")
	}
	if got := e.scalar(`SELECT done_pct FROM work_task_day_progress WHERE task_id='TD' AND work_date='2026-12-01'`); got != "100.0" && got != "100" {
		t.Fatalf("오늘 분 완료 비율 %q", got)
	}
	if r := e.do("POST", "/workboard/tasks/TD/complete-multi", url.Values{"mode": {"all"}, "date": {"2026-12-02"}}, e.oa); strings.Contains(loc(r), "err=") {
		t.Fatalf("전체 완료 %s", loc(r))
	}
	if got := e.scalar(`SELECT status||'|'||due_date||'|'||complete_date FROM work_tasks WHERE task_id='TD'`); got != "complete|2026-12-02|2026-12-02" {
		t.Fatalf("전체 완료 뒤 %q (이후 일정 제외)", got)
	}
}

// 브라우저: 조치 화면 진행률 패널 — 일자별 부분 완료, 수동 진행률, 연기→대기 카드→오늘 업무(1440/390).
func Test95_226_BrowserProgressPanel(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e := setup226(t)
	e.e.GET("/workboard/register", e.h.Workboard.Register) // 대기 카드 확인용
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.e.Static("/static", staticDir)
	srv := httptest.NewServer(e.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-226"))
	cmd := exec.Command("node", "95_226_progress_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, e.oa).Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	if got := e.scalar(`SELECT COALESCE(work_date,'') FROM work_tasks WHERE task_id='T1'`); got == "" {
		t.Fatal("브라우저에서 대기 업무를 오늘 업무로 등록하지 못했다")
	}
}

// 95-175 구조 화면: 하부 업무 부하량(%) 입력. 합계 100 초과·음수·비숫자 거부, 사이트가 있는 하부 업무는 부하량 없음, 비우면 균등.
func TestHTTP95_226_PackageLoadPct(t *testing.T) {
	v := setup183(t)
	edit := func(pkg, name, track, load string) (int, string) {
		return v.post(t, "/projects/"+v.id+"/packages/"+pkg, url.Values{"name": {name}, "track_mode": {track}, "load_pct": {load}}, v.admin)
	}
	if code, l := edit(v.qtyPkg, "수량 업무", model.TrackModePhase, "60"); code != http.StatusSeeOther || strings.Contains(l, "err=") {
		t.Fatalf("부하량 60 저장 %d %s", code, l)
	}
	if _, l := edit(v.emptyPkg, "빈 업무", model.TrackModeSite, "50"); !strings.Contains(l, "err=") {
		t.Fatalf("합 110 이 통과했다 %s", l)
	}
	if _, l := edit(v.emptyPkg, "빈 업무", model.TrackModeSite, "-5"); !strings.Contains(l, "err=") {
		t.Fatal("음수가 통과했다")
	}
	if _, l := edit(v.emptyPkg, "빈 업무", model.TrackModeSite, "abc"); !strings.Contains(l, "err=") {
		t.Fatal("비숫자가 통과했다")
	}
	if _, l := edit(v.pkg, "홍성", model.TrackModeSite, "10"); !strings.Contains(l, "err=") {
		t.Fatalf("사이트가 있는 하부 업무에 부하량이 통과했다 %s", l)
	}
	view := v.wb.PhaseLoadView(v.phase)
	var got = map[string]float64{}
	for _, r := range view {
		got[r.ID] = r.Effective
	}
	if got[v.qtyPkg] < 59.9 || got[v.qtyPkg] > 60.1 || got[v.emptyPkg] < 39.9 || got[v.emptyPkg] > 40.1 {
		t.Fatalf("부하량 현황(수량 60 · 빈 업무 자동 40) %v", got)
	}
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	if !strings.Contains(body, "data-load-pct") {
		t.Fatal("구조 화면 하부 업무 수정에 부하량 입력이 없다")
	}
	// 비우면 자동(균등)으로 돌아간다
	if _, l := edit(v.qtyPkg, "수량 업무", model.TrackModePhase, ""); strings.Contains(l, "err=") {
		t.Fatalf("부하량 비우기 %s", l)
	}
	for _, r := range v.wb.PhaseLoadView(v.phase) {
		if r.ID == v.qtyPkg && r.Given != nil {
			t.Fatal("비웠는데 값이 남았다")
		}
	}
}
