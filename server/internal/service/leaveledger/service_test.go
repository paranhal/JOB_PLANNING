package leaveledger

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/leavecalc"
)

func testLedger(t *testing.T) (*Service, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "led.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	u := &model.User{Username: "led1", PasswordHash: "x", FullName: "원장", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	s.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC) }
	return s, u.UserID
}

func Test95_161_UnregisteredNoGrants(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.EnsureFiscalGrants(s.DB, uid, "2026-06-01"); err != nil {
		t.Fatal(err)
	}
	if s.GrantCount(uid) != 0 {
		t.Fatalf("grants %d", s.GrantCount(uid))
	}
	bal, err := s.Balance(uid, "2026-06-01")
	if err != nil || bal.Measurable {
		t.Fatalf("bal %+v err=%v", bal, err)
	}
}

func Test95_161_NoHireNoAutoGrant(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureFiscalGrants(s.DB, uid, "2026-06-01"); err != nil {
		t.Fatal(err)
	}
	if s.GrantCount(uid) != 0 {
		t.Fatal("auto without hire")
	}
	if err := s.AddOpening(uid, "2026-01-01", "2026-12-31", 5*480, "1"); err != nil {
		t.Fatal(err)
	}
	bal, err := s.Balance(uid, "2026-06-01")
	if err != nil || !bal.Measurable || bal.AvailableMinutes != 5*480 {
		t.Fatalf("opening %+v err=%v", bal, err)
	}
}

func Test95_161_EnsureFiscalIdempotent(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", "2023-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureFiscalGrants(s.DB, uid, "2025-01-01"); err != nil {
		t.Fatal(err)
	}
	n := s.GrantCount(uid)
	if err := s.EnsureFiscalGrants(s.DB, uid, "2025-01-01"); err != nil {
		t.Fatal(err)
	}
	if s.GrantCount(uid) != n || n < 3 {
		t.Fatalf("n=%d again=%d", n, s.GrantCount(uid))
	}
	bal, err := s.Balance(uid, "2025-01-01")
	if err != nil || !bal.Measurable {
		t.Fatal(err)
	}
	var found bool
	for _, g := range bal.Grants {
		if g.GrantKey == "F:2025" && g.Minutes == 16*480 {
			found = true
		}
	}
	if !found {
		t.Fatalf("3년차 16일 %+v", bal.Grants)
	}
}

func Test95_161_ExpireDec31(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureFiscalGrants(s.DB, uid, "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.ExpireAsOf("2026-12-31"); err != nil {
		t.Fatal(err)
	}
	bal, err := s.Balance(uid, "2026-12-31")
	if err != nil || bal.AvailableMinutes != 0 {
		t.Fatalf("expired %+v", bal)
	}
	if err := s.ExpireAsOf("2026-12-31"); err != nil {
		t.Fatal(err)
	}
}

func Test95_161_PlanConsentFIFO(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	_, abs, err := s.PlanDeduction(s.DB, uid, leavecalc.TypeAnnual, 20*480, "2026-06-01", false)
	if err != leavecalc.ErrAbsenceConsent || abs != 0 {
		t.Fatalf("consent err=%v abs=%d", err, abs)
	}
	deduct, abs, err := s.PlanDeduction(s.DB, uid, leavecalc.TypeAnnual, 16*480, "2026-06-01", true)
	if err != nil || deduct != 15*480 || abs != 480 {
		t.Fatalf("split d=%d a=%d err=%v", deduct, abs, err)
	}
	if _, _, err := s.PlanDeduction(s.DB, uid, leavecalc.TypeMilitaryTraining, 480, "2026-06-01", false); err != nil {
		t.Fatal(err)
	}
	d0, a0, err := s.PlanDeduction(s.DB, uid, leavecalc.TypeMilitaryTraining, 480, "2026-06-01", false)
	if err != nil || d0 != 0 || a0 != 0 {
		t.Fatalf("military %d %d %v", d0, a0, err)
	}
}

func Test95_161_ConsumeRestoreAbsence(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeApproved(s.DB, uid, "LR1", "2026-06-01", 480); err != nil {
		t.Fatal(err)
	}
	if err := s.ConvertAbsence(s.DB, uid, "LR1", 480, "부족"); err != nil {
		t.Fatal(err)
	}
	if s.UseNet("LR1") != 480 || s.AbsenceNet("LR1") != 480 {
		t.Fatalf("use=%d abs=%d", s.UseNet("LR1"), s.AbsenceNet("LR1"))
	}
	if err := s.RestoreApproved(s.DB, uid, "LR1"); err != nil {
		t.Fatal(err)
	}
	if s.UseNet("LR1") != 0 || s.AbsenceNet("LR1") != 0 {
		t.Fatalf("after restore use=%d abs=%d", s.UseNet("LR1"), s.AbsenceNet("LR1"))
	}
	bal, _ := s.Balance(uid, "2026-06-01")
	if bal.AvailableMinutes != 15*480 {
		t.Fatalf("restored avail %d", bal.AvailableMinutes)
	}
}

func Test95_161_ConcurrentPlan(t *testing.T) {
	s, uid := testLedger(t)
	if err := s.Enable(uid, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureFiscalGrants(s.DB, uid, "2026-06-01"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var deducts []int
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := s.DB.Begin()
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.Exec(`UPDATE employee_profiles SET phone=COALESCE(phone,'') WHERE user_id=?`, uid); err != nil {
				t.Error(err)
				return
			}
			d, _, err := s.PlanDeduction(tx, uid, leavecalc.TypeAnnual, 15*480, "2026-06-01", true)
			if err != nil {
				t.Error(err)
				return
			}
			doc := "VI-근태-" + uuidish()
			if _, err := tx.Exec(`INSERT INTO leave_requests(leave_req_id,org_id,user_id,doc_no,type_key,period_mode,start_date,end_date,work_minutes,reason,status,deduct_minutes,absence_minutes,absence_consent)
				VALUES (?,?,?,?,'annual','days','2026-08-10','2026-08-10',?,?, 'pending',?,?,1)`,
				"LR-"+uuidish(), "O01", uid, doc, 15*480, "동시", d, 15*480-d); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			deducts = append(deducts, d)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(deducts) != 2 {
		t.Fatalf("got %v", deducts)
	}
	sum := deducts[0] + deducts[1]
	if sum != 15*480 {
		t.Fatalf("concurrent deduct sum %d %v", sum, deducts)
	}
}

var uniqSeq int64

func uuidish() string {
	return fmt.Sprintf("%d%d", time.Now().UnixNano(), atomic.AddInt64(&uniqSeq, 1))
}
