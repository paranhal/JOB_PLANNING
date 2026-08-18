package repository

import (
	"database/sql"
	"strings"
	"time"

	"customer-support/internal/model"
)

type AttachmentRepo struct{ db *sql.DB }

func NewAttachmentRepo(db *sql.DB) *AttachmentRepo { return &AttachmentRepo{db: db} }

const attachmentSelect = `
		SELECT attachment_id, ref_type, ref_id, file_name, file_path,
		       COALESCE(file_size,0), COALESCE(mime_type,''),
		       COALESCE(keywords,''), COALESCE(slot_no,0), uploaded_at`

func scanAttachment(scanner interface {
	Scan(dest ...any) error
}) (*model.Attachment, error) {
	var a model.Attachment
	var uploadedStr string
	if err := scanner.Scan(&a.AttachmentID, &a.RefType, &a.RefID,
		&a.FileName, &a.FilePath, &a.FileSize, &a.MIMEType,
		&a.Keywords, &a.SlotNo, &uploadedStr); err != nil {
		return nil, err
	}
	a.UploadedAt = parseTime(uploadedStr)
	return &a, nil
}

func (r *AttachmentRepo) ListByRef(refType, refID string) ([]model.Attachment, error) {
	rows, err := r.db.Query(attachmentSelect+`
		FROM attachments WHERE ref_type=? AND ref_id=?
		ORDER BY CASE WHEN slot_no>0 THEN slot_no ELSE 999 END ASC, uploaded_at ASC`,
		refType, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *a)
	}
	return items, rows.Err()
}

func (r *AttachmentRepo) CountByRef(refType, refID string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE ref_type=? AND ref_id=?`, refType, refID).Scan(&n)
	return n, err
}

// CountByRefIDs 여러 대상의 첨부 건수를 한 번에 센다.
func (r *AttachmentRepo) CountByRefIDs(refType string, ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(ids))
	ph = strings.TrimSuffix(ph, ",")
	args := make([]interface{}, 0, 1+len(ids))
	args = append(args, refType)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.Query(`SELECT ref_id, COUNT(*) FROM attachments WHERE ref_type=? AND ref_id IN (`+ph+`) GROUP BY ref_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// NextAssetImageSlot 사용 중인 슬롯(1~3)을 피해 다음 빈 번호 반환. 없으면 0.
func (r *AttachmentRepo) NextAssetImageSlot(assetID string) (int, error) {
	rows, err := r.db.Query(`SELECT COALESCE(slot_no,0) FROM attachments WHERE ref_type='asset' AND ref_id=?`, assetID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := map[int]bool{}
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			return 0, err
		}
		if s >= 1 && s <= 3 {
			used[s] = true
		}
	}
	for i := 1; i <= 3; i++ {
		if !used[i] {
			return i, nil
		}
	}
	return 0, nil
}

func (r *AttachmentRepo) Create(a *model.Attachment) error {
	a.AttachmentID = newID("ATT")
	_, err := r.db.Exec(`
		INSERT INTO attachments
		(attachment_id,ref_type,ref_id,file_name,file_path,file_size,mime_type,keywords,slot_no,uploaded_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.AttachmentID, a.RefType, a.RefID, a.FileName, a.FilePath,
		a.FileSize, a.MIMEType, a.Keywords, a.SlotNo,
		time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return err
	}
	logCreate(r.db, "attachments", "attachment_id", a.AttachmentID, a.FileName)
	return nil
}

func (r *AttachmentRepo) GetByID(id string) (*model.Attachment, error) {
	a, err := scanAttachment(r.db.QueryRow(attachmentSelect+` FROM attachments WHERE attachment_id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AttachmentRepo) UpdateKeywords(id, keywords string) error {
	return touchUpdate(r.db, "attachments", "attachment_id", id, "첨부", func() error {
		_, err := r.db.Exec(`UPDATE attachments SET keywords=? WHERE attachment_id=?`, keywords, id)
		return err
	})
}

func (r *AttachmentRepo) Delete(id string) error {
	return touchDelete(r.db, "attachments", "attachment_id", id, "첨부", func() error {
		_, err := r.db.Exec(`DELETE FROM attachments WHERE attachment_id=?`, id)
		return err
	})
}

// BackfillAssetImageSlots 기존 자산 이미지에 slot_no가 없으면 업로드순으로 1~3 부여
func BackfillAssetImageSlots(db *sql.DB) {
	rows, err := db.Query(`
		SELECT attachment_id, ref_id FROM attachments
		WHERE ref_type='asset' AND (slot_no IS NULL OR slot_no=0)
		ORDER BY ref_id, uploaded_at ASC, attachment_id ASC`)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct{ id, ref string }
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.ref) != nil {
			continue
		}
		list = append(list, r)
	}
	counters := map[string]int{}
	for _, r := range list {
		counters[r.ref]++
		slot := counters[r.ref]
		if slot > 3 {
			continue
		}
		db.Exec(`UPDATE attachments SET slot_no=? WHERE attachment_id=?`, slot, r.id)
	}
}
