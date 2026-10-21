package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSchoolMasterImportCreateParentAndFold(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewCustomerRepo(db)
	raw, err := BuildSchoolMasterWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	fprev, err := repo.PreviewSchoolMaster(raw, model.OrgIDLibrary, false)
	if err == nil && fprev.NewN != 0 {
		t.Fatal("빈 양식은 행이 없어야 한다")
	}

	p := &model.Customer{OrgName: model.SchoolParentOfficialName, OfficialName: model.SchoolParentOfficialName, IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	b, err := BuildSchoolMasterWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	xlsx, err := BuildSchoolDataWorkbook([][]string{
		schoolHdr,
		{model.SchoolParentOfficialName, "계룡고등학교", "G001", "계룡시", "충남 계룡시", "042-000-0000", ""},
		{"", "한솔유치원", "", "세종", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := repo.PreviewSchoolMaster(xlsx, model.OrgIDLibrary, false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN != 0 || prev.NewN != 2 {
		t.Fatalf("preview new=%d err=%d %+v", prev.NewN, prev.ErrN, prev.Rows)
	}
	if prev.Rows[1].ParentName != model.SchoolParentOfficialName {
		t.Fatalf("빈 상위=%s", prev.Rows[1].ParentName)
	}
	app, err := repo.ApplySchoolMaster("U1", model.OrgIDLibrary, xlsx, false)
	if err != nil {
		t.Fatal(err)
	}
	if app.NewN != 2 {
		t.Fatalf("apply %d", app.NewN)
	}
	items, total, err := repo.List(model.OrgIDLibrary, "", "", "", "", "asc", 1, 50, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatal("접힌 목록이 비었다")
	}
	foundChild := false
	for _, it := range items {
		if it.OrgName == "계룡고등학교" {
			foundChild = true
		}
		if it.CustomerID == p.CustomerID && it.ChildCount < 2 {
			t.Fatalf("하위 N=%d", it.ChildCount)
		}
	}
	if foundChild {
		t.Fatal("기본 목록에 하위 학교가 펼쳐졌다")
	}
	open, n, err := repo.List(model.OrgIDLibrary, "", p.CustomerID, "", "", "asc", 1, 50, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("펼침 n=%d %+v", n, open)
	}
}
