package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestContractImportPipelineZeroAmount(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cimp-2c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	si, err := repo.resolveContractOrg("SI사업팀")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('C-A','발주A','발주A',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{
		append([]string{}, contractHdr...),
		{"금액있음", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1000", "", "", "", ""},
		{"금액0-1", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-03-01", "2025-12-31", "0", "", "", "", ""},
		{"금액0-2", "SI사업팀", "발주A", "", "용역", "", "매출", "2025-04-01", "2026-04-01", "0", "", "", "", ""},
		{"금액0-3", "SI사업팀", "발주A", "", "구축", "", "매출", "2025-05-01", "2025-11-01", "0", "", "", "", ""},
		{"금액빈칸", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "", "", "", "", ""},
		{"타부서0", "미디어사업팀", "발주A", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "0", "", "", "", ""},
		{"없는고객", "SI사업팀", "없는곳", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "10", "", "", "", ""},
	}
	raw, err := BuildContractWorkbook(ContractGuideLines(), rows)
	if err != nil {
		t.Fatal(err)
	}
	scope := CatImportScope{Allowed: map[string]bool{si: true}}
	today := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	prev, err := repo.PreviewContractImport(raw, "z.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if prev.NewN != 1 || prev.PipeN != 3 || prev.ErrDeniedN != 1 || prev.ErrMissingN != 1 {
		t.Fatalf("class new=%d pipe=%d denied=%d miss=%d other=%d %+v", prev.NewN, prev.PipeN, prev.ErrDeniedN, prev.ErrMissingN, prev.ErrOtherN, errRows(prev))
	}
	if prev.Summary.Total != len(prev.Rows) {
		t.Fatalf("total %d rows %d", prev.Summary.Total, len(prev.Rows))
	}
	for _, r := range prev.Rows {
		switch r.Name {
		case "금액있음":
			if r.Action != model.ContractActionNew {
				t.Fatalf("%+v", r)
			}
		case "금액0-1", "금액0-2", "금액0-3":
			if r.Action != model.ContractActionPipeline {
				t.Fatalf("%+v", r)
			}
		case "금액빈칸":
			if r.ErrKind != model.ContractErrAmountInvalid {
				t.Fatalf("%+v", r)
			}
		case "타부서0":
			if r.ErrKind != model.ContractErrOrgForbidden {
				t.Fatalf("%+v", r)
			}
		case "없는고객":
			if r.ErrKind != model.ContractErrCustomerMiss {
				t.Fatalf("%+v", r)
			}
		}
	}
	_, err = repo.ApplyContractImport("U1", "z.xlsx", raw, scope, today, model.ContractImportApplyOpt{})
	if err != ErrPipelineNeedsConfirm {
		t.Fatalf("confirm %v", err)
	}
	no, err := repo.ApplyContractImport("U1", "z.xlsx", raw, scope, today, model.ContractImportApplyOpt{Pipeline: "no"})
	if err != nil {
		t.Fatal(err)
	}
	if no.NewN != 1 || no.PipeN != 0 || no.ErrN < 3 {
		t.Fatalf("no-pipe %+v", no.Summary)
	}
	var wp, sp int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE name LIKE '금액0%'`).Scan(&wp)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects`).Scan(&sp)
	if wp != 0 || sp != 0 {
		t.Fatalf("no apply wp=%d sp=%d", wp, sp)
	}
	yes, err := repo.ApplyContractImport("U1", "z.xlsx", raw, scope, today, model.ContractImportApplyOpt{Pipeline: "yes", Stage: model.SalesStage4Propose})
	if err != nil {
		t.Fatal(err)
	}
	if yes.NewN != 0 || yes.PipeN != 3 {
		t.Fatalf("yes after skip-new? new=%d pipe=%d skip=%d %+v", yes.NewN, yes.PipeN, yes.SkipN, errRows(yes))
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE name LIKE '금액0%'`).Scan(&wp)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects`).Scan(&sp)
	if wp != 0 || sp != 3 {
		t.Fatalf("wp=%d sp=%d", wp, sp)
	}
	var stage, deal, notes, ym string
	var amt, conf, ymConf int
	var org, cid string
	if err := db.QueryRow(`SELECT stage, deal_type, notes, expected_ym, expected_amount, expected_amount_confirmed, expected_ym_confirmed, org_id, TRIM(COALESCE(customer_id,''))
		FROM sales_projects WHERE name='금액0-1'`).Scan(&stage, &deal, &notes, &ym, &amt, &conf, &ymConf, &org, &cid); err != nil {
		t.Fatal(err)
	}
	if stage != model.SalesStage4Propose || deal != model.SalesDealBuild || amt != 0 || conf != 0 || ymConf != 0 || org != si || cid != "C-A" {
		t.Fatalf("row %s %s amt=%d org=%s cid=%s", stage, deal, amt, org, cid)
	}
	if !strings.Contains(notes, "엑셀 업로드(금액 0)") || ym != "2025-03" {
		t.Fatalf("notes %s ym %s", notes, ym)
	}
	var hist int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_stage_history WHERE reason='엑셀 업로드'`).Scan(&hist)
	if hist != 3 {
		t.Fatalf("hist %d", hist)
	}
	again, err := repo.PreviewContractImport(raw, "z.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	dupN := 0
	for _, r := range again.Rows {
		if r.ErrKind == model.ContractErrDupPipeline {
			dupN++
			if !strings.Contains(r.Msg, "이미 영업 파이프라인에 있습니다") {
				t.Fatalf("%+v", r)
			}
		}
	}
	if dupN != 3 {
		t.Fatalf("dup %d %+v", dupN, errRows(again))
	}
}

func TestContractImportPipelineLineExcludeAndCancel(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cimp-2c2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	si, err := repo.resolveContractOrg("SI사업팀")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C-A','발주A','발주A',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{
		append([]string{}, contractHdr...),
		{"P1", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "0", "", "", "", ""},
		{"P2", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-02-01", "2025-12-31", "0", "", "", "", ""},
	}
	raw, err := BuildContractWorkbook(ContractGuideLines(), rows)
	if err != nil {
		t.Fatal(err)
	}
	scope := CatImportScope{Allowed: map[string]bool{si: true}}
	today := time.Now()
	prev, err := repo.PreviewContractImport(raw, "p.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	var line1 int
	for _, r := range prev.Rows {
		if r.Name == "P1" {
			line1 = r.Line
		}
	}
	one, err := repo.ApplyContractImport("U1", "p.xlsx", raw, scope, today, model.ContractImportApplyOpt{
		Pipeline: "yes", Lines: map[int]bool{line1: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if one.PipeN != 1 || one.ErrN != 1 {
		t.Fatalf("exclude pipe=%d err=%d", one.PipeN, one.ErrN)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE name='P1'`).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE name='P2'`).Scan(&n)
	if n != 0 {
		t.Fatal("P2")
	}
}

func TestContractImport47StillAmountPositive(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cimp-47.db"))
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
	prev, err := repo.PreviewContractImport(raw, "47.xlsx", scope, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if prev.PipeN != 0 || prev.NewN != 47 || prev.Summary.Total != 47 {
		t.Fatalf("47 pipe=%d new=%d total=%d", prev.PipeN, prev.NewN, prev.Summary.Total)
	}
}
