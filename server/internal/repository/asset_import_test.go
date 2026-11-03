package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_62_PreviewGanamAndSkipAndMissingCustomer(t *testing.T) {
	raw, err := BuildGanam27Workbook()
	if err != nil {
		t.Fatal(err)
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C-GNAM','가남초등학교','가남초등학교',1,'O03','customer')`); err != nil {
		t.Fatal(err)
	}
	ar := NewAssetRepo(db)
	cr := NewCustomerRepo(db)
	prev, err := ar.PreviewAssetImport(cr, "O03", "g.xlsx", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.OKN != 27 || prev.ErrN != 0 {
		t.Fatalf("미리보기 ok=%d err=%d", prev.OKN, prev.ErrN)
	}
	warnLoc, fileDup := 0, 0
	for _, r := range prev.Rows {
		if r.LocationWarn {
			warnLoc++
		}
		if r.FileDup {
			fileDup++
		}
	}
	if warnLoc != 1 {
		t.Fatalf("위치 경고 %d", warnLoc)
	}
	if fileDup != 3 {
		t.Fatalf("파일 중복 경고 %d", fileDup)
	}
	nCust := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&nCust)
	if nCust != 1 {
		t.Fatalf("고객을 만들면 안 됨 %d", nCust)
	}

	shared, err := ar.PreviewAssetImport(cr, "O01", "g.xlsx", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if shared.OKN != 27 || shared.ErrN != 0 {
		t.Fatalf("전사 고객 매칭 ok=%d err=%d", shared.OKN, shared.ErrN)
	}
}

func Test95_62_ParseFormulaAndOldManagementType(t *testing.T) {
	hdr := append([]string{}, assetImportDataHeaders...)
	line := make([]string, len(hdr))
	for i, h := range hdr {
		switch normalizeAssetHeader(h) {
		case "기관명":
			line[i] = "가남초등학교"
		case "제품명":
			line[i] = "=cmd"
		case "설치위치":
			line[i] = "본관/1층/101"
		}
	}
	raw, err := BuildAssetImportFixture([][]string{line})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseAssetImportWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != model.AssetImportError || !strings.Contains(strings.Join(rows[0].Msgs, ""), "수식") {
		t.Fatalf("%+v", rows)
	}
	line2 := make([]string, len(hdr))
	copy(line2, line)
	for i, h := range hdr {
		switch normalizeAssetHeader(h) {
		case "제품명":
			line2[i] = "AP"
		case "관리유형":
			line2[i] = "직접유지보수"
		}
	}
	raw2, err := BuildAssetImportFixture([][]string{line2})
	if err != nil {
		t.Fatal(err)
	}
	rows2, err := parseAssetImportWorkbook(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows2) != 1 || rows2[0].Action != model.AssetImportError {
		t.Fatalf("구 관리유형 %+v", rows2)
	}
}

func Test95_62_AmbiguousCustomerInOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "amb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C1','동일학교','동일학교',1,'O01','customer'),
		       ('C2','동일학교','동일학교B',1,'O01','customer')`); err != nil {
		t.Fatal(err)
	}
	raw, err := BuildAssetImportFixture([][]string{{
		"동일학교", "", "AP", "본관/1층/101", "", "", "", "네트워크장비", "", "", "", "", "", "", "정기점검",
	}})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := NewAssetRepo(db).PreviewAssetImport(NewCustomerRepo(db), "O01", "t.xlsx", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN != 1 || prev.ErrDupN != 1 || !strings.Contains(strings.Join(prev.Rows[0].Msgs, ""), "동명 여러 개") {
		t.Fatalf("%+v", prev)
	}
}

func oneAssetLine(site string) [][]string {
	return [][]string{{site, "", "AP", "본관/1층/101", "", "", "", "네트워크장비", "", "", "", "", "", "", "정기점검"}}
}

func Test95_62A_MatchReasonsOfficialShortNoOrgEmptySheet(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a62a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O01','O01','도서관사업팀','도서관',1,1),
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, party_kind)
		VALUES ('C-OFF','가남초','가남초등학교',1,'O03','customer'),
		       ('C-LIB','청양초등학교','청양초등학교',1,'O01','customer'),
		       ('C-NO','미지정학교','미지정학교',1,'','customer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name) VALUES ('C-OFF','가남')`); err != nil {
		t.Fatal(err)
	}
	ar := NewAssetRepo(db)
	cr := NewCustomerRepo(db)

	rawOff, err := BuildAssetImportFixture(oneAssetLine("가남초등학교"))
	if err != nil {
		t.Fatal(err)
	}
	prev, err := ar.PreviewAssetImport(cr, "O03", "t.xlsx", rawOff, false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.OKN != 1 || prev.Rows[0].CustomerID != "C-OFF" {
		t.Fatalf("공식명 %+v", prev)
	}
	rawShort, err := BuildAssetImportFixture(oneAssetLine("가남"))
	if err != nil {
		t.Fatal(err)
	}
	ps, err := ar.PreviewAssetImport(cr, "O03", "t.xlsx", rawShort, false)
	if err != nil {
		t.Fatal(err)
	}
	if ps.OKN != 1 || ps.Rows[0].CustomerID != "C-OFF" {
		t.Fatalf("짧은이름 %+v", ps)
	}

	rawLib, err := BuildAssetImportFixture(oneAssetLine("청양초등학교"))
	if err != nil {
		t.Fatal(err)
	}
	po, err := ar.PreviewAssetImport(cr, "O03", "t.xlsx", rawLib, false)
	if err != nil {
		t.Fatal(err)
	}
	if po.OKN != 1 || po.Rows[0].CustomerID != "C-LIB" {
		t.Fatalf("전사 공용 고객 %+v", po)
	}

	rawMiss, err := BuildAssetImportFixture(oneAssetLine("없는학교"))
	if err != nil {
		t.Fatal(err)
	}
	pm, err := ar.PreviewAssetImport(cr, "O03", "t.xlsx", rawMiss, false)
	if err != nil {
		t.Fatal(err)
	}
	if pm.ErrMissingN != 1 || !strings.Contains(strings.Join(pm.Rows[0].Msgs, ""), "등록된 기관 없음") {
		t.Fatalf("없음 %+v", pm)
	}

	rawNo, err := BuildAssetImportFixture(oneAssetLine("미지정학교"))
	if err != nil {
		t.Fatal(err)
	}
	pn, err := ar.PreviewAssetImport(cr, "O03", "t.xlsx", rawNo, false)
	if err != nil {
		t.Fatal(err)
	}
	if pn.OKN != 1 || pn.Rows[0].CustomerID != "C-NO" {
		t.Fatalf("조직 미지정 고객도 매칭 %+v", pn)
	}

	empty, err := BuildAssetImportTemplate()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ar.PreviewAssetImport(cr, "O03", "empty.xlsx", empty, false)
	if err == nil || !strings.Contains(err.Error(), "자산입력") || !strings.Contains(err.Error(), "2행") {
		t.Fatalf("빈 시트 %v", err)
	}
}
