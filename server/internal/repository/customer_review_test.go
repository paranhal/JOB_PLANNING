package repository

import (
	"path/filepath"
	"testing"
)

// 확인 필요 표식: 자동 생성 고객 백필 → 목록 필터 → 표식 해제
func TestCustomerNeedsReviewFlag(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := NewCustomerRepo(db)

	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, notes)
		 VALUES (?,?,?,1,?)`,
		"C-IMP", "광천도서관", "광천도서관", "AS 완료내역 엑셀 적재 시 자동 생성"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		 VALUES (?,?,?,1)`, "C-OK", "정상기관", "정상기관"); err != nil {
		t.Fatal(err)
	}

	if err := repo.SetNeedsReview("C-IMP", true, ReviewReasonImportedCustomer); err != nil {
		t.Fatal(err)
	}

	if n := repo.CountNeedsReview(); n != 1 {
		t.Fatalf("확인 필요 건수 = %d, want 1", n)
	}

	items, total, err := repo.List("", "", "", "", "", 1, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("확인 필요 필터: total=%d len=%d", total, len(items))
	}
	if items[0].CustomerID != "C-IMP" || !items[0].NeedsReview || items[0].ReviewReason == "" {
		t.Fatalf("확인 필요 항목: %+v", items[0])
	}

	all, totalAll, err := repo.List("", "", "", "", "", 1, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if totalAll != 2 || len(all) != 2 {
		t.Fatalf("전체 목록: total=%d len=%d", totalAll, len(all))
	}

	cust, err := repo.GetByID("C-IMP")
	if err != nil || cust == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !cust.NeedsReview || cust.ReviewReason != ReviewReasonImportedCustomer {
		t.Fatalf("상세 표식: needs=%v reason=%q", cust.NeedsReview, cust.ReviewReason)
	}

	// 수작업 보정이 끝나면 표식과 사유가 함께 지워진다.
	if err := repo.SetNeedsReview("C-IMP", false, ""); err != nil {
		t.Fatal(err)
	}
	cust, _ = repo.GetByID("C-IMP")
	if cust.NeedsReview || cust.ReviewReason != "" {
		t.Fatalf("표식 해제 후: needs=%v reason=%q", cust.NeedsReview, cust.ReviewReason)
	}
	if n := repo.CountNeedsReview(); n != 0 {
		t.Fatalf("해제 후 건수 = %d, want 0", n)
	}
}

// 고객 정보를 수정해도 확인 필요 표식은 유지돼야 한다.
func TestCustomerUpdateKeepsReviewFlag(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "review_update.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := NewCustomerRepo(db)
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		 VALUES (?,?,?,1)`, "C-1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetNeedsReview("C-1", true, "확인 필요"); err != nil {
		t.Fatal(err)
	}

	cust, _ := repo.GetByID("C-1")
	cust.MainPhone = "041-000-0000"
	if err := repo.Update(cust); err != nil {
		t.Fatal(err)
	}

	after, _ := repo.GetByID("C-1")
	if !after.NeedsReview {
		t.Fatal("수정 후 확인 필요 표식이 사라졌다")
	}
}
