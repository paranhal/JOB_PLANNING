package repository

import (
	"database/sql"
	"fmt"
	"time"

	"customer-support/internal/model"
)

type ASRepo struct {
	db *sql.DB
}

func NewASRepo(db *sql.DB) *ASRepo {
	return &ASRepo{db: db}
}

// List AS 목록 조회
func (r *ASRepo) List(status, search string, page, pageSize int) ([]model.ASListItem, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       c.org_name, COALESCE(a.product_name,'') AS product_name,
		       ar.symptom, ar.urgency, ar.status, COALESCE(ar.assigned_to,''),
		       CAST(julianday('now') - julianday(ar.receipt_datetime) AS INTEGER) AS days_elapsed
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE 1=1`

	countQuery := `SELECT COUNT(*) FROM as_receipts ar JOIN customers c ON c.customer_id=ar.customer_id WHERE 1=1`
	args := []interface{}{}
	countArgs := []interface{}{}

	if status != "" {
		baseQuery += ` AND ar.status = ?`
		countQuery += ` AND ar.status = ?`
		args = append(args, status)
		countArgs = append(countArgs, status)
	}

	if search != "" {
		like := "%" + search + "%"
		baseQuery += ` AND (ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ?)`
		countQuery += ` AND (ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ?)`
		args = append(args, like, like, like)
		countArgs = append(countArgs, like, like, like)
	}

	baseQuery += ` ORDER BY ar.receipt_datetime DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	var total int
	if err := r.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.ASListItem
	for rows.Next() {
		var item model.ASListItem
		var receiptStr string
		if err := rows.Scan(
			&item.ASID, &item.ASNumber, &receiptStr,
			&item.OrgName, &item.ProductName,
			&item.Symptom, &item.Urgency, &item.Status, &item.AssignedTo,
			&item.DaysElapsed,
		); err != nil {
			return nil, 0, err
		}
		item.ReceiptDatetime, _ = time.Parse("2006-01-02 15:04:05", receiptStr)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// GetByID AS 단건 조회
func (r *ASRepo) GetByID(id string) (*model.ASReceipt, error) {
	query := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       ar.customer_id, COALESCE(ar.asset_id,''),
		       COALESCE(ar.receipt_channel,''), COALESCE(ar.requester,''),
		       COALESCE(ar.symptom,''), COALESCE(ar.urgency,'normal'),
		       COALESCE(ar.priority,'normal'), COALESCE(ar.requester_type,''),
		       COALESCE(ar.requester_name,''), COALESCE(ar.assigned_to,''),
		       ar.status, COALESCE(ar.process_type,''), COALESCE(ar.cause_type,''),
		       COALESCE(ar.action_taken,''), COALESCE(ar.parts_used,''),
		       ar.is_recurrence, COALESCE(ar.result_code,''),
		       COALESCE(ar.followup_action,''),
		       c.org_name, COALESCE(a.product_name,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.as_id = ?`

	var as model.ASReceipt
	var receiptStr string
	var isRecurrence int

	err := r.db.QueryRow(query, id).Scan(
		&as.ASID, &as.ASNumber, &receiptStr,
		&as.CustomerID, &as.AssetID,
		&as.ReceiptChannel, &as.Requester,
		&as.Symptom, &as.Urgency, &as.Priority,
		&as.RequesterType, &as.RequesterName, &as.AssignedTo,
		&as.Status, &as.ProcessType, &as.CauseType,
		&as.ActionTaken, &as.PartsUsed,
		&isRecurrence, &as.ResultCode, &as.FollowupAction,
		&as.OrgName, &as.ProductName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	as.ReceiptDatetime, _ = time.Parse("2006-01-02 15:04:05", receiptStr)
	as.IsRecurrence = isRecurrence == 1
	return &as, nil
}

// Create AS 접수 등록
func (r *ASRepo) Create(as *model.ASReceipt) error {
	as.ASID = newID("AS")
	as.ASNumber = generateASNumber()
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, asset_id,
			receipt_channel, requester, symptom, urgency, priority,
			requester_type, requester_name, assigned_to, status,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		as.ASID, as.ASNumber, now, as.CustomerID, nullStr(as.AssetID),
		as.ReceiptChannel, as.Requester, as.Symptom, as.Urgency, as.Priority,
		as.RequesterType, as.RequesterName, as.AssignedTo, "received",
		now, now,
	)
	return err
}

// Update AS 상태 및 처리 내용 수정
func (r *ASRepo) Update(as *model.ASReceipt) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		UPDATE as_receipts SET
			status=?, assigned_to=?, process_type=?, cause_type=?,
			action_taken=?, parts_used=?, result_code=?, followup_action=?,
			updated_at=?
		WHERE as_id=?`,
		as.Status, as.AssignedTo, as.ProcessType, as.CauseType,
		as.ActionTaken, as.PartsUsed, as.ResultCode, as.FollowupAction,
		now, as.ASID,
	)
	return err
}

// Stats AS 현황 통계 (빈 테이블에서 SUM이 NULL 반환 → COALESCE로 0 처리)
func (r *ASRepo) Stats() (*model.ASStats, error) {
	var stats model.ASStats
	err := r.db.QueryRow(`
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status IN ('received','in_progress') THEN 1 ELSE 0 END), 0) AS in_progress,
			COALESCE(SUM(CASE WHEN status IN ('completed','closed') THEN 1 ELSE 0 END), 0) AS completed,
			COALESCE(SUM(CASE WHEN status IN ('received','in_progress')
			         AND julianday('now')-julianday(receipt_datetime) > 3 THEN 1 ELSE 0 END), 0) AS overdue,
			COALESCE(SUM(CASE WHEN date(receipt_datetime)=date('now') THEN 1 ELSE 0 END), 0) AS today
		FROM as_receipts`,
	).Scan(
		&stats.TotalReceived, &stats.InProgress,
		&stats.Completed, &stats.Overdue, &stats.TodayReceived,
	)
	return &stats, err
}

// generateASNumber AS번호 생성 (AS-202600001 형식)
func generateASNumber() string {
	return fmt.Sprintf("AS-%d%05d",
		time.Now().Year(),
		time.Now().UnixNano()%100000,
	)
}
