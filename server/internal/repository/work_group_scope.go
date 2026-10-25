package repository

import (
	"database/sql"
	"strings"

	"customer-support/internal/model"
)

func applyASWorkGroupV86(db *sql.DB) {
	if db == nil {
		return
	}
	addTextColumn(db, "as_receipts", "work_group_id")
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS ix_as_receipts_work_group ON as_receipts(work_group_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS ix_wgm_user_group_dates ON work_group_members(user_id, group_id, joined_on, left_on)`)
}

func ApplyASWorkGroupV86ForTest(db *sql.DB) { applyASWorkGroupV86(db) }

// TaskGroupMemberOn 업무 날짜에 사용자 U 가 그룹 G 구성원인지. 모든 화면이 이 판정을 쓴다.
func (r *WorkGroupRepo) TaskGroupMemberOn(groupID, userID, date string) bool {
	if r == nil {
		return false
	}
	return r.memberOn(groupID, userID, date) != nil
}

func (r *WorkGroupRepo) LeaderOf(a model.WorkGroupActor, groupID string) bool {
	if r == nil || strings.TrimSpace(a.UserID) == "" || strings.TrimSpace(groupID) == "" {
		return false
	}
	if !a.CanWrite() {
		return false
	}
	m := r.memberOn(groupID, a.UserID, todayYMD())
	return m != nil && m.Role == model.WorkGroupLeader
}

func (r *WorkGroupRepo) CanManageGroup(a model.WorkGroupActor, g *model.WorkGroup) bool {
	if g == nil || !a.CanWrite() {
		return false
	}
	if a.CanManageOrg(g.OrgID) {
		return true
	}
	return r.LeaderOf(a, g.GroupID)
}

func (r *WorkGroupRepo) ActiveRolesForUser(userID, date string) (memberIDs, leaderIDs map[string]bool) {
	memberIDs = map[string]bool{}
	leaderIDs = map[string]bool{}
	if r == nil || r.db == nil || strings.TrimSpace(userID) == "" {
		return memberIDs, leaderIDs
	}
	if date == "" {
		date = todayYMD()
	}
	rows, err := r.db.Query(`SELECT group_id, role FROM work_group_members
		WHERE user_id=? AND joined_on<=? AND (left_on IS NULL OR left_on>=?)`, userID, date, date)
	if err != nil {
		return memberIDs, leaderIDs
	}
	defer rows.Close()
	for rows.Next() {
		var gid, role string
		if err := rows.Scan(&gid, &role); err != nil {
			continue
		}
		memberIDs[gid] = true
		if role == model.WorkGroupLeader {
			leaderIDs[gid] = true
		}
	}
	return memberIDs, leaderIDs
}

func (r *WorkGroupRepo) MembersAll(groupID string) ([]model.WorkGroupPerson, error) {
	rows, err := r.db.Query(`SELECT id, group_id, user_id, role, joined_on, COALESCE(left_on,''), added_by, COALESCE(removed_by,'')
		FROM work_group_members WHERE group_id=? ORDER BY joined_on DESC, id DESC`, groupID)
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

type WorkGroupEventRow struct {
	At        string
	ActorID   string
	ActorName string
	Action    string
	Detail    string
}

func (r *WorkGroupRepo) ListEvents(groupID string, limit int) ([]WorkGroupEventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(`SELECT at, actor_id, action, COALESCE(detail,'') FROM work_group_events
		WHERE group_id=? ORDER BY id DESC LIMIT ?`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkGroupEventRow
	for rows.Next() {
		var e WorkGroupEventRow
		if err := rows.Scan(&e.At, &e.ActorID, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		e.ActorName = r.userName(e.ActorID)
		out = append(out, e)
	}
	return out, rows.Err()
}

func sqlGroupMemberOn(groupCol, dateCol string) string {
	d := `date(COALESCE(NULLIF(TRIM(` + dateCol + `),''),'9999-12-31'))`
	return `TRIM(COALESCE(` + groupCol + `,'')) != '' AND EXISTS (
		SELECT 1 FROM work_group_members wgm
		 WHERE wgm.group_id=` + groupCol + ` AND wgm.user_id=?
		   AND wgm.joined_on<=` + d + `
		   AND (wgm.left_on IS NULL OR wgm.left_on>=` + d + `))`
}

func mineOrGroupCond(kind, alias, userID string, keys []string) (string, []interface{}) {
	person, args := assigneeMatchKeys(kind, alias, mergeAssigneeKeys(userID, "", keys...))
	uid := strings.TrimSpace(userID)
	groupCol, dateCol := groupDateCols(kind, alias)
	if uid == "" || groupCol == "" {
		return person, args
	}
	g := sqlGroupMemberOn(groupCol, dateCol)
	if person == "" {
		return " AND (" + g + ")", []interface{}{uid}
	}
	inner := strings.TrimPrefix(person, " AND ")
	return " AND (" + inner + " OR (" + g + "))", append(args, uid)
}

func groupDateCols(kind, alias string) (groupCol, dateCol string) {
	alias = normalizeSQLAlias(alias)
	switch kind {
	case AssigneeKindAS:
		return alias + "work_group_id", `COALESCE(NULLIF(TRIM(` + alias + `visit_scheduled_date),''), date(` + alias + `receipt_datetime))`
	case AssigneeKindMaintenance:
		return alias + "work_group_id", alias + "visit_date"
	case AssigneeKindTask:
		return alias + "work_group_id", `COALESCE(NULLIF(TRIM(` + alias + `work_date),''), ` + alias + `due_date)`
	default:
		return "", ""
	}
}

func CountDistinctByGroup(db *sql.DB, table, idCol string, groupID string) (int, error) {
	if db == nil {
		return 0, nil
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(DISTINCT `+idCol+`) FROM `+table+` WHERE work_group_id=?`, groupID).Scan(&n)
	return n, err
}
