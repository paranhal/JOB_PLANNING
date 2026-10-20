package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

type WorkGroupRepo struct {
	db    *sql.DB
	users *UserRepo
}

func NewWorkGroupRepo(db *sql.DB) *WorkGroupRepo {
	return &WorkGroupRepo{db: db, users: NewUserRepo(db)}
}

func (r *WorkGroupRepo) Get(id string) (*model.WorkGroup, error) {
	g, err := r.scanGroup(`WHERE g.group_id=?`, id)
	if err != nil || g == nil {
		return g, err
	}
	return g, nil
}

func (r *WorkGroupRepo) scanGroup(where string, args ...any) (*model.WorkGroup, error) {
	q := `SELECT g.group_id, g.org_id, g.name, g.purpose, COALESCE(g.project_id,''), g.status,
		COALESCE(g.start_date,''), COALESCE(g.end_date,''), g.created_by, g.created_at,
		COALESCE(g.closed_by,''), COALESCE(g.closed_at,''),
		COALESCE((SELECT user_id FROM work_group_members m WHERE m.group_id=g.group_id AND m.role='leader' AND m.left_on IS NULL LIMIT 1),''),
		COALESCE((SELECT COUNT(*) FROM work_group_members m WHERE m.group_id=g.group_id AND m.left_on IS NULL),0)
		FROM work_groups g ` + where
	row := r.db.QueryRow(q, args...)
	var g model.WorkGroup
	if err := row.Scan(&g.GroupID, &g.OrgID, &g.Name, &g.Purpose, &g.ProjectID, &g.Status,
		&g.StartDate, &g.EndDate, &g.CreatedBy, &g.CreatedAt, &g.ClosedBy, &g.ClosedAt, &g.LeaderID, &g.MemberN); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	g.LeaderName = r.userName(g.LeaderID)
	return &g, nil
}

func (r *WorkGroupRepo) userName(id string) string {
	if id == "" || r.users == nil {
		return ""
	}
	u, _ := r.users.GetByID(id)
	if u == nil {
		return ""
	}
	return u.FullName
}

func (r *WorkGroupRepo) ListGroups(orgID, status string) ([]model.WorkGroup, error) {
	q := `SELECT g.group_id FROM work_groups g WHERE 1=1`
	var args []any
	if orgID != "" && orgID != OrgAll {
		q += ` AND g.org_id=?`
		args = append(args, orgID)
	}
	switch status {
	case model.WorkGroupActive, model.WorkGroupClosed:
		q += ` AND g.status=?`
		args = append(args, status)
	}
	q += ` ORDER BY g.name`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkGroup
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		g, err := r.Get(id)
		if err != nil || g == nil {
			continue
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *WorkGroupRepo) ListMyGroups(userID, date string) ([]model.WorkGroup, error) {
	if date == "" {
		date = todayYMD()
	}
	rows, err := r.db.Query(`SELECT DISTINCT group_id FROM work_group_members
		WHERE user_id=? AND joined_on<=? AND (left_on IS NULL OR left_on>=?)`, userID, date, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkGroup
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		g, err := r.Get(id)
		if err != nil || g == nil {
			continue
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *WorkGroupRepo) MembersOn(groupID, date string) ([]model.WorkGroupPerson, error) {
	if date == "" {
		date = todayYMD()
	}
	rows, err := r.db.Query(`SELECT id, group_id, user_id, role, joined_on, COALESCE(left_on,''), added_by, COALESCE(removed_by,'')
		FROM work_group_members WHERE group_id=? AND joined_on<=? AND (left_on IS NULL OR left_on>=?)
		ORDER BY role DESC, joined_on`, groupID, date, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkGroupPerson
	for rows.Next() {
		var m model.WorkGroupPerson
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role, &m.JoinedOn, &m.LeftOn, &m.AddedBy, &m.RemovedBy); err != nil {
			return nil, err
		}
		m.FullName = r.userName(m.UserID)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *WorkGroupRepo) memberOn(groupID, userID, date string) *model.WorkGroupPerson {
	ms, _ := r.MembersOn(groupID, date)
	for i := range ms {
		if ms[i].UserID == userID {
			return &ms[i]
		}
	}
	return nil
}

func (r *WorkGroupRepo) CanView(a model.WorkGroupActor, g *model.WorkGroup) bool {
	if g == nil {
		return false
	}
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleCLevel {
		return true
	}
	if rle == model.RoleOrgAdmin || rle == model.RoleAdmin {
		return a.HomeOrg() == g.OrgID
	}
	return r.memberOn(g.GroupID, a.UserID, todayYMD()) != nil
}

func (r *WorkGroupRepo) CanEditMembers(a model.WorkGroupActor, g *model.WorkGroup) bool {
	if g == nil || !a.CanWrite() || g.ExpiredOn(todayYMD()) {
		return false
	}
	if a.CanManageOrg(g.OrgID) {
		return true
	}
	m := r.memberOn(g.GroupID, a.UserID, todayYMD())
	return m != nil && m.Role == model.WorkGroupLeader
}

func (r *WorkGroupRepo) Create(a model.WorkGroupActor, g *model.WorkGroup) error {
	if g == nil {
		return fmt.Errorf("그룹")
	}
	g.OrgID = strings.TrimSpace(g.OrgID)
	if !a.CanManageOrg(g.OrgID) {
		return errWGForbidden
	}
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return fmt.Errorf("이름을 입력하세요")
	}
	if g.Status == "" {
		g.Status = model.WorkGroupActive
	}
	n, err := NextSeq(r.db, "work_group")
	if err != nil {
		return err
	}
	g.GroupID = fmt.Sprintf("WG-%04d", n)
	g.CreatedBy = a.UserID
	_, err = r.db.Exec(`INSERT INTO work_groups(group_id,org_id,name,purpose,project_id,status,start_date,end_date,created_by)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		g.GroupID, g.OrgID, g.Name, g.Purpose, nullIfEmpty(g.ProjectID), g.Status, nullIfEmpty(g.StartDate), nullIfEmpty(g.EndDate), a.UserID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("같은 이름의 활성 그룹이 있습니다")
		}
		return err
	}
	r.event(g.GroupID, a.UserID, "create", map[string]string{"name": g.Name})
	audit.Log(audit.ActionCreate, "work_groups", "group_id", g.GroupID, g.Name, "", g.Name)
	return nil
}

func (r *WorkGroupRepo) Update(a model.WorkGroupActor, g *model.WorkGroup) error {
	cur, err := r.Get(g.GroupID)
	if err != nil || cur == nil {
		return errWGNotFound
	}
	if !a.CanManageOrg(cur.OrgID) {
		return errWGForbidden
	}
	before := cur.Name
	_, err = r.db.Exec(`UPDATE work_groups SET name=?, purpose=?, project_id=?, start_date=?, end_date=? WHERE group_id=?`,
		strings.TrimSpace(g.Name), g.Purpose, nullIfEmpty(g.ProjectID), nullIfEmpty(g.StartDate), nullIfEmpty(g.EndDate), g.GroupID)
	if err != nil {
		return err
	}
	r.event(g.GroupID, a.UserID, "update", map[string]string{"name": g.Name})
	audit.Log(audit.ActionUpdate, "work_groups", "group_id", g.GroupID, g.Name, before, g.Name)
	return nil
}

func (r *WorkGroupRepo) Close(a model.WorkGroupActor, id string) error {
	g, err := r.Get(id)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if !a.CanManageOrg(g.OrgID) {
		return errWGForbidden
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`UPDATE work_groups SET status=?, closed_by=?, closed_at=? WHERE group_id=?`,
		model.WorkGroupClosed, a.UserID, now, id)
	if err != nil {
		return err
	}
	r.event(id, a.UserID, "close", nil)
	audit.Log(audit.ActionUpdate, "work_groups", "group_id", id, g.Name, model.WorkGroupActive, model.WorkGroupClosed)
	return nil
}

func (r *WorkGroupRepo) Reopen(a model.WorkGroupActor, id string) error {
	g, err := r.Get(id)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if !a.CanManageOrg(g.OrgID) {
		return errWGForbidden
	}
	_, err = r.db.Exec(`UPDATE work_groups SET status=?, closed_by=NULL, closed_at=NULL WHERE group_id=?`,
		model.WorkGroupActive, id)
	if err != nil {
		return err
	}
	r.event(id, a.UserID, "reopen", nil)
	return nil
}

func (r *WorkGroupRepo) AddMember(a model.WorkGroupActor, groupID, userID, role, joinedOn string) error {
	g, err := r.Get(groupID)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if !r.CanEditMembers(a, g) {
		return errWGForbidden
	}
	if g.ExpiredOn(todayYMD()) {
		return fmt.Errorf("종료된 그룹에는 넣을 수 없습니다")
	}
	if role == "" {
		role = model.WorkGroupMember
	}
	if role == model.WorkGroupLeader && !a.CanManageOrg(g.OrgID) {
		return errWGForbidden
	}
	u, err := r.users.GetByID(userID)
	if err != nil || u == nil || !u.IsActive || u.IsDeleted() {
		return fmt.Errorf("사용자를 찾을 수 없습니다")
	}
	if !workGroupMemberRoleOK(u) {
		return fmt.Errorf("구성원이 될 수 없습니다")
	}
	if !r.sameOrg(u, g.OrgID) {
		return fmt.Errorf("같은 조직의 구성원만 넣을 수 있습니다")
	}
	if r.memberOn(groupID, userID, todayYMD()) != nil {
		return fmt.Errorf("이미 이 그룹에 있습니다")
	}
	if joinedOn == "" {
		joinedOn = todayYMD()
	}
	_, err = r.db.Exec(`INSERT INTO work_group_members(group_id,user_id,role,joined_on,added_by) VALUES(?,?,?,?,?)`,
		groupID, userID, role, joinedOn, a.UserID)
	if err != nil {
		return err
	}
	r.event(groupID, a.UserID, "add_member", map[string]string{"user_id": userID, "role": role})
	return nil
}

func workGroupMemberRoleOK(u *model.User) bool {
	if u == nil || u.IsReadOnly || u.IsTest {
		return false
	}
	raw := strings.TrimSpace(u.Role)
	if raw == "partner" {
		return false
	}
	switch model.NormalizeRole(raw) {
	case model.RoleCLevel, model.RoleObserver, model.RoleTester:
		return false
	default:
		return true
	}
}

func (r *WorkGroupRepo) sameOrg(u *model.User, orgID string) bool {
	if u == nil {
		return false
	}
	uo := strings.TrimSpace(u.OrgID)
	if uo == "" {
		uo = model.OrgIDLibrary
	}
	return uo == orgID
}

func (r *WorkGroupRepo) RemoveMember(a model.WorkGroupActor, groupID, userID string) error {
	g, err := r.Get(groupID)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if !r.CanEditMembers(a, g) {
		return errWGForbidden
	}
	if userID == a.UserID && !a.CanManageOrg(g.OrgID) {
		return fmt.Errorf("본인은 제거할 수 없습니다")
	}
	m := r.memberOn(groupID, userID, todayYMD())
	if m == nil {
		return fmt.Errorf("구성원이 아닙니다")
	}
	if m.Role == model.WorkGroupLeader && !a.CanManageOrg(g.OrgID) {
		return fmt.Errorf("그룹장은 조직관리자가 바꿉니다")
	}
	_, err = r.db.Exec(`UPDATE work_group_members SET left_on=?, removed_by=? WHERE id=?`, todayYMD(), a.UserID, m.ID)
	if err != nil {
		return err
	}
	r.event(groupID, a.UserID, "remove_member", map[string]string{"user_id": userID})
	return nil
}

func (r *WorkGroupRepo) SetLeader(a model.WorkGroupActor, groupID, userID string) error {
	g, err := r.Get(groupID)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if !a.CanManageOrg(g.OrgID) {
		return errWGForbidden
	}
	if userID == "" {
		cur := r.memberOn(groupID, g.LeaderID, todayYMD())
		if cur != nil {
			_, err = r.db.Exec(`UPDATE work_group_members SET role=? WHERE id=?`, model.WorkGroupMember, cur.ID)
			if err != nil {
				return err
			}
			r.event(groupID, a.UserID, "set_leader", map[string]string{"user_id": ""})
		}
		return nil
	}
	m := r.memberOn(groupID, userID, todayYMD())
	if m == nil {
		if err := r.AddMember(a, groupID, userID, model.WorkGroupMember, todayYMD()); err != nil {
			return err
		}
		m = r.memberOn(groupID, userID, todayYMD())
		if m == nil {
			return fmt.Errorf("구성원을 넣을 수 없습니다")
		}
	}
	if g.LeaderID != "" && g.LeaderID != userID {
		_, err = r.db.Exec(`UPDATE work_group_members SET role=? WHERE group_id=? AND user_id=? AND left_on IS NULL AND role=?`,
			model.WorkGroupMember, groupID, g.LeaderID, model.WorkGroupLeader)
		if err != nil {
			return err
		}
	}
	_, err = r.db.Exec(`UPDATE work_group_members SET role=? WHERE id=?`, model.WorkGroupLeader, m.ID)
	if err != nil {
		return err
	}
	act := "set_leader"
	if g.LeaderID != "" && g.LeaderID != userID {
		act = "change_leader"
	}
	r.event(groupID, a.UserID, act, map[string]string{"from": g.LeaderID, "to": userID})
	return nil
}

func (r *WorkGroupRepo) event(groupID, actor, action string, detail map[string]string) {
	b, _ := json.Marshal(detail)
	if detail == nil {
		b = []byte("{}")
	}
	_, _ = r.db.Exec(`INSERT INTO work_group_events(group_id,actor_id,action,detail) VALUES(?,?,?,?)`,
		groupID, actor, action, string(b))
}

var (
	errWGForbidden = fmt.Errorf("forbidden")
	errWGNotFound  = fmt.Errorf("not found")
)

func IsWorkGroupForbidden(err error) bool { return err == errWGForbidden }
func IsWorkGroupNotFound(err error) bool  { return err == errWGNotFound }
