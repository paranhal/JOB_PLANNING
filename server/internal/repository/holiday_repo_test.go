package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestHolidayCRUDAndCountYear(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "hol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewHolidayRepo(db)

	n, err := repo.CountAll()
	if err != nil || n == 0 {
		t.Fatalf("InitDB 후 시드가 있어야 함 n=%d err=%v", n, err)
	}
	if repo.YearMissing(2026) {
		t.Fatal("2026은 시드되어 있어야 함")
	}
	if !repo.YearMissing(2028) {
		t.Fatal("2028은 미등록이어야 함")
	}

	if err := repo.Create(model.Holiday{Date: "2026-07-15", Name: "임시휴일", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	h, err := repo.Get("2026-07-15")
	if err != nil || h == nil || h.Name != "임시휴일" || h.Source != model.HolidaySourceManual {
		t.Fatalf("%+v err=%v", h, err)
	}

	if err := repo.Update("2026-07-15", model.Holiday{Date: "2026-07-15", Name: "창립기념일", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	h, _ = repo.Get("2026-07-15")
	if h.Name != "창립기념일" || h.Source != model.HolidaySourceManual {
		t.Fatalf("수정 실패 %+v", h)
	}

	inserted, err := repo.SeedBuiltinIfEmpty([]model.Holiday{
		{Date: "2025-01-01", Year: 2025, Name: "신정", Kind: model.HolidayKindPublic},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 0 {
		t.Fatalf("이미 행이 있으면 시드하지 않음 got %d", inserted)
	}

	if err := repo.Delete("2026-07-15"); err != nil {
		t.Fatal(err)
	}
	if repo.YearMissing(2026) {
		t.Fatal("시드가 있으면 2026은 남아 있어야 함")
	}
}

func TestHolidayImportYearSkipExisting(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewHolidayRepo(db)
	if err := repo.Create(model.Holiday{Date: "2028-01-01", Name: "회사 지정", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	items := []model.Holiday{
		{Date: "2028-01-01", Year: 2028, Name: "신정", Kind: model.HolidayKindPublic},
		{Date: "2028-05-05", Year: 2028, Name: "어린이날", Kind: model.HolidayKindPublic},
	}
	n, err := repo.ImportYear(2028, items)
	if err != nil || n != 1 {
		t.Fatalf("추가 %d err=%v want 1", n, err)
	}
	h, err := repo.Get("2028-01-01")
	if err != nil || h == nil || h.Kind != model.HolidayKindCompany || h.Name != "회사 지정" {
		t.Fatalf("기존 행을 덮으면 안 됨 %+v err=%v", h, err)
	}
}

func TestHolidayImportYearEmpty(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "nob.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = NewHolidayRepo(db).ImportYear(2028, nil)
	if err == nil {
		t.Fatal("기본 데이터가 없으면 오류여야 함")
	}
}

func TestHolidaySeedHas2026ChuseokNotCopied(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewHolidayRepo(db)
	n, err := repo.CountYear(2026)
	if err != nil || n != 20 {
		t.Fatalf("2026 시드=%d err=%v want 20", n, err)
	}
	for _, d := range []string{"2026-09-24", "2026-09-25", "2026-09-26"} {
		ok, _ := repo.HasDate(d)
		if !ok {
			t.Fatalf("없음 %s", d)
		}
	}
	for _, d := range []string{"2026-10-06", "2026-10-07", "2026-10-08"} {
		ok, _ := repo.HasDate(d)
		if ok {
			t.Fatalf("오기재 %s", d)
		}
	}
}
