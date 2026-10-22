package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func sampleContractXLSX(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "..", "docs", "양식", "사업등록_업로드_최종_47건.xlsx")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestContractImportPreviewApplySample(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cimp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	raw := sampleContractXLSX(t)
	seedUniqueContractCustomers(t, db, raw, model.OrgIDLibrary)
	repo := NewProjectRepo(db)
	scope := CatImportScope{Allowed: map[string]bool{"O01": true, "O02": true, "O03": true}}
	today := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	prev, err := repo.PreviewContractImport(raw, "최종47.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN != 0 || prev.NewN != 47 || prev.SkipN != 0 {
		t.Fatalf("rows new=%d err=%d skip=%d %+v", prev.NewN, prev.ErrN, prev.SkipN, errRows(prev))
	}
	si, media, lib, costN, suN, bidN, propN, emptyM := 0, 0, 0, 0, 0, 0, 0, 0
	amtSI, amtM := 0, 0
	var hotel, school, giga, extra, extra2 *model.ContractImportRow
	for i := range prev.Rows {
		r := &prev.Rows[i]
		if r.Direction == model.ContractDirCost {
			costN++
		}
		if strings.Contains(r.OrgLabel, "SI") {
			si++
			amtSI += r.Amount
		} else if strings.Contains(r.OrgLabel, "미디어") {
			media++
			amtM += r.Amount
		} else if strings.Contains(r.OrgLabel, "도서") {
			lib++
		}
		switch r.ContractType {
		case "수의":
			suN++
		case "입찰":
			bidN++
		case "제안입찰":
			propN++
		case "":
			emptyM++
		}
		switch r.Name {
		case "호텔인터시티 인터넷서비스 재계약 투자 구매[비용]":
			hotel = r
		case "학교 네트워크 진단 및 개선":
			school = r
		case "학교 10Giga 인터넷 구축 사업 충청남도교육청 장비 유지보수 구매계약":
			giga = r
		case "충청남도교육청 학교 10Giga 인터넷 무선랜 구축 공사계약":
			extra = r
		case "충청남도교육청 학교 10Giga 인터넷 무선랜 구축 공사계약 (추가 계약)":
			extra2 = r
		}
		if r.StartDate == "" || r.EndDate == "" || r.ContractNo != "" {
			t.Fatalf("날짜·계약번호 %+v", r)
		}
	}
	if si != 14 || media != 33 || lib != 0 || costN != 0 {
		t.Fatalf("org si=%d media=%d lib=%d cost=%d", si, media, lib, costN)
	}
	if amtSI != 6814717272 || amtM != 2606224400 || amtSI+amtM != 9420941672 {
		t.Fatalf("amt si=%d media=%d", amtSI, amtM)
	}
	if emptyM != 18 || suN != 19 || bidN != 6 || propN != 4 {
		t.Fatalf("method empty=%d 수의=%d 입찰=%d 제안=%d", emptyM, suN, bidN, propN)
	}
	if hotel == nil || len(hotel.Years) != 5 || hotel.Years[0].Amount != 3348536 || hotel.Years[4].Amount != 3632932 {
		t.Fatalf("hotel %+v", hotel)
	}
	if hotel.Years[4].StartDate != "2029-01-17" || hotel.Years[4].EndDate != "2030-02-16" {
		t.Fatalf("hotel y5 %+v", hotel.Years[4])
	}
	if school == nil || len(school.Years) != 2 || school.Years[0].Amount != 4500486176 || school.StartDate != "2024-12-30" {
		t.Fatalf("school %+v", school)
	}
	if giga == nil || len(giga.Years) != 7 || giga.Years[6].Amount != 23533107 {
		t.Fatalf("giga %+v", giga)
	}
	if extra == nil || extra2 == nil || extra.Amount != 119308860 || extra2.Amount != 17292000 || extra.Name == extra2.Name {
		t.Fatalf("추가계약 extra=%+v extra2=%+v", extra, extra2)
	}
	nBefore := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&nBefore)
	app, err := repo.ApplyContractImport("U1", "최종47.xlsx", raw, scope, today, model.ContractImportApplyOpt{})
	if err != nil {
		t.Fatal(err)
	}
	if app.NewN != 47 {
		t.Fatalf("apply new %d", app.NewN)
	}
	var after, vat, cost, years, batch, salesN int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&after)
	if after-nBefore != 47 {
		t.Fatalf("inserted %d", after-nBefore)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE amount_vat_included=1`).Scan(&vat)
	if vat < 47 {
		t.Fatalf("vat %d", vat)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE direction='cost'`).Scan(&cost)
	if cost != 0 {
		t.Fatalf("cost %d", cost)
	}
	wantYears := 0
	for _, r := range prev.Rows {
		wantYears += len(r.Years)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM contract_years`).Scan(&years)
	if years != wantYears {
		t.Fatalf("years %d want %d", years, wantYears)
	}
	var yearProjects int
	_ = db.QueryRow(`SELECT COUNT(DISTINCT project_id) FROM contract_years`).Scan(&yearProjects)
	if yearProjects != 6 {
		t.Fatalf("year projects %d", yearProjects)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM contract_import_batches`).Scan(&batch)
	if batch != 1 {
		t.Fatalf("batch %d", batch)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE TRIM(COALESCE(sales_project_id,''))!=''`).Scan(&salesN)
	if salesN != 0 {
		t.Fatalf("sales %d", salesN)
	}
	again, err := repo.PreviewContractImport(raw, "최종47.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if again.SkipN != 47 || again.NewN != 0 {
		t.Fatalf("재업로드 skip %d new %d", again.SkipN, again.NewN)
	}
}

func errRows(p *model.ContractImportPreview) []string {
	var out []string
	if p == nil {
		return out
	}
	for _, r := range p.Rows {
		if r.Action == "err" {
			out = append(out, fmt.Sprintf("%d %s %s", r.Line, r.Name, r.Msg))
		}
	}
	return out
}

func TestContractImportValidation(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	org, err := repo.resolveContractOrg("SI사업팀")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','발주A','발주A',1,?)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id) VALUES
		('U-S','suser','x','김영업','sales',1,?),('U-E','euser','x','이수행','tech',1,?)`, org, org); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{
		append([]string{}, contractHdr...),
		{"정상", "SI사업팀", "발주A", "", "용역", "수의계약", "매출", "2025.01.01", "2026.06.30", "1000", "", "김영업", "이수행", ""},
		{"수식", "SI사업팀", "=cmd", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
		{"날짜역전", "SI사업팀", "발주A", "", "공사", "", "매출", "2026-01-01", "2025-01-01", "1", "", "", "", ""},
		{"없는발주", "SI사업팀", "없는곳", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
		{"구매", "SI사업팀", "발주A", "", "구매", "", "매입", "2025-01-01", "2025-12-31", "10", "", "", "", ""},
		{"없는날", "SI사업팀", "발주A", "", "공사", "", "매출", "2026-02-30", "2026-03-01", "1", "", "", "", ""},
	}
	raw, err := BuildContractWorkbook(ContractGuideLines(), rows)
	if err != nil {
		t.Fatal(err)
	}
	scope := CatImportScope{Allowed: map[string]bool{org: true}}
	prev, err := repo.PreviewContractImport(raw, "t.xlsx", scope, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN != 4 || prev.NewN != 2 {
		t.Fatalf("new=%d err=%d %+v", prev.NewN, prev.ErrN, prev.Rows)
	}
	foundMissing := false
	for _, r := range prev.Rows {
		if r.Name == "없는발주" {
			foundMissing = true
			if r.ErrKind != model.ContractErrCustomerMiss || !strings.Contains(r.Msg, "발주처 고객 마스터에 없습니다") || !strings.Contains(r.Msg, "없는곳") {
				t.Fatalf("없는발주 %+v", r)
			}
		}
	}
	if !foundMissing {
		t.Fatal("없는발주")
	}
	foundDev, foundOther := false, false
	for _, r := range prev.Rows {
		if r.Name == "정상" && r.Kind == model.ProjectKindDev && r.ContractType == "수의" && r.SalesID != "" && r.ExecID != "" && len(r.Years) >= 1 {
			foundDev = true
		}
		if r.Name == "구매" && r.Kind == model.ProjectKindOther && r.Direction == model.ContractDirCost {
			foundOther = true
		}
	}
	if !foundDev || !foundOther {
		t.Fatalf("kind map %+v", prev.Rows)
	}
	oaScope := CatImportScope{Allowed: map[string]bool{model.OrgIDLibrary: true}}
	oa, err := repo.PreviewContractImport(raw, "t.xlsx", oaScope, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if oa.ErrN < 2 {
		t.Fatalf("권한 오류 %d", oa.ErrN)
	}
	if oa.ErrDeniedN < 2 {
		t.Fatalf("권한 유형 %d", oa.ErrDeniedN)
	}
	for _, r := range oa.Rows {
		if r.ErrKind != model.ContractErrOrgForbidden {
			continue
		}
		if !strings.Contains(r.Msg, "이 계정으로는") || !strings.Contains(r.Msg, "시스템 관리자") {
			t.Fatalf("권한 문구 %+v", r)
		}
	}
}

func seedUniqueContractCustomers(t *testing.T, db *sql.DB, raw []byte, orgID string) {
	t.Helper()
	rows, err := ParseContractWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	n := 1
	for i, row := range rows {
		if i == 0 {
			continue
		}
		for _, col := range []int{2, 3} {
			for _, name := range model.SplitEndCustomers(cell(row, col)) {
				key := strings.TrimSpace(name)
				if key == "" || seen[key] {
					continue
				}
				seen[key] = true
				cid := fmt.Sprintf("CIMP-%03d", n)
				n++
				if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`, cid, name, name, orgID); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
