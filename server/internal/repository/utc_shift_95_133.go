package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// UTCShiftTarget 보정 대상 표와 시각 열.
type UTCShiftTarget struct{ Table, Column string }

// UTCShiftTables 95-133 과거 UTC(CURRENT_TIMESTAMP) 저장 행을 현지(KST)로 옮길 표·열.
// 저장 코드가 LocalNowString 으로 바뀐 열만 넣는다. 쓰기가 계속 UTC 인 열(updated_at 등)이나
// 표시할 때 변환하는 열(FormatUTCDateTime)은 넣지 않는다 — 같은 열에 UTC·KST 가 섞이면 구분할 수 없다.
var UTCShiftTables = []UTCShiftTarget{
	{"work_activities", "created_at"},
	{"mail_outbox", "created_at"},
	{"sms_outbox", "created_at"},
	{"work_tasks", "created_at"},     // 95-133B: 업무 등록일(날짜)
	{"org_head_history", "acted_at"}, // 95-133B: 팀장 변경 이력
}

// UTCShiftTableReport 표 하나의 보정 대상.
type UTCShiftTableReport struct {
	Table       string
	Column      string
	Target      int    // cutoff 보다 이른 행 수(보정 대상)
	After       int    // cutoff 이후 행 수(그대로 둠)
	Oldest      string // 대상 중 가장 이른 created_at
	Newest      string // 대상 중 가장 늦은 created_at
	DateChanged int    // 대상 중 +9시간 하면 날짜(일자)가 바뀌는 행 수 — 일자별 통계가 달라진다
	Shifted     int    // 실제로 +9시간 한 행 수(미리보기면 0)
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

	for _, tg := range UTCShiftTables {
		table, col := tg.Table, tg.Column
		tr := UTCShiftTableReport{Table: table, Column: col}
		if err := db.QueryRow(`
			SELECT COUNT(*), COALESCE(MIN(`+col+`),''), COALESCE(MAX(`+col+`),''),
			       COALESCE(SUM(CASE WHEN date(`+col+`) != date(`+col+`,'+9 hours') THEN 1 ELSE 0 END),0)
			  FROM `+table+`
			 WHERE TRIM(COALESCE(`+col+`,'')) != '' AND datetime(`+col+`) < datetime(?)`, c).
			Scan(&tr.Target, &tr.Oldest, &tr.Newest, &tr.DateChanged); err != nil {
			return rep, fmt.Errorf("%s 집계: %w", table, err)
		}
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM `+table+`
			 WHERE TRIM(COALESCE(`+col+`,'')) != '' AND datetime(`+col+`) >= datetime(?)`, c).Scan(&tr.After); err != nil {
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
	for i, tg := range UTCShiftTables {
		res, err := tx.Exec(`
			UPDATE `+tg.Table+`
			   SET `+tg.Column+` = datetime(`+tg.Column+`, '+9 hours')
			 WHERE TRIM(COALESCE(`+tg.Column+`,'')) != '' AND datetime(`+tg.Column+`) < datetime(?)`, c)
		if err != nil {
			return rep, fmt.Errorf("%s 보정: %w", tg.Table, err)
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
