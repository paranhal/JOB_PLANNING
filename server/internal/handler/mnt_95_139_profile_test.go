package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/dbprof"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_139_T1_ProfileOffNoServerTiming(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "")
	e, _, planID := newMnt139App(t, false)
	rec := doGetWith(t, e, "/maintenance/"+planID+"?view=list", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String()[:min(300, rec.Body.Len())])
	}
	if rec.Header().Get("Server-Timing") != "" {
		t.Fatalf("꺼진 측정이 헤더를 남김: %s", rec.Header().Get("Server-Timing"))
	}
}

func Test95_139_T2_ProfileOnWritesServerTiming(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	e, _, planID := newMnt139App(t, true)
	rec := doGetWith(t, e, "/maintenance/"+planID+"?view=calendar&month=10", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	st := rec.Header().Get("Server-Timing")
	if !strings.Contains(st, "total;dur=") || !strings.Contains(st, `desc="n=`) {
		t.Fatalf("Server-Timing 없음: %s", st)
	}
	if parseTimingQueries(st) < 1 {
		t.Fatalf("쿼리 수 0: %s", st)
	}
	src, err := os.ReadFile(mnt139MainGoPath())
	if err != nil {
		t.Fatal(err)
	}
	txt := string(src)
	if !strings.Contains(txt, "dbprof.OpenAppDB") || !strings.Contains(txt, "MntProfileMiddleware") {
		t.Fatal("cmd/server 가 MNT_PROFILE 경로를 안 탄다")
	}
}

func Test95_139_T4_DefaultDriverDoesNotCount(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	e, _, planID := newMnt139App(t, false)
	rec := doGetWith(t, e, "/maintenance/"+planID+"?view=list", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	st := rec.Header().Get("Server-Timing")
	if parseTimingQueries(st) != 0 {
		t.Fatalf("sqlite 드라이버면 쿼리 카운트는 0이어야 한다: %s", st)
	}
}

func Test95_139_T5_FixtureUsesTempDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mnt139.db")
	if strings.Contains(path, string(filepath.Separator)+"data"+string(filepath.Separator)+"app.db") {
		t.Fatal("운영 DB 경로")
	}
	t.Setenv(dbprof.EnvKey, "1")
	dbprof.Register()
	db, err := repository.InitDBDriver(dbprof.DriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
}

func Test95_139_T3_Measure850Visits(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	e, db, planID := newMnt139App(t, true)
	seedMnt139Visits(t, db, planID, 850, 130)
	var nVisits, nOpen int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits WHERE plan_id=?`, planID).Scan(&nVisits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits WHERE plan_id=? AND completed=0`, planID).Scan(&nOpen); err != nil {
		t.Fatal(err)
	}
	if nVisits != 850 || nOpen != 130 {
		t.Fatalf("픽스처 visits=%d open=%d", nVisits, nOpen)
	}

	type row struct {
		name, path string
		status     int
		wallMs     float64
		queries    int64
		kb         float64
		timing     string
	}
	screens := []struct{ name, path string }{
		{"목록", "/maintenance/" + planID + "?view=list"},
		{"목록·월10", "/maintenance/" + planID + "?view=list&month=10"},
		{"캘린더·월10", "/maintenance/" + planID + "?view=calendar&month=10"},
		{"캘린더·월11", "/maintenance/" + planID + "?view=calendar&month=11"},
		{"캘린더·월10·접수", "/maintenance/" + planID + "?view=calendar&month=10&show=plan"},
		{"캘린더·월10·완료", "/maintenance/" + planID + "?view=calendar&month=10&show=act"},
		{"칸반·월10", "/maintenance/" + planID + "?view=kanban&month=10"},
		{"칸반·월11", "/maintenance/" + planID + "?view=kanban&month=11"},
		{"관리", "/maintenance/" + planID + "/manage"},
		{"관리·월10", "/maintenance/" + planID + "/manage?um=10"},
	}
	warmup := doGetWith(t, e, screens[0].path, jwtCookie(t))
	if warmup.Code != http.StatusOK {
		t.Fatalf("warmup %d", warmup.Code)
	}
	var rows []row
	for _, sc := range screens {
		start := time.Now()
		rec := doGetWith(t, e, sc.path, jwtCookie(t))
		wall := time.Since(start)
		st := rec.Header().Get("Server-Timing")
		r := row{
			name: sc.name, path: sc.path, status: rec.Code,
			wallMs: msDur(wall), queries: parseTimingQueries(st),
			kb: float64(rec.Body.Len()) / 1024, timing: st,
		}
		rows = append(rows, r)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status %d", sc.name, rec.Code)
		}
		if r.queries < 1 {
			t.Errorf("%s 쿼리 수 없음 %s", sc.name, st)
		}
	}
	fmt.Printf("\n95-139 측정 (방문 %d · 미완료 %d, TempDB)\n", nVisits, nOpen)
	fmt.Printf("%-18s %8s %8s %8s  Server-Timing\n", "화면", "ms", "쿼리", "KB")
	for _, r := range rows {
		t.Logf("%s status=%d wall=%.0fms q=%d kb=%.1f timing=%s", r.name, r.status, r.wallMs, r.queries, r.kb, r.timing)
		fmt.Printf("%-18s %8.0f %8d %8.1f  %s\n", r.name, r.wallMs, r.queries, r.kb, compactTiming(r.timing))
	}
}

func msDur(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func parseTimingQueries(st string) int64 {
	m := regexp.MustCompile(`desc="n=(\d+)"`).FindStringSubmatch(st)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	return n
}

func compactTiming(st string) string {
	st = strings.ReplaceAll(st, " ", "")
	if len(st) > 180 {
		return st[:180] + "…"
	}
	return st
}

func mnt139MainGoPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "cmd", "server", "main.go"))
}

func newMnt139App(t *testing.T, profileDriver bool) (*echo.Echo, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m139.db")
	var db *sql.DB
	var err error
	if profileDriver {
		dbprof.Register()
		db, err = repository.InitDBDriver(dbprof.DriverName, path)
	} else {
		db, err = repository.InitDB(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c139','측정사이트','측정사이트',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	p, err := mnt.CreatePlanForOrg(model.OrgIDLibrary, 2026, "측정")
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(MntProfileMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.GET("/maintenance/:id/manage", h.Maintenance.ManagePlan)
	return e, db, p.PlanID
}

func seedMnt139Visits(t *testing.T, db *sql.DB, planID string, total, incomplete int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category,
		 notes, assignee, product_type, completed, completed_date, round_no, data_origin)
		VALUES (?,?,?,?,?,?, 'normal', '', '관리자', ?, ?, ?, ?, 'app')`)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		var date string
		var round, completed int
		doneOn := ""
		if i < incomplete {
			date = oct.AddDate(0, 0, i%31).Format("2006-01-02")
			round = i/31 + 1
			completed = 0
		} else {
			j := i - incomplete
			date = jan.AddDate(0, 0, j%365).Format("2006-01-02")
			round = j/365 + 20
			completed = 1
			doneOn = date
		}
		if _, err := stmt.Exec(fmt.Sprintf("m139_%04d", i), planID, date, "c139", i, 0, fmt.Sprintf("KLAS-%d", i), completed, doneOn, round); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func Test95_139_OpenLiveMatchesVisitHasOpenLive(t *testing.T) {
	e, db, planID := newMnt139App(t, true)
	_ = e
	seedMnt139Visits(t, db, planID, 40, 12)
	openIDs := seedMnt139WorkTasks(t, db, 500, 10, []string{"m139_0001", "m139_0002"})
	h := New(db)
	visits, err := repository.NewMaintenanceRepo(db).ListVisits(planID)
	if err != nil {
		t.Fatal(err)
	}
	batch := h.Maintenance.openLiveVisitIDs(visits)
	for _, v := range visits {
		got := batch[v.VisitID]
		want := h.Maintenance.visitHasOpenLive(v.VisitID)
		if got != want {
			t.Fatalf("%s batch=%v visitHasOpenLive=%v", v.VisitID, got, want)
		}
	}
	if !batch["m139_0001"] || !batch["m139_0002"] {
		t.Fatalf("열린 구간 방문이 빠짐 %+v openSeed=%v", batch, openIDs)
	}
	if batch["m139_0003"] {
		t.Fatal("닫힌 구간 방문이 열린 것으로 나옴")
	}
	if batch["m139_0010"] {
		t.Fatal("연결 작업 없는 미완료가 열린 것으로 나옴")
	}
}

func Test95_139_QueryCap60(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	e, db, planID := newMnt139App(t, true)
	seedMnt139Visits(t, db, planID, 850, 130)
	seedMnt139WorkTasks(t, db, 5000, 800, []string{"m139_0000", "m139_0005", "m139_0010"})
	paths := []struct {
		path string
		max  int64
	}{
		{"/maintenance/" + planID + "?view=list", 70},
		{"/maintenance/" + planID + "?view=list&month=10", 60},
		{"/maintenance/" + planID + "?view=calendar&month=10", 60},
		{"/maintenance/" + planID + "?view=kanban&month=10", 60},
	}
	for _, p := range paths {
		rec := doGetWith(t, e, p.path, jwtCookie(t))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", p.path, rec.Code)
		}
		n := parseTimingQueries(rec.Header().Get("Server-Timing"))
		t.Logf("%s q=%d max=%d", p.path, n, p.max)
		if n > p.max {
			t.Errorf("%s 쿼리 %d > %d", p.path, n, p.max)
		}
	}
}

func Test95_139_BatchVsNaive5000(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	_, db, planID := newMnt139App(t, true)
	seedMnt139Visits(t, db, planID, 850, 130)
	seedMnt139WorkTasks(t, db, 5000, 800, nil)
	visits, err := repository.NewMaintenanceRepo(db).ListVisits(planID)
	if err != nil {
		t.Fatal(err)
	}
	var incomplete []string
	for _, v := range visits {
		if !v.Completed {
			incomplete = append(incomplete, v.VisitID)
		}
	}
	wb := repository.NewWBRepo(db)
	var naive int64
	dbprof.Begin(&naive)
	for _, id := range incomplete {
		_, _ = wb.GetTaskBySource(model.WBSourceMaintenance, id)
	}
	dbprof.End()
	var batch int64
	dbprof.Begin(&batch)
	_, err = wb.ListTasksBySourceIDs(model.WBSourceMaintenance, incomplete)
	if err != nil {
		t.Fatal(err)
	}
	dbprof.End()
	t.Logf("work_tasks 5000 · 방문연결 800 · 미완료 %d · GetTaskBySource×N=%d · ListTasksBySourceIDs=%d", len(incomplete), naive, batch)
	if batch == 0 || naive == 0 {
		t.Fatalf("측정 실패 naive=%d batch=%d", naive, batch)
	}
	if batch >= naive {
		t.Fatalf("일괄이 더 많음 naive=%d batch=%d", naive, batch)
	}
}

func Test95_139_ExplainUsesSourceIndex(t *testing.T) {
	_, db, _ := newMnt139App(t, false)
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT task_id FROM work_tasks WHERE source_type=? AND source_id IN (?,?)`,
		model.WBSourceMaintenance, "m139_0000", "m139_0001")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var a, b, c int
		var d string
		if err := rows.Scan(&a, &b, &c, &d); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(d)
		plan.WriteByte('\n')
	}
	txt := plan.String()
	t.Log(txt)
	if strings.Contains(strings.ToLower(txt), "scan table work_tasks") && !strings.Contains(strings.ToLower(txt), "using index") {
		t.Fatalf("work_tasks 풀스캔: %s", txt)
	}
	srows, err := db.Query(`EXPLAIN QUERY PLAN SELECT DISTINCT task_id FROM work_task_segments WHERE task_id IN (?) AND kind=? AND TRIM(COALESCE(end_time,''))=''`,
		"WT1", model.SegKindActual)
	if err != nil {
		t.Fatal(err)
	}
	defer srows.Close()
	plan.Reset()
	for srows.Next() {
		var a, b, c int
		var d string
		if err := srows.Scan(&a, &b, &c, &d); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(d)
		plan.WriteByte('\n')
	}
	txt = plan.String()
	t.Log(txt)
	if strings.Contains(strings.ToLower(txt), "scan table work_task_segments") && !strings.Contains(strings.ToLower(txt), "using index") {
		t.Fatalf("work_task_segments 풀스캔: %s", txt)
	}
}

func Test95_139_T3_Measure850VisitsAfter5000Tasks(t *testing.T) {
	t.Setenv(dbprof.EnvKey, "1")
	e, db, planID := newMnt139App(t, true)
	seedMnt139Visits(t, db, planID, 850, 130)
	seedMnt139WorkTasks(t, db, 5000, 800, []string{"m139_0000"})
	screens := []struct{ name, path string }{
		{"목록", "/maintenance/" + planID + "?view=list"},
		{"캘린더·월10", "/maintenance/" + planID + "?view=calendar&month=10"},
		{"캘린더·월11", "/maintenance/" + planID + "?view=calendar&month=11"},
		{"칸반·월10", "/maintenance/" + planID + "?view=kanban&month=10"},
		{"관리", "/maintenance/" + planID + "/manage"},
	}
	fmt.Printf("\n95-139 구현 후 (방문 850 · 미완료 130 · work_tasks 5000 · 방문연결 800)\n")
	fmt.Printf("%-18s %8s %8s %8s\n", "화면", "ms", "쿼리", "KB")
	for _, sc := range screens {
		start := time.Now()
		rec := doGetWith(t, e, sc.path, jwtCookie(t))
		st := rec.Header().Get("Server-Timing")
		fmt.Printf("%-18s %8.0f %8d %8.1f  %s\n", sc.name, msDur(time.Since(start)), parseTimingQueries(st), float64(rec.Body.Len())/1024, compactTiming(st))
		if rec.Code != http.StatusOK {
			t.Errorf("%s status %d", sc.name, rec.Code)
		}
	}
	if sc := screens[len(screens)-1]; sc.name == "관리" {
		_ = sc
	}
}

func seedMnt139WorkTasks(t *testing.T, db *sql.DB, total, linked int, openVisitIDs []string) []string {
	t.Helper()
	if total < linked {
		t.Fatalf("total=%d linked=%d", total, linked)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, status, priority, assignee, source_type, source_id, org_id)
		VALUES (?,?,?,?,?,'waiting','normal','관리자',?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	seg, err := tx.Prepare(`INSERT INTO work_task_segments
		(segment_id, task_id, kind, seq, work_date, start_time, end_time)
		VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer seg.Close()
	openSet := map[string]bool{}
	for _, id := range openVisitIDs {
		openSet[id] = true
	}
	var opened []string
	for i := 0; i < total; i++ {
		tid := fmt.Sprintf("wt139_%04d", i)
		srcType, srcID := "", ""
		if i < linked {
			srcType = model.WBSourceMaintenance
			srcID = fmt.Sprintf("m139_%04d", i)
		} else {
			srcType = "other"
			srcID = tid
		}
		if _, err := stmt.Exec(tid, model.WBWorkMaintenance, "더미"+tid, "2026-10-01", "2026-10-01", srcType, srcID, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
		end := "11:00"
		if openSet[srcID] {
			end = ""
			opened = append(opened, srcID)
		}
		if _, err := seg.Exec("sg139_"+tid, tid, model.SegKindActual, 1, "2026-10-01", "09:00", end); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return opened
}
