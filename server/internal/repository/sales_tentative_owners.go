package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"customer-support/internal/model"
)

func applySalesTentativeOwnersV101(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := salesTentativeOwnersMigrationSQL()
	if err != nil {
		log.Printf("101 sales tentative owners: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("101 sales tentative owners apply: %v", err)
	}
}

func salesTentativeOwnersMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("101_sales_tentative_owners.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("101_sales_tentative_owners.sql 없음")
}

func (r *SalesRepo) ListOwnerIDs(salesID string) ([]string, error) {
	salesID = strings.TrimSpace(salesID)
	if r == nil || r.db == nil || salesID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT user_id FROM sales_project_owners
		 WHERE sales_id=?
		 ORDER BY is_primary DESC, user_id`, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (r *SalesRepo) attachOwners(items []model.SalesProject) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	idx := map[string][]int{}
	for i := range items {
		id := strings.TrimSpace(items[i].SalesID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
		idx[id] = append(idx[id], i)
	}
	if len(ids) == 0 {
		return
	}
	args := make([]any, len(ids))
	ph := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id
		ph[i] = "?"
	}
	q := `SELECT sales_id, user_id, COALESCE(is_primary,0)
		FROM sales_project_owners
		WHERE sales_id IN (` + strings.Join(ph, ",") + `)
		ORDER BY is_primary DESC, user_id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sid, uid string
		var primary int
		if err := rows.Scan(&sid, &uid, &primary); err != nil {
			return
		}
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		for _, i := range idx[sid] {
			items[i].SalesOwnerIDs = append(items[i].SalesOwnerIDs, uid)
		}
	}
}

func (r *SalesRepo) ReplaceSalesOwners(salesID, primaryID, createdBy string, userIDs []string) error {
	salesID = strings.TrimSpace(salesID)
	if r == nil || r.db == nil || salesID == "" {
		return nil
	}
	primaryID = strings.TrimSpace(primaryID)
	seen := map[string]struct{}{}
	var ids []string
	for _, raw := range userIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if primaryID != "" {
		if _, ok := seen[primaryID]; !ok {
			ids = append([]string{primaryID}, ids...)
			seen[primaryID] = struct{}{}
		}
	} else if len(ids) > 0 {
		primaryID = ids[0]
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sales_project_owners WHERE sales_id=?`, salesID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for _, id := range ids {
		isP := 0
		if id == primaryID {
			isP = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO sales_project_owners (sales_id, user_id, is_primary)
			VALUES (?,?,?)`, salesID, id, isP); err != nil {
			return err
		}
	}
	_ = createdBy
	return tx.Commit()
}

func (r *SalesRepo) HasSalesOwner(salesID, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	ids, _ := r.ListOwnerIDs(salesID)
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}
