package repository

import (
	"database/sql"
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

func reopenSalesLegacyMigrations(db *sql.DB) {
	_, _ = db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, sales4StageMetaKey)
}

func TestSales4StageMigrateTableAndIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales44c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		INSERT INTO sales_projects (sales_id, name, stage, probability, status, deal_type, expected_amount, lost_reason, lead_source, probability_override, contracted_at, won_at)
		VALUES
			('SP-CT','발굴','contact',10,'active','build',100, '', 'existing', NULL, '', ''),
			('SP-LD','검토','lead',25,'active','build',0, '', '', NULL, '', ''),
			('SP-PR','견적','proposal',40,'active','build',0, '', 'referral', NULL, '', ''),
			('SP-NG','협상','negotiation',70,'active','build',10000000, '', '', NULL, '', ''),
			('SP-WP','승격수주','won',100,'promoted','build',0, '', '', NULL, '', '2026-01-10'),
			('SP-WN','수수주','won',100,'active','build',0, '', '', NULL, '', '2026-01-11'),
			('SP-WC','계약수주','won',100,'active','build',0, '', '', NULL, '2026-04-01', '2026-03-01'),
			('SP-LS','실주','lost',0,'lost','build',0, '가격', '', NULL, '', ''),
			('SP-IQ','문의','inquiry',0,'active','supply',0, '', '', NULL, '', ''),
			('SP-QT','견적제출','quoted',0,'active','supply',0, '', '', NULL, '', ''),
			('SP-OR','발주','ordered',0,'active','supply',0, '', '', NULL, '', ''),
			('SP-DL','납품','delivered',0,'active','supply',0, '', '', NULL, '', ''),
			('SP-DR','취소','dropped',0,'active','supply',0, '예산부족', '', NULL, '', ''),
			('SP-Q8','폐견적','quote',20,'active','build',0, '', '', NULL, '', ''),
			('SP-OV','수동견적','proposal',40,'active','build',0, '', '', 40, '', '')
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO work_projects (project_id, name, status, sales_project_id, start_date, contract_amount)
		VALUES ('WP-1','승격사업','active','SP-WP','2026-02-01',5000000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sales_stage_history (history_id, sales_id, from_stage, to_stage, reason)
		VALUES ('SH-KEEP','SP-CT','lead','contact','옛이력')`); err != nil {
		t.Fatal(err)
	}

	var beforeN, beforeH int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects`).Scan(&beforeN)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_stage_history`).Scan(&beforeH)

	reopenSalesLegacyMigrations(db)
	applySales4Stage(db)

	type row struct {
		id, stage, bid, close, status, legacy, target, dropCode, dropReason, dropFrom, lead, contracted, rfp string
		prob                                                                                                 int
		win, fin                                                                                             sql.NullInt64
		amount                                                                                               int
	}
	get := func(id string) row {
		t.Helper()
		var r row
		r.id = id
		if err := db.QueryRow(`
			SELECT stage, bid_status, close_reason, status, legacy_stage, contract_target,
				drop_reason_code, drop_reason, dropped_from_stage, lead_source, contracted_at, rfp_received_at,
				probability, win_prob, probability_final, contract_amount
			FROM sales_projects WHERE sales_id=?`, id).Scan(
			&r.stage, &r.bid, &r.close, &r.status, &r.legacy, &r.target,
			&r.dropCode, &r.dropReason, &r.dropFrom, &r.lead, &r.contracted, &r.rfp,
			&r.prob, &r.win, &r.fin, &r.amount); err != nil {
			t.Fatal(id, err)
		}
		return r
	}

	ct := get("SP-CT")
	if ct.stage != "discover" || ct.status != "active" || ct.legacy != "contact" || ct.prob != 10 || ct.lead != "customer_request" {
		t.Fatalf("contact: %+v", ct)
	}
	ld := get("SP-LD")
	if ld.stage != "discover" || ld.legacy != "lead" {
		t.Fatalf("lead: %+v", ld)
	}
	pr := get("SP-PR")
	if pr.stage != "propose" || pr.prob != 20 || pr.rfp != "" || pr.win.Valid || pr.lead != "internal_contact" {
		t.Fatalf("proposal: %+v", pr)
	}
	ng := get("SP-NG")
	if ng.stage != "bid" || ng.bid != "pending" || ng.prob != 70 || !ng.fin.Valid || ng.fin.Int64 != 70 {
		t.Fatalf("negotiation: %+v", ng)
	}
	wp := get("SP-WP")
	if wp.stage != "closed" || wp.close != "contracted" || wp.status != "promoted" || wp.contracted != "2026-02-01" || wp.amount != 5000000 || wp.prob != 100 {
		t.Fatalf("won promoted: %+v", wp)
	}
	wn := get("SP-WN")
	if wn.stage != "bid" || wn.bid != "won" || wn.status != "active" {
		t.Fatalf("won no contract: %+v", wn)
	}
	wc := get("SP-WC")
	if wc.stage != "closed" || wc.close != "contracted" || wc.status != "contracted" {
		t.Fatalf("won contracted: %+v", wc)
	}
	ls := get("SP-LS")
	if ls.stage != "closed" || ls.close != "lost" || ls.status != "lost" || ls.prob != 0 {
		t.Fatalf("lost: %+v", ls)
	}
	if get("SP-IQ").stage != "discover" || get("SP-IQ").target != "goods_buy" {
		t.Fatalf("inquiry: %+v", get("SP-IQ"))
	}
	if get("SP-QT").stage != "propose" {
		t.Fatalf("quoted: %+v", get("SP-QT"))
	}
	or := get("SP-OR")
	if or.stage != "bid" || or.bid != "won" {
		t.Fatalf("ordered: %+v", or)
	}
	dl := get("SP-DL")
	if dl.stage != "closed" || dl.close != "contracted" || dl.status != "contracted" {
		t.Fatalf("delivered: %+v", dl)
	}
	dr := get("SP-DR")
	if dr.stage != "closed" || dr.close != "dropped" || dr.status != "dropped" || dr.dropCode != "etc" || dr.dropReason != "예산부족" || dr.dropFrom != "discover" {
		t.Fatalf("dropped: %+v", dr)
	}
	if get("SP-Q8").stage != "propose" {
		t.Fatalf("dead quote: %+v", get("SP-Q8"))
	}
	ov := get("SP-OV")
	if ov.stage != "propose" || !ov.win.Valid || ov.win.Int64 != 40 || ov.rfp == "" || ov.prob != 40 {
		t.Fatalf("proposal override: %+v", ov)
	}

	var afterN, afterH, keep int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_projects`).Scan(&afterN)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_stage_history`).Scan(&afterH)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_stage_history WHERE history_id='SH-KEEP'`).Scan(&keep)
	if afterN != beforeN {
		t.Fatalf("건수 %d → %d", beforeN, afterN)
	}
	if afterH < beforeH || keep != 1 {
		t.Fatalf("이력 before=%d after=%d keep=%d", beforeH, afterH, keep)
	}

	var sst, sss int
	_ = db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage' AND is_active=1`).Scan(&sst)
	_ = db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_supply_stage' AND is_active=1`).Scan(&sss)
	if sst != 0 || sss != 0 {
		t.Fatalf("옛 코드가 켜져 있다 sst=%d sss=%d", sst, sss)
	}

	applySales4Stage(db)
	applySalesStagesV227(db)
	applySalesDealTypeV228(db)
	applySalesStageLabels(db)
	if get("SP-NG").stage != "bid" || get("SP-NG").bid != "pending" {
		t.Fatal("두 번째 이관이 값을 바꿨다")
	}
	var sst2 int
	_ = db.QueryRow(`SELECT is_active FROM codes WHERE code_id='SST01'`).Scan(&sst2)
	if sst2 != 0 {
		t.Fatal("031 재실행 후 SST01 이 켜졌다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_stage_history`).Scan(&afterH)
	if afterH < beforeH {
		t.Fatal("이력 줄 수가 줄었다")
	}
}
