package repository

import (
	"database/sql"
	"log"
)

const salesProjectsMetaKey = "__meta:sales_projects_v1"
const salesStageCodesMetaKey = "__meta:sales_stage_codes_v1"

const salesProjectsSchema = `
CREATE TABLE IF NOT EXISTS sales_projects (
  sales_id                   TEXT PRIMARY KEY,
  name                       TEXT NOT NULL,
  is_tentative_name          INTEGER NOT NULL DEFAULT 0,
  stage                      TEXT NOT NULL DEFAULT 'lead',
  probability                INTEGER NOT NULL DEFAULT 10,
  probability_override       INTEGER,
  customer_id                TEXT,
  prospect_name              TEXT NOT NULL DEFAULT '',
  prospect_region            TEXT NOT NULL DEFAULT '',
  customer_confirmed         INTEGER NOT NULL DEFAULT 0,
  expected_ym                TEXT NOT NULL DEFAULT '',
  expected_precision         TEXT NOT NULL DEFAULT 'month',
  expected_ym_confirmed      INTEGER NOT NULL DEFAULT 0,
  expected_amount            INTEGER NOT NULL DEFAULT 0,
  expected_amount_confirmed  INTEGER NOT NULL DEFAULT 0,
  sales_owner                TEXT NOT NULL DEFAULT '',
  sales_owner_id             TEXT,
  competitor                 TEXT NOT NULL DEFAULT '',
  lead_source                TEXT NOT NULL DEFAULT '',
  lost_reason                TEXT NOT NULL DEFAULT '',
  status                     TEXT NOT NULL DEFAULT 'active',
  notes                      TEXT NOT NULL DEFAULT '',
  created_at                 DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at                 DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const salesStageHistorySchema = `
CREATE TABLE IF NOT EXISTS sales_stage_history (
  history_id     TEXT PRIMARY KEY,
  sales_id       TEXT NOT NULL,
  from_stage     TEXT NOT NULL DEFAULT '',
  to_stage       TEXT NOT NULL,
  reason         TEXT NOT NULL DEFAULT '',
  changed_by     TEXT NOT NULL DEFAULT '',
  changed_by_id  TEXT NOT NULL DEFAULT '',
  changed_at     DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// applySalesProjects 마이그레이션 013. §32.1 — work_projects 와 표를 나눈다.
func applySalesProjects(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(salesProjectsSchema); err != nil {
		log.Printf("013 sales_projects: %v", err)
		return
	}
	if _, err := db.Exec(salesStageHistorySchema); err != nil {
		log.Printf("013 sales_stage_history: %v", err)
		return
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_projects_stage ON sales_projects(stage)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_projects_status ON sales_projects(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_projects_expected_ym ON sales_projects(expected_ym)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_projects_owner ON sales_projects(sales_owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_stage_history_sales ON sales_stage_history(sales_id, changed_at)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("013 sales index: %v", err)
		}
	}
	markMetaDone(db, salesProjectsMetaKey)
	seedSalesStageCodes(db)
	applySalesDealTypeV228(db)
}

func seedSalesStageCodes(db *sql.DB) {
	if metaDone(db, salesStageCodesMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
		('SST01','sales_stage','lead','검토',1,1),
		('SST02','sales_stage','contact','발굴',2,1),
		('SST03','sales_stage','proposal','견적',3,1),
		('SST04','sales_stage','quote','견적 요청',4,1),
		('SST05','sales_stage','rfp','RFP 제안',5,1),
		('SST06','sales_stage','submit','제안서 제출',6,1),
		('SST07','sales_stage','won','수주확정',7,1),
		('SST08','sales_stage','contracted','계약완료',8,1),
		('SST09','sales_stage','lost','수주실패',9,1),
		('SSP01','sales_stage_prob','lead','10',1,1),
		('SSP02','sales_stage_prob','contact','10',2,1),
		('SSP03','sales_stage_prob','proposal','15',3,1),
		('SSP04','sales_stage_prob','quote','20',4,1),
		('SSP05','sales_stage_prob','rfp','30',5,1),
		('SSP06','sales_stage_prob','submit','50',6,1),
		('SSP07','sales_stage_prob','won','90',7,1),
		('SSP08','sales_stage_prob','contracted','100',8,1),
		('SSP09','sales_stage_prob','lost','0',9,1),
		('SLS01','sales_lead_source','existing','기존 고객',1,1),
		('SLS02','sales_lead_source','bid','입찰공고',2,1),
		('SLS03','sales_lead_source','referral','소개',3,1),
		('SLS04','sales_lead_source','exhibition','전시회',4,1),
		('SLS05','sales_lead_source','other','기타',5,1)`); err != nil {
		log.Printf("013 sales_stage codes: %v", err)
		return
	}
	markMetaDone(db, salesStageCodesMetaKey)
}

const salesActivitiesMetaKey = "__meta:sales_activities_v1"
const salesActivityTypeCodesMetaKey = "__meta:sales_activity_type_codes_v1"

const salesActivitiesSchema = `
CREATE TABLE IF NOT EXISTS sales_activities (
  activity_id       TEXT PRIMARY KEY,
  sales_id          TEXT NOT NULL,
  activity_date     TEXT NOT NULL,
  start_time        TEXT NOT NULL DEFAULT '',
  duration_min      INTEGER NOT NULL DEFAULT 30,
  activity_type     TEXT NOT NULL DEFAULT '',
  title             TEXT NOT NULL DEFAULT '',
  content           TEXT NOT NULL DEFAULT '',
  place             TEXT NOT NULL DEFAULT '',
  our_members       TEXT NOT NULL DEFAULT '',
  counterparts      TEXT NOT NULL DEFAULT '',
  next_action       TEXT NOT NULL DEFAULT '',
  next_action_date  TEXT NOT NULL DEFAULT '',
  stage_at_time     TEXT NOT NULL DEFAULT '',
  created_by        TEXT NOT NULL DEFAULT '',
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// applySalesActivities 마이그레이션 014. §32.8 — 영업 활동 로그.
func applySalesActivities(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(salesActivitiesSchema); err != nil {
		log.Printf("014 sales_activities: %v", err)
		return
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_activities_sales ON sales_activities(sales_id, activity_date)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_activities_date ON sales_activities(activity_date)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("014 sales_activities index: %v", err)
		}
	}
	markMetaDone(db, salesActivitiesMetaKey)
	seedSalesActivityTypeCodes(db)
}

func seedSalesActivityTypeCodes(db *sql.DB) {
	if metaDone(db, salesActivityTypeCodesMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
		('SAT01','sales_activity_type','research','정보수집',1,1),
		('SAT02','sales_activity_type','call','전화',2,1),
		('SAT03','sales_activity_type','visit','방문미팅',3,1),
		('SAT04','sales_activity_type','online','온라인미팅',4,1),
		('SAT05','sales_activity_type','mail','메일',5,1),
		('SAT06','sales_activity_type','material','자료송부',6,1),
		('SAT07','sales_activity_type','quote','견적제출',7,1),
		('SAT08','sales_activity_type','proposal','제안서제출',8,1),
		('SAT09','sales_activity_type','bid','입찰',9,1),
		('SAT10','sales_activity_type','other','기타',10,1)`); err != nil {
		log.Printf("014 sales_activity_type codes: %v", err)
		return
	}
	markMetaDone(db, salesActivityTypeCodesMetaKey)
}

const salesPartiesMetaKey = "__meta:sales_parties_v1"
const salesPartyCodesMetaKey = "__meta:sales_party_codes_v1"

const salesPartiesSchema = `
CREATE TABLE IF NOT EXISTS sales_parties (
  party_id         TEXT PRIMARY KEY,
  sales_id         TEXT NOT NULL,
  party_type       TEXT NOT NULL DEFAULT 'own',
  org_name         TEXT NOT NULL DEFAULT '',
  person_name      TEXT NOT NULL DEFAULT '',
  title            TEXT NOT NULL DEFAULT '',
  phone            TEXT NOT NULL DEFAULT '',
  email            TEXT NOT NULL DEFAULT '',
  party_role       TEXT NOT NULL DEFAULT '',
  is_primary       INTEGER NOT NULL DEFAULT 0,
  user_id          TEXT,
  contact_id       TEXT,
  is_active        INTEGER NOT NULL DEFAULT 1,
  replaced_by      TEXT,
  replaced_reason  TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  is_auto          INTEGER NOT NULL DEFAULT 0,
  created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const salesChangesSchema = `
CREATE TABLE IF NOT EXISTS sales_changes (
  change_id    TEXT PRIMARY KEY,
  sales_id     TEXT NOT NULL,
  field_key    TEXT NOT NULL,
  old_value    TEXT NOT NULL DEFAULT '',
  new_value    TEXT NOT NULL DEFAULT '',
  changed_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  changed_by   TEXT NOT NULL DEFAULT '',
  note         TEXT NOT NULL DEFAULT ''
)`

// applySalesParties 마이그레이션 015. §32.6~§32.7
func applySalesParties(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(salesPartiesSchema); err != nil {
		log.Printf("015 sales_parties: %v", err)
		return
	}
	if _, err := db.Exec(salesChangesSchema); err != nil {
		log.Printf("015 sales_changes: %v", err)
		return
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_parties_sales ON sales_parties(sales_id, party_type, is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_changes_sales ON sales_changes(sales_id, changed_at)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("015 sales_parties index: %v", err)
		}
	}
	markMetaDone(db, salesPartiesMetaKey)
	seedSalesPartyCodes(db)
}

func seedSalesPartyCodes(db *sql.DB) {
	if metaDone(db, salesPartyCodesMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
		('SPT01','sales_party_type','own','당사',1,1),
		('SPT02','sales_party_type','partner','협력사',2,1),
		('SPT03','sales_party_type','customer','고객',3,1),
		('SPR01','sales_party_role','decision','결정권자',1,1),
		('SPR02','sales_party_role','working','실무',2,1),
		('SPR03','sales_party_role','purchase','구매',3,1),
		('SPR04','sales_party_role','tech','기술검토',4,1),
		('SPR05','sales_party_role','sales','영업',5,1),
		('SPR06','sales_party_role','other','기타',6,1)`); err != nil {
		log.Printf("015 sales_party codes: %v", err)
		return
	}
	markMetaDone(db, salesPartyCodesMetaKey)
}

const salesPromoteMetaKey = "__meta:sales_promote_v1"

// applySalesPromote 마이그레이션 016. §32.10 — work_projects 에 원본 영업 건만 잇는다.
func applySalesPromote(db *sql.DB) {
	if db == nil {
		return
	}
	if !workProjectHasColumn(db, "sales_project_id") {
		if _, err := db.Exec(`ALTER TABLE work_projects ADD COLUMN sales_project_id TEXT`); err != nil {
			log.Printf("016 sales_project_id: %v", err)
			return
		}
	}
	if _, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_work_projects_sales
		ON work_projects(sales_project_id)
		WHERE sales_project_id IS NOT NULL AND TRIM(sales_project_id) != ''`); err != nil {
		log.Printf("016 idx_work_projects_sales: %v", err)
	}
	markMetaDone(db, salesPromoteMetaKey)
}
