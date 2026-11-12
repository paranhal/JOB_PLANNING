package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// UTCShiftTables 95-133 과거 UTC(CURRENT_TIMESTAMP) 저장 행을 현지(KST)로 옮길 표.
// 화면에 날짜+시각으로 나가고, 저장 코드가 LocalNowString 으로 바뀐 표만 넣는다.
var UTCShiftTables = []string{"work_activities", "mail_outbox", "sms_outbox"}

// UTCShiftTableReport 표 하나의 보정 대상.
type UTCShiftTableReport struct {
	Table   string
	Target  int    // cutoff 보다 이른 행 수(보정 대상)
	After   int    // cutoff 이후 행 수(그대로 둠)
	Oldest  string // 대상 중 가장 이른 created_at
	Newest  string // 대상 중 가장 늦은 created_at
	Shifted int    // 실제로 +9시간 한 행 수(미리보기면 0)
}

// UTCShiftReport 미리보기·반영 결과.
type UTCShiftReport struct {
	Cutoff       string
	FirstBootRef string // app_settings.95_133_local_now_since (참고용. cutoff 로 쓰지 않는다)
	Applied      bool
	Tables       []UTCShiftTableReport
}

const utcShiftCutoffLayout = "2006-01-02 15:04:05"

// ParseUTCShiftCutoff cutoff 문자열을 검증해 표준 형식으로 돌려준다.
func ParseUTCShiftCutoff(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{utcShiftCutoffLayout, "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, model.KST()); err == nil {
			return t.Format(utcShiftCutoffLayout), nil
		}
	}
	return "", fmt.Errorf("cutoff 형식은 'YYYY-MM-DD HH:MM[:SS]' (한국 시각) 입니다: %q", s)
}

// ShiftUTCToKST 과거에 UTC 로 저장된 created_at 에 +9시간을 한다. 95-133.
//
// cutoff 는 「저장 코드가 현지 시각(LocalNowString)으로 바뀐 빌드가 처음 기동한 시각」(한국 시각)이다.
// UTC 로 저장된 행의 숫자는 그 시각보다 최소 9시간 이르고, 현지로 저장된 행은 그 시각 이후이므로
// created_at < cutoff 이면 UTC, 이상이면 현지로 가를 수 있다.
// 이 값은 운영에서 그 빌드를 올린 시각을 사람이 정해 넣는다. 코드가 추정하지 않는다
// (app_settings.95_133_local_now_since 는 95-161A 부터 기록돼 실제 저장 변경 반영 시각과 다를 수 있다).
//
// apply=false 이면 아무것도 쓰지 않고 건수만 센다. apply=true 이면 한 트랜잭션으로 +9시간하고
// 마커(95_133_utc_shift_done)를 남긴다. 마커가 있으면 두 번째 반영을 거절한다(이중 +9 방지).
func ShiftUTCToKST(db *sql.DB, cutoff string, apply bool) (UTCShiftReport, error) {
	var rep UTCShiftReport
	if db == nil {
		return rep, fmt.Errorf("db")
	}
	c, err := ParseUTCShiftCutoff(cutoff)
	if err != nil {
		return rep, err
	}
	rep.Cutoff = c
	if now := model.LocalNowString(); c > now {
		return rep, fmt.Errorf("cutoff %s 가 지금(%s)보다 미래입니다", c, now)
	}
	_ = db.QueryRow(`SELECT COALESCE(setting_value,'') FROM app_settings WHERE setting_key=?`, settingLocalNowSince).Scan(&rep.FirstBootRef)

	var done string
	_ = db.QueryRow(`SELECT COALESCE(setting_value,'') FROM app_settings WHERE setting_key=?`, settingUTCShiftDone).Scan(&done)
	if apply && strings.TrimSpace(done) != "" {
		return rep, fmt.Errorf("이미 반영됨(%s). 두 번 +9시간 하지 않는다. 되돌리려면 백업을 복원한다", done)
	}

	for _, table := range UTCShiftTables {
		tr := UTCShiftTableReport{Table: table}
		if err := db.QueryRow(`
			SELECT COUNT(*), COALESCE(MIN(created_at),''), COALESCE(MAX(created_at),'')
			  FROM `+table+`
			 WHERE TRIM(COALESCE(created_at,'')) != '' AND datetime(created_at) < datetime(?)`, c).
			Scan(&tr.Target, &tr.Oldest, &tr.Newest); err != nil {
			return rep, fmt.Errorf("%s 집계: %w", table, err)
		}
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM `+table+`
			 WHERE TRIM(COALESCE(created_at,'')) != '' AND datetime(created_at) >= datetime(?)`, c).Scan(&tr.After); err != nil {
			return rep, fmt.Errorf("%s 집계: %w", table, err)
		}
		rep.Tables = append(rep.Tables, tr)
	}
	if !apply {
		return rep, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return rep, err
	}
	defer tx.Rollback()
	for i, table := range UTCShiftTables {
		res, err := tx.Exec(`
			UPDATE `+table+`
			   SET created_at = datetime(created_at, '+9 hours')
			 WHERE TRIM(COALESCE(created_at,'')) != '' AND datetime(created_at) < datetime(?)`, c)
		if err != nil {
			return rep, fmt.Errorf("%s 보정: %w", table, err)
		}
		n, _ := res.RowsAffected()
		rep.Tables[i].Shifted = int(n)
	}
	now := model.LocalNowString()
	if _, err := tx.Exec(`
		INSERT INTO app_settings(setting_key, setting_value, updated_at) VALUES (?,?,?)
		ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value, updated_at=excluded.updated_at`,
		settingUTCShiftDone, "cutoff="+c+" applied="+now, now); err != nil {
		return rep, err
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	rep.Applied = true
	return rep, nil
}
