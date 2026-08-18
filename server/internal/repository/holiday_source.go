package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

// applyHolidaySource 마이그레이션 008. source·synced_at 추가, 하드코딩 시드 1회 이관. §23.13.2·§23.13.3
func applyHolidaySource(db *sql.DB) {
	if db == nil {
		return
	}
	for _, col := range []struct{ name, typ string }{
		{"source", "TEXT"},
		{"synced_at", "DATETIME"},
	} {
		q := `ALTER TABLE holidays ADD COLUMN ` + col.name + ` ` + col.typ
		if _, err := db.Exec(q); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("008 holidays.%s: %v", col.name, err)
		}
	}

	if _, err := db.Exec(`
		UPDATE holidays SET source = 'manual'
		 WHERE kind = 'company' AND TRIM(COALESCE(source,'')) = ''`); err != nil {
		log.Printf("008 holidays source=manual: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE holidays SET source = 'api'
		 WHERE kind IN ('public','substitute') AND TRIM(COALESCE(source,'')) = ''`); err != nil {
		log.Printf("008 holidays source=api: %v", err)
	}

	// 이미 시드된 2026 오기재(2025 추석 복사)는 회사 휴무일이 아니면 지운다.
	for _, d := range wrong2026CopiedChuseok {
		if _, err := db.Exec(`
			DELETE FROM holidays
			 WHERE holiday_date = ?
			   AND COALESCE(source,'') != 'manual'
			   AND kind != ?`, d, model.HolidayKindCompany); err != nil {
			log.Printf("008 2026 오기재 삭제 %s: %v", d, err)
		}
	}

	repo := NewHolidayRepo(db)
	n, err := repo.insertIgnore(HolidaySeedItems())
	if err != nil {
		log.Printf("008 holidays 시드: %v", err)
	} else if n > 0 {
		log.Printf("008 holidays 시드: %d건 (2025~2027, 이후 API로 갱신)", n)
	}
	BumpHolidayData()
}

// ReapplyHolidaySource 테스트·재기동 시 008을 다시 적용한다.
func ReapplyHolidaySource(db *sql.DB) {
	applyHolidaySource(db)
}
