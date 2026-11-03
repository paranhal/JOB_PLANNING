package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func testCauseCatRepo(t *testing.T) *ASRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "causecat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	return NewASRepo(db)
}

func causeBook(t *testing.T, rows [][]string) []byte {
	t.Helper()
	raw, err := BuildCauseCatWorkbook(CauseCatGuideLines(), rows, CauseTypeSheetRows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func Test95_97_T1_ListCauseCategoriesForOrg(t *testing.T) {
	r := testCauseCatRepo(t)
	lib, err := r.ListCauseCategoriesForOrg(model.LegacyCauseOrgID)
	if err != nil || len(lib) == 0 {
		t.Fatalf("도서관 기존 표 %d %v", len(lib), err)
	}
	legacy, err := r.ListCauseCategories()
	if err != nil || len(legacy) != len(lib) {
		t.Fatalf("기존 ListCauseCategories 불변 %d vs %d", len(legacy), len(lib))
	}
	empty, err := r.ListCauseCategoriesForOrg("O03")
	if err != nil || len(empty) != 0 {
		t.Fatalf("빈 표 O03 %d %v", len(empty), err)
	}
	raw := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"디스플레이", "LED", "패널", "hw", "Y", "10", "Y"},
	})
	if _, err := r.ApplyCauseCatImport("u", "a.xlsx", "O03", raw); err != nil {
		t.Fatal(err)
	}
	o3, _ := r.ListCauseCategoriesForOrg("O03")
	if model.CauseCatalogEmpty(o3) {
		t.Fatal("O03 표가 비어 있다")
	}
	o2, _ := r.ListCauseCategoriesForOrg("O02")
	if len(o2) != 0 {
		t.Fatal("O02에 O03 표가 보인다")
	}
	lib2, _ := r.ListCauseCategoriesForOrg(model.LegacyCauseOrgID)
	if len(lib2) != len(lib) {
		t.Fatal("도서관 표가 바뀌었다")
	}
}

func Test95_97_T3_CauseCatParser(t *testing.T) {
	r := testCauseCatRepo(t)
	if _, err := ParseCauseCatWorkbook(nil); err == nil {
		t.Fatal("빈 파일")
	}
	bad := causeBook(t, [][]string{{"1차", "2차", "3차", "유형", "장애", "정렬", "사용"}})
	if _, err := ParseCauseCatWorkbook(bad); err == nil || !strings.Contains(err.Error(), "헤더") {
		t.Fatalf("헤더 %v", err)
	}
	skip := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"A", "", "C", "", "", "", ""},
	})
	prev, err := r.PreviewCauseCatImport(skip, "s.xlsx", "O03")
	if err != nil || prev.ErrN == 0 {
		t.Fatalf("건너뜀 %v %+v", err, prev)
	}
	dup := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"A", "B", "", "", "", "", ""},
		{"A", "B", "", "", "", "", ""},
	})
	prev, _ = r.PreviewCauseCatImport(dup, "d.xlsx", "O03")
	if prev == nil || prev.ErrN == 0 {
		t.Fatal("중복")
	}
	long := strings.Repeat("가", 41)
	too := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{long, "", "", "", "", "", ""},
	})
	prev, _ = r.PreviewCauseCatImport(too, "l.xlsx", "O03")
	if prev == nil || prev.ErrN == 0 {
		t.Fatal("글자 수")
	}
	yn := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"A", "", "", "", "X", "", ""},
	})
	prev, _ = r.PreviewCauseCatImport(yn, "y.xlsx", "O03")
	if prev == nil || prev.ErrN == 0 {
		t.Fatal("값 오류")
	}
	emptyLine := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"", "", "", "", "", "", ""},
		{"정상", "", "", "", "", "", ""},
	})
	prev, err = r.PreviewCauseCatImport(emptyLine, "e.xlsx", "O03")
	if err != nil || prev.ErrN != 0 {
		t.Fatalf("빈 줄 %v %+v", err, prev)
	}
	inj := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"=CMD", "", "", "", "", "", ""},
	})
	prev, _ = r.PreviewCauseCatImport(inj, "i.xlsx", "O03")
	if prev == nil || prev.ErrN == 0 {
		t.Fatal("수식")
	}
	hdr := append([]string{}, causeCatHdr...)
	big := [][]string{hdr}
	for i := 0; i < 2001; i++ {
		big = append(big, []string{"N" + itoaCause(i), "", "", "", "", "", ""})
	}
	raw, err := BuildCauseCatWorkbook(CauseCatGuideLines(), big, CauseTypeSheetRows(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCauseCatWorkbook(raw); err == nil {
		t.Fatal("2000행 초과")
	}
}

func itoaCause(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func Test95_97_T4_StableCodeAndRename(t *testing.T) {
	r := testCauseCatRepo(t)
	raw := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"서버", "HW", "팬", "", "Y", "10", "Y"},
	})
	a, err := r.ApplyCauseCatImport("u", "a.xlsx", "O03", raw)
	if err != nil {
		t.Fatal(err)
	}
	var n1 int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id='O03'`).Scan(&n1)
	if n1 == 0 {
		t.Fatal("적용 0")
	}
	code := model.CauseOrgPathCode("O03", "서버", "HW", "팬")
	again, err := r.ApplyCauseCatImport("u", "a.xlsx", "O03", raw)
	if err != nil {
		t.Fatal(err)
	}
	var n2 int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id='O03'`).Scan(&n2)
	if n2 != n1 {
		t.Fatalf("재적용 행수 %d→%d", n1, n2)
	}
	if again.NewN != 0 {
		t.Fatalf("재적용 new=%d previewFirst new=%d", again.NewN, a.NewN)
	}
	got, _ := r.getCauseCategoryOrg("O03", code)
	if got == nil {
		t.Fatal("안정 코드 없음")
	}
	ren := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"서버", "HW", "팬교체", "", "Y", "20", "Y"},
	})
	if _, err := r.ApplyCauseCatImport("u", "b.xlsx", "O03", ren); err != nil {
		t.Fatal(err)
	}
	old, _ := r.getCauseCategoryOrg("O03", code)
	if old == nil || old.Label != "팬" {
		t.Fatal("옛 분류가 사라졌다")
	}
	var n3 int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id='O03'`).Scan(&n3)
	if n3 <= n1 {
		t.Fatalf("이름 변경 후 추가 안 됨 %d", n3)
	}
}

func Test95_97_T5_PreviewNoWriteApplyTx(t *testing.T) {
	r := testCauseCatRepo(t)
	raw := causeBook(t, [][]string{
		append([]string{}, causeCatHdr...),
		{"정상", "하위", "", "hw", "Y", "1", "Y"},
		{"=BAD", "", "", "", "", "", ""},
	})
	prev, err := r.PreviewCauseCatImport(raw, "p.xlsx", "O03")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs`).Scan(&n)
	if n != 0 {
		t.Fatal("미리보기가 DB를 바꿈")
	}
	if prev.ErrN == 0 || prev.NewN == 0 {
		t.Fatalf("미리보기 집계 %+v", prev)
	}
	if _, err := r.ApplyCauseCatImport("u", "p.xlsx", "O03", raw); err != nil {
		t.Fatal(err)
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id='O03'`).Scan(&n)
	if n == 0 {
		t.Fatal("오류 행만 건너뛴 적용이 안 됐다")
	}
}
