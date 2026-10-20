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
	if got[model.SalesActTypeResearch] != "정보 수집" || got[model.SalesActTypeProposal] != "제안서 제출" {
		t.Fatalf("유형 라벨 %+v", got)
	}
	if _, ok := got[model.SalesActTypeVisit]; ok {
		t.Fatal("방법이 유형 활성에 남았다")
	}
	methods, err := NewCodeRepo(db).ActiveByGroup(model.SalesCodeGroupActivityMethod)
	if err != nil {
		t.Fatal(err)
	}
	mm := map[string]string{}
	for _, c := range methods {
		mm[c.CodeValue] = c.CodeName
	}
	if mm[model.SalesActMethodVisit] != "방문" || mm[model.SalesActMethodMail] != "이메일" || mm[model.SalesActMethodInternal] != "내부회의" {
		t.Fatalf("방법 라벨 %+v", mm)
	}
}
