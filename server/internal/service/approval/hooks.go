package approval

import (
	"database/sql"
	"fmt"
)

type Hooks interface {
	Snapshot(tx *sql.Tx, docType, docID string) (title, snapshot string, err error)
	AfterApproved(tx *sql.Tx, req Request) error
}

// MemoryHooks 71-C 테스트용 가짜 문서.
type MemoryHooks struct {
	Title     string
	JSON      string
	FailAfter bool
	ApprovedN int
}

func (m *MemoryHooks) Snapshot(tx *sql.Tx, docType, docID string) (string, string, error) {
	title := m.Title
	if title == "" {
		title = docType + " " + docID
	}
	snap := m.JSON
	if snap == "" {
		snap = `{"doc_id":"` + docID + `"}`
	}
	return title, snap, nil
}

func (m *MemoryHooks) AfterApproved(tx *sql.Tx, req Request) error {
	if m.FailAfter {
		return fmt.Errorf("부작용 실패")
	}
	m.ApprovedN++
	return nil
}
