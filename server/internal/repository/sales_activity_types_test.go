package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSalesActivityTypesV227Labels(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	codes, err := NewCodeRepo(db).ActiveByGroup(model.SalesCodeGroupActivityType)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range codes {
		got[c.CodeValue] = c.CodeName
	}
	if got[model.SalesActTypeVisit] != "방문" || got[model.SalesActTypeMail] != "이메일" {
		t.Fatalf("라벨 유지 실패: visit=%q mail=%q", got["visit"], got["mail"])
	}
	if got[model.SalesActTypeInternal] != "내부회의" {
		t.Fatalf("내부회의 없음: %+v", got)
	}
	if got[model.SalesActTypeQuote] == "" || got[model.SalesActTypeProposal] == "" || got[model.SalesActTypeBid] == "" || got[model.SalesActTypeRFP] == "" {
		t.Fatalf("진척 열쇠 유형 누락: %+v", got)
	}
}
