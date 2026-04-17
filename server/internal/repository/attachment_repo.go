package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type AttachmentRepo struct{ db *sql.DB }

func NewAttachmentRepo(db *sql.DB) *AttachmentRepo { return &AttachmentRepo{db: db} }

func (r *AttachmentRepo) ListByRef(refType, refID string) ([]model.Attachment, error) {
	rows, err := r.db.Query(`
		SELECT attachment_id, ref_type, ref_id, file_name, file_path,
		       COALESCE(file_size,0), COALESCE(mime_type,''), uploaded_at
		FROM attachments WHERE ref_type=? AND ref_id=? ORDER BY uploaded_at DESC`,
		refType, refID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Attachment
	for rows.Next() {
		var a model.Attachment
		var uploadedStr string
		if err := rows.Scan(&a.AttachmentID, &a.RefType, &a.RefID,
			&a.FileName, &a.FilePath, &a.FileSize, &a.MIMEType, &uploadedStr); err != nil {
			return nil, err
		}
		a.UploadedAt = parseTime(uploadedStr)
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *AttachmentRepo) Create(a *model.Attachment) error {
	a.AttachmentID = newID("ATT")
	_, err := r.db.Exec(`
		INSERT INTO attachments (attachment_id,ref_type,ref_id,file_name,file_path,file_size,mime_type,uploaded_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		a.AttachmentID, a.RefType, a.RefID, a.FileName, a.FilePath,
		a.FileSize, a.MIMEType, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

func (r *AttachmentRepo) GetByID(id string) (*model.Attachment, error) {
	var a model.Attachment
	var uploadedStr string
	err := r.db.QueryRow(`
		SELECT attachment_id, ref_type, ref_id, file_name, file_path,
		       COALESCE(file_size,0), COALESCE(mime_type,''), uploaded_at
		FROM attachments WHERE attachment_id=?`, id).
		Scan(&a.AttachmentID, &a.RefType, &a.RefID,
			&a.FileName, &a.FilePath, &a.FileSize, &a.MIMEType, &uploadedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.UploadedAt = parseTime(uploadedStr)
	return &a, nil
}

func (r *AttachmentRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM attachments WHERE attachment_id=?`, id)
	return err
}
