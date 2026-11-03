package repository

import (
	"database/sql"
	"testing"

	"customer-support/internal/model"
)

func mustSalesForQuote(t *testing.T, db *sql.DB) string {
	t.Helper()
	p := &model.SalesProject{OrgID: model.OrgIDLibrary, Name: "견적 연결 사업"}
	if err := NewSalesRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	return p.SalesID
}
