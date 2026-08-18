package service

import (
	"fmt"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type stubHolidayAPI struct {
	months map[int][]model.Holiday
	err    error
	calls  int
}

func (s *stubHolidayAPI) FetchRestDe(year, month int) ([]model.Holiday, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return append([]model.Holiday(nil), s.months[month]...), nil
}

func TestSyncYearPreservesManualAndSkipsOnFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewHolidayRepo(db)
	if err := repo.Create(model.Holiday{Date: "2026-07-15", Name: "창립기념일", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	before, err := repo.CountYear(2026)
	if err != nil || before == 0 {
		t.Fatalf("시드 n=%d err=%v", before, err)
	}

	failAPI := &stubHolidayAPI{err: fmt.Errorf("portal down")}
	if _, err := SyncYear(repo, failAPI, 2026); err == nil {
		t.Fatal("실패해야 함")
	}
	afterFail, _ := repo.CountYear(2026)
	if afterFail != before {
		t.Fatalf("실패 시 테이블을 바꾸면 안 됨 %d→%d", before, afterFail)
	}
	h, _ := repo.Get("2026-07-15")
	if h == nil || h.Name != "창립기념일" {
		t.Fatal("실패 시 manual 이 사라짐")
	}

	okAPI := &stubHolidayAPI{months: map[int][]model.Holiday{
		9: {
			{Date: "2026-09-24", Name: "추석 연휴", Kind: model.HolidayKindPublic},
			{Date: "2026-09-25", Name: "추석", Kind: model.HolidayKindPublic},
			{Date: "2026-09-26", Name: "추석 연휴", Kind: model.HolidayKindPublic},
		},
	}}
	n, err := SyncYear(repo, okAPI, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Fatalf("upsert=%d", n)
	}
	h, _ = repo.Get("2026-07-15")
	if h == nil || h.Source != model.HolidaySourceManual || h.Name != "창립기념일" {
		t.Fatalf("manual 을 건드리면 안 됨 %+v", h)
	}
	ok, _ := repo.HasDate("2026-01-01")
	if ok {
		t.Fatal("API에 없는 api 행은 빠져야 함")
	}
	ok, _ = repo.HasDate("2026-09-25")
	if !ok {
		t.Fatal("추석이 없다")
	}
}

func TestSyncYearDoesNotOverwriteManualSameDate(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "ov.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewHolidayRepo(db)
	if err := repo.Update("2026-01-01", model.Holiday{Date: "2026-01-01", Name: "회사 신정", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	api := &stubHolidayAPI{months: map[int][]model.Holiday{
		1: {{Date: "2026-01-01", Name: "신정", Kind: model.HolidayKindPublic}},
	}}
	if _, err := SyncYear(repo, api, 2026); err != nil {
		t.Fatal(err)
	}
	h, err := repo.Get("2026-01-01")
	if err != nil || h == nil || h.Name != "회사 신정" || h.Source != model.HolidaySourceManual {
		t.Fatalf("같은 날짜 manual 을 덮으면 안 됨 %+v err=%v", h, err)
	}
}
