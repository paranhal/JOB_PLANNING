package service

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestNextWorkdayTable(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "wd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES ('O02','O02','타조직','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	u1 := &model.User{Username: "a84", PasswordHash: "password10xx", FullName: "갑", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u2 := &model.User{Username: "b84", PasswordHash: "x", FullName: "을", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u3 := &model.User{Username: "c84", PasswordHash: "x", FullName: "병", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	for _, u := range []*model.User{u1, u2, u3} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	g1, _ := users.GetByUsername("a84")
	g2, _ := users.GetByUsername("b84")
	g3, _ := users.GetByUsername("c84")
	leaves := repository.NewStaffLeaveRepo(db)
	hol := repository.NewHolidayRepo(db)
	if _, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-11-10', 2026, '임시휴일', 'company')`); err != nil {
		t.Fatal(err)
	}
	mustLeave := func(uid, date, kind string) {
		t.Helper()
		if err := leaves.Create(model.StaffLeave{UserID: uid, Date: date, Kind: kind}); err != nil {
			t.Fatal(err)
		}
	}
	mustLeave(g1.UserID, "2026-11-11", model.LeaveKindAnnual)
	mustLeave(g1.UserID, "2026-11-12", model.LeaveKindAnnual)
	mustLeave(g1.UserID, "2026-11-13", model.LeaveKindAnnual)
	mustLeave(g2.UserID, "2026-11-16", model.LeaveKindAnnual)
	mustLeave(g1.UserID, "2026-11-17", model.LeaveKindAnnual)
	mustLeave(g2.UserID, "2026-11-17", model.LeaveKindAnnual)
	mustLeave(g1.UserID, "2026-11-18", model.LeaveKindHalfAM)
	mustLeave(g3.UserID, "2026-11-19", model.LeaveKindAnnual)
	f := WorkdayFinder{Cal: NewCalendar(hol), Leaves: leaves, YearMissing: hol.YearMissing}
	fri := time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local)
	got, skipped := f.NextWorkday(fri, nil, model.OrgIDLibrary, false)
	if got != "2026-11-09" {
		t.Fatalf("금→월 %s skip=%v", got, skipped)
	}
	mon := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	got, skipped = f.NextWorkday(mon, nil, model.OrgIDLibrary, false)
	if got != "2026-11-11" {
		t.Fatalf("화 공휴일 건너뜀 %s %v", got, skipped)
	}
	got, _ = f.NextWorkday(time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local), []string{g1.FullName, g1.UserID}, model.OrgIDLibrary, false)
	if got != "2026-11-16" {
		t.Fatalf("연차 3일 %s", got)
	}
	got, _ = f.NextWorkday(time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local), []string{g1.FullName, g2.FullName}, model.OrgIDLibrary, false)
	if got != "2026-11-16" {
		t.Fatalf("1명만 연차 건너뛰면 안 됨 %s", got)
	}
	_, _, _, some := f.InspectWorkday("2026-11-16", []string{g1.FullName, g2.FullName}, model.OrgIDLibrary)
	if len(some) == 0 {
		t.Fatal("일부 연차 안내 없음")
	}
	got, _ = f.NextWorkday(time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), []string{g1.FullName, g2.FullName}, model.OrgIDLibrary, false)
	if got != "2026-11-18" {
		t.Fatalf("전원 연차 %s", got)
	}
	got, _ = f.NextWorkday(time.Date(2026, 11, 17, 0, 0, 0, 0, time.Local), []string{g1.FullName}, model.OrgIDLibrary, false)
	if got != "2026-11-18" {
		t.Fatalf("반차는 건너뛰지 않음 %s", got)
	}
	fMiss := WorkdayFinder{Cal: NewCalendar(hol), Leaves: leaves, YearMissing: func(int) bool { return true }}
	got, skipped = fMiss.NextWorkday(time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local), nil, model.OrgIDLibrary, false)
	if got != "2026-11-09" {
		t.Fatalf("데이터 누락 평일 %s", got)
	}
	if len(skipped) == 0 || skipped[len(skipped)-1].Reason != "휴일 정보를 확인하세요" && !containsSkip(skipped, "휴일 정보를 확인하세요") {
		t.Fatalf("누락 안내 %v", skipped)
	}
	got, _ = f.NextWorkday(time.Date(2026, 11, 18, 0, 0, 0, 0, time.Local), []string{g1.FullName}, model.OrgIDLibrary, false)
	if got != "2026-11-19" {
		t.Fatalf("타 조직 연차는 무시 %s", got)
	}
}

func TestPrevWorkdayTable(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "wd_prev.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hol := repository.NewHolidayRepo(db)
	if _, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-11-10', 2026, '임시휴일', 'company')`); err != nil {
		t.Fatal(err)
	}
	f := WorkdayFinder{Cal: NewCalendar(hol), YearMissing: hol.YearMissing}
	mon := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	got, _ := f.PrevWorkday(mon, nil, "", false)
	if got != "2026-11-06" {
		t.Fatalf("월→금 %s", got)
	}
	tue := time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local)
	got, _ = f.PrevWorkday(tue, nil, "", false)
	if got != "2026-11-09" {
		t.Fatalf("화→월 %s", got)
	}
	got, _ = f.PrevWorkday(time.Date(2026, 11, 11, 0, 0, 0, 0, time.Local), nil, "", false)
	if got != "2026-11-09" {
		t.Fatalf("공휴일 건너뜀 %s", got)
	}
}

func containsSkip(list []SkipInfo, reason string) bool {
	for _, s := range list {
		if s.Reason == reason {
			return true
		}
	}
	return false
}
