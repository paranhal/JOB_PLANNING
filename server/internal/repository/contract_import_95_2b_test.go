package repository

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func TestContractImportSIAdmin47Oracle(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cimp-si.db"))
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
	siOrg, err := repo.resolveContractOrg("SI사업팀")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	scope := CatImportScope{Allowed: map[string]bool{siOrg: true}}
	prev, err := repo.PreviewContractImport(raw, "최종47.xlsx", scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if prev.NewN != 14 || prev.ErrN != 33 || prev.ErrDeniedN != 33 || prev.ErrMissingN != 0 {
		t.Fatalf("si preview new=%d err=%d denied=%d missing=%d %+v", prev.NewN, prev.ErrN, prev.ErrDeniedN, prev.ErrMissingN, errRows(prev))
	}
	for _, r := range prev.Rows {
		if r.ErrKind != "org_denied" {
			continue
		}
		if !strings.Contains(r.Msg, "미디어") || !strings.Contains(r.Msg, "이 계정으로는") {
			t.Fatalf("denied msg %+v", r)
		}
	}
	app, err := repo.ApplyContractImport("U-SI", "최종47.xlsx", raw, scope, today)
	if err != nil {
		t.Fatal(err)
	}
	if app.NewN != 14 || app.ErrDeniedN != 33 {
		t.Fatalf("si apply new=%d denied=%d", app.NewN, app.ErrDeniedN)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE org_id=?`, siOrg).Scan(&n); err != nil || n != 14 {
		t.Fatalf("inserted %d err=%v", n, err)
	}
	pick, err := NewWBRepo(db).ListWorkPickProjects(siOrg)
	if err != nil {
		t.Fatal(err)
	}
	if len(pick) < 14 {
		t.Fatalf("pick %d", len(pick))
	}
	var pid string
	for _, r := range app.Rows {
		if r.Action == "new" && strings.HasPrefix(r.Msg, "WP-") {
			pid = r.Msg
			break
		}
	}
	if pid == "" {
		t.Fatal("적용 ID")
	}
	got, err := repo.Get(pid)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.OrderingPartyName) == "" && strings.TrimSpace(got.OrderingParty) == "" {
		t.Fatalf("고객명 비어 있음 %+v", got)
	}
}

func TestContractImportCustomerLookup95_2B(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "clook.db"))
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
		('C-LIB','발주A','발주A',1,?),
		('C-SI','우선고객','우선고객',1,?),
		('C-MD','우선고객','우선고객',1,'O02'),
		('C-A1','모호고객','모호고객',1,?),
		('C-A2','모호고객','모호고객',1,'O02'),
		('C-OFF','비활성','비활성',0,?)`, model.OrgIDLibrary, si, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id) VALUES
		('U-MD','mduser','x','박타부서','tech',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	id, err := repo.lookupCustomerName("발주A", si)
	if err != nil || id != "C-LIB" {
		t.Fatalf("타 조직 조회 %s %v", id, err)
	}
	id, err = repo.lookupCustomerName("우선고객", si)
	if err != nil || id != "C-SI" {
		t.Fatalf("부서 우선 %s %v", id, err)
	}
	_, err = repo.lookupCustomerName("모호고객", si)
	if err == nil || !strings.Contains(err.Error(), "여러 개") {
		t.Fatalf("모호 %v", err)
	}
	_, err = repo.lookupCustomerName("비활성", si)
	if err == nil || !strings.Contains(err.Error(), "고객 마스터에 없습니다") {
		t.Fatalf("비활성 %v", err)
	}
	rows := [][]string{
		append([]string{}, contractHdr...),
		{"타부서담당", "SI사업팀", "발주A", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "박타부서", "", ""},
		{"모호행", "SI사업팀", "모호고객", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
		{"비활행", "SI사업팀", "비활성", "", "공사", "", "매출", "2025-01-01", "2025-12-31", "1", "", "", "", ""},
	}
	raw, err := BuildContractWorkbook(ContractGuideLines(), rows)
	if err != nil {
		t.Fatal(err)
	}
	scope := CatImportScope{Allowed: map[string]bool{si: true}}
	prev, err := repo.PreviewContractImport(raw, "t.xlsx", scope, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if prev.NewN != 1 || prev.ErrN != 2 {
		t.Fatalf("new=%d err=%d %+v", prev.NewN, prev.ErrN, prev.Rows)
	}
	for _, r := range prev.Rows {
		switch r.Name {
		case "타부서담당":
			if r.Action != "new" || r.SalesID != "" || !strings.Contains(r.Msg, "계약담당은 비우고") {
				t.Fatalf("담당 %+v", r)
			}
		case "모호행":
			if r.ErrKind != "ambiguous" || !strings.Contains(r.Msg, "모호고객") {
				t.Fatalf("모호행 %+v", r)
			}
		case "비활행":
			if r.ErrKind != "missing_customer" || !strings.Contains(r.Msg, "비활성") {
				t.Fatalf("비활행 %+v", r)
			}
		}
	}
}

func TestContractImportErrorWorkbookMissingSheet(t *testing.T) {
	prev := &model.ContractImportPreview{
		Rows: []model.ContractImportRow{
			{Line: 2, Action: "err", Name: "사업1", ErrKind: "missing_customer", MissingName: "학교A", MissingRole: "발주처", Msg: "발주처 고객 마스터에 없습니다: 학교A"},
			{Line: 3, Action: "err", Name: "사업2", ErrKind: "missing_customer", MissingName: "학교A", MissingRole: "발주처", Msg: "발주처 고객 마스터에 없습니다: 학교A"},
			{Line: 4, Action: "err", Name: "사업3", ErrKind: "missing_customer", MissingName: "학교B", MissingRole: "최종수요처", Msg: "최종수요처 고객 마스터에 없습니다: 학교B"},
			{Line: 5, Action: "err", Name: "사업4", ErrKind: "org_denied", Msg: "이 계정으로는 미디어사업팀 사업을 등록할 수 없습니다"},
			{Line: 6, Action: "new", Name: "사업5"},
		},
	}
	tallyContractPreview(prev)
	if prev.ErrMissingN != 3 || prev.ErrDeniedN != 1 || prev.NewN != 1 {
		t.Fatalf("tally %+v", prev)
	}
	raw, err := BuildContractErrorWorkbook(prev)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if idx, _ := f.GetSheetIndex("누락 고객"); idx < 0 {
		t.Fatalf("sheets %v", f.GetSheetList())
	}
	var names []string
	for i := 2; i <= 10; i++ {
		v, _ := f.GetCellValue("누락 고객", fmt.Sprintf("A%d", i))
		if strings.TrimSpace(v) == "" {
			break
		}
		role, _ := f.GetCellValue("누락 고객", fmt.Sprintf("B%d", i))
		used, _ := f.GetCellValue("누락 고객", fmt.Sprintf("C%d", i))
		names = append(names, v+"|"+role+"|"+used)
	}
	if len(names) != 2 {
		t.Fatalf("unique %v", names)
	}
	if !strings.Contains(names[0], "학교A|발주처") || !strings.Contains(names[0], "사업1") || !strings.Contains(names[0], "사업2") {
		t.Fatalf("학교A %v", names)
	}
	if !strings.Contains(names[1], "학교B|최종수요처") {
		t.Fatalf("학교B %v", names)
	}
}
