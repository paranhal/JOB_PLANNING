package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"customer-support/internal/model"
)

var legacyCauseOrgID = model.LegacyCauseOrgID

func applyASCauseCategoryOrgsV102(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := asCauseCategoryOrgsMigrationSQL()
	if err != nil {
		log.Printf("102 as_cause_category_orgs: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("102 as_cause_category_orgs apply: %v", err)
	}
}

func asCauseCategoryOrgsMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("102_as_cause_category_orgs.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("102_as_cause_category_orgs.sql 없음")
}

func (r *ASRepo) orgCauseTableHasRows(orgID string) bool {
	if r == nil || r.db == nil {
		return false
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return false
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM as_cause_category_orgs WHERE org_id=?`, orgID).Scan(&n)
	return err == nil && n > 0
}

func (r *ASRepo) listLegacyCauseCategories(activeOnly bool) ([]model.CauseCategory, error) {
	q := `
		SELECT code, level, COALESCE(parent_code,''), label, COALESCE(sort_order,0), COALESCE(is_active,1),
		       COALESCE(cause_type_map,''), COALESCE(is_fault,1)
		  FROM as_cause_categories`
	if activeOnly {
		q += ` WHERE COALESCE(is_active,1)=1`
	}
	q += ` ORDER BY level, sort_order, code`
	return r.scanCauseCategories(q)
}

func (r *ASRepo) listOrgCauseCategories(orgID string, activeOnly bool) ([]model.CauseCategory, error) {
	q := `
		SELECT code, level, COALESCE(parent_code,''), label, COALESCE(sort_order,0), COALESCE(is_active,1),
		       COALESCE(cause_type_map,''), COALESCE(is_fault,1)
		  FROM as_cause_category_orgs
		 WHERE org_id=?`
	args := []interface{}{orgID}
	if activeOnly {
		q += ` AND COALESCE(is_active,1)=1`
	}
	q += ` ORDER BY level, sort_order, code`
	return r.scanCauseCategories(q, args...)
}

func (r *ASRepo) scanCauseCategories(q string, args ...interface{}) ([]model.CauseCategory, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CauseCategory
	for rows.Next() {
		var c model.CauseCategory
		var active, fault int
		if err := rows.Scan(&c.Code, &c.Level, &c.ParentCode, &c.Label, &c.SortOrder, &active, &c.CauseTypeMap, &fault); err != nil {
			return nil, err
		}
		c.IsActive = active == 1
		c.IsFault = fault == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListCauseCategoriesForOrg 조치 화면용(활성). 조직 표 → 도서관사업팀 기존 표 → 빈 표.
func (r *ASRepo) ListCauseCategoriesForOrg(orgID string) ([]model.CauseCategory, error) {
	return r.listCauseCategoriesForOrg(orgID, true)
}

// ListCauseCategoriesForOrgAll 라벨 표시용. 비활성 포함.
func (r *ASRepo) ListCauseCategoriesForOrgAll(orgID string) ([]model.CauseCategory, error) {
	return r.listCauseCategoriesForOrg(orgID, false)
}

func (r *ASRepo) listCauseCategoriesForOrg(orgID string, activeOnly bool) ([]model.CauseCategory, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	orgID = strings.TrimSpace(orgID)
	if orgID != "" && orgID != OrgAll && r.orgCauseTableHasRows(orgID) {
		return r.listOrgCauseCategories(orgID, activeOnly)
	}
	if orgID == OrgAll {
		orgRows, err := r.listAllOrgCauseCategories(activeOnly)
		if err != nil {
			return nil, err
		}
		legacy, err := r.listLegacyCauseCategories(activeOnly)
		if err != nil {
			return nil, err
		}
		return append(orgRows, legacy...), nil
	}
	if orgID == "" || orgID == legacyCauseOrgID {
		return r.listLegacyCauseCategories(activeOnly)
	}
	return nil, nil
}

func (r *ASRepo) listAllOrgCauseCategories(activeOnly bool) ([]model.CauseCategory, error) {
	q := `
		SELECT code, level, COALESCE(parent_code,''), label, COALESCE(sort_order,0), COALESCE(is_active,1),
		       COALESCE(cause_type_map,''), COALESCE(is_fault,1)
		  FROM as_cause_category_orgs`
	if activeOnly {
		q += ` WHERE COALESCE(is_active,1)=1`
	}
	q += ` ORDER BY org_id, level, sort_order, code`
	return r.scanCauseCategories(q)
}

func (r *ASRepo) upsertCauseCategoryOrgTx(tx *sql.Tx, orgID, userID string, row model.CauseCatImportRow) error {
	active, fault := 0, 0
	if row.IsActive {
		active = 1
	}
	if row.IsFault {
		fault = 1
	}
	_, err := tx.Exec(`
		INSERT INTO as_cause_category_orgs
			(org_id, code, level, parent_code, label, sort_order, is_active, cause_type_map, is_fault, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,datetime('now','localtime'),datetime('now','localtime'))
		ON CONFLICT(org_id, code) DO UPDATE SET
			level=excluded.level,
			parent_code=excluded.parent_code,
			label=excluded.label,
			sort_order=excluded.sort_order,
			is_active=excluded.is_active,
			cause_type_map=excluded.cause_type_map,
			is_fault=excluded.is_fault,
			updated_at=datetime('now','localtime')`,
		orgID, row.Code, row.Level, row.ParentCode, row.Label, row.SortOrder, active, row.CauseTypeMap, fault, userID)
	return err
}

func (r *ASRepo) getCauseCategoryOrg(orgID, code string) (*model.CauseCategory, error) {
	rows, err := r.scanCauseCategories(`
		SELECT code, level, COALESCE(parent_code,''), label, COALESCE(sort_order,0), COALESCE(is_active,1),
		       COALESCE(cause_type_map,''), COALESCE(is_fault,1)
		  FROM as_cause_category_orgs
		 WHERE org_id=? AND code=?`, orgID, code)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	c := rows[0]
	return &c, nil
}
