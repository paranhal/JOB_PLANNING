package approval

import (
	"database/sql"
	"fmt"
	"strings"
)

type rowQuery interface {
	QueryRow(query string, args ...any) *sql.Row
}

func scanOrgHead(q rowQuery, orgID string) (userID string, err error) {
	err = q.QueryRow(`SELECT user_id FROM employee_profiles WHERE org_id=? AND is_org_head=1`, orgID).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return userID, err
}

// scanOrgApprover 팀장(is_org_head)이 있으면 팀장, 없으면 같은 조직의 쓰기 가능한 조직관리자.
func scanOrgApprover(q rowQuery, orgID string) (userID string, err error) {
	if userID, err = scanOrgHead(q, orgID); err != nil || userID != "" {
		return userID, err
	}
	err = q.QueryRow(`SELECT user_id FROM users
		WHERE org_id=? AND role IN ('org_admin','admin')
		  AND COALESCE(is_readonly,0)=0 AND COALESCE(is_active,1)=1
		ORDER BY CASE role WHEN 'org_admin' THEN 0 ELSE 1 END, COALESCE(full_name,''), user_id
		LIMIT 1`, orgID).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return userID, err
}

func (e *Engine) OrgHead(orgID string) (userID string, err error) {
	return scanOrgHead(e.DB, orgID)
}

func (e *Engine) OrgApprover(orgID string) (userID string, err error) {
	return scanOrgApprover(e.DB, strings.TrimSpace(orgID))
}

// OrgHeadTx 같은 트랜잭션 안에서 팀장을 읽는다. DB 풀을 또 쓰지 않는다.
func (e *Engine) OrgHeadTx(tx *sql.Tx, orgID string) (string, error) {
	if tx == nil {
		return e.OrgHead(orgID)
	}
	return scanOrgHead(tx, orgID)
}

func (e *Engine) OrgApproverTx(tx *sql.Tx, orgID string) (string, error) {
	if tx == nil {
		return e.OrgApprover(orgID)
	}
	return scanOrgApprover(tx, strings.TrimSpace(orgID))
}

func validateHeadActor(actor Actor, orgID string) error {
	if !actor.CanProcess() || (actor.Role != "org_admin" && actor.Role != "admin") {
		return fmt.Errorf("팀장을 지정할 권한이 없다")
	}
	if strings.TrimSpace(actor.OrgID) != strings.TrimSpace(orgID) {
		return fmt.Errorf("다른 조직의 팀장을 지정할 수 없다")
	}
	return nil
}

func (e *Engine) SetOrgHead(actor Actor, userID, orgID string) error {
	userID, orgID = strings.TrimSpace(userID), strings.TrimSpace(orgID)
	if userID == "" || orgID == "" {
		return fmt.Errorf("대상이 없다")
	}
	if err := validateHeadActor(actor, orgID); err != nil {
		return err
	}
	var role string
	var userOrg string
	var ro, active int
	err := e.DB.QueryRow(`SELECT role, COALESCE(org_id,''), COALESCE(is_readonly,0), COALESCE(is_active,1) FROM users WHERE user_id=?`, userID).Scan(&role, &userOrg, &ro, &active)
	if err == sql.ErrNoRows {
		return fmt.Errorf("사용자가 없다")
	}
	if err != nil {
		return err
	}
	if (role != "org_admin" && role != "admin") || ro != 0 || active == 0 {
		return fmt.Errorf("팀장은 쓰기 가능한 조직관리자여야 한다")
	}
	if strings.TrimSpace(userOrg) != orgID {
		return fmt.Errorf("다른 조직의 팀장을 지정할 수 없다")
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

func (e *Engine) ClearOrgHead(actor Actor, orgID string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	if err := validateHeadActor(actor, orgID); err != nil {
		return err
	}
	_, err := e.DB.Exec(`UPDATE employee_profiles SET is_org_head=0, updated_at=CURRENT_TIMESTAMP WHERE org_id=? AND is_org_head=1`, orgID)
	return err
}

type OrgHeadOption struct {
	UserID   string
	FullName string
}

func (e *Engine) OrgHeadOptions(orgID string) ([]OrgHeadOption, error) {
	rows, err := e.DB.Query(`SELECT user_id, COALESCE(full_name,'') FROM users
		WHERE org_id=? AND role IN ('org_admin','admin')
		  AND COALESCE(is_readonly,0)=0 AND COALESCE(is_active,1)=1
		ORDER BY COALESCE(full_name,''), user_id`, strings.TrimSpace(orgID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgHeadOption
	for rows.Next() {
		var v OrgHeadOption
		if err := rows.Scan(&v.UserID, &v.FullName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (e *Engine) EnsureProfile(userID, orgID string) error {
	_, err := e.DB.Exec(`INSERT OR IGNORE INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,0)`, userID, orgID)
	return err
}
