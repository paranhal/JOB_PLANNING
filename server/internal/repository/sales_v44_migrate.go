package repository

import (
	"database/sql"
	"log"
	"strings"
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
			('SAT13','sales_activity_type','requirement','요구사항 파악',13,1),
			('SAT14','sales_activity_type','rfp_received','RFP 접수',14,1)`,
		`UPDATE codes SET code_name='제안서 제출' WHERE code_group='sales_activity_type' AND code_value='rfp'`,
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
