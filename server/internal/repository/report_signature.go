package repository

import (
	"database/sql"
	"log"
	"strconv"
	"strings"

	"customer-support/internal/model"
)

const reportSignatureV62MetaKey = "__meta:report_signature_v62"
const reportSignatureV62B2MetaKey = "__meta:report_signature_v62b2"
const reportSignaturePath2MetaKey = "__meta:report_signature_path2"

func applyReportSignatureV62(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at)
		VALUES
		  (?, ?, datetime('now','localtime')),
		  (?, ?, datetime('now','localtime')),
		  (?, ?, datetime('now','localtime'))`,
		model.SettingReportSignatureSize, strconv.Itoa(model.DefaultReportSignatureSize),
		model.SettingReportSignatureRightGap, strconv.Itoa(model.DefaultReportSignatureRightGap),
		model.SettingReportSignatureNudgeY, strconv.Itoa(model.DefaultReportSignatureNudgeY),
	); err != nil {
		log.Printf("report signature settings: %v", err)
	}
	if !metaDone(db, reportSignatureV62B2MetaKey) {
		if _, err := db.Exec(`
			UPDATE app_settings SET setting_value=?, updated_at=datetime('now','localtime')
			WHERE setting_key=? AND setting_value IN ('-300','-600')`,
			strconv.Itoa(model.DefaultReportSignatureNudgeY), model.SettingReportSignatureNudgeY); err != nil {
			log.Printf("report signature nudge_y: %v", err)
		}
		markMetaDone(db, reportSignatureV62B2MetaKey)
	}
	if metaDone(db, reportSignatureV62MetaKey) {
		return
	}
	addNamedColumn(db, "users", "signature_box",
		`ALTER TABLE users ADD COLUMN signature_box TEXT DEFAULT ''`)
	markMetaDone(db, reportSignatureV62MetaKey)
}

func applyReportSignaturePath2(db *sql.DB) {
	if db == nil {
		return
	}
	if metaDone(db, reportSignaturePath2MetaKey) {
		return
	}
	addNamedColumn(db, "users", "signature_path2",
		`ALTER TABLE users ADD COLUMN signature_path2 TEXT DEFAULT ''`)
	markMetaDone(db, reportSignaturePath2MetaKey)
}

func (r *SettingsRepo) ReportSignatureBox() model.ReportSignatureBox {
	out := model.DefaultReportSignatureBox()
	if r == nil || r.db == nil {
		return out
	}
	out.Size = settingInt(r.db, model.SettingReportSignatureSize, out.Size, true)
	out.Gap = settingInt(r.db, model.SettingReportSignatureRightGap, out.Gap, false)
	out.NY = settingInt(r.db, model.SettingReportSignatureNudgeY, out.NY, false)
	return out
}

func settingInt(db *sql.DB, key string, fallback int, positiveOnly bool) int {
	var v string
	if err := db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, key).Scan(&v); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	if positiveOnly && n <= 0 {
		return fallback
	}
	return n
}
