package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type CoopRepo struct {
	db     *sql.DB
	users  *UserRepo
	orgs   *OrgRepo
	groups *WorkGroupRepo
	wb     *WBRepo
	as     *ASRepo
	proj   *ProjectRepo
	cust   *CustomerRepo
}

func NewCoopRepo(db *sql.DB) *CoopRepo {
	return &CoopRepo{
		db: db, users: NewUserRepo(db), orgs: NewOrgRepo(db), groups: NewWorkGroupRepo(db),
		wb: NewWBRepo(db), as: NewASRepo(db), proj: NewProjectRepo(db), cust: NewCustomerRepo(db),
	}
}

var (
	errCoopForbidden = fmt.Errorf("forbidden")
	errCoopNotFound  = fmt.Errorf("not found")
)

func IsCoopForbidden(err error) bool { return err == errCoopForbidden }
func IsCoopNotFound(err error) bool  { return err == errCoopNotFound }

func (r *CoopRepo) orgDeputy(orgID, today string) string {
	var head string
	_ = r.db.QueryRow(`SELECT user_id FROM employee_profiles WHERE org_id=? AND is_org_head=1`, orgID).Scan(&head)
	if head == "" {
		return ""
	}
	var to string
	err := r.db.QueryRow(`SELECT to_user_id FROM approval_delegations
		WHERE from_user_id=? AND revoked_at IS NULL AND start_date<=? AND end_date>=?
		ORDER BY created_at DESC LIMIT 1`, head, today, today).Scan(&to)
	if err != nil {
		return ""
	}
	return to
}

func (r *CoopRepo) CanInbox(a model.CoopActor, targetOrg string) bool {
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleCLevel {
		return true
	}
	if (rle == model.RoleOrgAdmin || rle == model.RoleAdmin) && a.HomeOrg() == targetOrg {
		return true
	}
	return a.UserID != "" && a.UserID == r.orgDeputy(targetOrg, todayYMD())
}

func (r *CoopRepo) CanActInbox(a model.CoopActor, targetOrg string) bool {
	if !a.CanWrite() {
		return false
	}
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin {
		return true
	}
	if (rle == model.RoleOrgAdmin || rle == model.RoleAdmin) && a.HomeOrg() == targetOrg {
		return true
	}
	return a.UserID != "" && a.UserID == r.orgDeputy(targetOrg, todayYMD())
}

func (r *CoopRepo) CanSee(a model.CoopActor, c *model.Cooperation) bool {
	if c == nil {
		return false
	}
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleCLevel {
		return true
	}
	if c.ReqUserID == a.UserID {
		return true
	}
	if (rle == model.RoleOrgAdmin || rle == model.RoleAdmin) && a.HomeOrg() == c.ReqOrgID {
		return true
	}
	if r.CanInbox(a, c.TargetOrgID) {
		return true
	}
	for _, asg := range c.Assignees {
		if asg.UserID == a.UserID {
			return true
		}
	}
	return false
}

func (r *CoopRepo) Get(id string) (*model.Cooperation, error) {
	row := r.db.QueryRow(`SELECT coop_id, req_org_id, req_user_id, target_org_id, kind, title, body,
		COALESCE(project_id,''), COALESCE(want_start,''), COALESCE(want_end,''), status,
		COALESCE(reject_reason,''), COALESCE(plan_start,''), COALESCE(plan_end,''), instruction,
		COALESCE(work_group_id,''), COALESCE(decided_by,''), COALESCE(decided_at,''), COALESCE(completed_reason,''), created_at
		FROM work_cooperations WHERE coop_id=?`, id) // coop-scope: PK 단건, 열람은 CanSee
	var c model.Cooperation
	if err := row.Scan(&c.CoopID, &c.ReqOrgID, &c.ReqUserID, &c.TargetOrgID, &c.Kind, &c.Title, &c.Body,
		&c.ProjectID, &c.WantStart, &c.WantEnd, &c.Status, &c.RejectReason, &c.PlanStart, &c.PlanEnd,
		&c.Instruction, &c.WorkGroupID, &c.DecidedBy, &c.DecidedAt, &c.CompletedReason, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.decorate(&c)
	return &c, nil
}

func (r *CoopRepo) decorate(c *model.Cooperation) {
	if o, _ := r.orgs.Get(c.ReqOrgID); o != nil {
		c.ReqOrgName = o.OrgName
	}
	if o, _ := r.orgs.Get(c.TargetOrgID); o != nil {
		c.TargetOrgName = o.OrgName
	}
	if u, _ := r.users.GetByID(c.ReqUserID); u != nil {
		c.ReqUserName = u.FullName
	}
	if c.WorkGroupID != "" {
		if g, _ := r.groups.Get(c.WorkGroupID); g != nil {
			c.WorkGroupName = g.Name
		}
	}
	if c.ProjectID != "" && r.proj != nil {
		if p, err := r.proj.Get(c.ProjectID); err == nil && p != nil {
			c.ProjectName = p.Name
		}
	}
	c.Assignees, _ = r.listAssignees(c.CoopID)
	day := todayYMD()
	created := c.CreatedAt
	if len(created) >= 10 {
		created = created[:10]
	}
	if c.Status == model.CoopRequested {
		if n := coopWorkingDays(created, day); n > 3 {
			c.Overdue = true
		}
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE cooperation_id=? AND coop_incomplete=1`, c.CoopID).Scan(&n)
	c.IncompleteAS = n > 0
}

func (r *CoopRepo) listAssignees(id string) ([]model.CoopAssignee, error) {
	rows, err := r.db.Query(`SELECT user_id, role FROM cooperation_assignees WHERE cooperation_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CoopAssignee
	for rows.Next() {
		var a model.CoopAssignee
		if err := rows.Scan(&a.UserID, &a.Role); err != nil {
			return nil, err
		}
		if u, _ := r.users.GetByID(a.UserID); u != nil {
			a.FullName = u.FullName
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *CoopRepo) ListInbox(a model.CoopActor) ([]model.Cooperation, error) {
	q := `SELECT coop_id FROM work_cooperations WHERE 1=1`
	var args []any
	rle := model.NormalizeRole(a.Role)
	if rle != model.RoleVisionAdmin && rle != model.RoleCLevel {
		q += ` AND target_org_id=?` // coop-scope: 받은 협조는 대상 조직
		args = append(args, a.HomeOrg())
		if !r.CanInbox(a, a.HomeOrg()) {
			return nil, nil
		}
	}
	q += ` ORDER BY created_at DESC`
	return r.listIDs(q, args...)
}

func (r *CoopRepo) ListSent(a model.CoopActor) ([]model.Cooperation, error) {
	q := `SELECT coop_id FROM work_cooperations WHERE 1=1`
	var args []any
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleCLevel {
		// coop-scope: 비전·C-Level 전 조직 보낸 협조
	} else if rle == model.RoleOrgAdmin || rle == model.RoleAdmin {
		q += ` AND req_org_id=?` // coop-scope: 요청 조직 전체
		args = append(args, a.HomeOrg())
	} else {
		q += ` AND req_user_id=?` // coop-scope: 요청자 본인
		args = append(args, a.UserID)
	}
	q += ` ORDER BY created_at DESC`
	return r.listIDs(q, args...)
}

func (r *CoopRepo) listIDs(q string, args ...any) ([]model.Cooperation, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Cooperation
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		c, err := r.Get(id)
		if err != nil || c == nil {
			continue
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *CoopRepo) InboxCount(a model.CoopActor) int {
	items, err := r.ListInbox(a)
	if err != nil {
		return 0
	}
	n := 0
	for _, it := range items {
		if it.Status == model.CoopRequested || it.Status == model.CoopAccepted {
			n++
		}
	}
	return n
}

func (r *CoopRepo) Create(a model.CoopActor, c *model.Cooperation) error {
	if !a.CanRequest() {
		return errCoopForbidden
	}
	c.Title = strings.TrimSpace(c.Title)
	c.Body = strings.TrimSpace(c.Body)
	if c.Title == "" || c.Body == "" || c.TargetOrgID == "" {
		return fmt.Errorf("대상 팀·제목·내용은 필수입니다")
	}
	c.ReqOrgID = a.HomeOrg()
	if c.ReqOrgID == c.TargetOrgID {
		return fmt.Errorf("자기 팀에는 요청할 수 없습니다")
	}
	if c.WantStart != "" && c.WantEnd != "" && c.WantEnd < c.WantStart {
		return fmt.Errorf("희망 종료가 시작보다 앞입니다")
	}
	switch c.Kind {
	case model.CoopAS, model.CoopInspection, model.CoopAdmin, model.CoopProject:
	default:
		return fmt.Errorf("구분을 고르세요")
	}
	n, err := NextSeq(r.db, "work_coop")
	if err != nil {
		return err
	}
	c.CoopID = fmt.Sprintf("COP-%04d", n)
	c.ReqUserID = a.UserID
	c.Status = model.CoopRequested
	_, err = r.db.Exec(`INSERT INTO work_cooperations(coop_id,req_org_id,req_user_id,target_org_id,kind,title,body,project_id,want_start,want_end,status)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		c.CoopID, c.ReqOrgID, c.ReqUserID, c.TargetOrgID, c.Kind, c.Title, c.Body, nullIfEmpty(c.ProjectID), nullIfEmpty(c.WantStart), nullIfEmpty(c.WantEnd), c.Status)
	if err != nil {
		return err
	}
	r.event(c.CoopID, a.UserID, "요청", "", "")
	return nil
}

func (r *CoopRepo) Cancel(a model.CoopActor, id string) error {
	c, err := r.must(id)
	if err != nil {
		return err
	}
	if c.ReqUserID != a.UserID || !a.CanRequest() {
		return errCoopForbidden
	}
	if c.Status != model.CoopRequested {
		return fmt.Errorf("수락 전에만 취소할 수 있습니다")
	}
	return r.setStatus(a, c, model.CoopCancelled, "", "")
}

func (r *CoopRepo) Reject(a model.CoopActor, id, reason, onBehalf string) error {
	c, err := r.must(id)
	if err != nil {
		return err
	}
	if !r.CanActInbox(a, c.TargetOrgID) {
		return errCoopForbidden
	}
	if c.Status != model.CoopRequested {
		return fmt.Errorf("요청됨만 거부할 수 있습니다")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("거부 사유를 입력하세요")
	}
	if _, err := r.db.Exec(`UPDATE work_cooperations SET status=?, reject_reason=?, decided_by=?, decided_at=?, updated_at=CURRENT_TIMESTAMP WHERE coop_id=?`,
		model.CoopRejected, reason, a.UserID, time.Now().Format("2006-01-02 15:04:05"), id); err != nil {
		return err
	}
	r.event(id, a.UserID, "거부", reason, onBehalf)
	return nil
}

func (r *CoopRepo) Accept(a model.CoopActor, id, onBehalf string) error {
	c, err := r.must(id)
	if err != nil {
		return err
	}
	if !r.CanActInbox(a, c.TargetOrgID) {
		return errCoopForbidden
	}
	if c.Status != model.CoopRequested {
		return fmt.Errorf("요청됨만 수락할 수 있습니다")
	}
	return r.setStatus(a, c, model.CoopAccepted, onBehalf, "수락")
}

func (r *CoopRepo) Complete(a model.CoopActor, id, reason, onBehalf string) error {
	c, err := r.must(id)
	if err != nil {
		return err
	}
	if !r.CanActInbox(a, c.TargetOrgID) {
		return errCoopForbidden
	}
	if c.Status != model.CoopAccepted && c.Status != model.CoopAssigned {
		return fmt.Errorf("수락·배정됨만 완료할 수 있습니다")
	}
	open := r.openLinked(id)
	if open > 0 && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("미완료 업무가 있으면 사유가 필요합니다")
	}
	if _, err := r.db.Exec(`UPDATE work_cooperations SET status=?, completed_reason=?, decided_by=?, decided_at=?, updated_at=CURRENT_TIMESTAMP WHERE coop_id=?`,
		model.CoopCompleted, strings.TrimSpace(reason), a.UserID, time.Now().Format("2006-01-02 15:04:05"), id); err != nil {
		return err
	}
	r.event(id, a.UserID, "완료", reason, onBehalf)
	return nil
}

func (r *CoopRepo) OpenLinked(id string) int { return r.openLinked(id) }

func (r *CoopRepo) openLinked(id string) int {
	var n int
	_ = r.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM work_tasks WHERE cooperation_id=? AND status NOT IN ('complete','cancelled')) +
		(SELECT COUNT(*) FROM as_receipts WHERE cooperation_id=? AND status NOT IN ('completed','cancelled')) +
		(SELECT COUNT(*) FROM maintenance_visits WHERE cooperation_id=? AND COALESCE(completed,0)=0)`,
		id, id, id).Scan(&n)
	return n
}

func (r *CoopRepo) setStatus(a model.CoopActor, c *model.Cooperation, st, onBehalf, action string) error {
	if _, err := r.db.Exec(`UPDATE work_cooperations SET status=?, decided_by=?, decided_at=?, updated_at=CURRENT_TIMESTAMP WHERE coop_id=?`,
		st, a.UserID, time.Now().Format("2006-01-02 15:04:05"), c.CoopID); err != nil {
		return err
	}
	r.event(c.CoopID, a.UserID, action, st, onBehalf)
	return nil
}

func (r *CoopRepo) must(id string) (*model.Cooperation, error) {
	c, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errCoopNotFound
	}
	return c, nil
}

func (r *CoopRepo) event(id, actor, action, detail, onBehalf string) {
	_, _ = r.db.Exec(`INSERT INTO cooperation_events(cooperation_id,actor_id,action,detail,on_behalf) VALUES(?,?,?,?,?)`,
		id, actor, action, detail, nullIfEmpty(onBehalf))
}

func (r *CoopRepo) Events(id string) ([]model.CoopEvent, error) {
	rows, err := r.db.Query(`SELECT at, actor_id, action, detail, COALESCE(on_behalf,'') FROM cooperation_events WHERE cooperation_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CoopEvent
	for rows.Next() {
		var e model.CoopEvent
		if err := rows.Scan(&e.At, &e.ActorID, &e.Action, &e.Detail, &e.OnBehalf); err != nil {
			return nil, err
		}
		if u, _ := r.users.GetByID(e.ActorID); u != nil {
			e.Actor = u.FullName
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *CoopRepo) ReRequest(a model.CoopActor, id string) (*model.Cooperation, error) {
	src, err := r.must(id)
	if err != nil {
		return nil, err
	}
	if src.ReqUserID != a.UserID && !(model.NormalizeRole(a.Role) == model.RoleOrgAdmin && a.HomeOrg() == src.ReqOrgID) && model.NormalizeRole(a.Role) != model.RoleVisionAdmin {
		return nil, errCoopForbidden
	}
	n := *src
	n.CoopID = ""
	n.Status = ""
	n.RejectReason = ""
	n.PlanStart, n.PlanEnd, n.Instruction, n.WorkGroupID = "", "", "", ""
	if err := r.Create(a, &n); err != nil {
		return nil, err
	}
	r.event(n.CoopID, a.UserID, "재요청", src.CoopID, "")
	return &n, nil
}

func (r *CoopRepo) onBehalfOf(a model.CoopActor, targetOrg string) string {
	if model.NormalizeRole(a.Role) == model.RoleVisionAdmin {
		return ""
	}
	if (model.NormalizeRole(a.Role) == model.RoleOrgAdmin || model.NormalizeRole(a.Role) == model.RoleAdmin) && a.HomeOrg() == targetOrg {
		return ""
	}
	if a.UserID == r.orgDeputy(targetOrg, todayYMD()) {
		return a.UserID
	}
	return ""
}

func jsonDetail(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func coopWorkingDays(from, to string) int {
	a, err1 := time.Parse("2006-01-02", from)
	b, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || b.Before(a) {
		return 0
	}
	n := 0
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		w := d.Weekday()
		if w != time.Saturday && w != time.Sunday {
			n++
		}
	}
	return n
}
