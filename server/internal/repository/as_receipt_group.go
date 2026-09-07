package repository

import (
	"database/sql"
	"log"
	"strings"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

// applyASReceiptGroup 마이그레이션 025. 함께 접수한 건 묶음. §34.2.1
func applyASReceiptGroup(db *sql.DB) {
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN receipt_group_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("025 as_receipts.receipt_group_id: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_receipts_group ON as_receipts(receipt_group_id)`); err != nil {
		log.Printf("025 idx_as_receipts_group: %v", err)
	}
}

// NewReceiptGroupID 함께 접수한 건이 공유하는 묶음 ID. §34.2.1
func NewReceiptGroupID() string {
	return "RG-" + uuid.New().String()[:12]
}

// ListByReceiptGroup 같은 묶음의 접수. groupID 가 비면 nil.
func (r *ASRepo) ListByReceiptGroup(groupID string) ([]model.ASListItem, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       c.org_name, COALESCE(a.product_name,'') AS product_name,
		       ar.symptom, ar.urgency, ar.status, COALESCE(ar.assigned_to,''),
		       COALESCE(ar.visit_scheduled_date,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.receipt_group_id = ?
		ORDER BY ar.as_number ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASListItem
	for rows.Next() {
		var it model.ASListItem
		var receiptStr string
		if err := rows.Scan(&it.ASID, &it.ASNumber, &receiptStr,
			&it.OrgName, &it.ProductName, &it.Symptom, &it.Urgency, &it.Status,
			&it.AssignedTo, &it.VisitScheduledDate); err != nil {
			return nil, err
		}
		it.ReceiptDatetime = parseTime(receiptStr)
		it.GroupSize = 0 // 호출 쪽에서 len 을 쓴다
		out = append(out, it)
	}
	n := len(out)
	for i := range out {
		out[i].GroupSize = n
	}
	return out, rows.Err()
}
