package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

// 정기점검 방문의 완료 표시 — 같은 날 점검 대상이 다르면 별도 방문으로 들어가야 하고,
// 지난 일정 일괄 완료가 기준일까지만 적용돼야 한다.
func TestMaintenanceVisitCompletion(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}

	repo := NewMaintenanceRepo(db)
	plan, err := repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}

	// 같은 날·같은 기관이라도 점검 대상이 다르면 둘 다 들어간다.
	for _, product := range []string{"KLAS", "앤로보틱스"} {
		if err := repo.InsertVisitFull(model.MaintenanceVisit{
			PlanID: plan.PlanID, VisitDate: "2026-08-03", CustomerID: "cust_a",
			ProductType: product, Assignee: "최혜영", Completed: true,
		}); err != nil {
			t.Fatalf("%s 방문 등록: %v", product, err)
		}
	}
	// 같은 대상을 또 넣으면 막혀야 한다.
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: "2026-08-03", CustomerID: "cust_a", ProductType: "KLAS",
	}); err == nil {
		t.Fatal("같은 날·같은 점검 대상 중복이 허용됐다")
	}

	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: "2026-08-04", CustomerID: "cust_a",
		ProductType: "KLAS", Assignee: "양기헌",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: "2026-08-20", CustomerID: "cust_a",
		ProductType: "KLAS", Assignee: "양기헌",
	}); err != nil {
		t.Fatal(err)
	}

	visits, err := repo.ListVisits(plan.PlanID)
	if err != nil || len(visits) != 4 {
		t.Fatalf("ListVisits: len=%d err=%v", len(visits), err)
	}
	for _, v := range visits {
		if v.VisitDate == "2026-08-03" {
			if !v.Completed {
				t.Fatalf("%s %s: 완료로 들어가야 한다", v.VisitDate, v.ProductType)
			}
			if v.CompletedDate != "2026-08-03" {
				t.Fatalf("완료일이 방문일과 달라졌다: %q", v.CompletedDate)
			}
			if v.Assignee != "최혜영" {
				t.Fatalf("담당자: %q", v.Assignee)
			}
		}
	}

	// 8월 4일까지 일괄 완료 — 8월 20일 건은 그대로 예정.
	n, err := repo.CompleteVisitsUntil(plan.PlanID, "2026-08-04", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("일괄 완료 건수: %d (이미 완료된 건은 세면 안 된다)", n)
	}

	visits, _ = repo.ListVisits(plan.PlanID)
	done := 0
	for _, v := range visits {
		if v.Completed {
			done++
		}
		if v.VisitDate == "2026-08-20" && v.Completed {
			t.Fatal("기준일 이후 방문까지 완료 처리됐다")
		}
	}
	if done != 3 {
		t.Fatalf("완료 건수: %d", done)
	}

	// 완료 취소는 실제 방문일도 지운다.
	var target model.MaintenanceVisit
	for _, v := range visits {
		if v.VisitDate == "2026-08-04" {
			target = v
		}
	}
	if err := repo.SetVisitCompleted(target.VisitID, false, ""); err != nil {
		t.Fatal(err)
	}
	visits, _ = repo.ListVisits(plan.PlanID)
	for _, v := range visits {
		if v.VisitID == target.VisitID {
			if v.Completed || v.CompletedDate != "" {
				t.Fatalf("완료 취소 후: completed=%v date=%q", v.Completed, v.CompletedDate)
			}
		}
	}

	// 실제 방문일이 예정일과 다른 경우도 저장돼야 한다.
	if err := repo.SetVisitCompleted(target.VisitID, true, "2026-08-06"); err != nil {
		t.Fatal(err)
	}
	visits, _ = repo.ListVisits(plan.PlanID)
	for _, v := range visits {
		if v.VisitID == target.VisitID && v.CompletedDate != "2026-08-06" {
			t.Fatalf("실제 방문일: %q", v.CompletedDate)
		}
	}
}
