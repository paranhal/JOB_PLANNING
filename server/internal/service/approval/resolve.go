package approval

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ActDecision struct {
	OK      bool
	Mode    string
	ActedBy string
	Label   string
	Reason  string
}

// ResolveActor 승인자 결정. §2.5.3
func (e *Engine) ResolveActor(policy Policy, step Step, drafterID, orgID string, actor Actor, today string) ActDecision {
	if !actor.CanProcess() {
		return ActDecision{Reason: "읽기 전용 또는 시점 보기"}
	}
	if today == "" {
		today = e.today()
	}
	if actor.UserID == step.ApproverID {
		return ActDecision{OK: true, Mode: ModeDirect, ActedBy: actor.UserID}
	}
	del, _ := e.activeDelegate(step.ApproverID, today)
	if del != "" && actor.UserID == del {
		if actor.UserID == drafterID {
			return ActDecision{Reason: "대리인은 자기 문서를 대결할 수 없다"}
		}
		return ActDecision{OK: true, Mode: ModeDelegate, ActedBy: actor.UserID, Label: "대리 결재"}
	}
	if policy.AdminEmergencyApprove && del == "" && isAdminRole(actor.Role) {
		return ActDecision{OK: true, Mode: ModeAdminEmergency, ActedBy: actor.UserID, Label: "관리자 대결"}
	}
	return ActDecision{Reason: "처리할 권한이 없다"}
}

func isAdminRole(role string) bool {
	r := strings.TrimSpace(role)
	return r == "org_admin" || r == "admin" || r == "vision_admin"
}

func (e *Engine) activeDelegate(headID, today string) (string, error) {
	var to string
	err := e.DB.QueryRow(`SELECT to_user_id FROM approval_delegations
		WHERE from_user_id=? AND revoked_at IS NULL AND start_date<=? AND end_date>=?
		ORDER BY created_at DESC LIMIT 1`, headID, today, today).Scan(&to)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return to, err
}

func (e *Engine) CreateDelegation(actor Actor, orgID, fromID, toID, start, end, source, leaveReqID string) (string, error) {
	if !actor.CanProcess() {
		return "", fmt.Errorf("지정할 수 없다")
	}
	p, err := e.GetPolicy(orgID)
	if err != nil {
		return "", err
	}
	switch source {
	case SrcHead:
		if !p.DelegByHead {
			return "", fmt.Errorf("정책 A가 꺼져 있다")
		}
		if actor.UserID != fromID && !isAdminRole(actor.Role) {
			return "", fmt.Errorf("팀장만 지정할 수 있다")
		}
	case SrcAdmin:
		if !p.DelegByAdmin {
			return "", fmt.Errorf("정책 B가 꺼져 있다")
		}
		if !isAdminRole(actor.Role) {
			return "", fmt.Errorf("관리자만 지정할 수 있다")
		}
	case SrcLeave:
		if !p.DelegWithLeave {
			return "", fmt.Errorf("정책 C가 꺼져 있다")
		}
	default:
		return "", fmt.Errorf("지정 출처가 없다")
	}
	if fromID == toID {
		return "", fmt.Errorf("팀장 본인은 대리인이 될 수 없다")
	}
	if err := e.assertSameOrgActive(orgID, toID); err != nil {
		return "", err
	}
	id := "AD-" + newID()
	_, err = e.DB.Exec(`INSERT INTO approval_delegations(
		delegation_id, org_id, from_user_id, to_user_id, start_date, end_date, source, leave_req_id, created_by)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, orgID, fromID, toID, start, end, source, nullIfEmpty(leaveReqID), actor.UserID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "overlap") {
			return "", fmt.Errorf("기간이 겹치는 지정이 있다")
		}
		return "", err
	}
	e.enqueue(orgID, toID, actor.UserID, "대리 결재 지정", "대리 결재자로 지정되었습니다.")
	return id, nil
}

func (e *Engine) RevokeDelegation(actor Actor, id string) error {
	if !actor.CanProcess() {
		return fmt.Errorf("종료할 수 없다")
	}
	_, err := e.DB.Exec(`UPDATE approval_delegations SET revoked_at=?, revoked_by=? WHERE delegation_id=? AND revoked_at IS NULL`,
		e.now().Format(time.RFC3339), actor.UserID, id)
	return err
}

func RequireLeaveDeputy(p Policy, drafterIsHead bool) bool {
	return p.DelegWithLeave && drafterIsHead
}

func (e *Engine) OnLeaveApprovedCreateDelegation(actor Actor, orgID, headID, deputyID, start, end, leaveReqID string) (string, error) {
	p, err := e.GetPolicy(orgID)
	if err != nil {
		return "", err
	}
	if !p.DelegWithLeave {
		return "", nil
	}
	if strings.TrimSpace(deputyID) == "" {
		return "", fmt.Errorf("대리 결재자가 없다")
	}
	return e.CreateDelegation(actor, orgID, headID, deputyID, start, end, SrcLeave, leaveReqID)
}

func (e *Engine) assertSameOrgActive(orgID, userID string) error {
	var uorg string
	var active int
	err := e.DB.QueryRow(`SELECT COALESCE(org_id,''), COALESCE(is_active,1) FROM users WHERE user_id=?`, userID).Scan(&uorg, &active)
	if err == sql.ErrNoRows {
		return fmt.Errorf("대리인이 없다")
	}
	if err != nil {
		return err
	}
	if active == 0 {
		return fmt.Errorf("비활성 직원은 대리인이 될 수 없다")
	}
	if uorg != orgID {
		return fmt.Errorf("같은 조직 직원만 대리인이 될 수 있다")
	}
	return nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
