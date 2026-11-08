package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func seedExecDash(t *testing.T) (*ExecDashRepo, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "execdash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	date := "2026-11-04"
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','다른팀','business',1,20),('O09','O09','빈조직','support',1,90)`); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepo(db)
	a := &model.User{Username: "a90", PasswordHash: "x", FullName: "갑", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	b := &model.User{Username: "b90", PasswordHash: "x", FullName: "을", Role: model.RoleTech, IsActive: true, OrgID: "O02"}
	c := &model.User{Username: "c90", PasswordHash: "x", FullName: "병", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	d := &model.User{Username: "d90", PasswordHash: "x", FullName: "정", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	e := &model.User{Username: "e90", PasswordHash: "x", FullName: "무", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	f := &model.User{Username: "f90", PasswordHash: "x", FullName: "기", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	g := &model.User{Username: "g90", PasswordHash: "x", FullName: "경", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	obs := &model.User{Username: "o90", PasswordHash: "x", FullName: "옵", Role: model.RoleObserver, IsActive: true, IsReadOnly: true, OrgID: "O01"}
	inact := &model.User{Username: "i90", PasswordHash: "x", FullName: "휴면", Role: model.RoleTech, IsActive: false, OrgID: "O01"}
	for _, u := range []*model.User{a, b, c, d, e, f, g, obs, inact} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	mustExecMeeting(t, db, `INSERT INTO work_tasks (
		task_id, work_type, title, work_date, due_date, status, assignee, org_id, complete_date, updated_at
	) VALUES
		('WT-d','admin','완료','`+date+`','`+date+`','complete','갑','O01','`+date+`','`+date+` 10:00:00'),
		('WT-p','admin','진행','`+date+`','`+date+`','in_progress','갑','O01',NULL,'`+date+` 10:00:00'),
		('WT-w','admin','미착수','`+date+`','`+date+`','waiting','갑','O01',NULL,'`+date+` 10:00:00'),
		('WT-nodue','admin','종료일없음','`+date+`',NULL,'waiting','갑','O01',NULL,'`+date+` 10:00:00'),
		('WT-b','admin','B완료','`+date+`','`+date+`','complete','을','O02','`+date+`','`+date+` 10:00:00'),
		('WT-help','admin','타조직지원자','`+date+`','`+date+`','waiting','을','O01',NULL,'`+date+` 10:00:00')`)
	lv := NewStaffLeaveRepo(db)
	if err := lv.Create(model.StaffLeave{UserID: a.UserID, UserName: a.FullName, Date: date, Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: c.UserID, Date: date, Kind: model.LeaveKindHalfAM}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: d.UserID, Date: date, Kind: model.LeaveKindHalfPM}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: e.UserID, Date: date, Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: f.UserID, Date: date, Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: g.UserID, Date: date, Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: b.UserID, Date: "2026-11-01", Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := lv.Create(model.StaffLeave{UserID: inact.UserID, Date: "2026-12-01", Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	repo := NewExecDashRepo(NewOrgRepo(db), NewWorkBoardRepo(db), users, lv)
	return repo, date
}

func Test95_90_ExecDashTodayCounts(t *testing.T) {
	repo, date := seedExecDash(t)
	orgs, tot, err := repo.Today(date, nil)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ExecOrgToday{}
	for _, o := range orgs {
		by[o.Org.OrgID] = o
		if o.Total != o.Done+o.InProgress+o.NotStarted+o.Delayed {
			t.Fatalf("%s 합 %+v", o.Org.OrgID, o)
		}
	}
	if _, ok := by["O09"]; ok {
		t.Fatal("인원 0 조직이 보인다")
	}
	a := by["O01"]
	// 오늘 예정 조건(due_date >= 오늘)에 안 걸리는 지난 종료일은 PlannedN 에 없다. 지연 칸은 그 집합 안에서만 센다.
	if a.Done != 1 || a.InProgress != 1 || a.NotStarted != 3 || a.Delayed != 0 {
		t.Fatalf("O01 %+v", a)
	}
	if a.Total != 5 {
		t.Fatalf("O01 total %d", a.Total)
	}
	if a.DonePct != 20 {
		t.Fatalf("완료율 %d", a.DonePct)
	}
	b := by["O02"]
	if b.Done != 1 || b.Total != 1 || b.InProgress != 0 {
		t.Fatalf("O02 %+v", b)
	}
	if tot.Total != a.Total+b.Total || tot.Done != a.Done+b.Done {
		t.Fatalf("전사 %+v", tot)
	}
	if len(a.OnLeave) != 5 || a.LeaveMore != 1 {
		t.Fatalf("휴가 표시 %d more=%d", len(a.OnLeave), a.LeaveMore)
	}
	joined := ""
	for _, lv := range a.OnLeave {
		joined += lv.Name + lv.Kind
	}
	if strings.Contains(joined, "을") {
		t.Fatal("O02 휴가가 O01 에 섞였다")
	}
	if b.LeaveMore != 0 || len(b.OnLeave) != 0 {
		t.Fatalf("O02 오늘 휴가 없음이어야 함 %+v", b.OnLeave)
	}
	onlyB, totB, err := repo.Today(date, []string{"O02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyB) != 1 || onlyB[0].Org.OrgID != "O02" || totB.Total != 1 {
		t.Fatalf("격리 %+v tot=%+v", onlyB, totB)
	}
	href := ExecMeetingHref("O01", date)
	if !strings.Contains(href, "view_org_id=O01") || !strings.Contains(href, "from=exec") || !strings.HasPrefix(href, "/meeting?") {
		t.Fatalf("href %s", href)
	}
	board := ExecBoardHref("O01", date)
	if !strings.HasPrefix(board, "/meeting/board?") || !strings.Contains(board, "src=exec") || !strings.Contains(board, "view_org_id=O01") {
		t.Fatalf("board href %s", board)
	}
	if strings.Contains(href, "/meeting?view_org_id=") && !strings.Contains(href, "date=") {
		t.Fatal("date 누락")
	}
}

func Test95_90_ExecDashQueryBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	raw, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	raw.Close()
	var n int64
	db, err := openQueryCountDB(path, &n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearQueryCount()
		db.Close()
	})
	users := NewUserRepo(db)
	for i, id := range []string{"O02", "O03", "O04"} {
		if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES(?,?,?,?,1,?)`,
			id, id, "팀"+id, "business", (i+2)*10); err != nil {
			t.Fatal(err)
		}
		u := &model.User{Username: "u" + id, PasswordHash: "x", FullName: "사원" + id, Role: model.RoleTech, IsActive: true, OrgID: id}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	u1 := &model.User{Username: "uO01", PasswordHash: "x", FullName: "사원O01", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(u1); err != nil {
		t.Fatal(err)
	}
	repo := NewExecDashRepo(NewOrgRepo(db), NewWorkBoardRepo(db), users, NewStaffLeaveRepo(db))
	clearQueryCount()
	n = 0
	qcountActive.Store(&n)
	if _, _, err := repo.Today("2026-11-04", nil); err != nil {
		t.Fatal(err)
	}
	first := n
	if first > 40 {
		t.Fatalf("쿼리 %d", first)
	}
	for _, id := range []string{"O05", "O06", "O07", "O08"} {
		if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES(?,?,?,?,1,50)`,
			id, id, "팀"+id, "business"); err != nil {
			t.Fatal(err)
		}
		u := &model.User{Username: "u" + id, PasswordHash: "x", FullName: "사원" + id, Role: model.RoleTech, IsActive: true, OrgID: id}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	clearQueryCount()
	n = 0
	qcountActive.Store(&n)
	if _, _, err := repo.Today("2026-11-04", nil); err != nil {
		t.Fatal(err)
	}
	if n > first+3 {
		t.Fatalf("조직이 늘어도 쿼리가 커진다 first=%d now=%d", first, n)
	}
}
