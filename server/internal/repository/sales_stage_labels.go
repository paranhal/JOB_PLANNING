package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applySalesStageLabels 마이그레이션 053. 단계 표시 이름만 바꾼다. §46.2
func applySalesStageLabels(db *sql.DB) {
	if db == nil {
		return
	}
	if metaDone(db, sales4StageMetaKey) {
		return
	}
	for _, q := range []string{
		`UPDATE codes SET code_name='발굴' WHERE code_id='SST02' AND code_value='contact'`,
		`UPDATE codes SET code_name='검토' WHERE code_id='SST01' AND code_value='lead'`,
		`UPDATE codes SET code_name='견적' WHERE code_id='SST03' AND code_value='proposal'`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("053 sales stage labels: %v", err)
		}
	}
}
