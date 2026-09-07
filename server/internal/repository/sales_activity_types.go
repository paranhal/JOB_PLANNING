package repository

import (
	"database/sql"
	"log"
)

// applySalesActivityTypesV227 마이그레이션 032. §32.8.2 칩 6 + 드롭다운 5. 내부회의 추가.
func applySalesActivityTypesV227(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SAT12','sales_activity_type','internal','내부회의',5,1)`,
		`UPDATE codes SET code_name='방문', sort_order=1, is_active=1 WHERE code_id='SAT03'`,
		`UPDATE codes SET code_name='전화', sort_order=2, is_active=1 WHERE code_id='SAT02'`,
		`UPDATE codes SET code_name='이메일', sort_order=3, is_active=1 WHERE code_id='SAT05'`,
		`UPDATE codes SET code_name='온라인미팅', sort_order=4, is_active=1 WHERE code_id='SAT04'`,
		`UPDATE codes SET code_name='내부회의', sort_order=5, is_active=1 WHERE code_id='SAT12'`,
		`UPDATE codes SET code_name='정보수집', sort_order=6, is_active=1 WHERE code_id='SAT01'`,
		`UPDATE codes SET code_name='자료송부', sort_order=7, is_active=1 WHERE code_id='SAT06'`,
		`UPDATE codes SET code_name='견적제출', sort_order=8, is_active=1 WHERE code_id='SAT07'`,
		`UPDATE codes SET code_name='제안서제출', sort_order=9, is_active=1 WHERE code_id='SAT08'`,
		`UPDATE codes SET code_name='입찰', sort_order=10, is_active=1 WHERE code_id='SAT09'`,
		`UPDATE codes SET code_name='기타', sort_order=11, is_active=1 WHERE code_id='SAT10'`,
		`UPDATE codes SET is_active=1, sort_order=12 WHERE code_id='SAT11'`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("032 sales activity types: %v", err)
		}
	}
	applySalesActivitySearch(db)
}
