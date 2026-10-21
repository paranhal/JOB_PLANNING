package repository

import (
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

type ASPartsUsedRepo struct{ db *sql.DB }

func NewASPartsUsedRepo(db *sql.DB) *ASPartsUsedRepo {
	return &ASPartsUsedRepo{db: db}
}

func (r *ASPartsUsedRepo) ListByAS(asID string) ([]model.ASPartUsed, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT part_id, as_id, COALESCE(process_id,''), item_id, part_name,
		       COALESCE(part_code,''), qty, sort_order,
		       COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM as_parts_used
		WHERE as_id=?
		ORDER BY sort_order ASC, part_id ASC`, asID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ASPartUsed
	for rows.Next() {
		var p model.ASPartUsed
		var itemID sql.NullString
		if err := rows.Scan(&p.PartID, &p.ASID, &p.ProcessID, &itemID, &p.PartName,
			&p.PartCode, &p.Qty, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if itemID.Valid {
			p.ItemID = itemID.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ASPartsUsedRepo) DeleteByAS(asID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.Exec(`DELETE FROM as_parts_used WHERE as_id=?`, strings.TrimSpace(asID))
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

func (r *ASPartsUsedRepo) ReplaceByAS(asID, processID string, parts []model.ASPartUsed) error {
	if r == nil || r.db == nil {
		return nil
	}
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return nil
	}
	if err := r.DeleteByAS(asID); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for i, p := range parts {
		name := strings.TrimSpace(p.PartName)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(p.PartID)
		if id == "" {
			id = "AP-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
		}
		qty := p.Qty
		if qty < 1 {
			qty = 1
		}
		var item any
		if s := strings.TrimSpace(p.ItemID); s != "" {
			item = s
		} else {
			item = nil
		}
		_, err := r.db.Exec(`
			INSERT INTO as_parts_used
			(part_id, as_id, process_id, item_id, part_name, part_code, qty, sort_order, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, asID, strings.TrimSpace(processID), item, name, strings.TrimSpace(p.PartCode),
			qty, i, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SalesItemRepo) SuggestParts(orgID, q string, limit int) ([]model.SalesItem, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 2 {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	orgID = strings.TrimSpace(orgID)
	pat := likeContains(q)
	sqlQ := salesItemSelect + `
		WHERE COALESCE(i.is_active,1)=1
		  AND (i.name LIKE ? ESCAPE '\'
		    OR COALESCE(i.gov_item_no,'') LIKE ? ESCAPE '\'
		    OR COALESCE(i.model,'') LIKE ? ESCAPE '\'
		    OR COALESCE(i.spec,'') LIKE ? ESCAPE '\')`
	args := []interface{}{pat, pat, pat, pat}
	if orgID != "" && orgID != OrgAll {
		sqlQ += ` AND (COALESCE(i.org_id,'')='' OR i.org_id=?)`
		args = append(args, orgID)
	}
	sqlQ += ` ORDER BY i.name COLLATE NOCASE, i.item_id LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(sqlQ, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanSalesItems(rows)
}
