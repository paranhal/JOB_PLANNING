package repository

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/audit"
)

const SettingASCompletedEditPassword = "as_completed_edit_password_hash"

// SettingMaintenanceDeletePassword 정기점검 계획 삭제 전용 비밀번호 (§23.11).
const SettingMaintenanceDeletePassword = "maintenance_delete_password_hash"

// SettingMaintenanceAutoDailyLimit 자동 초안 하루 배치 상한 (§23.5). 기본 4건.
const SettingMaintenanceAutoDailyLimit = "maintenance_auto_daily_limit"

// SettingMetricsBaseDate 지표 집계 하한일 (§4.5). 값은 app_settings, 코드에 박지 않는다.
const SettingMetricsBaseDate = "metrics_base_date"

// DefaultASCompletedEditPassword 완료·종료 AS 수정 잠금 해제 기본 비밀번호 (최초 시드).
// 관리자는 /users 화면에서 변경할 수 있다.
const DefaultASCompletedEditPassword = "as-edit"

// DefaultMaintenanceAutoDailyLimit 자동 초안 하루 최대 방문 건수.
const DefaultMaintenanceAutoDailyLimit = 4

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
	before := rowJSON(r.db, "app_settings", "setting_key", key)
	wasCreate := strings.TrimSpace(before) == "{}"
	_, err := r.db.Exec(`
		INSERT INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)
		ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value, updated_at=excluded.updated_at`,
		key, value, now)
	if err != nil {
		return err
	}
	after := rowJSON(r.db, "app_settings", "setting_key", key)
	if strings.Contains(strings.ToLower(key), "password") {
		if !wasCreate {
			before = maskSettingJSON(key)
		}
		after = maskSettingJSON(key)
	}
	if wasCreate {
		audit.Use(r.db)
		audit.Log(audit.ActionCreate, "app_settings", "setting_key", key, key, "", after)
	} else {
		audit.Use(r.db)
		audit.Log(audit.ActionUpdate, "app_settings", "setting_key", key, key, before, after)
	}
	return nil
}

func maskSettingJSON(key string) string {
	return `{"setting_key":"` + key + `","setting_value":"***"}`
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

const mntDeletePasswordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomMaintenanceDeletePassword() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "mnt-" + time.Now().Format("150405")
	}
	out := make([]byte, 10)
	for i, v := range b {
		out[i] = mntDeletePasswordAlphabet[int(v)%len(mntDeletePasswordAlphabet)]
	}
	return string(out)
}

// EnsureMaintenanceDeletePassword 없으면 임의 비밀번호를 만들어 해시 저장한다.
// 새로 만든 평문을 반환한다(이미 있으면 빈 문자열). 호출측에서 1회만 로그한다.
func (r *SettingsRepo) EnsureMaintenanceDeletePassword(hashFn func(string) string) (string, error) {
	cur, err := r.Get(SettingMaintenanceDeletePassword)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cur) != "" {
		return "", nil
	}
	plain := randomMaintenanceDeletePassword()
	h := hashFn(plain)
	if h == "" {
		return "", fmt.Errorf("삭제 비밀번호 해시를 만들지 못했습니다")
	}
	if err := r.Set(SettingMaintenanceDeletePassword, h); err != nil {
		return "", err
	}
	return plain, nil
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
