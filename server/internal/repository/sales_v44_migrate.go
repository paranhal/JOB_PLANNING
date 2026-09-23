package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// sales4StageMetaKey 44-C 이관이 끝나면 031·034·053 이 옛 단계를 되살리지 못하게 한다. §47.15.1
const sales4StageMetaKey = "__meta:sales_4stage_v1"

// applySales4StageSchema 44-B. 열·코드만 만든다. 데이터 이관은 applySales4Stage(44-C).
func applySales4StageSchema(db *sql.DB) {
	if db == nil {
		return
	}
	for _, col := range []struct{ name, ddl string }{
		{"sales_no", `ALTER TABLE sales_projects ADD COLUMN sales_no TEXT NOT NULL DEFAULT ''`},
		{"bid_status", `ALTER TABLE sales_projects ADD COLUMN bid_status TEXT NOT NULL DEFAULT ''`},
		{"close_reason", `ALTER TABLE sales_projects ADD COLUMN close_reason TEXT NOT NULL DEFAULT ''`},
		{"rfp_received_at", `ALTER TABLE sales_projects ADD COLUMN rfp_received_at TEXT NOT NULL DEFAULT ''`},
		{"win_prob", `ALTER TABLE sales_projects ADD COLUMN win_prob INTEGER`},
		{"probability_final", `ALTER TABLE sales_projects ADD COLUMN probability_final INTEGER`},
		{"awarded_amount", `ALTER TABLE sales_projects ADD COLUMN awarded_amount INTEGER NOT NULL DEFAULT 0`},
		{"contract_amount", `ALTER TABLE sales_projects ADD COLUMN contract_amount INTEGER NOT NULL DEFAULT 0`},
		{"contract_target", `ALTER TABLE sales_projects ADD COLUMN contract_target TEXT NOT NULL DEFAULT ''`},
		{"procurement_route", `ALTER TABLE sales_projects ADD COLUMN procurement_route TEXT NOT NULL DEFAULT ''`},
		{"contract_method", `ALTER TABLE sales_projects ADD COLUMN contract_method TEXT NOT NULL DEFAULT ''`},
		{"bid_eval_method", `ALTER TABLE sales_projects ADD COLUMN bid_eval_method TEXT NOT NULL DEFAULT ''`},
		{"mall_contract_type", `ALTER TABLE sales_projects ADD COLUMN mall_contract_type TEXT NOT NULL DEFAULT ''`},
		{"drop_reason_code", `ALTER TABLE sales_projects ADD COLUMN drop_reason_code TEXT NOT NULL DEFAULT ''`},
		{"drop_reason", `ALTER TABLE sales_projects ADD COLUMN drop_reason TEXT NOT NULL DEFAULT ''`},
		{"dropped_at", `ALTER TABLE sales_projects ADD COLUMN dropped_at TEXT NOT NULL DEFAULT ''`},
		{"dropped_by", `ALTER TABLE sales_projects ADD COLUMN dropped_by TEXT NOT NULL DEFAULT ''`},
		{"dropped_from_stage", `ALTER TABLE sales_projects ADD COLUMN dropped_from_stage TEXT NOT NULL DEFAULT ''`},
		{"prev_sales_id", `ALTER TABLE sales_projects ADD COLUMN prev_sales_id TEXT NOT NULL DEFAULT ''`},
		{"biz_type", `ALTER TABLE sales_projects ADD COLUMN biz_type TEXT NOT NULL DEFAULT ''`},
		{"budget_year", `ALTER TABLE sales_projects ADD COLUMN budget_year INTEGER NOT NULL DEFAULT 0`},
		{"budget_status", `ALTER TABLE sales_projects ADD COLUMN budget_status TEXT NOT NULL DEFAULT ''`},
		{"dormant_until", `ALTER TABLE sales_projects ADD COLUMN dormant_until TEXT NOT NULL DEFAULT ''`},
		{"dormant_reason", `ALTER TABLE sales_projects ADD COLUMN dormant_reason TEXT NOT NULL DEFAULT ''`},
		{"dormant_at", `ALTER TABLE sales_projects ADD COLUMN dormant_at TEXT NOT NULL DEFAULT ''`},
		{"dormant_by", `ALTER TABLE sales_projects ADD COLUMN dormant_by TEXT NOT NULL DEFAULT ''`},
		{"bid_ym", `ALTER TABLE sales_projects ADD COLUMN bid_ym TEXT NOT NULL DEFAULT ''`},
		{"revenue_ym", `ALTER TABLE sales_projects ADD COLUMN revenue_ym TEXT NOT NULL DEFAULT ''`},
		{"revenue_from", `ALTER TABLE sales_projects ADD COLUMN revenue_from TEXT NOT NULL DEFAULT ''`},
		{"revenue_to", `ALTER TABLE sales_projects ADD COLUMN revenue_to TEXT NOT NULL DEFAULT ''`},
		{"billing_cycle", `ALTER TABLE sales_projects ADD COLUMN billing_cycle TEXT NOT NULL DEFAULT ''`},
		{"amount_vat_included", `ALTER TABLE sales_projects ADD COLUMN amount_vat_included INTEGER NOT NULL DEFAULT 0`},
	} {
		addSalesProjectColumn(db, col.name, col.ddl)
	}
	addNamedColumn(db, "sales_stage_history", "detail", `ALTER TABLE sales_stage_history ADD COLUMN detail TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "data_change_logs", "reason", `ALTER TABLE data_change_logs ADD COLUMN reason TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_quotes", "quote_kind", `ALTER TABLE sales_quotes ADD COLUMN quote_kind TEXT NOT NULL DEFAULT ''`)

	for _, q := range []string{
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SS401','sales_stage4','discover','발굴',1,1),
			('SS402','sales_stage4','propose','제안',2,1),
			('SS403','sales_stage4','bid','입찰',3,1),
			('SS404','sales_stage4','closed','사업 종료',4,1),
			('S4P01','sales_stage4_prob','discover','10',1,1),
			('S4P02','sales_stage4_prob','propose','20',2,1),
			('SBS01','sales_bid_status','pending','결과 대기',1,1),
			('SBS02','sales_bid_status','won','수주',2,1),
			('SBS03','sales_bid_status','negotiating','협상 중',3,1),
			('SCR01','sales_close_reason','contracted','계약',1,1),
			('SCR02','sales_close_reason','lost','실주',2,1),
			('SCR03','sales_close_reason','dropped','포기',3,1),
			('SDR01','sales_drop_reason','budget','고객 예산 미확보·사업 취소',1,1),
			('SDR02','sales_drop_reason','postponed','고객 사업 무기한 연기',2,1),
			('SDR03','sales_drop_reason','low_margin','사업성 부족',3,1),
			('SDR04','sales_drop_reason','not_qualified','참가 자격·실적 미달',4,1),
			('SDR05','sales_drop_reason','competitor','경쟁 열세로 참여 포기',5,1),
			('SDR06','sales_drop_reason','resource','당사 인력·일정 사정',6,1),
			('SDR07','sales_drop_reason','duplicate','중복 등록',7,1),
			('SDR08','sales_drop_reason','etc','기타',8,1),
			('SLS06','sales_lead_source','customer_request','고객 요청',6,1),
			('SLS07','sales_lead_source','competitor_info','타사 정보 입수',7,1),
			('SLS08','sales_lead_source','internal_contact','당사 담당자 정보 입수',8,1),
			('SLS09','sales_lead_source','self_found','자체 발굴',9,1),
			('SCT01','sales_contract_target','construction','공사',1,1),
			('SCT02','sales_contract_target','goods_make','물품 제조',2,1),
			('SCT03','sales_contract_target','goods_buy','물품 구매',3,1),
			('SCT04','sales_contract_target','service','용역',4,1),
			('SPR01','sales_procurement_route','self','자체조달',1,1),
			('SPR02','sales_procurement_route','pps_central','조달청 중앙조달',2,1),
			('SPR03','sales_procurement_route','pps_mall','나라장터 종합쇼핑몰',3,1),
			('SPR04','sales_procurement_route','private','민간(해당 없음)',4,1),
			('SCM01','sales_contract_method','open','일반경쟁',1,1),
			('SCM02','sales_contract_method','limited','제한경쟁',2,1),
			('SCM03','sales_contract_method','nominated','지명경쟁',3,1),
			('SCM04','sales_contract_method','private_contract','수의계약',4,1),
			('SEM01','sales_bid_eval_method','qualification','적격심사',1,1),
			('SEM02','sales_bid_eval_method','negotiation','협상에 의한 계약',2,1),
			('SEM03','sales_bid_eval_method','two_stage','2단계 경쟁',3,1),
			('SEM04','sales_bid_eval_method','spec_price','규격가격 동시입찰',4,1),
			('SEM05','sales_bid_eval_method','comprehensive','종합심사',5,1),
			('SEM06','sales_bid_eval_method','lowest','최저가·가격입찰',6,1),
			('SMC01','sales_mall_contract_type','third_party','제3자 단가계약',1,1),
			('SMC02','sales_mall_contract_type','mas','다수공급자계약(MAS)',2,1),
			('SMC03','sales_mall_contract_type','mas_two_stage','MAS 2단계경쟁',3,1),
			('SBT01','sales_biz_type','build','구축',1,1),
			('SBT02','sales_biz_type','maintenance','유지보수',2,1),
			('SBT03','sales_biz_type','goods','물품 납품',3,1),
			('SBT04','sales_biz_type','develop','개발',4,1),
			('SBT05','sales_biz_type','construction','공사',5,1),
			('SBT06','sales_biz_type','etc','기타',6,1),
			('SBSU1','sales_budget_status','unknown','미확인',1,1),
			('SBSU2','sales_budget_status','not_reflected','미반영',2,1),
			('SBSU3','sales_budget_status','requesting','요청·협의 중',3,1),
			('SBSU4','sales_budget_status','confirmed','반영 확정',4,1),
			('SBSU5','sales_budget_status','no_plan','예산 계획 없음',5,1),
			('SAT13','sales_activity_type','requirement','요구사항 파악',13,1),
			('SAT14','sales_activity_type','rfp_received','RFP 접수',14,1),
			('SBC01','sales_billing_cycle','once','일시',1,1),
			('SBC02','sales_billing_cycle','month','월',2,1),
			('SBC03','sales_billing_cycle','quarter','분기',3,1),
			('SBC04','sales_billing_cycle','half','반기',4,1),
			('SBC05','sales_billing_cycle','year','연',5,1)`,
		`UPDATE codes SET code_name='제안서 제출' WHERE code_group='sales_activity_type' AND code_value='rfp'`,
		`INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at) VALUES ('sales_dormant_default_month','6', datetime('now'))`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("44-B sales 4stage schema: %v", err)
		}
	}
}

func addNamedColumn(db *sql.DB, table, name, ddl string) {
	if tableHasColumn(db, table, name) {
		return
	}
	if _, err := db.Exec(ddl); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("44-B %s.%s: %v", table, name, err)
	}
}

func applySales4Stage(db *sql.DB) {
	if db == nil {
		return
	}
	applySales4StageSchema(db)
	if !metaDone(db, sales4StageMetaKey) {
		if err := migrateSales4Stage(db); err != nil {
			log.Printf("44-C sales 4stage: %v", err)
		} else {
			markMetaDone(db, sales4StageMetaKey)
		}
	}
	applySalesNoV1(db)
	applySalesGroupsSchema(db)
}

// applySalesNoV1 기존 사업에 S{YYMM}-{NNN} 을 채우고 시퀀스를 달 최댓값에 맞춘다. §47.10.3
func applySalesNoV1(db *sql.DB) {
	if db == nil {
		return
	}
	applySales4StageSchema(db)
	rows, err := db.Query(`
		SELECT sales_id, COALESCE(created_at,'')
		FROM sales_projects
		WHERE TRIM(COALESCE(sales_no,''))=''
		ORDER BY created_at, sales_id`)
	if err != nil {
		log.Printf("44-G sales_no list: %v", err)
	} else {
		defer rows.Close()
		monthN := map[string]int{}
		exist, e2 := db.Query(`SELECT sales_no FROM sales_projects WHERE TRIM(COALESCE(sales_no,''))!=''`)
		if e2 == nil {
			for exist.Next() {
				var no string
				if exist.Scan(&no) == nil {
					if ym, n, ok := parseSalesNo(no); ok && n > monthN[ym] {
						monthN[ym] = n
					}
				}
			}
			exist.Close()
		}
		for rows.Next() {
			var id, created string
			if err := rows.Scan(&id, &created); err != nil {
				continue
			}
			t := parseSalesCreatedAt(created)
			ym := t.Format("0601")
			monthN[ym]++
			no := fmt.Sprintf("S%s-%03d", ym, monthN[ym])
			if _, err := db.Exec(`UPDATE sales_projects SET sales_no=? WHERE sales_id=?`, no, id); err != nil {
				log.Printf("44-G sales_no %s: %v", id, err)
			}
		}
	}
	alignSalesNoSequences(db)
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_projects_sales_no ON sales_projects(sales_no) WHERE sales_no != ''`); err != nil {
		log.Printf("44-G idx_sales_projects_sales_no: %v", err)
	}
	markMetaDone(db, salesNoMetaKey)
}

func parseSalesNo(no string) (ym string, n int, ok bool) {
	no = strings.TrimSpace(no)
	var seq int
	if _, err := fmt.Sscanf(no, "S%4s-%d", &ym, &seq); err != nil {
		return "", 0, false
	}
	if len(ym) != 4 || seq < 1 {
		return "", 0, false
	}
	return ym, seq, true
}

func parseSalesCreatedAt(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now()
	}
	if len(s) > 19 && s[10] == ' ' {
		s = s[:19]
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func alignSalesNoSequences(db *sql.DB) {
	rows, err := db.Query(`SELECT sales_no FROM sales_projects WHERE TRIM(COALESCE(sales_no,''))!=''`)
	if err != nil {
		return
	}
	defer rows.Close()
	maxN := map[string]int{}
	for rows.Next() {
		var no string
		if rows.Scan(&no) != nil {
			continue
		}
		if ym, n, ok := parseSalesNo(no); ok && n > maxN[ym] {
			maxN[ym] = n
		}
	}
	for ym, n := range maxN {
		key := salesNoSeqKey(ym)
		var last int
		err := db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, key).Scan(&last)
		if err == sql.ErrNoRows {
			_, _ = db.Exec(`INSERT INTO id_sequences(seq_key, last_no) VALUES(?,?)`, key, n)
			continue
		}
		if err == nil && n > last {
			_, _ = db.Exec(`UPDATE id_sequences SET last_no=? WHERE seq_key=?`, n, key)
		}
	}
}

type sales4MigIn struct {
	stage, status, dealType, contractedAt, wonAt, lostReason string
	probOverride                                             sql.NullInt64
	contractAmount, wpAmount                                 int
	wpStart                                                  string
	hasWP                                                    bool
}

type sales4MigOut struct {
	stage, bidStatus, closeReason, status string
	contractedAt, wonAt                   string
	contractAmount                        int
	dropCode, dropReason, dropFrom        string
	winProb, probFinal                    sql.NullInt64
	rfpAt                                 string
	probability                           int
	detail                                string
	fromDead, fromNegotiation             bool
}

func clampWinProb(n int) int {
	if n < 10 {
		return 10
	}
	if n > 90 {
		return 90
	}
	return n
}

func mapSales4Stage(in sales4MigIn, migrateDay string) sales4MigOut {
	st := strings.TrimSpace(in.stage)
	out := sales4MigOut{
		status:         strings.TrimSpace(in.status),
		contractedAt:   strings.TrimSpace(in.contractedAt),
		wonAt:          strings.TrimSpace(in.wonAt),
		contractAmount: in.contractAmount,
	}
	switch st {
	case "quote", "rfp", "submit":
		out.fromDead = true
		st = "proposal"
	case "contracted":
		out.fromDead = true
		st = "won"
	}
	promoted := out.status == "promoted" || in.hasWP
	switch {
	case st == "contact" || st == "lead":
		out.stage, out.status = "discover", "active"
	case st == "proposal":
		out.stage, out.status = "propose", "active"
	case st == "negotiation":
		out.stage, out.bidStatus, out.status = "bid", "pending", "active"
		out.fromNegotiation = true
	case st == "won" && promoted:
		out.stage, out.closeReason, out.status = "closed", "contracted", "promoted"
		if out.contractedAt == "" {
			if s := strings.TrimSpace(in.wpStart); s != "" {
				out.contractedAt = s
			} else {
				out.contractedAt = out.wonAt
			}
		}
		if out.contractAmount == 0 && in.wpAmount > 0 {
			out.contractAmount = in.wpAmount
		}
	case st == "won" && out.contractedAt == "" && !promoted:
		out.stage, out.bidStatus, out.status = "bid", "won", "active"
	case st == "won" && out.contractedAt != "":
		out.stage, out.closeReason = "closed", "contracted"
		if out.status == "promoted" {
			out.status = "promoted"
		} else {
			out.status = "contracted"
		}
	case st == "lost":
		out.stage, out.closeReason, out.status = "closed", "lost", "lost"
	case st == "inquiry":
		out.stage, out.status = "discover", "active"
	case st == "quoted":
		out.stage, out.status = "propose", "active"
	case st == "ordered":
		out.stage, out.bidStatus, out.status = "bid", "won", "active"
	case st == "delivered":
		out.stage, out.closeReason = "closed", "contracted"
		if out.status == "promoted" {
			out.status = "promoted"
		} else {
			out.status = "contracted"
		}
	case st == "dropped":
		out.stage, out.closeReason, out.status = "closed", "dropped", "dropped"
		out.dropCode, out.dropReason, out.dropFrom = "etc", strings.TrimSpace(in.lostReason), "discover"
	default:
		out.stage, out.status = "discover", "active"
	}

	switch {
	case out.stage == "discover":
		out.probability = 10
	case out.stage == "propose":
		out.probability = 20
		if in.probOverride.Valid {
			w := clampWinProb(int(in.probOverride.Int64))
			out.winProb = sql.NullInt64{Int64: int64(w), Valid: true}
			out.rfpAt = migrateDay
			out.probability = w
		}
	case out.stage == "bid" && out.bidStatus == "pending":
		if in.probOverride.Valid {
			w := clampWinProb(int(in.probOverride.Int64))
			out.probFinal = sql.NullInt64{Int64: int64(w), Valid: true}
			out.probability = w
		} else {
			out.probFinal = sql.NullInt64{Int64: 70, Valid: true}
			out.winProb = sql.NullInt64{Int64: 70, Valid: true}
			out.probability = 70
		}
	case out.stage == "bid":
		out.probability = 100
	case out.stage == "closed" && out.closeReason == "contracted":
		out.probability = 100
	default:
		out.probability = 0
	}
	if out.bidStatus != "" {
		out.detail = out.bidStatus
	} else {
		out.detail = out.closeReason
	}
	return out
}

func remapSalesLeadSource(v string) string {
	switch strings.TrimSpace(v) {
	case "existing":
		return "customer_request"
	case "referral":
		return "internal_contact"
	case "bid":
		return "self_found"
	default:
		return strings.TrimSpace(v)
	}
}

func nullIntArg(n sql.NullInt64) interface{} {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

func migrateSales4Stage(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT p.sales_id, p.name, COALESCE(p.stage,''), COALESCE(p.status,''), COALESCE(p.deal_type,'build'),
			COALESCE(p.legacy_stage,''), COALESCE(p.contracted_at,''), COALESCE(p.won_at,''),
			COALESCE(p.lost_reason,''), COALESCE(p.lead_source,''), COALESCE(p.probability,0),
			p.probability_override, COALESCE(p.expected_amount,0), COALESCE(p.contract_amount,0),
			COALESCE((SELECT w.start_date FROM work_projects w WHERE w.sales_project_id=p.sales_id AND TRIM(COALESCE(w.sales_project_id,''))!='' LIMIT 1), ''),
			COALESCE((SELECT w.contract_amount FROM work_projects w WHERE w.sales_project_id=p.sales_id AND TRIM(COALESCE(w.sales_project_id,''))!='' LIMIT 1), 0),
			CASE WHEN EXISTS(SELECT 1 FROM work_projects w WHERE w.sales_project_id=p.sales_id AND TRIM(COALESCE(w.sales_project_id,''))!='') THEN 1 ELSE 0 END
		FROM sales_projects p`)
	if err != nil {
		return err
	}

	type item struct {
		id, name, stage, status, deal, legacy, contractedAt, wonAt, lost, lead string
		prob, expected, contractAmount, wpAmount                               int
		override                                                               sql.NullInt64
		wpStart                                                                string
		hasWP                                                                  bool
	}
	var items []item
	for rows.Next() {
		var it item
		var hasWP int
		if err := rows.Scan(&it.id, &it.name, &it.stage, &it.status, &it.deal, &it.legacy, &it.contractedAt, &it.wonAt,
			&it.lost, &it.lead, &it.prob, &it.override, &it.expected, &it.contractAmount, &it.wpStart, &it.wpAmount, &hasWP); err != nil {
			rows.Close()
			return err
		}
		it.hasWP = hasWP == 1
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	migrateDay := time.Now().In(time.Local).Format("2006-01-02")
	now := time.Now().Format("2006-01-02 15:04:05")
	var dead, pendingFromNeg, weightedBefore, weightedAfter int
	var nDiscover, nPropose, nBidPending, nBidWon, nClosedC, nClosedL, nClosedD int

	for _, it := range items {
		weightedBefore += it.expected * it.prob / 100
		out := mapSales4Stage(sales4MigIn{
			stage: it.stage, status: it.status, dealType: it.deal,
			contractedAt: it.contractedAt, wonAt: it.wonAt, lostReason: it.lost,
			probOverride: it.override, contractAmount: it.contractAmount,
			wpAmount: it.wpAmount, wpStart: it.wpStart, hasWP: it.hasWP,
		}, migrateDay)
		if out.fromDead {
			dead++
		}
		if out.fromNegotiation {
			pendingFromNeg++
		}
		legacy := strings.TrimSpace(it.legacy)
		if legacy == "" {
			legacy = it.stage
		}
		lead := remapSalesLeadSource(it.lead)
		beforeB, _ := json.Marshal(map[string]any{
			"stage": it.stage, "status": it.status, "probability": it.prob, "lead_source": it.lead,
		})
		afterB, _ := json.Marshal(map[string]any{
			"stage": out.stage, "bid_status": out.bidStatus, "close_reason": out.closeReason,
			"status": out.status, "probability": out.probability, "lead_source": lead,
		})
		if _, err := tx.Exec(`
			UPDATE sales_projects SET
				legacy_stage=?, stage=?, bid_status=?, close_reason=?, status=?,
				contracted_at=?, won_at=?, contract_amount=?,
				drop_reason_code=?, drop_reason=?, dropped_from_stage=?,
				contract_target=CASE WHEN deal_type='supply' AND TRIM(COALESCE(contract_target,''))='' THEN 'goods_buy' ELSE contract_target END,
				lead_source=?, win_prob=?, probability_final=?, rfp_received_at=?, probability=?,
				updated_at=CURRENT_TIMESTAMP
			WHERE sales_id=?`,
			legacy, out.stage, out.bidStatus, out.closeReason, out.status,
			out.contractedAt, out.wonAt, out.contractAmount,
			out.dropCode, out.dropReason, out.dropFrom,
			lead, nullIntArg(out.winProb), nullIntArg(out.probFinal), out.rfpAt, out.probability,
			it.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO sales_stage_history (history_id, sales_id, from_stage, to_stage, detail, reason, changed_by, changed_by_id)
			VALUES (?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("SH44-%s", it.id), it.id, it.stage, out.stage, out.detail, "44단계 이관", "system", ""); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO data_change_logs (
				log_id, occurred_at, user_id, username, user_name,
				action, table_name, pk_column, entity_id, entity_label, summary,
				before_json, after_json, rolled_back, reason)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,?)`,
			"L"+uuid.New().String(), now, "", "system", "system",
			"update", "sales_projects", "sales_id", it.id, it.name, "44단계 이관",
			string(beforeB), string(afterB), "44단계 이관"); err != nil {
			return err
		}
		weightedAfter += it.expected * out.probability / 100
		switch {
		case out.stage == "discover":
			nDiscover++
		case out.stage == "propose":
			nPropose++
		case out.stage == "bid" && out.bidStatus == "pending":
			nBidPending++
		case out.stage == "bid":
			nBidWon++
		case out.stage == "closed" && out.closeReason == "contracted":
			nClosedC++
		case out.stage == "closed" && out.closeReason == "lost":
			nClosedL++
		case out.stage == "closed" && out.closeReason == "dropped":
			nClosedD++
		}
	}

	if dead > 0 {
		log.Printf("44 이관: 폐 코드 quote/rfp/submit/contracted %d건 → proposal/won 규칙", dead)
	}
	for _, q := range []string{
		`UPDATE codes SET is_active=0 WHERE code_group IN ('sales_stage','sales_stage_prob','sales_supply_stage')`,
		`UPDATE codes SET is_active=0 WHERE code_group='sales_lead_source' AND code_value IN ('exhibition','other')`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("44 이관: 총 %d건 | discover %d · propose %d · bid(pending %d · won %d) · closed(contracted %d · lost %d · dropped %d)\n         | 가중 파이프라인 전 %d원 → 후 %d원 | negotiation→pending 확인 필요 %d건",
		len(items), nDiscover, nPropose, nBidPending, nBidWon, nClosedC, nClosedL, nClosedD,
		weightedBefore, weightedAfter, pendingFromNeg)
	return nil
}
