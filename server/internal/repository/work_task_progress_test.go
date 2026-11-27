package repository

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

type env226 struct {
	db *sql.DB
	wb *WBRepo
}

func setup226(t *testing.T) env226 {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "p226.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','가','가',1,'O01'),('C2','나','나',1,'O01'),('C3','다','다',1,'O01'),('C4','라','라',1,'O01')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O01')`,
		`INSERT INTO project_phases(phase_id,project_id,org_id,name,seq,status,created_at,updated_at) VALUES ('PH1','WP1','O01','구축',1,'active',datetime('now'),datetime('now'))`,
		`INSERT INTO phase_packages(package_id,phase_id,seq,name,created_at,updated_at) VALUES ('PK1','PH1',1,'설계',datetime('now'),datetime('now')),('PK2','PH1',2,'제작',datetime('now'),datetime('now')),('PK3','PH1',3,'설치',datetime('now'),datetime('now'))`,
		`INSERT INTO package_sites(package_site_id,package_id,project_site_id,updated_at) VALUES ('PS1','PK3','X1',datetime('now'))`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	return env226{db: db, wb: NewWBRepo(db)}
}

func (e env226) task(t *testing.T, id, parent, work, due, status string) {
	t.Helper()
	if _, err := e.db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,parent_task_id,work_date,due_date,created_by_user_id) VALUES (?,'admin',?,?,'O01',NULLIF(?,''),?,?,'U1')`,
		id, "업무 "+id, status, parent, work, due); err != nil {
		t.Fatal(err)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.06 }
func f(v float64) *float64   { return &v }

// 부하량: 입력 해석·검증·균등 분할.
func TestLoadParseValidateEffective226(t *testing.T) {
	for _, bad := range []string{"-1", "abc", "101", "1e999"} {
		if _, err := ParseLoad(bad); err == nil {
			t.Fatalf("%q 가 통과했다", bad)
		}
	}
	if v, err := ParseLoad(""); err != nil || v != nil {
		t.Fatal("빈 값은 자동(nil)이어야 한다")
	}
	if v, err := ParseLoad(" 33.3% "); err != nil || !near(*v, 33.3) {
		t.Fatalf("소수·퍼센트 허용 %v %v", v, err)
	}
	if ValidateLoads([]*float64{f(60), f(41)}) == nil {
		t.Fatal("합계 101 이 통과했다")
	}
	if ValidateLoads([]*float64{f(60), f(30)}) != nil {
		t.Fatal("합계 90(100 미만)은 통과해야 한다")
	}
	eq := EffectiveLoads([]*float64{nil, nil, nil, nil})
	for _, v := range eq {
		if !near(v, 25) {
			t.Fatalf("균등 분할 %v", eq)
		}
	}
	mix := EffectiveLoads([]*float64{f(50), nil, nil})
	if !near(mix[0], 50) || !near(mix[1], 25) || !near(mix[2], 25) {
		t.Fatalf("일부 입력 시 나머지 균등 %v", mix)
	}
}

// 하부 업무·하위 업무 부하량 저장: 합 100 초과 거부(아무것도 저장 안 됨), 사이트가 있는 하부 업무는 부하량 없음, 균등 분할 표시.
func TestSaveLoads226(t *testing.T) {
	e := setup226(t)
	// 하부 업무: PK3 는 사이트가 있어 부하량을 쓰지 않는다
	if err := e.wb.SavePackageLoads("PH1", map[string]*float64{"PK1": f(60), "PK2": f(50)}, "u"); err != ErrLoadSum {
		t.Fatalf("합 110 %v want 거부", err)
	}
	var n int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM phase_packages WHERE load_pct IS NOT NULL`).Scan(&n)
	if n != 0 {
		t.Fatal("거부했는데 일부가 저장됐다")
	}
	if err := e.wb.SavePackageLoads("PH1", map[string]*float64{"PK3": f(10)}, "u"); err != ErrLoadSites {
		t.Fatalf("사이트 있는 하부 업무 %v want ErrLoadSites", err)
	}
	if err := e.wb.SavePackageLoads("PH1", map[string]*float64{"PK1": f(70)}, "u"); err != nil {
		t.Fatal(err)
	}
	view := e.wb.PhaseLoadView("PH1")
	if len(view) != 3 || !near(view[0].Effective, 70) || !near(view[1].Effective, 30) || !view[2].HasSites || view[2].Effective != 0 {
		t.Fatalf("부하량 현황 %+v", view)
	}
	// 하위 업무
	e.task(t, "P", "", "2026-12-01", "2026-12-03", "waiting")
	e.task(t, "K1", "P", "2026-12-01", "2026-12-01", "waiting")
	e.task(t, "K2", "P", "2026-12-02", "2026-12-02", "waiting")
	if err := e.wb.SaveSubtaskLoads("P", map[string]*float64{"K1": f(-5)}, "u"); err == nil {
		t.Fatal("음수가 통과했다")
	}
	if err := e.wb.SaveSubtaskLoads("P", map[string]*float64{"K1": f(40)}, "u"); err != nil {
		t.Fatal(err)
	}
	sv := e.wb.SubtaskLoadView("P")
	if !near(sv[0].Effective, 40) || !near(sv[1].Effective, 60) {
		t.Fatalf("하위 업무 부하량 %+v", sv)
	}
	if err := e.wb.SaveSubtaskLoads("P", map[string]*float64{"NOPE": f(10)}, "u"); err == nil {
		t.Fatal("업무에 없는 하위 업무가 통과했다")
	}
	var logs int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE reason LIKE '%부하량 변경%'`).Scan(&logs)
	if logs < 2 {
		t.Fatalf("부하량 변경 이력 %d건", logs)
	}
}

// 진행률 자동 3모드(사이트 · 하위 부하량 · 일자별)와 단일, 수동 우선 표시.
func TestComputeProgress226(t *testing.T) {
	e := setup226(t)
	// 사이트 모드: 4곳 중 1곳 완료 = 25%, 남은 3
	e.task(t, "S", "", "2026-12-01", "2026-12-01", "waiting")
	for _, c := range []string{"C1", "C2", "C3", "C4"} {
		e.db.Exec(`INSERT INTO work_task_sites(task_id,customer_id) VALUES ('S',?)`, c)
	}
	e.db.Exec(`INSERT INTO work_task_site_progress(task_id,customer_id,work_date,status,checked_by,checked_at) VALUES ('S','C1','2026-12-01','done','u','x')`)
	p := e.wb.ComputeProgress("S")
	if p.Mode != "sites" || !near(p.Auto, 25) || p.Remaining != 3 || p.Source != "auto" {
		t.Fatalf("사이트 모드 %+v", p)
	}
	// 하위 업무 모드: 부하량 50·30·자동(20), 앞 둘 완료 = 80
	e.task(t, "P", "", "2026-12-01", "2026-12-03", "waiting")
	e.task(t, "K1", "P", "2026-12-01", "2026-12-01", "complete")
	e.task(t, "K2", "P", "2026-12-02", "2026-12-02", "complete")
	e.task(t, "K3", "P", "2026-12-03", "2026-12-03", "waiting")
	if err := e.wb.SaveSubtaskLoads("P", map[string]*float64{"K1": f(50), "K2": f(30)}, "u"); err != nil {
		t.Fatal(err)
	}
	p = e.wb.ComputeProgress("P")
	if p.Mode != "subtasks" || !near(p.Auto, 80) || p.Done != 2 || p.Remaining != 1 {
		t.Fatalf("하위 모드 %+v", p)
	}
	// 균등 분할(모두 비어 있음): 3개 중 1개 완료 = 33.3
	e.task(t, "Q", "", "2026-12-01", "2026-12-03", "waiting")
	e.task(t, "Q1", "Q", "2026-12-01", "2026-12-01", "complete")
	e.task(t, "Q2", "Q", "2026-12-02", "2026-12-02", "waiting")
	e.task(t, "Q3", "Q", "2026-12-03", "2026-12-03", "waiting")
	if p = e.wb.ComputeProgress("Q"); !near(p.Auto, 33.3) {
		t.Fatalf("균등 분할 진행률 %+v", p)
	}
	// 일자별 모드: 4일, 1일차 100% · 2일차 50% = 37.5
	e.task(t, "D", "", "2026-12-01", "2026-12-04", "waiting")
	if _, err := e.wb.SaveDayProgress("D", "2026-12-01", nil, 100, "", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.wb.SaveDayProgress("D", "2026-12-02", nil, 50, "절반", "u"); err != nil {
		t.Fatal(err)
	}
	p = e.wb.ComputeProgress("D")
	if p.Mode != "days" || !near(p.Auto, 37.5) || p.Remaining != 3 {
		t.Fatalf("일자별 모드 %+v", p)
	}
	// 수동: 자동값과 병기하고 수동을 먼저 보인다. 변경 이력이 남는다.
	if err := e.wb.SetManualProgress("D", f(60), "u"); err != nil {
		t.Fatal(err)
	}
	p = e.wb.ComputeProgress("D")
	if p.Source != "manual" || !near(p.Shown, 60) || !near(p.Auto, 37.5) || p.Manual == nil {
		t.Fatalf("수동 우선 %+v", p)
	}
	if err := e.wb.SetManualProgress("D", f(120), "u"); err != ErrManualPct {
		t.Fatalf("범위 밖 수동값 %v", err)
	}
	var logs int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE entity_id='D' AND reason LIKE '수동 진행률%'`).Scan(&logs)
	if logs != 1 {
		t.Fatalf("수동 진행률 이력 %d건", logs)
	}
	if err := e.wb.SetManualProgress("D", nil, "u"); err != nil || e.wb.ComputeProgress("D").Source != "auto" {
		t.Fatal("수동 해제 뒤 자동으로 돌아가야 한다")
	}
	// 남은 건수는 업무에 저장된다(다음 날 이어서 표시)
	var rem int
	_ = e.db.QueryRow(`SELECT remain_count FROM work_tasks WHERE task_id='S'`).Scan(&rem)
	if rem != 3 {
		t.Fatalf("저장된 남은 건수 %d", rem)
	}
}

// 부분 완료: 일자별 누적, 마지막 부분 완료 때 업무 완료 + complete_date. 95-223 #3 선택(오늘까지만/전체 완료).
func TestPartialCompleteAndChoice226(t *testing.T) {
	e := setup226(t)
	e.task(t, "D", "", "2026-12-01", "2026-12-03", "waiting")
	if _, err := e.wb.SaveDayProgress("D", "2026-12-09", nil, 10, "", "u"); err != ErrDayRange {
		t.Fatalf("기간 밖 날짜 %v", err)
	}
	if _, err := e.wb.SaveDayProgress("D", "2026-12-01", nil, 150, "", "u"); err != ErrDonePct {
		t.Fatalf("150 초과 %v", err)
	}
	if _, err := e.wb.SaveDayProgress("D", "2026-12-01", f(70), 100, "", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.wb.SaveDayProgress("D", "2026-12-02", f(50), 100, "", "u"); err != ErrLoadSum {
		t.Fatalf("부하량 합 120 %v", err)
	}
	done, err := e.wb.SaveDayProgress("D", "2026-12-02", f(20), 100, "", "u")
	if err != nil || done {
		t.Fatalf("2일차 %v %v", done, err)
	}
	var st string
	_ = e.db.QueryRow(`SELECT status FROM work_tasks WHERE task_id='D'`).Scan(&st)
	if st != "in_progress" {
		t.Fatalf("부분 완료 중 상태 %s", st)
	}
	done, err = e.wb.SaveDayProgress("D", "2026-12-03", nil, 100, "", "u")
	if err != nil || !done {
		t.Fatalf("마지막 부분 완료 %v %v", done, err)
	}
	var cd string
	_ = e.db.QueryRow(`SELECT status, complete_date FROM work_tasks WHERE task_id='D'`).Scan(&st, &cd)
	if st != "complete" || cd != "2026-12-03" {
		t.Fatalf("마지막 부분 완료 뒤 %s %s", st, cd)
	}
	// 선택창: 오늘까지만 → 남은 날 계속, 전체 완료 → 이후 일정 제외
	e.task(t, "A", "", "2026-12-01", "2026-12-03", "waiting")
	if c, err := e.wb.CompleteMultiDay("A", "today", "2026-12-01", "u"); err != nil || c {
		t.Fatalf("오늘까지만 %v %v", c, err)
	}
	days := e.wb.ListDays("A")
	if days[0].Done != 100 || days[1].Done != 0 {
		t.Fatalf("오늘까지만 뒤 일자 %+v", days)
	}
	_ = e.db.QueryRow(`SELECT status FROM work_tasks WHERE task_id='A'`).Scan(&st)
	if st == "complete" {
		t.Fatal("오늘까지만인데 업무가 완료됐다")
	}
	e.task(t, "B", "", "2026-12-01", "2026-12-03", "waiting")
	if c, err := e.wb.CompleteMultiDay("B", "all", "2026-12-01", "u"); err != nil || !c {
		t.Fatalf("전체 완료 %v %v", c, err)
	}
	var due string
	_ = e.db.QueryRow(`SELECT status, due_date, complete_date FROM work_tasks WHERE task_id='B'`).Scan(&st, &due, &cd)
	if st != "complete" || due != "2026-12-01" || cd != "2026-12-01" {
		t.Fatalf("전체 완료 뒤 %s %s %s (이후 일정이 제외돼야 한다)", st, due, cd)
	}
	if _, err := e.wb.CompleteMultiDay("B", "x", "2026-12-01", "u"); err == nil {
		t.Fatal("알 수 없는 완료 방식이 통과했다")
	}
}

// 연기 → 대기 → 오늘 업무 → 미완료 복귀. 연기는 사유 입력 없이 대기로 가고 지연 통계(work_task_delays)에 들어가지 않는다.
func TestDeferFlow226(t *testing.T) {
	e := setup226(t)
	e.task(t, "T", "", "2026-12-01", "2026-12-01", "in_progress")
	if err := e.wb.DeferTask("T", "u"); err != nil {
		t.Fatal(err)
	}
	var work, due, st, reason, at, from string
	_ = e.db.QueryRow(`SELECT work_date, due_date, status, no_date_reason, deferred_at, defer_from_date FROM work_tasks WHERE task_id='T'`).Scan(&work, &due, &st, &reason, &at, &from)
	if work != "" || due != "" || st != "waiting" || reason != DeferNoDateReason || at == "" || from != "2026-12-01" {
		t.Fatalf("연기 뒤 %q %q %q %q %q %q", work, due, st, reason, at, from)
	}
	var delays int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM work_task_delays WHERE task_id='T'`).Scan(&delays)
	if delays != 0 {
		t.Fatal("연기가 지연 이력(통계)에 들어갔다")
	}
	if err := e.wb.DeferTask("T", "u"); err != ErrNotDefer {
		t.Fatalf("대기 중인 업무를 다시 연기 %v", err)
	}
	// 미계획 목록과 겹치지 않는다
	wbr := NewWorkBoardRepo(e.db)
	items, _, _ := wbr.ListUnplanned("", nil, "")
	for _, it := range items {
		if it.RefID == "T" {
			t.Fatal("대기 업무가 미계획 목록에도 보인다")
		}
	}
	// 대기 목록(그룹 제목) — 등록자 범위
	e.db.Exec(`UPDATE work_tasks SET project_id='WP1' WHERE task_id='T'`)
	list := e.wb.ListDeferred("O01", "U1", false)
	if len(list) != 1 || list[0].Heading() != "사업A" {
		t.Fatalf("대기 목록 %+v", list)
	}
	if other := e.wb.ListDeferred("O01", "U9", false); len(other) != 0 {
		t.Fatalf("범위 밖 사용자에게 대기 업무가 보인다 %+v", other)
	}
	// 오늘 업무로 등록
	if err := e.wb.UndeferTask("T", "2026-12-05", "09:00", "10:00", "u"); err != nil {
		t.Fatal(err)
	}
	var fd int
	_ = e.db.QueryRow(`SELECT work_date, from_deferred, COALESCE(deferred_at,'') FROM work_tasks WHERE task_id='T'`).Scan(&work, &fd, &at)
	if work != "2026-12-05" || fd != 1 || at != "" {
		t.Fatalf("대기→오늘 %q %d %q", work, fd, at)
	}
	if err := e.wb.UndeferTask("T", "2026-12-06", "", "", "u"); err != ErrNotDeferred {
		t.Fatalf("대기가 아닌 업무 등록 %v", err)
	}
	// 그날이 지나도 미완료면 다시 대기로, 완료한 것은 그대로
	if n := e.wb.ReturnUnfinishedDeferred("2026-12-05"); n != 0 {
		t.Fatalf("당일에는 복귀하지 않는다 %d", n)
	}
	if n := e.wb.ReturnUnfinishedDeferred("2026-12-06"); n != 1 {
		t.Fatalf("미완료 복귀 %d want 1", n)
	}
	_ = e.db.QueryRow(`SELECT work_date, COALESCE(deferred_at,'') FROM work_tasks WHERE task_id='T'`).Scan(&work, &at)
	if work != "" || at == "" {
		t.Fatal("미완료 업무가 대기로 돌아가지 않았다")
	}
	e.task(t, "U", "", "2026-12-01", "2026-12-01", "waiting")
	e.wb.DeferTask("U", "u")
	e.wb.UndeferTask("U", "2026-12-05", "", "", "u")
	e.db.Exec(`UPDATE work_tasks SET status='complete' WHERE task_id='U'`)
	if n := e.wb.ReturnUnfinishedDeferred("2026-12-09"); n != 0 {
		t.Fatalf("완료한 업무가 복귀했다 %d", n)
	}
	if err := e.wb.DeferTask("U", "u"); err != ErrNotDefer {
		t.Fatalf("완료 업무 연기 %v", err)
	}
}

// 95-223 #4: 완료 → 진행중 → 재완료해도 완료일이 오늘로 바뀌지 않는다(이전 완료일 보존). 직접 고친 완료일은 그 값.
func TestCompleteDateKept226(t *testing.T) {
	e := setup226(t)
	e.task(t, "T", "", "2026-12-01", "2026-12-01", "waiting")
	get := func() *model.WorkTask {
		tk, err := e.wb.GetTask("T")
		if err != nil || tk == nil {
			t.Fatal(err)
		}
		return tk
	}
	tk := get()
	tk.Status, tk.CompleteDate = model.WBTaskComplete, "2026-12-02"
	if err := e.wb.UpdateTask(tk); err != nil {
		t.Fatal(err)
	}
	tk = get()
	tk.Status, tk.CompleteDate = model.WBTaskInProgress, ""
	if err := e.wb.UpdateTask(tk); err != nil {
		t.Fatal(err)
	}
	var prev, cur string
	_ = e.db.QueryRow(`SELECT COALESCE(prev_complete_date,''), COALESCE(complete_date,'') FROM work_tasks WHERE task_id='T'`).Scan(&prev, &cur)
	if prev != "2026-12-02" || cur != "" {
		t.Fatalf("되돌린 뒤 prev=%q cur=%q", prev, cur)
	}
	tk = get()
	tk.Status, tk.CompleteDate = model.WBTaskComplete, ""
	if err := e.wb.UpdateTask(tk); err != nil {
		t.Fatal(err)
	}
	_ = e.db.QueryRow(`SELECT COALESCE(complete_date,'') FROM work_tasks WHERE task_id='T'`).Scan(&cur)
	if cur != "2026-12-02" {
		t.Fatalf("재완료 완료일 %q want 2026-12-02(보존)", cur)
	}
	tk = get()
	tk.Status, tk.CompleteDate = model.WBTaskInProgress, ""
	_ = e.wb.UpdateTask(tk)
	tk = get()
	tk.Status, tk.CompleteDate = model.WBTaskComplete, "2026-12-07"
	_ = e.wb.UpdateTask(tk)
	_ = e.db.QueryRow(`SELECT COALESCE(complete_date,'') FROM work_tasks WHERE task_id='T'`).Scan(&cur)
	if cur != "2026-12-07" {
		t.Fatalf("직접 고친 완료일 %q", cur)
	}
}
