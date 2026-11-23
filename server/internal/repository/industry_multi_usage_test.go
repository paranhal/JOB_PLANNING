package repository

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"customer-support/internal/model"
)

func seedIndustryCustomers(t *testing.T, path string) {
	t.Helper()
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for id, row := range map[string][2]string{
		"c1": {"단일도서관", "도서관"}, "c2": {"단일학교", "학교"}, "c3": {"충남도서관", "도서관,학교"},
		"c4": {"업종없음", ""}, "c5": {"공공기관A", "공공기관"}, "c6": {"별칭학교", "school"},
	} {
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, industry, org_id, party_kind) VALUES (?,?,?,1,?,'O01','customer')`, id, row[0], row[0], row[1]); err != nil {
			t.Fatal(err)
		}
	}
}

func listIDs(t *testing.T, r *CustomerRepo, industry string) []string {
	t.Helper()
	items, _, err := r.List(OrgAll, "", "", industry, "org_name", "asc", 1, 0, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range items {
		ids = append(ids, it.CustomerID)
	}
	sort.Strings(ids)
	return ids
}

// 95-219: 단일 업종 데이터의 결과는 그대로이고, 다중 업종 고객은 각 업종에 나타난다.
func TestIndustryMultiUsage95_219(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ind2.db")
	seedIndustryCustomers(t, path)
	db, err := InitDB(path) // 서버 재시작과 같다: 기동 시 보정이 다중 값을 덮으면 안 된다
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var cur string
	_ = db.QueryRow(`SELECT industry FROM customers WHERE customer_id='c3'`).Scan(&cur)
	if cur != "도서관,학교" {
		t.Fatalf("다시 열었더니 업종이 %q 로 바뀌었다(기동 시 UPDATE 제거 확인)", cur)
	}
	cr := NewCustomerRepo(db)

	// 목록 필터: 「도서관」에 c1·c3, 「학교」에 c2·c3·c6(별칭), 미지정 none → c4
	if got := listIDs(t, cr, "도서관"); !reflect.DeepEqual(got, []string{"c1", "c3"}) {
		t.Errorf("도서관 필터 %v", got)
	}
	if got := listIDs(t, cr, "학교"); !reflect.DeepEqual(got, []string{"c2", "c3", "c6"}) {
		t.Errorf("학교 필터 %v", got)
	}
	if got := listIDs(t, cr, "none"); !reflect.DeepEqual(got, []string{"c4"}) {
		t.Errorf("미지정 필터 %v", got)
	}
	if got := listIDs(t, cr, "공공기관"); !reflect.DeepEqual(got, []string{"c5"}) {
		t.Errorf("공공기관 필터(단일 데이터 불변) %v", got)
	}

	// 통계: 다중 고객은 각 업종에 1번씩, 별칭은 합쳐지고, 미지정 건수 불변
	stats, err := cr.ListIndustries()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, s := range stats {
		got[s.Industry] = s.Count
	}
	want := map[string]int{"도서관": 2, "학교": 3, "공공기관": 1, "": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("업종 통계 %v want %v", got, want)
	}
	if stats[len(stats)-1].Industry != "" {
		t.Error("미지정은 마지막이어야 한다")
	}

	// 정기점검 일괄 업종 목록도 같은 방식
	bulk, err := NewMaintenanceRepo(db).ListSiteBulkIndustries()
	if err != nil {
		t.Fatal(err)
	}
	bg := map[string]int{}
	for _, b := range bulk {
		bg[b.Key] = b.Count
	}
	if bg["도서관"] != 2 || bg["학교"] != 3 || bg["공공기관"] != 1 || bg[model.IndustryUnspecified] != 1 {
		t.Errorf("일괄 업종 목록 %v", bg)
	}

	// 후보 조회(Lookup): 학교 후보에 도서관이자 학교인 고객이 나온다
	opts, _, err := cr.Lookup(CustomerQuery{Scope: ScopeSite, Industry: "학교"})
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, o := range opts {
		has[o.ID] = true
	}
	if !has["c3"] || !has["c2"] || has["c1"] || has["c5"] {
		t.Errorf("학교 후보 %v", has)
	}

	// 사이트 유형: 도서관이자 학교는 두 유형에 모두 속하고, 업종이 비면 종전처럼 민간업체
	if got := siteTypesOfIndustry("도서관,학교"); !reflect.DeepEqual(got, []string{"library", "school"}) {
		t.Errorf("유형 %v", got)
	}
	if got := siteTypesOfIndustry(""); !reflect.DeepEqual(got, []string{"private"}) {
		t.Errorf("빈 업종 유형 %v", got)
	}
	if siteTypeOfIndustry("학교") != "school" || siteTypeOfIndustry("도서관") != "library" || siteTypeOfIndustry("교육청") != "private" {
		t.Error("단일 업종 유형은 종전과 같아야 한다")
	}
}
