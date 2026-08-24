package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

const salesProjectCodesMetaKey = "__meta:sales_project_codes_v1"

var salesProjectAlters = []string{
	`ALTER TABLE work_projects ADD COLUMN sales_stage TEXT`,
	`ALTER TABLE work_projects ADD COLUMN expected_ym TEXT`,
	`ALTER TABLE work_projects ADD COLUMN expected_precision TEXT NOT NULL DEFAULT 'month'`,
	`ALTER TABLE work_projects ADD COLUMN expected_note TEXT`,
	`ALTER TABLE work_projects ADD COLUMN expected_undated_reason TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_name TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_region TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_contact_name TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_contact_title TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_contact_phone TEXT`,
	`ALTER TABLE work_projects ADD COLUMN prospect_contact_email TEXT`,
	`ALTER TABLE work_projects ADD COLUMN sales_owner TEXT`,
	`ALTER TABLE work_projects ADD COLUMN sales_owner_id TEXT`,
	`ALTER TABLE work_projects ADD COLUMN expected_amount INTEGER`,
	`ALTER TABLE work_projects ADD COLUMN win_probability INTEGER`,
	`ALTER TABLE work_projects ADD COLUMN competitor TEXT`,
	`ALTER TABLE work_projects ADD COLUMN lead_source TEXT`,
}

// applySalesProject 마이그레이션 012. §22.1.1 ②~⑦ · ⑪단계 2
func applySalesProject(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range salesProjectAlters {
		if _, err := db.Exec(q); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("012 work_projects sales: %v", err)
		}
	}
	seedSalesProjectCodes(db)
}

func seedSalesProjectCodes(db *sql.DB) {
	if metaDone(db, salesProjectCodesMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('SS001','sales_stage','lead','발굴',1),
		('SS002','sales_stage','proposal','제안',2),
		('SS003','sales_stage','quote','견적',3),
		('SS004','sales_stage','bid','입찰·협상',4),
		('SS005','sales_stage','won','수주',5),
		('SS006','sales_stage','lost','실주',6),
		('SS007','sales_stage','dropped','보류',7),
		('SLS001','sales_lead_source','existing','기존 고객',1),
		('SLS002','sales_lead_source','bid','입찰공고',2),
		('SLS003','sales_lead_source','referral','소개',3),
		('SLS004','sales_lead_source','exhibition','전시회',4),
		('SLS005','sales_lead_source','other','기타',5)`); err != nil {
		log.Printf("012 sales_project codes: %v", err)
		return
	}
	markMetaDone(db, salesProjectCodesMetaKey)
}

func salesProjectInsertCols() string {
	return `sales_stage, expected_ym, expected_precision, expected_note, expected_undated_reason,
			prospect_name, prospect_region,
			prospect_contact_name, prospect_contact_title, prospect_contact_phone, prospect_contact_email,
			sales_owner, sales_owner_id, expected_amount, win_probability, competitor, lead_source`
}

func salesProjectInsertPlaceholders() string {
	return `?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?`
}

func salesProjectArgs(p *model.WorkProject) []interface{} {
	if p == nil {
		return nil
	}
	prec := p.ExpectedPrecision
	if prec == "" {
		prec = model.ExpectedPrecisionMonth
	}
	return []interface{}{
		p.SalesStage, p.ExpectedYM, prec, p.ExpectedNote, p.ExpectedUndatedReason,
		p.ProspectName, p.ProspectRegion,
		p.ProspectContactName, p.ProspectContactTitle, p.ProspectContactPhone, p.ProspectContactEmail,
		p.SalesOwner, nullStr(p.SalesOwnerID), p.ExpectedAmount, p.WinProbability, p.Competitor, p.LeadSource,
	}
}
