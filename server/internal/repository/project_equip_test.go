package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func testEquipRepo(t *testing.T) (*ProjectEquipRepo, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "pe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,org_id,contract_amount) VALUES('WP95','다년',?,1000)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	return NewProjectEquipRepo(db), "WP95"
}

func TestProjectEquipYearOverlapAndSumWarn(t *testing.T) {
	r, pid := testEquipRepo(t)
	if err := r.SaveYear(&model.ContractYear{ProjectID: pid, YearNo: 1, Label: "1차", StartDate: "2026-01-01", EndDate: "2026-12-31", Amount: 400, Status: model.YearPlanned, OrgID: model.OrgIDLibrary}, "u"); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveYear(&model.ContractYear{ProjectID: pid, YearNo: 1, Label: "겹순번", StartDate: "2027-01-01", EndDate: "2027-12-31", Amount: 1, Status: model.YearPlanned, OrgID: model.OrgIDLibrary}, "u"); err == nil {
		t.Fatal("순번 중복")
	}
	if err := r.SaveYear(&model.ContractYear{ProjectID: pid, YearNo: 2, Label: "겹침", StartDate: "2026-06-01", EndDate: "2027-05-31", Amount: 1, Status: model.YearPlanned, OrgID: model.OrgIDLibrary}, "u"); err == nil || !strings.Contains(err.Error(), "겹칩") {
		t.Fatalf("기간 겹침 err=%v", err)
	}
	if err := r.SaveYear(&model.ContractYear{ProjectID: pid, YearNo: 2, Label: "2차", StartDate: "2027-01-01", EndDate: "2027-12-31", Amount: 400, Status: model.YearActive, OrgID: model.OrgIDLibrary}, "u"); err != nil {
		t.Fatal(err)
	}
	sum, warn := r.YearSumWarn(pid, 1000)
	if sum != 800 || !warn {
		t.Fatalf("sum=%d warn=%v", sum, warn)
	}
}

func TestProjectEquipCategoryOrgAndInUse(t *testing.T) {
	r, pid := testEquipRepo(t)
	c := &model.EquipmentCategory{OrgID: model.OrgIDLibrary, Code: "DSP", Name: "디스플레이", IsActive: true}
	if err := r.SaveCategory(c); err != nil {
		t.Fatal(err)
	}
	o2 := &model.EquipmentCategory{OrgID: "O02", Code: "DSP", Name: "다른조직", IsActive: true}
	if err := r.SaveCategory(o2); err != nil {
		t.Fatal(err)
	}
	got, _ := r.CategoryByCode("O02", "DSP")
	if got == nil || got.ID == c.ID {
		t.Fatal("조직 격리")
	}
	f := &model.EquipmentField{CategoryID: c.ID, FieldKey: "inch", Label: "인치", FieldType: model.EquipFieldNumber, IsActive: true}
	if err := r.SaveField(f); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveField(&model.EquipmentField{CategoryID: c.ID, FieldKey: "bad", Label: "x", FieldType: "xml"}); err == nil {
		t.Fatal("형식")
	}
	s := &model.ProjectSite{ProjectID: pid, SiteName: "A교", Status: model.SiteActive, OrgID: model.OrgIDLibrary}
	if err := r.SaveSite(s); err != nil {
		t.Fatal(err)
	}
	e := &model.ProjectEquipment{ProjectID: pid, SiteID: s.ID, CategoryID: c.ID, ProductName: "패널", Qty: 1, Status: model.EquipPlanned, OrgID: model.OrgIDLibrary, Values: map[string]string{"inch": "x"}}
	if err := r.SaveEquipment(e); err == nil {
		t.Fatal("숫자 검증")
	}
	e.Values["inch"] = "65"
	if err := r.SaveEquipment(e); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteCategory(c.ID, model.OrgIDLibrary); err == nil {
		t.Fatal("사용 중 삭제")
	}
	if err := r.DeleteField(f.ID); err == nil {
		t.Fatal("사용 중 항목")
	}
}

func TestProjectEquipImportPreviewApplySerial(t *testing.T) {
	r, pid := testEquipRepo(t)
	raw, err := handlerEquipBook(
		[][]string{{"사이트명", "지역", "주소", "상태", "비고"}, {"서울교", "서울", "", "운영", ""}, {"", "", "", "", ""}},
		[][]string{{"사이트명", "분류코드", "제품명", "모델", "제조사", "수량", "시리얼", "설치위치", "설치일", "상태", "연차번호", "비고", "inch"},
			{"서울교", "NOPE", "TV", "", "", "1", "SN1", "", "", "예정", "", ""},
			{"서울교", "", "TV", "", "", "1", "SN1", "", "", "예정", "", ""},
			{"서울교", "", "=CMD", "", "", "1", "SN2", "", "", "예정", "", ""}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prev, err := r.PreviewEquipImport(pid, raw, "a.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_sites`).Scan(&n)
	if n != 0 {
		t.Fatal("미리보기가 DB를 바꿈")
	}
	if prev.ErrN == 0 {
		t.Fatal("오류 행 필요")
	}
	applied, err := r.ApplyEquipImport(pid, "u", "a.xlsx", raw)
	if err != nil {
		t.Fatal(err)
	}
	if applied.SitesNew+applied.EquipNew == 0 {
		t.Fatal("적용 건수")
	}
	sites, _, _ := r.ListSites(pid, "", "", "", 1, 50)
	if len(sites) != 1 {
		t.Fatalf("sites=%d", len(sites))
	}
	eq, _, _ := r.ListEquipment(pid, "", "", "", 1, 50)
	if len(eq) < 1 {
		t.Fatal("장비")
	}
	again, err := r.ApplyEquipImport(pid, "u", "a.xlsx", raw)
	if err != nil {
		t.Fatal(err)
	}
	sites2, tot, _ := r.ListSites(pid, "", "", "", 1, 50)
	if tot != 1 || len(sites2) != 1 {
		t.Fatalf("재업로드 사이트 tot=%d", tot)
	}
	_ = again
	eq2, et, _ := r.ListEquipment(pid, "", "", "", 1, 50)
	if et != len(eq) {
		t.Fatalf("재업로드 장비 %d → %d", len(eq), et)
	}
	_ = eq2
	if !strings.HasPrefix(ExcelSafeText("=1+1"), "'") {
		t.Fatal("수식")
	}
}

func TestProjectEquipImportLimitsAndSafe(t *testing.T) {
	if ExcelSafeText("+cmd") != "'+cmd" || ExcelSafeText("ok") != "ok" {
		t.Fatal("ExcelSafeText")
	}
	big := make([][]string, 3002)
	big[0] = []string{"사이트명"}
	for i := 1; i < 3002; i++ {
		big[i] = []string{fmt.Sprintf("S%d", i)}
	}
	raw, err := handlerEquipBook(big, [][]string{{"사이트명"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseEquipWorkbook(raw); err == nil {
		t.Fatal("사이트 한도")
	}
}

func TestProjectEquipPagingPerf(t *testing.T) {
	r, pid := testEquipRepo(t)
	tx, err := r.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id) VALUES('PS-0',?,'기준','in_progress',?)`, pid, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 1000; i++ {
		if _, err := tx.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id,sort_order) VALUES(?,?,?,'in_progress',?,?)`,
			fmt.Sprintf("PS-%d", i), pid, fmt.Sprintf("사이트%04d", i), model.OrgIDLibrary, i); err != nil {
			t.Fatal(err)
		}
	}
	sid := "PS-1"
	for i := 1; i <= 20000; i++ {
		if _, err := tx.Exec(`INSERT INTO project_equipment(id,project_id,site_id,product_name,qty,status,org_id,is_active) VALUES(?,?,?,'장비',1,'planned',?,1)`,
			fmt.Sprintf("EQ-%d", i), pid, sid, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	st := time.Now()
	items, tot, err := r.ListSites(pid, "사이트0001", "", "", 1, 50)
	if err != nil || tot < 1 || len(items) != 1 {
		t.Fatalf("search tot=%d n=%d err=%v", tot, len(items), err)
	}
	_, tot2, err := r.ListEquipment(pid, sid, "", "", 2, 50)
	if err != nil || tot2 != 20000 {
		t.Fatalf("equip tot=%d err=%v", tot2, err)
	}
	if d := time.Since(st); d > 3*time.Second {
		t.Fatalf("느림 %s", d)
	}
	t.Logf("paging %s", time.Since(st))
}

func handlerEquipBook(sites, equips [][]string) ([]byte, error) {
	return buildTestEquipXLSX(sites, equips)
}
