package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

// 95-225: 업무 배정. 하부 업무·사이트·하부 작업·AS 하부업무를 다른 사용자(타 조직 포함)에게 요청하고 수락/거부/보류로 처리한다.
// 협조(work_cooperations, 조직 단위 요청)와는 별개다. 공동 작업(풀 공유)과도 별개다.

const (
	AssignPending  = "pending"
	AssignAccepted = "accepted"
	AssignHeld     = "held"
	AssignRejected = "rejected"

	AssignTargetSub     = "sub"     // 하부 업무(phase_packages)
	AssignTargetSite    = "site"    // 사이트(work_task_sites)
	AssignTargetSubtask = "subtask" // 하부 작업(work_tasks.parent_task_id)
	AssignTargetASWork  = "as_work" // AS 하부업무(as_work_items)
)

var (
	ErrAssignTransition = errors.New("이 배정은 더 이상 처리할 수 없는 상태입니다")
	ErrAssignReason     = errors.New("거부 사유를 입력하세요")
	ErrAssignNotMine    = errors.New("내게 배정된 업무가 아닙니다")
	ErrAssignClosed     = errors.New("대상이 이미 종료되어 배정을 수락할 수 없습니다. 배정은 거부 처리되었습니다")
	ErrAssignInvalid    = errors.New("배정할 수 없는 대상입니다")
)

func applyWorkAssignmentsV118(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("118_work_assignments.sql")
	if err != nil {
		log.Printf("118 work_assignments: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("118 work_assignments apply: %v", err)
	}
}

type AssignmentRepo struct{ db *sql.DB }

func NewAssignmentRepo(db *sql.DB) *AssignmentRepo { return &AssignmentRepo{db: db} }

// AssignmentItem 팝업·보류 카드·상세 표시용(수락 전에는 이 최소 정보만 보인다).
type AssignmentItem struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	TargetType   string `json:"target_type"`
	TargetID     string `json:"target_id"`
	TargetLabel  string `json:"target_label"`
	ProjectID    string `json:"project_id,omitempty"`
	ProjectName  string `json:"project_name"`
	ActivityKind string `json:"activity_kind,omitempty"`
	CategoryName string `json:"category_name"` // 사업이 없을 때 업무 구분 이름
	FromUserID   string `json:"from_user_id"`
	FromName     string `json:"from_name"`
	FromOrgName  string `json:"from_org_name"`
	ToUserID     string `json:"to_user_id"`
	ToName       string `json:"to_name"`
	Status       string `json:"status"`
	RejectReason string `json:"reject_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	DecidedAt    string `json:"decided_at,omitempty"`
	TaskTitle    string `json:"task_title,omitempty"`
}

// Heading 「어떤 사업(또는 업무 구분)」 표시.
func (a AssignmentItem) Heading() string {
	if a.ProjectName != "" {
		return a.ProjectName
	}
	if a.CategoryName != "" {
		return a.CategoryName
	}
	return "(사업·구분 없음)"
}

func (r *AssignmentRepo) nextID() (string, error) {
	n, err := NextSeq(r.db, "work_assignment")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("WAS-%04d", n), nil
}

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

// targetInfo 대상이 업무에 속하는지·열려 있는지 확인하고 표시 이름을 돌려준다.
func (r *AssignmentRepo) targetInfo(taskID, targetType, targetID string) (label string, open bool, err error) {
	switch targetType {
	case AssignTargetSub:
		var name string
		var active int
		var pkg string
		e := r.db.QueryRow(`SELECT COALESCE(package_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&pkg)
		if e != nil || pkg == "" || pkg != targetID {
			return "", false, ErrAssignInvalid
		}
		if e := r.db.QueryRow(`SELECT name, COALESCE(is_active,1) FROM phase_packages WHERE package_id=?`, targetID).Scan(&name, &active); e != nil {
			return "", false, ErrAssignInvalid
		}
		var done string
		_ = r.db.QueryRow(`SELECT COALESCE(done_at,'') FROM phase_packages WHERE package_id=?`, targetID).Scan(&done)
		return name, active == 1 && done == "", nil
	case AssignTargetSite:
		var name string
		e := r.db.QueryRow(`SELECT COALESCE(NULLIF(TRIM(c.org_name),''), c.customer_id) FROM work_task_sites w JOIN customers c ON c.customer_id=w.customer_id
			WHERE w.task_id=? AND w.customer_id=?`, taskID, targetID).Scan(&name)
		if e != nil {
			return "", false, ErrAssignInvalid
		}
		var n int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_task_site_progress WHERE task_id=? AND customer_id=? AND status IN ('complete','done')`, taskID, targetID).Scan(&n)
		return name, n == 0, nil
	case AssignTargetSubtask:
		var title, st, parent string
		if e := r.db.QueryRow(`SELECT title, COALESCE(status,''), COALESCE(parent_task_id,'') FROM work_tasks WHERE task_id=?`, targetID).Scan(&title, &st, &parent); e != nil || parent != taskID {
			return "", false, ErrAssignInvalid
		}
		return title, st != model.WBTaskComplete && st != "cancelled", nil
	case AssignTargetASWork:
		var num, st, asID string
		if e := r.db.QueryRow(`SELECT COALESCE(work_number,work_id), COALESCE(status,''), COALESCE(as_id,'') FROM as_work_items WHERE work_id=?`, targetID).Scan(&num, &st, &asID); e != nil {
			return "", false, ErrAssignInvalid
		}
		var linked int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE task_id=? AND source_type IN ('as','as_work')`, taskID).Scan(&linked)
		if linked == 0 {
			return "", false, ErrAssignInvalid
		}
		return num, st != "done" && st != "complete" && st != "cancelled", nil
	}
	return "", false, ErrAssignInvalid
}

// Create 배정 요청을 만든다(등록 시점). 대상 소유 업무가 이미 닫혔거나 같은 대상에 진행 중인 배정이 있으면 거절한다.
func (r *AssignmentRepo) Create(taskID, targetType, targetID, fromUserID, toUserID string) (*AssignmentItem, error) {
	taskID, targetType, targetID = strings.TrimSpace(taskID), strings.TrimSpace(targetType), strings.TrimSpace(targetID)
	fromUserID, toUserID = strings.TrimSpace(fromUserID), strings.TrimSpace(toUserID)
	if taskID == "" || targetID == "" || fromUserID == "" || toUserID == "" {
		return nil, ErrAssignInvalid
	}
	if fromUserID == toUserID {
		return nil, fmt.Errorf("자신에게는 배정할 수 없습니다")
	}
	var active int
	if err := r.db.QueryRow(`SELECT COALESCE(is_active,1) FROM users WHERE user_id=?`, toUserID).Scan(&active); err != nil || active != 1 {
		return nil, fmt.Errorf("배정받을 사용자를 확인하세요")
	}
	var projectID, activity string
	if err := r.db.QueryRow(`SELECT COALESCE(project_id,''), COALESCE(activity_kind,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&projectID, &activity); err != nil {
		return nil, ErrAssignInvalid
	}
	label, open, err := r.targetInfo(taskID, targetType, targetID)
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, ErrAssignClosed
	}
	var pname string
	if projectID != "" {
		pname = workProjectName(r.db, projectID)
	}
	id, err := r.nextID()
	if err != nil {
		return nil, err
	}
	if _, err := r.db.Exec(`INSERT INTO work_assignments(id,task_id,target_type,target_id,target_label,project_id,project_name,activity_kind,from_user_id,to_user_id,status,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, id, taskID, targetType, targetID, label, projectID, pname, activity, fromUserID, toUserID, AssignPending, nowStr()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("이 대상에는 이미 진행 중인 배정이 있습니다")
		}
		return nil, err
	}
	r.logTransition(id, "", AssignPending, fromUserID, "배정 요청: "+label+" → "+toUserID)
	return r.Get(id)
}

func (r *AssignmentRepo) logTransition(id, before, after, actor, reason string) {
	audit.Use(r.db)
	audit.LogWithReason(audit.ActionUpdate, "work_assignments", "id", id, "업무 배정 "+id,
		fmt.Sprintf(`{"status":%q}`, before), fmt.Sprintf(`{"status":%q,"actor":%q}`, after, actor), reason)
}

const assignmentSelect = `
	SELECT a.id, a.task_id, a.target_type, a.target_id, a.target_label, a.project_id, a.project_name, a.activity_kind,
	       a.from_user_id, COALESCE(fu.full_name,''), COALESCE(fo.org_name,''),
	       a.to_user_id, COALESCE(tu.full_name,''), a.status, a.reject_reason, a.created_at, a.decided_at,
	       COALESCE(t.title,'')
	  FROM work_assignments a
	  LEFT JOIN users fu ON fu.user_id=a.from_user_id
	  LEFT JOIN orgs fo ON fo.org_id=fu.org_id
	  LEFT JOIN users tu ON tu.user_id=a.to_user_id
	  LEFT JOIN work_tasks t ON t.task_id=a.task_id`

func scanAssignments(rows *sql.Rows) ([]AssignmentItem, error) {
	defer rows.Close()
	var out []AssignmentItem
	for rows.Next() {
		var a AssignmentItem
		if err := rows.Scan(&a.ID, &a.TaskID, &a.TargetType, &a.TargetID, &a.TargetLabel, &a.ProjectID, &a.ProjectName, &a.ActivityKind,
			&a.FromUserID, &a.FromName, &a.FromOrgName, &a.ToUserID, &a.ToName, &a.Status, &a.RejectReason, &a.CreatedAt, &a.DecidedAt, &a.TaskTitle); err != nil {
			return nil, err
		}
		if a.ProjectName == "" {
			a.CategoryName = model.WeeklyActivityLabel(a.ActivityKind)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AssignmentRepo) Get(id string) (*AssignmentItem, error) {
	rows, err := r.db.Query(assignmentSelect+` WHERE a.id=?`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanAssignments(rows)
	if err != nil || len(list) == 0 {
		return nil, ErrAssignInvalid
	}
	return &list[0], nil
}

// ListForUser 내게 배정된 항목(상태 필터). 보류 그룹 카드·수락 목록에 쓴다.
func (r *AssignmentRepo) ListForUser(userID string, statuses ...string) ([]AssignmentItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}
	q := assignmentSelect + ` WHERE a.to_user_id=?`
	args := []interface{}{userID}
	if len(statuses) > 0 {
		q += ` AND a.status IN (` + placeholders(len(statuses)) + `)`
		args = append(args, iface(statuses)...)
	}
	q += ` ORDER BY a.created_at DESC, a.id DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	return scanAssignments(rows)
}

// ListForTask 업무에 걸린 배정(배정자가 결과·거부 사유를 보는 용도).
func (r *AssignmentRepo) ListForTask(taskID string) ([]AssignmentItem, error) {
	rows, err := r.db.Query(assignmentSelect+` WHERE a.task_id=? ORDER BY a.created_at, a.id`, taskID)
	if err != nil {
		return nil, err
	}
	return scanAssignments(rows)
}

// CountOpenForUser 내 대기(pending)·보류(held) 건수.
func (r *AssignmentRepo) CountOpenForUser(userID string) (pending, held int) {
	if strings.TrimSpace(userID) == "" {
		return 0, 0
	}
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(status='pending'),0), COALESCE(SUM(status='held'),0) FROM work_assignments WHERE to_user_id=?`, userID).Scan(&pending, &held)
	return
}

// PopupItems 팝업에 띄울 항목. 세션 첫 확인(first)이면 대기+보류 전부, 이후에는 대기 중인 것만. 돌려준 항목은 last_notified_at 을 찍는다.
func (r *AssignmentRepo) PopupItems(userID string, first bool) ([]AssignmentItem, error) {
	st := []string{AssignPending}
	if first {
		st = append(st, AssignHeld)
	}
	list, err := r.ListForUser(userID, st...)
	if err != nil {
		return nil, err
	}
	now := nowStr()
	for _, a := range list {
		_, _ = r.db.Exec(`UPDATE work_assignments SET last_notified_at=? WHERE id=?`, now, a.ID)
	}
	return list, nil
}

// Decide 수락(accept)·거부(reject, 사유 필수)·보류(hold). 허용 전이: pending→모두, held→accept/reject.
func (r *AssignmentRepo) Decide(id, userID, action, reason string) (*AssignmentItem, error) {
	a, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if a.ToUserID != strings.TrimSpace(userID) {
		return nil, ErrAssignNotMine
	}
	var next string
	switch action {
	case "accept":
		next = AssignAccepted
	case "reject":
		next = AssignRejected
	case "hold":
		next = AssignHeld
	default:
		return nil, ErrAssignTransition
	}
	if !(a.Status == AssignPending || (a.Status == AssignHeld && next != AssignHeld)) {
		return nil, ErrAssignTransition
	}
	reason = strings.TrimSpace(reason)
	if next == AssignRejected && reason == "" {
		return nil, ErrAssignReason
	}
	if next == AssignAccepted {
		_, open, terr := r.targetInfo(a.TaskID, a.TargetType, a.TargetID)
		if terr != nil || !open {
			// 이미 종결된 대상: 자동 거부 + 안내
			auto := "대상이 이미 종료되어 자동 거부됨"
			_, _ = r.db.Exec(`UPDATE work_assignments SET status=?, reject_reason=?, decided_at=? WHERE id=?`, AssignRejected, auto, nowStr(), id)
			r.logTransition(id, a.Status, AssignRejected, userID, auto)
			return nil, ErrAssignClosed
		}
		if err := r.applyAccept(a, userID); err != nil {
			return nil, err
		}
	}
	rj := ""
	if next == AssignRejected {
		rj = reason
	}
	if _, err := r.db.Exec(`UPDATE work_assignments SET status=?, reject_reason=?, decided_at=? WHERE id=?`, next, rj, nowStr(), id); err != nil {
		return nil, err
	}
	why := map[string]string{AssignAccepted: "수락", AssignHeld: "보류", AssignRejected: "거부: " + rj}[next]
	r.logTransition(id, a.Status, next, userID, why)
	return r.Get(id)
}

// applyAccept 수락 효과: 하부 작업·AS 하부업무는 담당자를 수락자로 바꾼다. 하부 업무·사이트는 배정 행 자체가 담당 연결이다.
func (r *AssignmentRepo) applyAccept(a *AssignmentItem, userID string) error {
	var name, userOrg string
	_ = r.db.QueryRow(`SELECT COALESCE(full_name,''), COALESCE(org_id,'') FROM users WHERE user_id=?`, userID).Scan(&name, &userOrg)
	// 95-215 규칙: 업무 조직 밖 사용자를 담당자 열에 쓰지 않는다. 타 조직 수락자는 배정 행이 담당 연결이다(카드·범위 화면으로 처리).
	var taskOrg string
	switch a.TargetType {
	case AssignTargetSubtask:
		_ = r.db.QueryRow(`SELECT COALESCE(org_id,'') FROM work_tasks WHERE task_id=?`, a.TargetID).Scan(&taskOrg)
	case AssignTargetASWork:
		_ = r.db.QueryRow(`SELECT COALESCE(ar.org_id,'') FROM as_work_items w JOIN as_receipts ar ON ar.as_id=w.as_id WHERE w.work_id=?`, a.TargetID).Scan(&taskOrg)
	}
	if strings.TrimSpace(taskOrg) != "" && strings.TrimSpace(userOrg) != strings.TrimSpace(taskOrg) {
		return nil
	}
	switch a.TargetType {
	case AssignTargetSubtask:
		_, err := r.db.Exec(`UPDATE work_tasks SET assignee=?, assignee_user_id=?, updated_at=datetime('now','localtime') WHERE task_id=?`, name, userID, a.TargetID)
		return err
	case AssignTargetASWork:
		_, err := r.db.Exec(`UPDATE as_work_items SET assigned_to=?, assigned_user_id=?, updated_at=datetime('now','localtime') WHERE work_id=?`, name, userID, a.TargetID)
		return err
	}
	return nil
}

// ReturnForUser 사용자를 비활성화할 때: 그 사용자에게 대기·보류 중인 배정을 배정자에게 되돌린다(거부 처리, 사유 기록). 되돌린 건수.
func (r *AssignmentRepo) ReturnForUser(userID string) int {
	list, err := r.ListForUser(userID, AssignPending, AssignHeld)
	if err != nil {
		return 0
	}
	n := 0
	for _, a := range list {
		reason := "배정받은 사용자가 비활성화되어 반환됨"
		if _, err := r.db.Exec(`UPDATE work_assignments SET status=?, reject_reason=?, decided_at=? WHERE id=?`, AssignRejected, reason, nowStr(), a.ID); err == nil {
			r.logTransition(a.ID, a.Status, AssignRejected, "system", reason)
			n++
		}
	}
	return n
}

// AcceptedScope 수락한 배정이 이 업무의 이 대상을 덮는지(수락자의 범위 판정). 사이트 체크·하부 업무 처리 권한에 쓴다.
func (r *AssignmentRepo) AcceptedScope(userID, taskID, targetType, targetID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_assignments WHERE to_user_id=? AND task_id=? AND target_type=? AND target_id=? AND status='accepted'`,
		userID, taskID, targetType, targetID).Scan(&n)
	return n > 0
}

// HasAcceptedOnTask 수락한 배정이 이 업무에 하나라도 있는지(업무 화면을 배정 범위만 열 때).
func (r *AssignmentRepo) HasAcceptedOnTask(userID, taskID string) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_assignments WHERE to_user_id=? AND task_id=? AND status='accepted'`, userID, taskID).Scan(&n)
	return n > 0
}

// OrgNames 조직 ID → 조직명(배정 대상 선택 목록 표시용).
func (r *AssignmentRepo) OrgNames() map[string]string {
	out := map[string]string{}
	rows, err := r.db.Query(`SELECT org_id, COALESCE(org_name,'') FROM orgs`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, nm string
		if rows.Scan(&id, &nm) == nil {
			out[id] = nm
		}
	}
	return out
}

// 공동 작업(95-225 §1): work_tasks.collab_scope = ”(없음) | 'org'(같은 조직 누구나) | 'members'(지정한 사용자만, 타 조직 포함).
const (
	CollabNone    = ""
	CollabOrg     = "org"
	CollabMembers = "members"
)

// CollabOf 업무의 공동 작업 범위와 지정 구성원.
func (r *AssignmentRepo) CollabOf(taskID string) (scope string, userIDs []string) {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return "", nil
	}
	_ = r.db.QueryRow(`SELECT COALESCE(collab_scope,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&scope)
	if scope != CollabMembers {
		return scope, nil
	}
	rows, err := r.db.Query(`SELECT user_id FROM work_collab_members WHERE task_id=? ORDER BY user_id`, taskID)
	if err != nil {
		return scope, nil
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			userIDs = append(userIDs, id)
		}
	}
	return scope, userIDs
}

// SetCollab 공동 작업 범위를 정하거나(해제는 scope="") 바꾼다. members 는 한 명 이상, 활성 사용자여야 한다.
func (r *AssignmentRepo) SetCollab(taskID, scope string, userIDs []string, actor string) error {
	taskID, scope = strings.TrimSpace(taskID), strings.TrimSpace(scope)
	if scope != CollabNone && scope != CollabOrg && scope != CollabMembers {
		return fmt.Errorf("공동 작업 범위가 올바르지 않습니다")
	}
	var before string
	if err := r.db.QueryRow(`SELECT COALESCE(collab_scope,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&before); err != nil {
		return ErrAssignInvalid
	}
	var clean []string
	seen := map[string]bool{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		var active int
		if err := r.db.QueryRow(`SELECT COALESCE(is_active,1) FROM users WHERE user_id=?`, id).Scan(&active); err != nil || active != 1 {
			return fmt.Errorf("공동 작업 구성원을 확인하세요")
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if scope == CollabMembers && len(clean) == 0 {
		return fmt.Errorf("지정한 조직원을 한 명 이상 고르세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE work_tasks SET collab_scope=NULLIF(?,''), updated_at=datetime('now','localtime') WHERE task_id=?`, scope, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM work_collab_members WHERE task_id=?`, taskID); err != nil {
		return err
	}
	if scope == CollabMembers {
		for _, id := range clean {
			if _, err := tx.Exec(`INSERT INTO work_collab_members(task_id,user_id,added_by,added_at) VALUES (?,?,?,?)`, taskID, id, actor, nowStr()); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if before != scope {
		audit.Use(r.db)
		audit.LogWithReason(audit.ActionUpdate, "work_tasks", "task_id", taskID, "공동 작업 범위",
			fmt.Sprintf(`{"collab_scope":%q}`, before), fmt.Sprintf(`{"collab_scope":%q}`, scope), "공동 작업 범위 변경: "+before+" → "+scope)
	}
	return nil
}

// TaskMeta 권한 판정용 업무 정보: 등록자 사용자 ID, 하부 업무(package) ID.
func (r *AssignmentRepo) TaskMeta(taskID string) (creatorUserID, packageID string) {
	_ = r.db.QueryRow(`SELECT COALESCE(created_by_user_id,''), COALESCE(package_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&creatorUserID, &packageID)
	return
}

// AcceptedCard 수락한 배정 1건의 일정표·칸반 표시 정보. 부모 업무의 날짜·시간을 따르되 수락자의 카드로 보인다.
type AcceptedCard struct {
	AssignmentID string
	TaskID       string
	TargetType   string
	TargetLabel  string
	Heading      string // 사업명 또는 업무 구분 이름
	ToUserID     string
	ToName       string
	ToOrgID      string
	FromName     string
	WorkDate     string
	DueDate      string
	StartTime    string
	EndTime      string
	DurationMin  int
	Status       string // 부모 업무 상태
}

// ListAcceptedCards from~to(포함) 사이에 걸리는 수락된 배정. orgID 가 비어 있거나 OrgAll 이면 조직 제한 없이, 아니면 수락자의 소속 조직 기준이다.
// 부모 업무가 열려 있는지는 호출측 화면 규칙을 따른다(완료·취소도 날짜가 맞으면 보인다).
func (r *AssignmentRepo) ListAcceptedCards(from, to, orgID string) []AcceptedCard {
	q := `SELECT a.id, a.task_id, a.target_type, a.target_label, COALESCE(NULLIF(a.project_name,''), a.activity_kind),
	             a.to_user_id, COALESCE(tu.full_name,''), COALESCE(tu.org_id,''), COALESCE(fu.full_name,''),
	             COALESCE(t.work_date,''), COALESCE(NULLIF(t.due_date,''), t.work_date), COALESCE(t.start_time,''), COALESCE(t.end_time,''),
	             COALESCE(t.duration_min,0), COALESCE(t.status,'')
	        FROM work_assignments a
	        JOIN work_tasks t ON t.task_id=a.task_id
	        LEFT JOIN users tu ON tu.user_id=a.to_user_id
	        LEFT JOIN users fu ON fu.user_id=a.from_user_id
	       WHERE a.status='accepted' AND COALESCE(t.work_date,'')<>'' AND t.work_date<=? AND COALESCE(NULLIF(t.due_date,''), t.work_date)>=?`
	args := []interface{}{to, from}
	if orgID = strings.TrimSpace(orgID); orgID != "" && orgID != OrgAll {
		q += ` AND tu.org_id=?`
		args = append(args, orgID)
	}
	q += ` ORDER BY t.work_date, t.start_time, a.id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AcceptedCard
	for rows.Next() {
		var c AcceptedCard
		var heading string
		if rows.Scan(&c.AssignmentID, &c.TaskID, &c.TargetType, &c.TargetLabel, &heading, &c.ToUserID, &c.ToName, &c.ToOrgID, &c.FromName,
			&c.WorkDate, &c.DueDate, &c.StartTime, &c.EndTime, &c.DurationMin, &c.Status) != nil {
			continue
		}
		c.Heading = heading
		if l := model.WeeklyActivityLabel(heading); l != "" {
			c.Heading = l
		}
		out = append(out, c)
	}
	return out
}
