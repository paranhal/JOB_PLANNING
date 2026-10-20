package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func sampleContractXLSX(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "..", "docs", "양식", "사업등록_업로드_작성본.xlsx")
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
		('O02','O02','SI사업팀','business',1,20),('O03','O03','미디어사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	raw := sampleContractXLSX(t)
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
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				cid := fmt.Sprintf("CIMP-%03d", n)
				n++
				if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, cid, name, name); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	repo := NewProjectRepo(db)
	scope := CatImportScope{Allowed: map[string]bool{"O01": true, "O02": true, "O03": true}}
	today := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	prev, err := repo.PreviewContractImport(raw, "작성본.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if prev.NewN+prev.ErrN+prev.SkipN != 48 {
		t.Fatalf("rows new=%d err=%d skip=%d", prev.NewN, prev.ErrN, prev.SkipN)
	}
	if prev.ErrN < 1 || prev.NewN < 20 {
		t.Fatalf("new=%d err=%d", prev.NewN, prev.ErrN)
	}
	var hotel, school *model.ContractImportRow
	costN := 0
	for i := range prev.Rows {
		r := &prev.Rows[i]
		if r.Direction == model.ContractDirCost {
			costN++
			hotel = r
		}
		if r.Name == "학교 네트워크 진단 및 개선" {
			school = r
		}
	}
	if costN != 1 || hotel == nil || hotel.Action != "new" || len(hotel.Years) != 5 {
		t.Fatalf("hotel %+v costN=%d", hotel, costN)
	}
	if hotel.Years[4].StartDate != "2029-01-17" || hotel.Years[4].EndDate != "2030-02-16" {
		t.Fatalf("hotel y5 %+v", hotel.Years[4])
	}
	hs := 0
	for _, y := range hotel.Years {
		hs += y.Amount
	}
	if hs != hotel.Amount {
		t.Fatalf("hotel year sum %d want %d", hs, hotel.Amount)
	}
	if school == nil || len(school.Years) != 2 || school.Years[0].Amount != 4500486176 {
		t.Fatalf("school %+v", school)
	}
	nBefore := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&nBefore)
	app, err := repo.ApplyContractImport("U1", "작성본.xlsx", raw, scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if app.NewN != prev.NewN {
		t.Fatalf("apply new %d want %d", app.NewN, prev.NewN)
	}
	var after, vat, cost, years, batch int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&after)
	if after-nBefore != app.NewN {
		t.Fatalf("inserted %d new %d before %d", after-nBefore, app.NewN, nBefore)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE amount_vat_included=1`).Scan(&vat)
	if vat < app.NewN {
		t.Fatalf("vat %d", vat)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE direction='cost'`).Scan(&cost)
	if cost != 1 {
		t.Fatalf("cost %d", cost)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM contract_years`).Scan(&years)
	if years < 5 {
		t.Fatalf("years %d", years)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM contract_import_batches`).Scan(&batch)
	if batch != 1 {
		t.Fatalf("batch %d", batch)
	}
	again, err := repo.PreviewContractImport(raw, "작성본.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if again.SkipN < app.NewN {
		t.Fatalf("재업로드 skip %d new was %d", again.SkipN, app.NewN)
	}
}

func TestContractImportValidation(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','SI사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','발주A','발주A',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id) VALUES
		('U-S','suser','x','김영업','sales',1,'O02'),('U-E','euser','x','이수행','tech',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{
		append([]string{}, contractHdr...),
		{"정상", "SI사업팀", "발주A", "", "용역", "수의", "매출", "2025-01-01", "2026-06-30", "1000", "", "김영업", "이수행", ""},
		{"수식", "SI사업팀", "=cmd", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
		{"날짜역전", "SI사업팀", "발주A", "", "공사", "", "매출", "2026-01-01", "2025-01-01", "1", "", "", "", ""},
		{"없는발주", "SI사업팀", "없는곳", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
		{"구매", "SI사업팀", "발주A", "", "구매", "", "매입", "2025-01-01", "2025-12-31", "10", "", "", "", ""},
	}
	raw, err := BuildContractWorkbook(ContractGuideLines(), rows)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	scope := CatImportScope{Allowed: map[string]bool{"O02": true}}
	prev, err := repo.PreviewContractImport(raw, "t.xlsx", scope, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if prev.ErrN != 3 || prev.NewN != 2 {
		t.Fatalf("new=%d err=%d %+v", prev.NewN, prev.ErrN, prev.Rows)
	}
	foundDev, foundOther := false, false
	for _, r := range prev.Rows {
		if r.Name == "정상" && r.Kind == model.ProjectKindDev && r.SalesID != "" && r.ExecID != "" && len(r.Years) >= 1 {
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
}
