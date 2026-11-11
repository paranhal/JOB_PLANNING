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

const (
	PendingKeep = "keep"
	PendingMove = "move"
)

type HeadChangeResult struct {
	PrevUserID string
	NewUserID  string
	Moved      int
}

type HeadPreview struct {
	CurrentID  string
	NextUserID string
	NextLabel  string
	Pending    int
}

type HeadHistoryRow struct {
	PrevLabel string
	NextLabel string
	Actor     string
	ActedAt   string
	Mode      string
	Moved     int
}

func (e *Engine) SetOrgHead(actor Actor, userID, orgID string) error {
	_, err := e.ChangeOrgHead(actor, userID, orgID, PendingKeep)
	return err
}

func (e *Engine) ClearOrgHead(actor Actor, orgID string) error {
	_, err := e.ChangeOrgHead(actor, "", orgID, PendingKeep)
	return err
}

// ChangeOrgHead 는 SetOrgHead·ClearOrgHead와 같은 지정 규칙으로 팀장을 바꾸고,
// 대기 문서 이관이 있으면 그 변경과 한 트랜잭션에서 처리한다.
func (e *Engine) ChangeOrgHead(actor Actor, userID, orgID, mode string) (HeadChangeResult, error) {
	userID, orgID = strings.TrimSpace(userID), strings.TrimSpace(orgID)
	if orgID == "" {
		return HeadChangeResult{}, fmt.Errorf("조직이 없다")
	}
	if mode != PendingMove {
		mode = PendingKeep
	}
	if err := validateHeadActor(actor, orgID); err != nil {
		return HeadChangeResult{}, err
	}
	if userID != "" {
		if err := e.validateHeadCandidate(userID, orgID); err != nil {
			return HeadChangeResult{}, err
		}
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return HeadChangeResult{}, err
	}
	defer tx.Rollback()
	prev, err := scanOrgHead(tx, orgID)
	if err != nil {
		return HeadChangeResult{}, err
	}
	if userID != "" {
		if _, err := tx.Exec(`UPDATE employee_profiles SET is_org_head=0 WHERE org_id=? AND user_id<>?`, orgID, userID); err != nil {
			return HeadChangeResult{}, err
		}
		if _, err := tx.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head)
			VALUES (?,?,1)
			ON CONFLICT(user_id) DO UPDATE SET org_id=excluded.org_id, is_org_head=1, updated_at=CURRENT_TIMESTAMP`,
			userID, orgID); err != nil {
			return HeadChangeResult{}, err
		}
	} else if _, err := tx.Exec(`UPDATE employee_profiles SET is_org_head=0, updated_at=CURRENT_TIMESTAMP WHERE org_id=? AND is_org_head=1`, orgID); err != nil {
		return HeadChangeResult{}, err
	}
	moved := 0
	if mode == PendingMove && prev != "" && prev != userID {
		target := userID
		if target == "" {
			target, err = scanWritableOrgAdmin(tx, orgID, prev)
			if err != nil {
				return HeadChangeResult{}, err
			}
		}
		if target != "" && target != prev {
			res, err := tx.Exec(`UPDATE approval_steps SET approver_id=?
				WHERE status='waiting' AND approver_id=?
				  AND request_id IN (SELECT request_id FROM approval_requests WHERE org_id=?)`,
				target, prev, orgID)
			if err != nil {
				return HeadChangeResult{}, err
			}
			n, _ := res.RowsAffected()
			moved = int(n)
		}
	}
	if prev != userID || moved > 0 {
		if _, err := tx.Exec(`INSERT INTO org_head_history(history_id, org_id, prev_user_id, new_user_id, acted_by, pending_mode, pending_moved)
			VALUES (?,?,?,?,?,?,?)`, newID(), orgID, prev, userID, strings.TrimSpace(actor.UserID), mode, moved); err != nil {
			return HeadChangeResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return HeadChangeResult{}, err
	}
	return HeadChangeResult{PrevUserID: prev, NewUserID: userID, Moved: moved}, nil
}

func (e *Engine) validateHeadCandidate(userID, orgID string) error {
	var role, userOrg string
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
	return nil
}

func scanWritableOrgAdmin(q rowQuery, orgID, except string) (string, error) {
	var userID string
	err := q.QueryRow(`SELECT user_id FROM users
		WHERE org_id=? AND role IN ('org_admin','admin')
		  AND COALESCE(is_readonly,0)=0 AND COALESCE(is_active,1)=1
		  AND user_id<>?
		ORDER BY CASE role WHEN 'org_admin' THEN 0 ELSE 1 END, COALESCE(full_name,''), user_id
		LIMIT 1`, orgID, except).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return userID, err
}

func (e *Engine) PreviewHeadChange(orgID, nextUserID string) (HeadPreview, error) {
	orgID, nextUserID = strings.TrimSpace(orgID), strings.TrimSpace(nextUserID)
	cur, err := e.OrgHead(orgID)
	if err != nil {
		return HeadPreview{}, err
	}
	var pending int
	if cur != "" {
		if err := e.DB.QueryRow(`SELECT COUNT(*) FROM approval_steps s
			JOIN approval_requests r ON r.request_id=s.request_id
			WHERE r.org_id=? AND s.status='waiting' AND s.approver_id=?`, orgID, cur).Scan(&pending); err != nil {
			return HeadPreview{}, err
		}
	}
	labelID := nextUserID
	label := "조직관리자"
	if labelID == "" {
		labelID, err = scanWritableOrgAdmin(e.DB, orgID, cur)
		if err != nil {
			return HeadPreview{}, err
		}
	}
	if labelID != "" {
		name := e.userLabel(labelID)
		if nextUserID != "" {
			label = name + "(팀장)"
		} else if name != "" {
			label = "조직관리자 " + name
		}
	}
	return HeadPreview{CurrentID: cur, NextUserID: nextUserID, NextLabel: label, Pending: pending}, nil
}

func (e *Engine) userLabel(userID string) string {
	var name, username string
	_ = e.DB.QueryRow(`SELECT COALESCE(full_name,''), COALESCE(username,'') FROM users WHERE user_id=?`, userID).Scan(&name, &username)
	if strings.TrimSpace(name) != "" {
		return name
	}
	return username
}

func (e *Engine) HeadHistory(orgID string, limit int) ([]HeadHistoryRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := e.DB.Query(`SELECT COALESCE(p.full_name, h.prev_user_id, ''),
		COALESCE(n.full_name, NULLIF(h.new_user_id,''), '해제'),
		COALESCE(a.full_name, h.acted_by, ''), h.acted_at, h.pending_mode, h.pending_moved
		FROM org_head_history h
		LEFT JOIN users p ON p.user_id=h.prev_user_id
		LEFT JOIN users n ON n.user_id=h.new_user_id
		LEFT JOIN users a ON a.user_id=h.acted_by
		WHERE h.org_id=?
		ORDER BY h.acted_at DESC, h.history_id DESC
		LIMIT ?`, strings.TrimSpace(orgID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeadHistoryRow
	for rows.Next() {
		var v HeadHistoryRow
		if err := rows.Scan(&v.PrevLabel, &v.NextLabel, &v.Actor, &v.ActedAt, &v.Mode, &v.Moved); err != nil {
			return nil, err
		}
		if v.PrevLabel == "" {
			v.PrevLabel = "없음"
		}
		out = append(out, v)
	}
	return out, rows.Err()
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
