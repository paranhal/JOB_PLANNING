package repository

import (
	"database/sql"

	"customer-support/internal/model"
)

const (
	settingLocalNowSince = "95_133_local_now_since"
	settingUTCShiftDone  = "95_133_utc_shift_done"
)

// markLocalNowSince 이 변경 첫 기동 시각. 과거 UTC 보정 cutoff. 값은 한 번만 넣는다.
func markLocalNowSince(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at)
		VALUES (?,?,?)`, settingLocalNowSince, model.LocalNowString(), model.LocalNowString())
}
