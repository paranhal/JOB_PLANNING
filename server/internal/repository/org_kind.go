package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

func applyOrgKind(db *sql.DB) {
	if db == nil {
		return
	}
	addNamedColumn(db, "orgs", "kind",
		`ALTER TABLE orgs ADD COLUMN kind TEXT DEFAULT 'business'`)
	if _, err := db.Exec(`UPDATE orgs SET kind=? WHERE TRIM(COALESCE(kind,''))=''`, model.OrgKindBusiness); err != nil {
		log.Printf("orgs.kind backfill: %v", err)
	}
}

func scanOrgKind(s string) string {
	return model.NormalizeOrgKind(strings.TrimSpace(s))
}
