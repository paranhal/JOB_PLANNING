package approval

import (
	"database/sql"
	"fmt"
	"strings"
)

func (e *Engine) OrgHead(orgID string) (userID string, err error) {
	err = e.DB.QueryRow(`SELECT user_id FROM employee_profiles WHERE org_id=? AND is_org_head=1`, orgID).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return userID, err
}

func (e *Engine) SetOrgHead(actor Actor, userID, orgID string) error {
	userID, orgID = strings.TrimSpace(userID), strings.TrimSpace(orgID)
	if userID == "" || orgID == "" {
		return fmt.Errorf("대상이 없다")
	}
	var role string
	var ro, active int
	err := e.DB.QueryRow(`SELECT role, COALESCE(is_readonly,0), COALESCE(is_active,1) FROM users WHERE user_id=?`, userID).Scan(&role, &ro, &active)
	if err == sql.ErrNoRows {
		return fmt.Errorf("사용자가 없다")
	}
	if err != nil {
		return err
	}
	if (role != "org_admin" && role != "admin") || ro != 0 || active == 0 {
		return fmt.Errorf("팀장은 쓰기 가능한 조직관리자여야 한다")
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE employee_profiles SET is_org_head=0 WHERE org_id=? AND user_id<>?`, orgID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head)
		VALUES (?,?,1)
		ON CONFLICT(user_id) DO UPDATE SET org_id=excluded.org_id, is_org_head=1, updated_at=CURRENT_TIMESTAMP`,
		userID, orgID); err != nil {
		return err
	}
	return tx.Commit()
}

func (e *Engine) EnsureProfile(userID, orgID string) error {
	_, err := e.DB.Exec(`INSERT OR IGNORE INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,0)`, userID, orgID)
	return err
}
