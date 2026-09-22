package repository

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesStagesV227MigrateKeepsLegacyAndRollback(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_v227.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	reopenSalesLegacyMigrations(db)
	applySalesStagesV227(db)
	LoadLookupCache(db)

	if _, err := db.Exec(`
		INSERT INTO sales_projects (sales_id, name, stage, probability, status)
		VALUES
			('SP-Q','견적건','quote',20,'active'),
			('SP-R','RFP건','rfp',30,'active'),
			('SP-S','제출건','submit',50,'active'),
			('SP-C','계약건','contracted',100,'active'),
			('SP-W','수주건','won',90,'active'),
			('SP-L','리드건','lead',10,'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sales_stage_history (history_id, sales_id, from_stage, to_stage, reason, changed_at)
		VALUES
			('SH-Q','SP-Q','lead','quote','','2026-01-15 10:00:00'),
			('SH-C','SP-C','won','contracted','','2026-03-01 09:00:00')`); err != nil {
		t.Fatal(err)
	}

	applySalesStagesV227(db)

	repo := NewSalesRepo(db)
	q, err := repo.Get("SP-Q")
	if err != nil {
		t.Fatal(err)
	}
	if q.Stage != model.SalesStageProposal || q.LegacyStage != model.SalesStageQuote {
		t.Fatalf("quote 이관: stage=%s legacy=%s", q.Stage, q.LegacyStage)
	}
	c, err := repo.Get("SP-C")
	if err != nil {
		t.Fatal(err)
	}
	if c.Stage != model.SalesStageWon || c.LegacyStage != model.SalesStageContracted {
		t.Fatalf("contracted 이관: stage=%s legacy=%s", c.Stage, c.LegacyStage)
	}
	if c.ContractedAt != "2026-03-01" {
		t.Fatalf("contracted_at=%q", c.ContractedAt)
	}
	lead, err := repo.Get("SP-L")
	if err != nil {
		t.Fatal(err)
	}
	if lead.Stage != model.SalesStageLead || lead.Probability != 25 {
		t.Fatalf("lead 확도: stage=%s prob=%d", lead.Stage, lead.Probability)
	}

	acts, err := repo.ListActivities("SP-Q")
	if err != nil {
		t.Fatal(err)
	}
	foundQuote := false
	for _, a := range acts {
		if a.Title == "[이관] 견적 요청" && a.ActivityType == model.SalesActTypeQuote && a.ActivityDate == "2026-01-15" {
			foundQuote = true
		}
	}
	if !foundQuote {
		t.Fatalf("이관 활동 없음: %+v", acts)
	}
	hist, err := repo.ListHistory("SP-Q")
	if err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, h := range hist {
		if h.ToStage == model.SalesStageQuote && h.ToLabel == "견적 요청" {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("이력 옛 코드가 사라졌다: %+v", hist)
	}

	stages, err := repo.Stages()
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 4 || stages[0].Code != model.SalesStage4Discover || stages[1].Code != model.SalesStage4Propose {
		t.Fatalf("4단계 순서: %+v", stages)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	downPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations", "031_sales_stages_v227.down.sql")
	raw, err := os.ReadFile(downPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		upper := strings.ToUpper(stmt)
		if !strings.Contains(upper, "UPDATE ") && !strings.Contains(upper, "DELETE ") {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("rollback %q: %v", stmt, err)
		}
	}
	q2, err := repo.Get("SP-Q")
	if err != nil {
		t.Fatal(err)
	}
	if q2.Stage != model.SalesStageQuote {
		t.Fatalf("롤백 후 stage=%s", q2.Stage)
	}
	c2, _ := repo.Get("SP-C")
	if c2.Stage != model.SalesStageContracted {
		t.Fatalf("롤백 후 contracted stage=%s", c2.Stage)
	}
	acts2, _ := repo.ListActivities("SP-Q")
	for _, a := range acts2 {
		if strings.HasPrefix(a.Title, "[이관]") {
			t.Fatal("롤백 후에도 이관 활동이 남았다")
		}
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage' AND is_active=1`).Scan(&active); err != nil || active != 9 {
		t.Fatalf("롤백 후 active stages=%d err=%v", active, err)
	}
}

func TestSalesContractUnsignedUnplanned(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_unsigned.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{
		Name: "날인 없는 수주", CustomerConfirmed: true,
		ExpectedYMConfirmed: true, ExpectedAmountConfirmed: true,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesDirectWin, "", "u1", "t", false); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	if _, err := db.Exec(`UPDATE sales_projects SET won_at=?, contracted_at='' WHERE sales_id=?`, old, p.SalesID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.SalesID)
	if !model.SalesContractUnsigned(got, time.Now()) {
		t.Fatal("30일 초과인데 계약 미체결이 아니다")
	}
	_, kinds, err := repo.ListFollowupGaps(time.Now().Format("2006-01"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ks := range kinds {
		for _, k := range ks {
			if k == model.UnplannedSalesUnsigned {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("미계획 계약 미체결 없음: %+v", kinds)
	}
}
