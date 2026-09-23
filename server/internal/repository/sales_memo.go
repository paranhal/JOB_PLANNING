package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *SalesRepo) ListMemos(salesID string) ([]model.SalesMemo, error) {
	salesID = strings.TrimSpace(salesID)
	if salesID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT memo_id, sales_id, content, sort_order, COALESCE(author_id,''), COALESCE(author_name,''), COALESCE(created_at,'')
		  FROM sales_memos WHERE sales_id=? ORDER BY sort_order, created_at, memo_id`, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesMemo
	for rows.Next() {
		var m model.SalesMemo
		if err := rows.Scan(&m.MemoID, &m.SalesID, &m.Content, &m.SortOrder, &m.AuthorID, &m.AuthorName, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SalesRepo) CreateMemo(m *model.SalesMemo) error {
	if m == nil {
		return fmt.Errorf("메모가 필요합니다")
	}
	m.SalesID = strings.TrimSpace(m.SalesID)
	m.Content = strings.TrimSpace(m.Content)
	if m.SalesID == "" || m.Content == "" {
		return fmt.Errorf("내용을 입력하세요")
	}
	id, err := NextSeq(r.db, "sales_memo")
	if err != nil {
		return err
	}
	m.MemoID = fmt.Sprintf("SM-%03d", id)
	var max int
	_ = r.db.QueryRow(`SELECT COALESCE(MAX(sort_order),0) FROM sales_memos WHERE sales_id=?`, m.SalesID).Scan(&max)
	m.SortOrder = max + 1
	_, err = r.db.Exec(`INSERT INTO sales_memos (memo_id, sales_id, content, sort_order, author_id, author_name)
		VALUES (?,?,?,?,?,?)`, m.MemoID, m.SalesID, m.Content, m.SortOrder, m.AuthorID, m.AuthorName)
	if err != nil {
		return err
	}
	logCreate(r.db, "sales_memos", "memo_id", m.MemoID, m.Content)
	return nil
}

func (r *SalesRepo) DeleteMemo(id, salesID string) error {
	id, salesID = strings.TrimSpace(id), strings.TrimSpace(salesID)
	if id == "" || salesID == "" {
		return fmt.Errorf("메모가 필요합니다")
	}
	return touchDelete(r.db, "sales_memos", "memo_id", id, id, func() error {
		_, err := r.db.Exec(`DELETE FROM sales_memos WHERE memo_id=? AND sales_id=?`, id, salesID)
		return err
	})
}
