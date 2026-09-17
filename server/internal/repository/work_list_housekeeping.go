package repository

import (
	"database/sql"
	"log"
	"sync"
	"time"
)

var housekeepingMu sync.Mutex

// RunWorkListHousekeeping 완료 접수의 open 하부업무 정리·반복 발생 지연 표시·AS↔일일업무 맞춤.
// 조회 화면에서 돌리지 않는다. 서버 기동·일일 백업에서 부른다. §38.9.1 · §42.4
// 기동과 당일 백업 보정이 겹쳐도 한 번에 하나만 돈다.
func RunWorkListHousekeeping(db *sql.DB) {
	if db == nil {
		return
	}
	housekeepingMu.Lock()
	defer housekeepingMu.Unlock()

	start := time.Now()
	log.Printf("일일업무 정리 시작")
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
	n3, err := ReconcileASDailyTasks(db)
	if err != nil {
		log.Printf("AS 일일업무 정리 실패: %v", err)
	} else if n3 > 0 {
		log.Printf("AS 일일업무 정리: %d건", n3)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
		log.Printf("WAL checkpoint: %v", err)
	}
	log.Printf("일일업무 정리 끝 (%s)", time.Since(start).Round(time.Millisecond))
}

// StartWALAutocheckpoint WAL 이 무한히 자라지 않게 주기적으로 합친다. §44.5 ②
// 운영 중에는 PASSIVE 만 쓴다. RESTART/TRUNCATE 는 독자를 막아 화면이 멈춘다.
func StartWALAutocheckpoint(db *sql.DB) {
	if db == nil {
		return
	}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			var busy, logFrames, ckpt int
			row := db.QueryRow(`PRAGMA wal_checkpoint(PASSIVE)`)
			if err := row.Scan(&busy, &logFrames, &ckpt); err != nil {
				continue
			}
			if logFrames > 4000 { // 약 16MB 넘게 밀렸다 = 되감기가 안 되고 있다
				log.Printf("WAL 경고: %d 프레임 밀림(합친 프레임 %d). 커넥션 설정을 확인하라", logFrames, ckpt)
			}
		}
	}()
}
