package approval

import (
	"encoding/json"
	"fmt"
	"strings"

	"customer-support/internal/audit"
)

func (e *Engine) EnsurePolicy(orgID string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	p := DefaultPolicy(orgID)
	_, err := e.DB.Exec(`INSERT OR IGNORE INTO approval_policies(
		org_id, deleg_by_head, deleg_by_admin, deleg_with_leave, admin_emergency_approve,
		self_approve_leave, self_approve_quote, notify_admin_self_approve)
		VALUES (?,?,?,?,?,?,?,?)`,
		orgID, b2i(p.DelegByHead), b2i(p.DelegByAdmin), b2i(p.DelegWithLeave), b2i(p.AdminEmergencyApprove),
		b2i(p.SelfApproveLeave), b2i(p.SelfApproveQuote), b2i(p.NotifyAdminSelfApprove))
	if err != nil {
		return err
	}
	return e.ensureTemplates(orgID)
}

func (e *Engine) GetPolicy(orgID string) (Policy, error) {
	if err := e.EnsurePolicy(orgID); err != nil {
		return Policy{}, err
	}
	var p Policy
	var a, b, c, d, sl, sq, n int
	err := e.DB.QueryRow(`SELECT org_id, deleg_by_head, deleg_by_admin, deleg_with_leave, admin_emergency_approve,
		self_approve_leave, self_approve_quote, notify_admin_self_approve, COALESCE(updated_by,'')
		FROM approval_policies WHERE org_id=?`, orgID).Scan(
		&p.OrgID, &a, &b, &c, &d, &sl, &sq, &n, &p.UpdatedBy)
	if err != nil {
		return Policy{}, err
	}
	p.DelegByHead, p.DelegByAdmin, p.DelegWithLeave = a == 1, b == 1, c == 1
	p.AdminEmergencyApprove = d == 1
	p.SelfApproveLeave, p.SelfApproveQuote, p.NotifyAdminSelfApprove = sl == 1, sq == 1, n == 1
	return p, nil
}

func (e *Engine) SavePolicy(actor Actor, orgID string, p Policy) error {
	if !actor.CanProcess() {
		return fmt.Errorf("읽기 전용·시점 보기에서는 저장할 수 없다")
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	if err := e.EnsurePolicy(orgID); err != nil {
		return err
	}
	before, _ := e.GetPolicy(orgID)
	p.OrgID = orgID
	_, err := e.DB.Exec(`UPDATE approval_policies SET
		deleg_by_head=?, deleg_by_admin=?, deleg_with_leave=?, admin_emergency_approve=?,
		self_approve_leave=?, self_approve_quote=?, notify_admin_self_approve=?,
		updated_by=?, updated_at=CURRENT_TIMESTAMP WHERE org_id=?`,
		b2i(p.DelegByHead), b2i(p.DelegByAdmin), b2i(p.DelegWithLeave), b2i(p.AdminEmergencyApprove),
		b2i(p.SelfApproveLeave), b2i(p.SelfApproveQuote), b2i(p.NotifyAdminSelfApprove),
		actor.UserID, orgID)
	if err != nil {
		return err
	}
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(p)
	pop := audit.Push(audit.Actor{UserID: actor.UserID, Username: actor.Username, Name: actor.Name, Role: actor.Role})
	defer pop()
	audit.LogWithReason(audit.ActionUpdate, "approval_policies", "org_id", orgID, "결재 설정", string(bj), string(aj), "결재 정책 변경")
	return nil
}

func (e *Engine) ensureTemplates(orgID string) error {
	for _, dt := range []string{DocLeave, DocQuote, DocTest} {
		var n int
		_ = e.DB.QueryRow(`SELECT COUNT(*) FROM approval_templates WHERE org_id=? AND doc_type=? AND is_active=1`, orgID, dt).Scan(&n)
		if n > 0 {
			continue
		}
		id := "T-" + orgID + "-" + dt
		if _, err := e.DB.Exec(`INSERT OR IGNORE INTO approval_templates(template_id, org_id, doc_type, name)
			VALUES (?,?,?,?)`, id, orgID, dt, "기본"); err != nil {
			return err
		}
		if _, err := e.DB.Exec(`INSERT OR IGNORE INTO approval_template_steps(template_id, step_no, step_type, approver_rule)
			VALUES (?,?,?,?)`, id, 1, TypeApprove, "org_head"); err != nil {
			return err
		}
	}
	return nil
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}
