package repository

import (
	"database/sql"
	"log"
	"time"
)

// RunWorkListHousekeeping 완료 접수의 open 하부업무 정리·반복 발생 지연 표시.
// 조회 화면에서 돌리지 않는다. 서버 기동·일일 백업에서 부른다. §38.9.1
func RunWorkListHousekeeping(db *sql.DB) {
	if db == nil {
		return
	}
	n, err := NewASWorkRepo(db).CloseOpenUnderClosedReceipts()
	if err != nil {
		log.Printf("as_work_items 정리 실패: %v", err)
	} else if n > 0 {
		log.Printf("as_work_items 정리: %d건 완료 처리", n)
	}
	today := time.Now().Format("2006-01-02")
	n2, err := NewWBRepo(db).MarkPastOccurrencesOverdue(today)
	if err != nil {
		log.Printf("반복 발생 지연 표시 실패: %v", err)
	} else if n2 > 0 {
		log.Printf("반복 발생 지연 표시: %d건", n2)
	}
}
