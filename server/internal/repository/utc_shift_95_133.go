package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

// Apply95_133ShiftUTC 과거 UTC CURRENT_TIMESTAMP 행을 +9시간. 승인 대기 — InitDB에서 호출하지 않는다.
// 멱등 마커 settingUTCShiftDone. cutoff=95_133_local_now_since(이 변경 첫 기동) 이전만.
func Apply95_133ShiftUTC(db *sql.DB) (activities, mail, sms int, err error) {
	if db == nil {
		return 0, 0, 0, fmt.Errorf("db")
	}
	var done string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingUTCShiftDone).Scan(&done)
	if strings.TrimSpace(done) != "" {
		log.Printf("95-133 UTC→KST 보정: 이미 적용됨 (work_activities=0 mail_outbox=0 sms_outbox=0)")
		return 0, 0, 0, nil
	}
	var cutoff string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingLocalNowSince).Scan(&cutoff)
	cutoff = strings.TrimSpace(cutoff)
	if cutoff == "" {
		return 0, 0, 0, fmt.Errorf("cutoff(95_133_local_now_since) 없음")
	}
	shift := func(table string) (int, error) {
		res, err := db.Exec(`
			UPDATE `+table+`
			   SET created_at = datetime(created_at, '+9 hours')
			 WHERE TRIM(COALESCE(created_at,'')) != ''
			   AND datetime(created_at) < datetime(?)`, cutoff)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	if activities, err = shift("work_activities"); err != nil {
		return 0, 0, 0, err
	}
	if mail, err = shift("mail_outbox"); err != nil {
		return 0, 0, 0, err
	}
	if sms, err = shift("sms_outbox"); err != nil {
		return 0, 0, 0, err
	}
	now := model.LocalNowString()
	if _, err = db.Exec(`
		INSERT INTO app_settings(setting_key, setting_value, updated_at) VALUES (?,?,?)
		ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value, updated_at=excluded.updated_at`,
		settingUTCShiftDone, now, now); err != nil {
		return activities, mail, sms, err
	}
	log.Printf("95-133 UTC→KST 보정: work_activities=%d mail_outbox=%d sms_outbox=%d", activities, mail, sms)
	return activities, mail, sms, nil
}
