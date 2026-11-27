package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/audit"
)

// 95-224 §3: 일반 담당자의 「1회 등록 요청」. as_edit_unlocks 에 `siteadd:<project_id>` 키로 담는다.
// 비밀번호 검증은 호출측(AS 수정 잠금 비밀번호). 여기서는 권한 부여·소멸·실패 잠금만 맡는다.

const (
	SiteAddKeyPrefix   = "siteadd:"
	SiteAddMaxFails    = 5
	SiteAddLockoutSpan = 10 * time.Minute
	SiteAddReason      = "1회 등록 요청으로 등록"
)

func SiteAddKey(projectID string) string { return SiteAddKeyPrefix + strings.TrimSpace(projectID) }

// Revoke 해당 키의 권한을 모두 지운다(1회 사용 후 소멸).
func (r *ASUnlockRepo) Revoke(key, userID string) error {
	_, err := r.db.Exec(`DELETE FROM as_edit_unlocks WHERE as_id=? AND user_id=?`, key, userID)
	return err
}

// SiteAddLockedUntil 사용자가 최근 10분 안에 5회 이상 틀렸으면 잠금이 풀리는 시각을 돌려준다.
// 마지막 성공 이후의 실패만 센다. 잠금 중 시도는 로그에 쌓지 않으므로 잠금이 연장되지 않는다.
func (r *ASUnlockRepo) SiteAddLockedUntil(userID string, now time.Time) (time.Time, bool) {
	rows, err := r.db.Query(`SELECT success, created_at FROM as_edit_unlock_log
		WHERE user_id=? AND as_id LIKE ? ORDER BY created_at DESC LIMIT 50`, userID, SiteAddKeyPrefix+"%")
	if err != nil {
		return time.Time{}, false
	}
	defer rows.Close()
	var fails []time.Time
	for rows.Next() {
		var ok int
		var at string
		if rows.Scan(&ok, &at) != nil {
			continue
		}
		if ok == 1 {
			break
		}
		t, e := time.Parse(time.RFC3339, at)
		if e != nil {
			continue
		}
		if now.Sub(t) <= SiteAddLockoutSpan {
			fails = append(fails, t)
		}
	}
	if len(fails) < SiteAddMaxFails {
		return time.Time{}, false
	}
	until := fails[SiteAddMaxFails-1].Add(SiteAddLockoutSpan)
	return until, now.Before(until)
}

// LogSiteAddByUnlock 1회 권한으로 등록한 사이트를 변경 이력에 남긴다(비밀번호는 남기지 않는다).
func (r *ASUnlockRepo) LogSiteAddByUnlock(projectID, userID string, count int) {
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionCreate, "project_sites", "project_id", projectID,
		fmt.Sprintf("사이트 %d곳 등록", count), "", fmt.Sprintf(`{"project_id":%q,"registered_by":%q,"count":%d}`, projectID, userID, count), SiteAddReason)
}
