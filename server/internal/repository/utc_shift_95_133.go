package repository

import (
	"database/sql"
	"fmt"
	"sort"
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

// UTCShiftJump 입력 순서(rowid)대로 본 시각 열에서 「+9시간쯤 앞으로 뛴」 지점. cutoff 후보다.
type UTCShiftJump struct {
	PrevID, PrevAt string // 뛰기 전 마지막 행
	NextID, NextAt string // 뛴 뒤 첫 행 — 이 시각이 cutoff 후보
	Gap            string // 두 시각 차이(예: 9h12m)
	Before, After  int    // 이 지점 앞·뒤 행 수
}

// DetectUTCShiftCutoff 저장 코드가 KST 로 바뀐 시점을 데이터에서 찾는 참고용 탐지(읽기 전용).
// work_activities 를 입력 순서(rowid)로 훑어, 이어지는 두 행의 created_at 이 8시간 30분~9시간 59분 앞으로 뛴 곳을 모은다.
// UTC 로 저장되던 때에서 KST 로 저장되기 시작하면 같은 순간이라도 숫자가 9시간 커지기 때문이다.
// 하지만 퇴근~출근처럼 자연스럽게 9시간쯤 비는 구간도 같아 보이므로 **후보일 뿐 확정이 아니다.**
// 확정은 배포 기록(운영 /version 의 started)이나 직접 만든 이력으로 한다.
func DetectUTCShiftCutoff(db *sql.DB, limit int) ([]UTCShiftJump, int, error) {
	if db == nil {
		return nil, 0, fmt.Errorf("db")
	}
	rows, err := db.Query(`SELECT activity_id, strftime('%Y-%m-%d %H:%M:%S', created_at) FROM work_activities
		WHERE TRIM(COALESCE(created_at,'')) != '' ORDER BY rowid`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type rec struct {
		id, at string
		t      time.Time
	}
	var all []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.at); err != nil {
			return nil, 0, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", r.at)
		if err != nil {
			continue
		}
		r.t = t
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var out []UTCShiftJump
	const lo, hi = 8*time.Hour + 30*time.Minute, 9*time.Hour + 59*time.Minute
	for i := 1; i < len(all); i++ {
		d := all[i].t.Sub(all[i-1].t)
		if d < lo || d > hi {
			continue
		}
		out = append(out, UTCShiftJump{
			PrevID: all[i-1].id, PrevAt: all[i-1].at, NextID: all[i].id, NextAt: all[i].at,
			Gap: fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60), Before: i, After: len(all) - i,
		})
	}
	// 앞뒤 행이 모두 많은 지점(양쪽 균형이 좋은 순)을 먼저 보여 준다.
	minSide := func(j UTCShiftJump) int {
		if j.After < j.Before {
			return j.After
		}
		return j.Before
	}
	sort.SliceStable(out, func(a, b int) bool { return minSide(out[a]) > minSide(out[b]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, len(all), nil
}
