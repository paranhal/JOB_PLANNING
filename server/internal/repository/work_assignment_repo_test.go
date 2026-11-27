package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
)

type env225 struct {
	db *sql.DB
	ar *AssignmentRepo
}

func setup225(t *testing.T) env225 {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "a225.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O01','O01','도서관사업팀',1),('O03','O03','미디어사업팀',1)`,
		`INSERT INTO users(user_id,username,password_hash,full_name,role,is_active,org_id) VALUES
			('UA','ua','x','배정자','tech',1,'O01'),('UB','ub','x','수락자','tech',1,'O01'),('UC','uc','x','타조직','tech',1,'O03'),('UD','ud','x','비활성','tech',0,'O03')`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','가도서관','가도서관',1,'O01'),('C2','나도서관','나도서관',1,'O01')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O01')`,
		`INSERT INTO project_phases(phase_id,project_id,org_id,name,seq,status,created_at,updated_at) VALUES ('PH1','WP1','O01','구축',1,'active',datetime('now'),datetime('now'))`,
		`INSERT INTO phase_packages(package_id,phase_id,seq,name,created_at,updated_at) VALUES ('PK1','PH1',1,'장비 설치',datetime('now'),datetime('now'))`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,package_id,work_date,due_date) VALUES ('T1','project_work','작업','waiting','O01','WP1','PK1','2026-12-01','2026-12-01')`,
		`INSERT INTO work_task_sites(task_id,customer_id) VALUES ('T1','C1'),('T1','C2')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,project_id,parent_task_id,work_date,due_date) VALUES ('T1S','admin','하부 작업','waiting','O01','WP1','T1','2026-12-01','2026-12-01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	return env225{db: db, ar: NewAssignmentRepo(db)}
}

func (e env225) status(t *testing.T, id string) string {
	t.Helper()
	var s string
	_ = e.db.QueryRow(`SELECT status FROM work_assignments WHERE id=?`, id).Scan(&s)
	return s
}

// 생성: 표시용 스냅샷(사업명)·진행 중 중복 금지·자기 배정/비활성 대상 거절.
func TestAssignmentCreate225(t *testing.T) {
	e := setup225(t)
	a, err := e.ar.Create("T1", AssignTargetSub, "PK1", "UA", "UB")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != AssignPending || a.ProjectName != "사업A" || a.TargetLabel != "장비 설치" || a.FromName != "배정자" || a.FromOrgName != "도서관사업팀" || a.Heading() != "사업A" {
		t.Fatalf("스냅샷 %+v", a)
	}
	if _, err := e.ar.Create("T1", AssignTargetSub, "PK1", "UA", "UC"); err == nil {
		t.Fatal("같은 대상에 진행 중인 배정이 둘이 됐다")
	}
	if _, err := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UA"); err == nil {
		t.Fatal("자기 자신 배정이 통과했다")
	}
	if _, err := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UD"); err == nil {
		t.Fatal("비활성 사용자 배정이 통과했다")
	}
	if _, err := e.ar.Create("T1", AssignTargetSite, "C-NOPE", "UA", "UB"); err == nil {
		t.Fatal("업무에 없는 사이트 배정이 통과했다")
	}
	if _, err := e.ar.Create("T1", AssignTargetSub, "PK-NOPE", "UA", "UB"); err == nil {
		t.Fatal("업무와 무관한 하부 업무 배정이 통과했다")
	}
	// 타 조직 사용자에게도 배정된다
	if _, err := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UC"); err != nil {
		t.Fatalf("타 조직 배정 %v", err)
	}
}

// 상태 전이 표 전부: pending→accepted/held/rejected, held→accepted/rejected, held→held 불가, 종결 후 재처리 불가, 남의 배정 불가.
func TestAssignmentTransitions225(t *testing.T) {
	e := setup225(t)
	mk := func(site string) string {
		a, err := e.ar.Create("T1", AssignTargetSite, site, "UA", "UB")
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	// pending → held → accepted
	id := mk("C1")
	if _, err := e.ar.Decide(id, "UC", "accept", ""); err != ErrAssignNotMine {
		t.Fatalf("남의 배정 처리 %v", err)
	}
	if a, err := e.ar.Decide(id, "UB", "hold", ""); err != nil || a.Status != AssignHeld {
		t.Fatalf("보류 %v %+v", err, a)
	}
	if _, err := e.ar.Decide(id, "UB", "hold", ""); err != ErrAssignTransition {
		t.Fatalf("held→held %v", err)
	}
	if a, err := e.ar.Decide(id, "UB", "accept", ""); err != nil || a.Status != AssignAccepted {
		t.Fatalf("보류→수락 %v", err)
	}
	for _, act := range []string{"accept", "reject", "hold"} {
		if _, err := e.ar.Decide(id, "UB", act, "사유"); err != ErrAssignTransition {
			t.Fatalf("수락 뒤 %s: %v want 종결", act, err)
		}
	}
	// pending → rejected (사유 필수), 종결
	id2 := mk("C2")
	if _, err := e.ar.Decide(id2, "UB", "reject", "   "); err != ErrAssignReason {
		t.Fatalf("빈 사유 %v", err)
	}
	if e.status(t, id2) != AssignPending {
		t.Fatal("사유 없는 거부가 상태를 바꿨다")
	}
	a, err := e.ar.Decide(id2, "UB", "reject", "일정이 겹칩니다")
	if err != nil || a.Status != AssignRejected || a.RejectReason != "일정이 겹칩니다" {
		t.Fatalf("거부 %v %+v", err, a)
	}
	if _, err := e.ar.Decide(id2, "UB", "accept", ""); err != ErrAssignTransition {
		t.Fatalf("거부 뒤 수락 %v", err)
	}
	// 거부된 대상은 다시 배정할 수 있다(진행 중 배정이 없음)
	if _, err := e.ar.Create("T1", AssignTargetSite, "C2", "UA", "UC"); err != nil {
		t.Fatalf("거부 뒤 재배정 %v", err)
	}
	// 이력: 모든 전이가 변경 이력에 남는다
	var n int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='work_assignments' AND entity_id=?`, id).Scan(&n)
	if n < 3 {
		t.Fatalf("전이 이력 %d건 (요청·보류·수락 3건 이상 필요)", n)
	}
}

// 종결된 대상에 대한 수락은 자동 거부 + 안내. 수락 효과: 하부 작업은 담당자 연결.
func TestAssignmentClosedTargetAndAcceptEffect225(t *testing.T) {
	e := setup225(t)
	id1, _ := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UB")
	if _, err := e.db.Exec(`INSERT INTO work_task_site_progress(task_id,customer_id,work_date,status,checked_by,checked_at) VALUES ('T1','C1','2026-12-01','done','UA','x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ar.Decide(id1.ID, "UB", "accept", ""); err != ErrAssignClosed {
		t.Fatalf("종결 대상 수락 %v", err)
	}
	if e.status(t, id1.ID) != AssignRejected {
		t.Fatalf("종결 대상 수락 뒤 상태 %s", e.status(t, id1.ID))
	}
	// 이미 완료된 사이트에는 새 배정도 안 된다
	if _, err := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UB"); err == nil {
		t.Fatal("완료된 사이트 배정이 통과했다")
	}
	// 하부 작업 수락 → 같은 조직 수락자는 담당자 연결, 타 조직 수락자는 담당 열을 건드리지 않는다(95-215)
	id2, err := e.ar.Create("T1", AssignTargetSubtask, "T1S", "UA", "UC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ar.Decide(id2.ID, "UC", "accept", ""); err != nil {
		t.Fatal(err)
	}
	var who, uid string
	_ = e.db.QueryRow(`SELECT COALESCE(assignee,''), COALESCE(assignee_user_id,'') FROM work_tasks WHERE task_id='T1S'`).Scan(&who, &uid)
	if who != "" || uid != "" {
		t.Fatalf("타 조직 수락자가 담당 열에 들어갔다 %q/%q", who, uid)
	}
	e.exec225(t, `UPDATE work_assignments SET status='rejected', reject_reason='재배정' WHERE id=?`, id2.ID)
	id3, _ := e.ar.Create("T1", AssignTargetSubtask, "T1S", "UA", "UB")
	if _, err := e.ar.Decide(id3.ID, "UB", "accept", ""); err != nil {
		t.Fatal(err)
	}
	_ = e.db.QueryRow(`SELECT COALESCE(assignee,''), COALESCE(assignee_user_id,'') FROM work_tasks WHERE task_id='T1S'`).Scan(&who, &uid)
	if who != "수락자" || uid != "UB" {
		t.Fatalf("같은 조직 수락 후 담당 %q/%q", who, uid)
	}
	id2 = id3
	// 범위 판정
	if !e.ar.HasAcceptedOnTask("UB", "T1") || e.ar.HasAcceptedOnTask("UC", "T1") {
		t.Fatal("수락 범위 판정이 틀렸다")
	}
	if !e.ar.AcceptedScope("UB", "T1", AssignTargetSubtask, "T1S") || e.ar.AcceptedScope("UB", "T1", AssignTargetSite, "C2") {
		t.Fatal("대상별 범위 판정이 틀렸다")
	}
}

// 팝업: 세션 첫 확인은 대기+보류, 이후 폴링은 대기만. 건수·비활성화 반환.
func TestAssignmentPopupAndReturn225(t *testing.T) {
	e := setup225(t)
	a1, _ := e.ar.Create("T1", AssignTargetSite, "C1", "UA", "UB")
	a2, _ := e.ar.Create("T1", AssignTargetSite, "C2", "UA", "UB")
	if _, err := e.ar.Decide(a2.ID, "UB", "hold", ""); err != nil {
		t.Fatal(err)
	}
	if p, h := e.ar.CountOpenForUser("UB"); p != 1 || h != 1 {
		t.Fatalf("건수 %d/%d", p, h)
	}
	first, _ := e.ar.PopupItems("UB", true)
	if len(first) != 2 {
		t.Fatalf("첫 확인 %d건 want 2(대기+보류)", len(first))
	}
	poll, _ := e.ar.PopupItems("UB", false)
	if len(poll) != 1 || poll[0].ID != a1.ID {
		t.Fatalf("폴링 %v want 대기 1건", poll)
	}
	var notified string
	_ = e.db.QueryRow(`SELECT last_notified_at FROM work_assignments WHERE id=?`, a1.ID).Scan(&notified)
	if notified == "" {
		t.Fatal("팝업에 띄운 항목에 last_notified_at 이 없다")
	}
	// 사용자 비활성화: 대기·보류 배정이 배정자에게 거부 사유와 함께 되돌아간다
	if n := e.ar.ReturnForUser("UB"); n != 2 {
		t.Fatalf("반환 %d건 want 2", n)
	}
	got, _ := e.ar.Get(a2.ID)
	if got.Status != AssignRejected || got.RejectReason == "" {
		t.Fatalf("반환된 배정 %+v", got)
	}
	if p, h := e.ar.CountOpenForUser("UB"); p != 0 || h != 0 {
		t.Fatalf("반환 뒤 건수 %d/%d", p, h)
	}
}

func (e env225) exec225(t *testing.T, q string, a ...interface{}) {
	t.Helper()
	if _, err := e.db.Exec(q, a...); err != nil {
		t.Fatal(q, err)
	}
}
