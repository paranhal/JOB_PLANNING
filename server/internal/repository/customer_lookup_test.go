package repository

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func setupLookupDB(t *testing.T) *CustomerRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "lookup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, party_kind, addr_sido, addr_sigungu, address, org_code)
		VALUES
		('C-EDU','충남교육청','충청남도교육청',1,'O01',0,'customer','대전','서구','대전 서구','EDU1'),
		('C-O02','타조직도서관','타조직도서관',1,'O02',0,'customer','세종','세종','세종','O0201'),
		('C-OFF','숨은기관','숨은기관',0,'O01',0,'customer','대전','유성','대전 유성',''),
		('C-PT','채움협력','채움협력',1,'O01',0,'partner','대전','서구','',''),
		('C-DUP2','동일명','동일명B',1,'O02',0,'customer','세종','세종','','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, parent_customer_id, party_kind, addr_sido, addr_sigungu, address, org_code)
		VALUES
		('C-GONG','공주생명과학고등학교','공주생명과학고등학교',1,'O01',1,'C-EDU','customer','충남','공주시','충남 공주시','GONG1'),
		('C-DUP','동일명','동일명A',1,'O01',1,'C-EDU','customer','충남','천안시','','')`); err != nil {
		t.Fatal(err)
	}
	return NewCustomerRepo(db)
}

func idsOf(opts []CustomerOption) map[string]bool {
	m := map[string]bool{}
	for _, o := range opts {
		m[o.ID] = true
	}
	return m
}

func TestLookupIncludesChildAndOtherOrg(t *testing.T) {
	r := setupLookupDB(t)
	opts, total, err := r.Lookup(CustomerQuery{Scope: ScopeSite})
	if err != nil {
		t.Fatal(err)
	}
	got := idsOf(opts)
	if !got["C-EDU"] || !got["C-GONG"] || !got["C-O02"] {
		t.Fatalf("상위·하위·타조직 누락 %+v total=%d", got, total)
	}
	if got["C-OFF"] || got["C-PT"] {
		t.Fatalf("비활성·partner 포함 %+v", got)
	}
	receipt, err := r.ListForReceipt()
	if err != nil {
		t.Fatal(err)
	}
	sel, err := r.ListSelectable()
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt) != len(sel) || len(receipt) != len(opts) {
		t.Fatalf("ListForReceipt=%d ListSelectable=%d Lookup=%d", len(receipt), len(sel), len(opts))
	}
}

func TestLookupScopeAndIncludeInactive(t *testing.T) {
	r := setupLookupDB(t)
	site, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite})
	if err != nil {
		t.Fatal(err)
	}
	if idsOf(site)["C-PT"] {
		t.Fatal("ScopeSite 에 partner")
	}
	party, _, err := r.Lookup(CustomerQuery{Scope: ScopeParty})
	if err != nil {
		t.Fatal(err)
	}
	if !idsOf(party)["C-PT"] {
		t.Fatal("ScopeParty 에 partner 없음")
	}
	inc, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, IncludeIDs: []string{"C-OFF"}})
	if err != nil {
		t.Fatal(err)
	}
	off := idsOf(inc)
	if !off["C-OFF"] || !off["C-GONG"] {
		t.Fatalf("IncludeIDs %+v", off)
	}
	for _, o := range inc {
		if o.ID == "C-OFF" && !o.Inactive {
			t.Fatal("비활성 표시 없음")
		}
	}
}

func TestLookupSearchRankAndEscape(t *testing.T) {
	r := setupLookupDB(t)
	opts, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "공주생명과학고등학교"})
	if err != nil || len(opts) == 0 || opts[0].ID != "C-GONG" {
		t.Fatalf("완전일치 %+v %v", opts, err)
	}
	pre, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "공주"})
	if err != nil || len(pre) == 0 || pre[0].ID != "C-GONG" {
		t.Fatalf("접두 %+v %v", pre, err)
	}
	par, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "충남교육청"})
	if err != nil {
		t.Fatal(err)
	}
	if !idsOf(par)["C-GONG"] {
		t.Fatal("상위기관명으로 하위가 안 나옴")
	}
	addr, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "공주시"})
	if err != nil || !idsOf(addr)["C-GONG"] {
		t.Fatalf("지역 %+v %v", addr, err)
	}
	if _, err := r.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind)
		VALUES ('C-PCT','퍼센트%학교','퍼센트%학교',1,'customer'),
		       ('C-UND','밑줄_학교','밑줄_학교',1,'customer')`); err != nil {
		t.Fatal(err)
	}
	pct, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "퍼센트%"})
	if err != nil || !idsOf(pct)["C-PCT"] {
		t.Fatalf("%% 이스케이프 %+v %v", pct, err)
	}
	for _, o := range pct {
		if strings.Contains(o.Name, "밑줄") {
			t.Fatal("% 가 _ 와 섞임")
		}
	}
	und, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "밑줄_"})
	if err != nil || !idsOf(und)["C-UND"] {
		t.Fatalf("_ 이스케이프 %+v %v", und, err)
	}
}

func TestLookupLabelAndLimitTotal(t *testing.T) {
	r := setupLookupDB(t)
	opts, total, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 || total < 4 {
		t.Fatalf("limit=%d total=%d", len(opts), total)
	}
	all, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "동일명"})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range all {
		if o.Name == "동일명" && !strings.Contains(o.Label, "(") {
			t.Fatalf("동명 라벨 %q", o.Label)
		}
	}
	gong := CustomerOption{}
	full, _, _ := r.Lookup(CustomerQuery{Scope: ScopeSite})
	for _, o := range full {
		if o.ID == "C-GONG" {
			gong = o
		}
	}
	if !strings.Contains(gong.Label, "충남교육청") {
		t.Fatalf("하위 라벨 %q", gong.Label)
	}
}

func TestLookup2500NoDrop(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lookup2500.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	st, err := tx.Prepare(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind) VALUES (?,?,?,1,'customer')`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2500; i++ {
		id := "B" + strconv.Itoa(i)
		name := "대량기관" + strconv.Itoa(i)
		if _, err := st.Exec(id, name, name); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r := NewCustomerRepo(db)
	opts, total, err := r.Lookup(CustomerQuery{Scope: ScopeSite})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2500 || len(opts) != 2500 {
		t.Fatalf("2500 누락 total=%d n=%d", total, len(opts))
	}
	lim, tot2, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "대량기관", Limit: 20})
	if err != nil || len(lim) != 20 || tot2 != 2500 {
		t.Fatalf("검색 limit n=%d total=%d %v", len(lim), tot2, err)
	}
}

func TestListForReceiptMatchesLookup(t *testing.T) {
	r := setupLookupDB(t)
	a, err := r.ListForReceipt()
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ListSelectable()
	if err != nil {
		t.Fatal(err)
	}
	am, bm := map[string]bool{}, map[string]bool{}
	for _, c := range a {
		am[c.CustomerID] = true
	}
	for _, c := range b {
		bm[c.CustomerID] = true
	}
	if len(am) != len(bm) {
		t.Fatalf("집합 크기 %d vs %d", len(am), len(bm))
	}
	for id := range am {
		if !bm[id] {
			t.Fatalf("ListSelectable 에 없음 %s", id)
		}
	}
	_ = model.PartyKindCustomer
}

func TestLookupOrgIDFilterAndNormalizedInOrg(t *testing.T) {
	r := setupLookupDB(t)
	all, _, err := r.Lookup(CustomerQuery{Scope: ScopeSite, Q: "동일명"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("전사 동명 %d", len(all))
	}
	o01, err := r.LookupNormalizedInOrg("O01", "동일명")
	if err != nil {
		t.Fatal(err)
	}
	if len(o01) != 0 {
		t.Fatalf("동명이면 고르지 않음 %+v", o01)
	}
	hit, err := r.LookupNormalizedInOrg("O02", "타 조직 도서관")
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 1 || hit[0].ID != "C-O02" {
		t.Fatalf("정규화 %+v", hit)
	}
	shared, err := r.LookupNormalizedInOrg("O01", "타조직도서관")
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 || shared[0].ID != "C-O02" {
		t.Fatalf("전사 공용이어야 함 %+v", shared)
	}
}
