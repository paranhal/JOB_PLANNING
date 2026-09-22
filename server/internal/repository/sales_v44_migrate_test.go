package repository

import (
	"path/filepath"
	"testing"
)

func TestSales4StageSchemaAddsColumnsAndCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales44b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO sales_projects (sales_id, name, stage, probability, status) VALUES ('SP-OLD','기존','lead',10,'active')`); err != nil {
		t.Fatal(err)
	}

	for _, col := range []string{
		"sales_no", "bid_status", "close_reason", "rfp_received_at", "win_prob", "probability_final",
		"awarded_amount", "contract_amount", "contract_target", "procurement_route", "contract_method",
		"bid_eval_method", "mall_contract_type", "drop_reason_code", "drop_reason", "dropped_at",
		"dropped_by", "dropped_from_stage", "prev_sales_id",
	} {
		if !salesProjectsHasColumn(db, col) {
			t.Fatalf("sales_projects.%s 없음", col)
		}
	}
	if !tableHasColumn(db, "sales_stage_history", "detail") {
		t.Fatal("sales_stage_history.detail 없음")
	}
	if !tableHasColumn(db, "data_change_logs", "reason") {
		t.Fatal("data_change_logs.reason 없음")
	}
	if !tableHasColumn(db, "sales_quotes", "quote_kind") {
		t.Fatal("sales_quotes.quote_kind 없음")
	}

	var salesNo, bid string
	var awarded int
	var winProb interface{}
	if err := db.QueryRow(`SELECT sales_no, bid_status, awarded_amount, win_prob FROM sales_projects WHERE sales_id='SP-OLD'`).
		Scan(&salesNo, &bid, &awarded, &winProb); err != nil {
		t.Fatal(err)
	}
	if salesNo != "" || bid != "" || awarded != 0 || winProb != nil {
		t.Fatalf("기존 행 기본값 sales_no=%q bid=%q awarded=%d win_prob=%v", salesNo, bid, awarded, winProb)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage4' AND is_active=1`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("sales_stage4 n=%d err=%v", n, err)
	}
	var rfpName string
	if err := db.QueryRow(`SELECT code_name FROM codes WHERE code_group='sales_activity_type' AND code_value='rfp'`).Scan(&rfpName); err != nil {
		t.Fatal(err)
	}
	if rfpName != "제안서 제출" {
		t.Fatalf("rfp 이름=%q", rfpName)
	}
	var oldName string
	if err := db.QueryRow(`SELECT code_name FROM codes WHERE code_id='SST02'`).Scan(&oldName); err != nil {
		t.Fatal(err)
	}
	if oldName == "" {
		t.Fatal("옛 SST02 가 지워졌다")
	}

	var quotes, orders int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sales_quotes'`).Scan(&quotes); err != nil || quotes != 1 {
		t.Fatalf("sales_quotes 없음 n=%d err=%v", quotes, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sales_orders'`).Scan(&orders); err != nil || orders != 1 {
		t.Fatalf("sales_orders 없음 n=%d err=%v", orders, err)
	}
}

func TestSalesLegacyMigrationsStopWhen4StageMetaDone(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales44b-guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`UPDATE codes SET is_active=0 WHERE code_id='SST01'`); err != nil {
		t.Fatal(err)
	}
	markMetaDone(db, sales4StageMetaKey)

	applySalesStagesV227(db)
	applySalesDealTypeV228(db)
	applySalesStageLabels(db)

	var active int
	if err := db.QueryRow(`SELECT is_active FROM codes WHERE code_id='SST01'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatal("메타 있는데 SST01 이 다시 켜졌다")
	}

	var quotes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sales_quotes'`).Scan(&quotes); err != nil || quotes != 1 {
		t.Fatalf("가드 뒤에도 applySalesItems 가 돌아야 한다 quotes=%d err=%v", quotes, err)
	}
}
