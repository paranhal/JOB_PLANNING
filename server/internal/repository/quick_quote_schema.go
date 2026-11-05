package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyQuickQuoteV106(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := quickQuoteSQL()
	if err != nil {
		log.Printf("106 quick_quote: %v", err)
	} else if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("106 quick_quote apply: %v", err)
	}
	addNamedColumn(db, "sales_quotes", "attn_phone", `ALTER TABLE sales_quotes ADD COLUMN attn_phone TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_quotes", "attn_email", `ALTER TABLE sales_quotes ADD COLUMN attn_email TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_quotes", "downloaded_at", `ALTER TABLE sales_quotes ADD COLUMN downloaded_at TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_quotes", "sent_at", `ALTER TABLE sales_quotes ADD COLUMN sent_at TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_quotes", "cut_scope", `ALTER TABLE sales_quotes ADD COLUMN cut_scope TEXT NOT NULL DEFAULT 'total'`)
	addNamedColumn(db, "sales_projects", "created_via", `ALTER TABLE sales_projects ADD COLUMN created_via TEXT NOT NULL DEFAULT ''`)
}

func quickQuoteSQL() (string, error) {
	for _, p := range workGroupSQLPaths("106_quick_quote.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("106_quick_quote.sql 없음")
}
