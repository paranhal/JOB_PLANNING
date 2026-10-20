package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func testCatRepo(t *testing.T) *ProjectEquipRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='equipment_category_import_batches'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("071 table n=%d err=%v", n, err)
	}
	return NewProjectEquipRepo(db)
}

func catBook(t *testing.T, cats, fields [][]string) []byte {
	t.Helper()
	raw, err := BuildEquipCatWorkbook(EquipCatGuideLines(), cats, fields, EquipCatSampleRows())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func allOrgs(r *ProjectEquipRepo) CatImportScope {
	ids, _ := r.ListActiveOrgIDs()
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return CatImportScope{Allowed: m}
}

func TestEquipCatImportPreviewApplyRoundtrip(t *testing.T) {
	r := testCatRepo(t)
	raw := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "디스플레이", "", "1", "Y"},
			{"O02", "DSP_LED", "LED", "DSP", "2", "Y"},
			{"O03", "NET", "네트워크", "", "1", ""},
		},
		[][]string{
			{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"},
			{"O02", "DSP_LED", "resolution", "해상도", "select", " 4K | FHD | FHD |HD ", "Y", "1"},
			{"O02", "DSP_LED", "inch", "인치", "number", "무시됨", "N", "2"},
		},
	)
	prev, err := r.PreviewCategoryImport(raw, "a.xlsx", allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM equipment_categories`).Scan(&n)
	if n != 0 {
		t.Fatal("미리보기가 DB를 바꿈")
	}
	if prev.ErrN != 0 || prev.NewN != 5 {
		t.Fatalf("preview new=%d err=%d", prev.NewN, prev.ErrN)
	}
	applied, err := r.ApplyCategoryImport("u", "a.xlsx", raw, allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	if applied.NewN != prev.NewN {
		t.Fatalf("적용 건수 preview=%d apply=%d", prev.NewN, applied.NewN)
	}
	again, err := r.ApplyCategoryImport("u", "a.xlsx", raw, allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	if again.NewN != 0 || again.SameN != 5 {
		t.Fatalf("재업로드 new=%d same=%d", again.NewN, again.SameN)
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM equipment_categories`).Scan(&n)
	if n != 3 {
		t.Fatalf("cats=%d", n)
	}
	c, _ := r.CategoryByCode("O02", "DSP_LED")
	if c == nil || c.ParentID == "" {
		t.Fatal("소분류 상위")
	}
	fs, _ := r.ListFields(c.ID, true)
	if len(fs) != 2 || fs[0].Options != "4K|FHD|HD" || fs[1].Options != "" {
		t.Fatalf("fields %+v", fs)
	}
	exportCats, exportFields, err := r.CurrentCategoryExport([]string{"O02", "O03"})
	if err != nil {
		t.Fatal(err)
	}
	round := catBook(t, exportCats, exportFields)
	rt, err := r.PreviewCategoryImport(round, "b.xlsx", allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	if rt.NewN != 0 || rt.ErrN != 0 {
		t.Fatalf("왕복 new=%d err=%d", rt.NewN, rt.ErrN)
	}
}

func TestEquipCatImportValidation(t *testing.T) {
	r := testCatRepo(t)
	if _, _, err := ParseEquipCatWorkbook(nil); err == nil {
		t.Fatal("빈 파일")
	}
	badHdr := catBook(t,
		[][]string{{"조직", "코드", "이름", "상위", "정렬", "사용"}},
		[][]string{append([]string{}, catHdrField...)},
	)
	if _, _, err := ParseEquipCatWorkbook(badHdr); err == nil || !strings.Contains(err.Error(), "헤더") {
		t.Fatalf("헤더 err=%v", err)
	}
	inj := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "=CMD", "", "1", "Y"},
		},
		[][]string{append([]string{}, catHdrField...)},
	)
	prev, err := r.PreviewCategoryImport(inj, "x.xlsx", allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN == 0 {
		t.Fatal("수식 주입")
	}
	n := 0
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM equipment_categories`).Scan(&n)
	if n != 0 {
		t.Fatal("미리보기 쓰기")
	}
	three := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "A", "A", "", "1", "Y"},
			{"O02", "B", "B", "A", "2", "Y"},
			{"O02", "C", "C", "B", "3", "Y"},
		},
		[][]string{append([]string{}, catHdrField...)},
	)
	p3, err := r.PreviewCategoryImport(three, "t.xlsx", allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range p3.Rows {
		if row.Code == "C" && row.Action == "err" && strings.Contains(row.Msg, "2단계") {
			found = true
		}
	}
	if !found {
		t.Fatal("3단계")
	}
	cycle := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "A", "A", "A", "1", "Y"},
		},
		[][]string{append([]string{}, catHdrField...)},
	)
	pc, _ := r.PreviewCategoryImport(cycle, "c.xlsx", allOrgs(r))
	if pc.ErrN == 0 {
		t.Fatal("순환")
	}
	cross := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "A", "A", "", "1", "Y"},
			{"O03", "B", "B", "A", "1", "Y"},
		},
		[][]string{append([]string{}, catHdrField...)},
	)
	px, _ := r.PreviewCategoryImport(cross, "x.xlsx", allOrgs(r))
	ok := false
	for _, row := range px.Rows {
		if row.Code == "B" && row.Action == "err" {
			ok = true
		}
	}
	if !ok {
		t.Fatal("타 조직 상위")
	}
}

func TestEquipCatImportPermAndInUse(t *testing.T) {
	r := testCatRepo(t)
	raw := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "디스플레이", "", "1", "Y"},
			{"O03", "NET", "네트워크", "", "1", "Y"},
		},
		[][]string{
			{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"},
			{"O02", "DSP", "kind", "종류", "select", "A|B", "N", "1"},
		},
	)
	oa := CatImportScope{Allowed: map[string]bool{model.OrgIDLibrary: true}}
	prev, err := r.PreviewCategoryImport(raw, "a.xlsx", oa)
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN < 2 {
		t.Fatalf("org_admin 타 조직 err=%d", prev.ErrN)
	}
	if _, err := r.ApplyCategoryImport("u", "a.xlsx", raw, allOrgs(r)); err != nil {
		t.Fatal(err)
	}
	keep := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "디스플레이", "", "1", "Y"},
		},
		[][]string{append([]string{}, catHdrField...)},
	)
	if _, err := r.ApplyCategoryImport("u", "b.xlsx", keep, allOrgs(r)); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM equipment_categories WHERE org_id='O03'`).Scan(&n)
	if n != 1 {
		t.Fatal("파일에 없는 분류 유지")
	}
	cat, _ := r.CategoryByCode("O02", "DSP")
	if _, err := r.db.Exec(`INSERT INTO work_projects(project_id,name,org_id) VALUES('WP95C','x',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id) VALUES('S1','WP95C','s','active',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO project_equipment(id,project_id,site_id,category_id,product_name,qty,status,org_id) VALUES('E1','WP95C','S1',?,'p',1,'planned',?)`, cat.ID, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	fs, _ := r.ListFields(cat.ID, true)
	if len(fs) == 0 {
		t.Fatal("field")
	}
	_, _ = r.db.Exec(`INSERT INTO project_equipment_values(equipment_id,field_id,value) VALUES('E1',?,'A')`, fs[0].ID)
	chg := catBook(t,
		[][]string{
			{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"},
			{"O02", "DSP", "디스플레이", "", "1", "Y"},
		},
		[][]string{
			{"조직코드", "분류코드", "필드키", "항목명", "형식", "선택값", "필수", "정렬"},
			{"O02", "DSP", "kind", "종류", "text", "", "N", "1"},
		},
	)
	bad, err := r.PreviewCategoryImport(chg, "c.xlsx", allOrgs(r))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range bad.Rows {
		if row.Kind == "field" && row.Action == "err" && strings.Contains(row.Msg, "형식") {
			found = true
		}
	}
	if !found {
		t.Fatal("사용 중 형식 변경")
	}
}

func TestEquipCatImportLimits(t *testing.T) {
	if ExcelFormulaInjected("=1") == false || ExcelFormulaInjected("ok") {
		t.Fatal("inject")
	}
	if ExcelFormulaInjected("-2") {
		t.Fatal("음수 정렬")
	}
	hdr := []string{"조직코드", "분류코드", "분류명", "상위분류코드", "정렬", "사용"}
	big := make([][]string, 502)
	big[0] = hdr
	for i := 1; i <= 501; i++ {
		big[i] = []string{"O02", fmt.Sprintf("C%d", i), "n", "", "1", "Y"}
	}
	raw := catBook(t, big, [][]string{append([]string{}, catHdrField...)})
	if _, _, err := ParseEquipCatWorkbook(raw); err == nil {
		t.Fatal("분류 한도")
	}
}
