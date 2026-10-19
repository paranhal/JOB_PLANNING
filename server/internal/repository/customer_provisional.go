package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applyCustomerProvisionalV74(db *sql.DB) {
	if db == nil {
		return
	}
	addNamedColumn(db, "customers", "is_provisional",
		`ALTER TABLE customers ADD COLUMN is_provisional INTEGER NOT NULL DEFAULT 0`)
	addNamedColumn(db, "customers", "provisional_by",
		`ALTER TABLE customers ADD COLUMN provisional_by TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "customers", "provisional_at",
		`ALTER TABLE customers ADD COLUMN provisional_at TEXT NOT NULL DEFAULT ''`)
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_customers_provisional ON customers(is_provisional)`); err != nil {
		log.Printf("061 provisional index: %v", err)
	}
}

func customerProvisionalMigrationFiles() (up, down string, err error) {
	candidates := []string{
		filepath.Join("..", "..", "migrations"),
		filepath.Join("migrations"),
	}
	var dir string
	for _, c := range candidates {
		if _, e := os.Stat(filepath.Join(c, "061_customer_provisional.sql")); e == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		return "", "", os.ErrNotExist
	}
	ub, err := os.ReadFile(filepath.Join(dir, "061_customer_provisional.sql"))
	if err != nil {
		return "", "", err
	}
	dbt, err := os.ReadFile(filepath.Join(dir, "061_customer_provisional.down.sql"))
	if err != nil {
		return "", "", err
	}
	return string(ub), string(dbt), nil
}

func (r *CustomerRepo) FindByNormalizedOrgName(orgID, name string) (*model.Customer, error) {
	key := model.NormalizeOrgLookupName(name)
	if key == "" {
		return nil, nil
	}
	q := customerAPISelect + ` WHERE 1=1`
	args := []interface{}{}
	if err := RequireOrgID(orgID); err != nil {
		return nil, err
	}
	if orgID != OrgAll {
		q += ` AND (org_id = ? OR TRIM(COALESCE(org_id,''))='')`
		args = append(args, orgID)
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var first *model.Customer
	for rows.Next() {
		c, err := scanCustomerAPI(rows)
		if err != nil {
			return nil, err
		}
		if !c.IsActive {
			continue
		}
		if model.NormalizeOrgLookupName(c.OrgName) == key || model.NormalizeOrgLookupName(c.OfficialName) == key {
			if !c.IsProvisional {
				return c, nil
			}
			if first == nil {
				first = c
			}
		}
	}
	return first, rows.Err()
}

// ResolveProvisional 직접입력 기관. 정규화 이름이 있으면 재사용, 없으면 임시 고객을 만든다.
// reusedOfficial 은 재사용한 고객이 정식(미확정이 아님)일 때 true.
func (r *CustomerRepo) ResolveProvisional(orgID, name, contact, phone, byUser string) (id string, reusedOfficial bool, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("기관명을 입력하세요")
	}
	if exist, err := r.FindByNormalizedOrgName(orgID, name); err != nil {
		return "", false, err
	} else if exist != nil {
		return exist.CustomerID, !exist.IsProvisional, nil
	}
	insertOrg := strings.TrimSpace(orgID)
	if insertOrg == "" || insertOrg == OrgAll {
		insertOrg = model.OrgIDLibrary
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	notes := "AS 접수 직접입력(미확정 기관)"
	if contact = strings.TrimSpace(contact); contact != "" {
		notes += " · 담당 " + contact
	}
	c := &model.Customer{
		OrgName:       name,
		OfficialName:  name,
		MainPhone:     strings.TrimSpace(phone),
		IsActive:      true,
		PartyKind:     model.PartyKindCustomer,
		OrgID:         insertOrg,
		IsProvisional: true,
		ProvisionalBy: strings.TrimSpace(byUser),
		ProvisionalAt: now,
		Notes:         notes,
	}
	if err := r.Create(c); err != nil {
		return "", false, err
	}
	if _, err := r.db.Exec(`UPDATE customers SET is_provisional=1, provisional_by=?, provisional_at=? WHERE customer_id=?`,
		c.ProvisionalBy, c.ProvisionalAt, c.CustomerID); err != nil {
		return "", false, err
	}
	return c.CustomerID, false, nil
}

func (r *CustomerRepo) PromoteProvisional(id, by string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("고객이 필요합니다")
	}
	cur, err := r.GetByID(id)
	if err != nil || cur == nil {
		return fmt.Errorf("고객을 찾을 수 없습니다")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	return touchUpdate(r.db, "customers", "customer_id", id, cur.OrgName+" · 정식 승격 ("+strings.TrimSpace(by)+")", func() error {
		_, err := r.db.Exec(`UPDATE customers SET is_provisional=0, updated_at=? WHERE customer_id=?`, now, id)
		return err
	})
}

func (r *CustomerRepo) MergeInto(fromID, toID, by string) error {
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if fromID == "" || toID == "" || fromID == toID {
		return fmt.Errorf("병합 대상이 올바르지 않습니다")
	}
	from, err := r.GetByID(fromID)
	if err != nil || from == nil {
		return fmt.Errorf("임시 고객을 찾을 수 없습니다")
	}
	if !from.IsProvisional {
		return fmt.Errorf("미확정 기관만 병합할 수 있습니다")
	}
	to, err := r.GetByID(toID)
	if err != nil || to == nil {
		return fmt.Errorf("대상 고객을 찾을 수 없습니다")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	moves := []string{
		`UPDATE as_receipts SET customer_id=? WHERE customer_id=?`,
		`UPDATE assets SET customer_id=? WHERE customer_id=?`,
		`UPDATE contacts SET customer_id=? WHERE customer_id=?`,
		`UPDATE maintenance_visits SET customer_id=? WHERE customer_id=?`,
		`UPDATE sales_projects SET customer_id=? WHERE customer_id=?`,
		`UPDATE work_tasks SET customer_id=? WHERE customer_id=?`,
	}
	for _, q := range moves {
		if _, err := r.db.Exec(q, toID, fromID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return err
		}
	}
	_, _ = r.db.Exec(`UPDATE maintenance_site_config SET customer_id=? WHERE customer_id=?`, toID, fromID)
	return touchUpdate(r.db, "customers", "customer_id", fromID, from.OrgName, func() error {
		_, err := r.db.Exec(`UPDATE customers SET is_active=0, is_provisional=0, notes=?, updated_at=? WHERE customer_id=?`,
			strings.TrimSpace(from.Notes+" · "+toID+" 로 병합 ("+by+")"), now, fromID)
		return err
	})
}

func (r *CustomerRepo) CountProvisional() int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE COALESCE(is_provisional,0)=1 AND COALESCE(is_active,1)=1`).Scan(&n)
	return n
}
