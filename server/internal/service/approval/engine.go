package approval

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (e *Engine) hooks() Hooks {
	if e.Hooks != nil {
		return e.Hooks
	}
	return &MemoryHooks{}
}

func (e *Engine) CreateDraft(orgID, docType, docID, drafterID, title string) (string, error) {
	if err := e.EnsurePolicy(orgID); err != nil {
		return "", err
	}
	var max sql.NullInt64
	_ = e.DB.QueryRow(`SELECT MAX(revision) FROM approval_requests WHERE doc_type=? AND doc_id=?`, docType, docID).Scan(&max)
	rev := 1
	if max.Valid {
		rev = int(max.Int64) + 1
	}
	id := "AR-" + newID()
	if strings.TrimSpace(title) == "" {
		title = docType + " " + docID
	}
	_, err := e.DB.Exec(`INSERT INTO approval_requests(request_id, org_id, doc_type, doc_id, revision, drafter_id, title, status)
		VALUES (?,?,?,?,?,?,?,?)`, id, orgID, docType, docID, rev, drafterID, title, StatusDraft)
	return id, err
}

func (e *Engine) Submit(actor Actor, requestID string) error {
	return e.submitTx(actor, requestID, false)
}

func (e *Engine) submitTx(actor Actor, requestID string, resubmit bool) error {
	if !actor.CanProcess() {
		return fmt.Errorf("상신할 수 없다")
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	req, err := loadRequestTx(tx, requestID)
	if err != nil {
		return err
	}
	if req.Status != StatusDraft {
		return fmt.Errorf("draft 에서만 상신할 수 있다")
	}
	if req.DrafterID != actor.UserID && !isAdminRole(actor.Role) {
		return fmt.Errorf("기안자만 상신할 수 있다")
	}
	head, err := e.OrgHead(req.OrgID)
	if err != nil {
		return err
	}
	if head == "" {
		return fmt.Errorf("팀장이 지정되지 않은 조직은 상신할 수 없다")
	}
	p, err := e.GetPolicy(req.OrgID)
	if err != nil {
		return err
	}
	self := head == req.DrafterID && p.HeadSelfApprove(req.DocType)
	if head == req.DrafterID && !p.HeadSelfApprove(req.DocType) {
		return fmt.Errorf("이 문서 종류의 자기 결재가 꺼져 있다")
	}
	title, snap, err := e.hooks().Snapshot(tx, req.DocType, req.DocID)
	if err != nil {
		return err
	}
	if title != "" {
		req.Title = title
	}
	req.SnapshotJSON = snap
	rules, err := e.templateRules(req.OrgID, req.DocType)
	if err != nil {
		return err
	}
	now := e.now().Format("2006-01-02 15:04:05")
	pendingSet := false
	for i, rule := range rules {
		st := Step{
			StepID: "ST-" + newID(), RequestID: req.RequestID, StepNo: i + 1,
			StepType: rule.typ, ApproverID: head, Status: StepWaiting,
		}
		if rule.rule == "drafter" {
			st.ApproverID = req.DrafterID
			st.Status = StepSkipped
			st.ActMode = ModeDirect
			st.ActedBy = req.DrafterID
			st.ActedAt = now
		} else if rule.rule == "org_head" || strings.HasPrefix(rule.rule, "user:") {
			if strings.HasPrefix(rule.rule, "user:") {
				st.ApproverID = strings.TrimPrefix(rule.rule, "user:")
			} else {
				st.ApproverID = head
			}
			if st.ApproverID == req.DrafterID && !self {
				return fmt.Errorf("일반 직원은 자기를 승인자로 둘 수 없다")
			}
			if st.StepType == TypeApprove {
				if self && st.ApproverID == req.DrafterID {
					st.Status = StepApproved
					st.ActMode = ModeSelf
					st.ActedBy = req.DrafterID
					st.ActedAt = now
				} else if !pendingSet {
					st.Status = StepPending
					pendingSet = true
				} else {
					st.Status = StepWaiting
				}
			}
		}
		if _, err := tx.Exec(`INSERT INTO approval_steps(step_id, request_id, step_no, step_type, approver_id, acted_by, act_mode, status, acted_at)
			VALUES (?,?,?,?,?,?,?,?,?)`, st.StepID, st.RequestID, st.StepNo, st.StepType, st.ApproverID, nullIfEmpty(st.ActedBy), nullIfEmpty(st.ActMode), st.Status, nullIfEmpty(st.ActedAt)); err != nil {
			return err
		}
	}
	status := StatusInProgress
	completed := ""
	if self {
		status = StatusApproved
		completed = now
	}
	if _, err := tx.Exec(`UPDATE approval_requests SET title=?, snapshot_json=?, status=?, submitted_at=?, completed_at=? WHERE request_id=?`,
		req.Title, req.SnapshotJSON, status, now, nullIfEmpty(completed), req.RequestID); err != nil {
		return err
	}
	if err := insertEvent(tx, req.RequestID, EvSubmit, actor.UserID, `{"revision":`+itoa(req.Revision)+`}`); err != nil {
		return err
	}
	if resubmit {
		if err := insertEvent(tx, req.RequestID, EvResubmit, actor.UserID, ""); err != nil {
			return err
		}
	}
	if self {
		if err := insertEvent(tx, req.RequestID, EvSelfApprove, actor.UserID, ""); err != nil {
			return err
		}
		req.Status = StatusApproved
		if err := e.hooks().AfterApproved(tx, req); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	e.enqueue(req.OrgID, head, actor.UserID, "결재 요청", req.Title)
	if self && p.NotifyAdminSelfApprove {
		e.enqueue(req.OrgID, head, actor.UserID, "자기 결재", req.Title)
	}
	return nil
}

type tmplRule struct{ typ, rule string }

func (e *Engine) templateRules(orgID, docType string) ([]tmplRule, error) {
	var tid string
	err := e.DB.QueryRow(`SELECT template_id FROM approval_templates WHERE org_id=? AND doc_type=? AND is_active=1 ORDER BY created_at LIMIT 1`, orgID, docType).Scan(&tid)
	if err != nil {
		return []tmplRule{{TypeApprove, "org_head"}}, nil
	}
	rows, err := e.DB.Query(`SELECT step_type, approver_rule FROM approval_template_steps WHERE template_id=? ORDER BY step_no`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tmplRule
	for rows.Next() {
		var r tmplRule
		if err := rows.Scan(&r.typ, &r.rule); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		out = []tmplRule{{TypeApprove, "org_head"}}
	}
	return out, rows.Err()
}

func (e *Engine) Approve(actor Actor, requestID, comment, emergencyReason string) error {
	if !actor.CanProcess() {
		return fmt.Errorf("승인할 수 없다")
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	req, err := loadRequestTx(tx, requestID)
	if err != nil {
		return err
	}
	if req.Status != StatusInProgress {
		return fmt.Errorf("%s 에서는 승인할 수 없다", req.Status)
	}
	step, err := pendingApproveTx(tx, requestID)
	if err != nil {
		return err
	}
	p, err := e.GetPolicy(req.OrgID)
	if err != nil {
		return err
	}
	dec := e.ResolveActor(p, step, req.DrafterID, req.OrgID, actor, e.today())
	if !dec.OK {
		return fmt.Errorf("%s", dec.Reason)
	}
	if dec.Mode == ModeAdminEmergency && strings.TrimSpace(emergencyReason) == "" {
		return fmt.Errorf("비상 승인은 사유가 필요하다")
	}
	now := e.now().Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`UPDATE approval_steps SET status=?, acted_by=?, act_mode=?, comment=?, acted_at=? WHERE step_id=?`,
		StepApproved, dec.ActedBy, dec.Mode, comment, now, step.StepID); err != nil {
		return err
	}
	ev := EvApprove
	if dec.Mode == ModeDelegate {
		ev = EvDelegateApprove
	}
	if dec.Mode == ModeAdminEmergency {
		ev = EvAdminEmergency
	}
	if err := insertEvent(tx, req.RequestID, ev, actor.UserID, commentJSON(comment, emergencyReason)); err != nil {
		return err
	}
	next, err := nextWaitingApproveTx(tx, requestID)
	if err != nil {
		return err
	}
	if next != "" {
		if _, err := tx.Exec(`UPDATE approval_steps SET status=? WHERE step_id=?`, StepPending, next); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE approval_requests SET status=?, completed_at=? WHERE request_id=?`, StatusApproved, now, requestID); err != nil {
			return err
		}
		req.Status = StatusApproved
		if err := e.hooks().AfterApproved(tx, req); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if dec.Mode == ModeDelegate {
		e.enqueue(req.OrgID, step.ApproverID, actor.UserID, "대결 처리", req.Title)
	}
	e.enqueue(req.OrgID, req.DrafterID, actor.UserID, "결재 결과", "승인")
	return nil
}

func (e *Engine) Reject(actor Actor, requestID, comment string) error {
	if !actor.CanProcess() {
		return fmt.Errorf("반려할 수 없다")
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	req, err := loadRequestTx(tx, requestID)
	if err != nil {
		return err
	}
	if req.Status != StatusInProgress {
		return fmt.Errorf("%s 에서는 반려할 수 없다", req.Status)
	}
	step, err := pendingApproveTx(tx, requestID)
	if err != nil {
		return err
	}
	p, _ := e.GetPolicy(req.OrgID)
	dec := e.ResolveActor(p, step, req.DrafterID, req.OrgID, actor, e.today())
	if !dec.OK {
		return fmt.Errorf("%s", dec.Reason)
	}
	now := e.now().Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`UPDATE approval_steps SET status=?, acted_by=?, act_mode=?, comment=?, acted_at=? WHERE step_id=?`,
		StepRejected, dec.ActedBy, dec.Mode, comment, now, step.StepID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE approval_requests SET status=?, completed_at=? WHERE request_id=?`, StatusRejected, now, requestID); err != nil {
		return err
	}
	if err := insertEvent(tx, requestID, EvReject, actor.UserID, commentJSON(comment, "")); err != nil {
		return err
	}
	return tx.Commit()
}

func (e *Engine) Withdraw(actor Actor, requestID string) error {
	if !actor.CanProcess() {
		return fmt.Errorf("회수할 수 없다")
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	req, err := loadRequestTx(tx, requestID)
	if err != nil {
		return err
	}
	if req.Status != StatusInProgress {
		return fmt.Errorf("%s 에서는 회수할 수 없다", req.Status)
	}
	if actor.UserID != req.DrafterID {
		return fmt.Errorf("기안자만 회수할 수 있다")
	}
	now := e.now().Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`UPDATE approval_requests SET status=?, completed_at=? WHERE request_id=?`, StatusWithdrawn, now, requestID); err != nil {
		return err
	}
	if err := insertEvent(tx, requestID, EvWithdraw, actor.UserID, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (e *Engine) Resubmit(actor Actor, requestID string) (string, error) {
	old, err := e.GetRequest(requestID)
	if err != nil {
		return "", err
	}
	if old.Status != StatusRejected && old.Status != StatusWithdrawn {
		return "", fmt.Errorf("반려·회수 건만 재상신한다")
	}
	nid, err := e.CreateDraft(old.OrgID, old.DocType, old.DocID, old.DrafterID, old.Title)
	if err != nil {
		return "", err
	}
	if err := e.submitTx(actor, nid, true); err != nil {
		return "", err
	}
	return nid, nil
}

func (e *Engine) GetRequest(id string) (Request, error) {
	return loadRequestTx(e.DB, id)
}

func (e *Engine) ListInbox(orgID, userID, today string) ([]Request, error) {
	if today == "" {
		today = e.today()
	}
	rows, err := e.DB.Query(`SELECT r.request_id, r.org_id, r.doc_type, r.doc_id, r.revision, r.drafter_id, r.title, r.status,
		COALESCE(r.snapshot_json,''), COALESCE(r.submitted_at,''), COALESCE(r.completed_at,'')
		FROM approval_requests r
		JOIN approval_steps s ON s.request_id=r.request_id AND s.step_type=? AND s.status=?
		WHERE r.org_id=? AND r.status=?
		AND (s.approver_id=? OR EXISTS (
			SELECT 1 FROM approval_delegations d
			WHERE d.from_user_id=s.approver_id AND d.to_user_id=? AND d.revoked_at IS NULL
			  AND d.start_date<=? AND d.end_date>=?
		))
		ORDER BY r.submitted_at DESC`, TypeApprove, StepPending, orgID, StatusInProgress, userID, userID, today, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func (e *Engine) ListSent(orgID, userID string) ([]Request, error) {
	rows, err := e.DB.Query(`SELECT request_id, org_id, doc_type, doc_id, revision, drafter_id, title, status,
		COALESCE(snapshot_json,''), COALESCE(submitted_at,''), COALESCE(completed_at,'')
		FROM approval_requests WHERE org_id=? AND drafter_id=? ORDER BY created_at DESC`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func scanRequests(rows *sql.Rows) ([]Request, error) {
	var out []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.RequestID, &r.OrgID, &r.DocType, &r.DocID, &r.Revision, &r.DrafterID, &r.Title, &r.Status, &r.SnapshotJSON, &r.SubmittedAt, &r.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (e *Engine) ListSteps(requestID string) ([]Step, error) {
	rows, err := e.DB.Query(`SELECT step_id, request_id, step_no, step_type, approver_id, COALESCE(acted_by,''), COALESCE(act_mode,''), status, COALESCE(comment,''), COALESCE(acted_at,'')
		FROM approval_steps WHERE request_id=? ORDER BY step_no`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Step
	for rows.Next() {
		var s Step
		if err := rows.Scan(&s.StepID, &s.RequestID, &s.StepNo, &s.StepType, &s.ApproverID, &s.ActedBy, &s.ActMode, &s.Status, &s.Comment, &s.ActedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (e *Engine) EventTypes(requestID string) ([]string, error) {
	rows, err := e.DB.Query(`SELECT event_type FROM approval_events WHERE request_id=? ORDER BY rowid`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type scanner interface {
	QueryRow(query string, args ...any) *sql.Row
}

func loadRequestTx(ex scanner, id string) (Request, error) {
	var r Request
	err := ex.QueryRow(`SELECT request_id, org_id, doc_type, doc_id, revision, drafter_id, title, status, COALESCE(snapshot_json,''), COALESCE(submitted_at,''), COALESCE(completed_at,'')
		FROM approval_requests WHERE request_id=?`, id).Scan(
		&r.RequestID, &r.OrgID, &r.DocType, &r.DocID, &r.Revision, &r.DrafterID, &r.Title, &r.Status, &r.SnapshotJSON, &r.SubmittedAt, &r.CompletedAt)
	if err == sql.ErrNoRows {
		return r, fmt.Errorf("결재 요청이 없다")
	}
	return r, err
}

func pendingApproveTx(tx *sql.Tx, requestID string) (Step, error) {
	var s Step
	err := tx.QueryRow(`SELECT step_id, request_id, step_no, step_type, approver_id, COALESCE(acted_by,''), COALESCE(act_mode,''), status
		FROM approval_steps WHERE request_id=? AND step_type=? AND status=?`, requestID, TypeApprove, StepPending).Scan(
		&s.StepID, &s.RequestID, &s.StepNo, &s.StepType, &s.ApproverID, &s.ActedBy, &s.ActMode, &s.Status)
	if err == sql.ErrNoRows {
		return s, fmt.Errorf("처리할 단계가 없다")
	}
	return s, err
}

func nextWaitingApproveTx(tx *sql.Tx, requestID string) (string, error) {
	var id string
	err := tx.QueryRow(`SELECT step_id FROM approval_steps WHERE request_id=? AND step_type=? AND status=? ORDER BY step_no LIMIT 1`,
		requestID, TypeApprove, StepWaiting).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func insertEvent(tx *sql.Tx, requestID, typ, actor, detail string) error {
	_, err := tx.Exec(`INSERT INTO approval_events(event_id, request_id, event_type, actor_id, detail_json) VALUES (?,?,?,?,?)`,
		"AE-"+uuid.NewString(), requestID, typ, actor, detail)
	return err
}

func commentJSON(c, reason string) string {
	return `{"comment":"` + strings.ReplaceAll(c, `"`, `'`) + `","reason":"` + strings.ReplaceAll(reason, `"`, `'`) + `"}`
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func AllowedTransition(from, action string) bool {
	switch from {
	case StatusDraft:
		return action == EvSubmit
	case StatusInProgress:
		return action == EvApprove || action == EvReject || action == EvWithdraw || action == EvSelfApprove
	case StatusRejected, StatusWithdrawn:
		return action == EvResubmit
	default:
		return false
	}
}
