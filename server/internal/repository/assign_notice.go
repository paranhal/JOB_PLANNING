package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applyWorkAssignNotices(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS work_assign_notices (
			notice_id    TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL,
			source_type  TEXT NOT NULL,
			source_id    TEXT NOT NULL,
			assigned_by  TEXT NOT NULL DEFAULT '',
			assigned_at  TEXT NOT NULL,
			seen_at      TEXT,
			acted_at     TEXT
		)`); err != nil {
		log.Printf("047 work_assign_notices: %v", err)
		return
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_assign_notices_unseen ON work_assign_notices(user_id, seen_at)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_assign_notices_source ON work_assign_notices(source_type, source_id)`)
}

type AssignNoticeRepo struct{ db *sql.DB }

func NewAssignNoticeRepo(db *sql.DB) *AssignNoticeRepo { return &AssignNoticeRepo{db: db} }

// RecordAssignment 담당자가 정해진 자리에서 알림을 만든다. §42.5
// 자기 배정은 만들지 않는다. 담당자가 바뀌면 옛 사람의 안 본 알림만 지운다.
func (r *AssignNoticeRepo) RecordAssignment(sourceType, sourceID, newUserID, assignedBy string) error {
	if r == nil || r.db == nil {
		return nil
	}
	sourceType = strings.TrimSpace(sourceType)
	sourceID = strings.TrimSpace(sourceID)
	newUserID = strings.TrimSpace(newUserID)
	assignedBy = strings.TrimSpace(assignedBy)
	if sourceType == "" || sourceID == "" {
		return nil
	}
	if newUserID == "" {
		_, err := r.db.Exec(`
			DELETE FROM work_assign_notices
			 WHERE source_type=? AND source_id=? AND seen_at IS NULL`, sourceType, sourceID)
		return ignoreNoTable(err)
	}
	if _, err := r.db.Exec(`
		DELETE FROM work_assign_notices
		 WHERE source_type=? AND source_id=? AND seen_at IS NULL AND user_id != ?`,
		sourceType, sourceID, newUserID); err != nil {
		return ignoreNoTable(err)
	}
	if newUserID == assignedBy {
		return nil
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_assign_notices
		 WHERE source_type=? AND source_id=? AND user_id=? AND seen_at IS NULL`,
		sourceType, sourceID, newUserID).Scan(&n)
	if err != nil {
		return ignoreNoTable(err)
	}
	if n > 0 {
		return nil
	}
	id, err := r.nextID()
	if err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`
		INSERT INTO work_assign_notices (notice_id, user_id, source_type, source_id, assigned_by, assigned_at)
		VALUES (?,?,?,?,?,?)`, id, newUserID, sourceType, sourceID, assignedBy, now)
	return ignoreNoTable(err)
}

func (r *AssignNoticeRepo) nextID() (string, error) {
	n, err := NextSeq(r.db, "work_assign_notice")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("WAN-%03d", n), nil
}

func (r *AssignNoticeRepo) CountUnseen(userID string) (int, error) {
	if r == nil || r.db == nil || strings.TrimSpace(userID) == "" {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_assign_notices
		 WHERE user_id=? AND seen_at IS NULL`, strings.TrimSpace(userID)).Scan(&n)
	if err != nil {
		return 0, ignoreNoTable(err)
	}
	return n, nil
}

func (r *AssignNoticeRepo) ListUnseen(userID string) ([]model.AssignNotice, error) {
	if r == nil || r.db == nil || strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT n.notice_id, n.user_id, n.source_type, n.source_id,
		       COALESCE(n.assigned_by,''), n.assigned_at,
		       COALESCE(n.seen_at,''), COALESCE(n.acted_at,''),
		       CASE n.source_type
		         WHEN 'as' THEN COALESCE(NULLIF(TRIM(ar.as_number),''), n.source_id)
		         WHEN 'maintenance' THEN COALESCE(NULLIF(TRIM(mv.visit_id),''), n.source_id)
		         ELSE COALESCE(NULLIF(TRIM(wt.task_id),''), n.source_id)
		       END,
		       CASE n.source_type
		         WHEN 'as' THEN COALESCE(c1.org_name,'')
		         WHEN 'maintenance' THEN COALESCE(c2.org_name,'')
		         ELSE COALESCE(NULLIF(TRIM(c3.org_name),''), NULLIF(TRIM(wt.customer_name),''), '')
		       END,
		       CASE n.source_type
		         WHEN 'as' THEN COALESCE(ar.symptom,'')
		         WHEN 'maintenance' THEN COALESCE(NULLIF(TRIM(mv.product_type),''), '정기점검')
		         ELSE COALESCE(wt.title,'')
		       END
		  FROM work_assign_notices n
		  LEFT JOIN as_receipts ar ON n.source_type='as' AND ar.as_id=n.source_id
		  LEFT JOIN customers c1 ON c1.customer_id=ar.customer_id
		  LEFT JOIN maintenance_visits mv ON n.source_type='maintenance' AND mv.visit_id=n.source_id
		  LEFT JOIN customers c2 ON c2.customer_id=mv.customer_id
		  LEFT JOIN work_tasks wt ON n.source_type='task' AND wt.task_id=n.source_id
		  LEFT JOIN customers c3 ON c3.customer_id=wt.customer_id
		 WHERE n.user_id=? AND n.seen_at IS NULL
		 ORDER BY n.assigned_at, n.notice_id`, strings.TrimSpace(userID))
	if err != nil {
		return nil, ignoreNoTable(err)
	}
	defer rows.Close()
	var out []model.AssignNotice
	for rows.Next() {
		var n model.AssignNotice
		if err := rows.Scan(&n.NoticeID, &n.UserID, &n.SourceType, &n.SourceID,
			&n.AssignedBy, &n.AssignedAt, &n.SeenAt, &n.ActedAt,
			&n.Number, &n.OrgName, &n.Title); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *AssignNoticeRepo) GetOwned(noticeID, userID string) (*model.AssignNotice, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var n model.AssignNotice
	err := r.db.QueryRow(`
		SELECT notice_id, user_id, source_type, source_id,
		       COALESCE(assigned_by,''), assigned_at,
		       COALESCE(seen_at,''), COALESCE(acted_at,'')
		  FROM work_assign_notices WHERE notice_id=? AND user_id=?`,
		strings.TrimSpace(noticeID), strings.TrimSpace(userID)).
		Scan(&n.NoticeID, &n.UserID, &n.SourceType, &n.SourceID,
			&n.AssignedBy, &n.AssignedAt, &n.SeenAt, &n.ActedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, ignoreNoTable(err)
	}
	return &n, nil
}

func (r *AssignNoticeRepo) MarkSeenAndActed(noticeIDs []string, userID string) error {
	if r == nil || r.db == nil || len(noticeIDs) == 0 || strings.TrimSpace(userID) == "" {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, id := range noticeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := r.db.Exec(`
			UPDATE work_assign_notices
			   SET seen_at=CASE WHEN seen_at IS NULL THEN ? ELSE seen_at END,
			       acted_at=?
			 WHERE notice_id=? AND user_id=?`, now, now, id, userID); err != nil {
			return ignoreNoTable(err)
		}
	}
	return nil
}

func ignoreNoTable(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}
