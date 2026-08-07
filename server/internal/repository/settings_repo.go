package repository

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

const SettingASCompletedEditPassword = "as_completed_edit_password_hash"

// DefaultASCompletedEditPassword 완료·종료 AS 수정 잠금 해제 기본 비밀번호 (최초 시드).
// 관리자는 /users 화면에서 변경할 수 있다.
const DefaultASCompletedEditPassword = "as-edit"

type SettingsRepo struct {
	db *sql.DB
}

func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

func (r *SettingsRepo) Get(key string) (string, error) {
	var v string
	err := r.db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (r *SettingsRepo) Set(key, value string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)
		ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value, updated_at=excluded.updated_at`,
		key, value, now)
	return err
}

// EnsureASCompletedEditPassword 해시가 없으면 기본 비밀번호로 시드한다.
func (r *SettingsRepo) EnsureASCompletedEditPassword(hashFn func(string) string) error {
	cur, err := r.Get(SettingASCompletedEditPassword)
	if err != nil {
		return err
	}
	if cur != "" {
		return nil
	}
	return r.Set(SettingASCompletedEditPassword, hashFn(DefaultASCompletedEditPassword))
}

type ASUnlockRepo struct {
	db *sql.DB
}

func NewASUnlockRepo(db *sql.DB) *ASUnlockRepo {
	return &ASUnlockRepo{db: db}
}

const asEditUnlockTTL = 30 * time.Minute

func (r *ASUnlockRepo) HasActive(asID, userID string) (bool, time.Time, error) {
	var expStr string
	err := r.db.QueryRow(`
		SELECT expires_at FROM as_edit_unlocks
		WHERE as_id=? AND user_id=? AND expires_at > ?
		ORDER BY expires_at DESC LIMIT 1`,
		asID, userID, time.Now().Format(time.RFC3339)).Scan(&expStr)
	if err == sql.ErrNoRows {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	exp, _ := time.Parse(time.RFC3339, expStr)
	return true, exp, nil
}

func (r *ASUnlockRepo) Grant(asID, userID string) (time.Time, error) {
	now := time.Now()
	exp := now.Add(asEditUnlockTTL)
	id := "UL-" + uuid.New().String()[:12]
	_, err := r.db.Exec(`INSERT INTO as_edit_unlocks(unlock_id, as_id, user_id, unlocked_at, expires_at) VALUES(?,?,?,?,?)`,
		id, asID, userID, now.Format(time.RFC3339), exp.Format(time.RFC3339))
	return exp, err
}

func (r *ASUnlockRepo) LogAttempt(asID, userID, username, ip string, success bool) error {
	ok := 0
	if success {
		ok = 1
	}
	_, err := r.db.Exec(`INSERT INTO as_edit_unlock_log(log_id, as_id, user_id, username, success, ip_address, created_at)
		VALUES(?,?,?,?,?,?,?)`,
		"ULG-"+uuid.New().String()[:12], asID, userID, username, ok, ip, time.Now().Format(time.RFC3339))
	return err
}
