package repository

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *WorkGroupRepo) AddEffort(a model.WorkGroupActor, date, userID, groupID, projectID string, minutes int, roleLabel, note string) (*model.WorkEffortEntry, error) {
	if !a.CanWrite() {
		return nil, errWGForbidden
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = todayYMD()
	}
	if minutes < 0 || minutes > 1440 {
		return nil, fmt.Errorf("시간은 0~1440분입니다")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("참여자")
	}
	mode := model.WorkEffortSelf
	orgID := a.HomeOrg()
	groupRole := ""
	if groupID != "" {
		g, err := r.Get(groupID)
		if err != nil || g == nil {
			return nil, errWGNotFound
		}
		if g.ExpiredOn(date) {
			return nil, fmt.Errorf("종료된 그룹에는 넣을 수 없습니다")
		}
		orgID = g.OrgID
		target := r.memberOn(groupID, userID, date)
		if target == nil {
			return nil, errWGForbidden
		}
		groupRole = target.Role
		if userID != a.UserID {
			lead := r.memberOn(groupID, a.UserID, date)
			if lead == nil || lead.Role != model.WorkGroupLeader {
				return nil, errWGForbidden
			}
			mode = model.WorkEffortByLeader
		}
	} else if userID != a.UserID {
		return nil, errWGForbidden
	}
	sum, err := r.dayMinutes(userID, date)
	if err != nil {
		return nil, err
	}
	if sum+minutes > 1440 {
		return nil, fmt.Errorf("하루 합계가 1440분을 넘습니다")
	}
	n, err := NextSeq(r.db, "work_effort")
	if err != nil {
		return nil, err
	}
	e := &model.WorkEffortEntry{
		EntryID: fmt.Sprintf("WE-%04d", n), OrgID: orgID, WorkDate: date, UserID: userID,
		ProjectID: projectID, SourceType: "manual", GroupID: groupID, GroupRole: groupRole,
		RoleLabel: strings.TrimSpace(roleLabel), Minutes: minutes, EntryMode: mode,
		EnteredBy: a.UserID, Note: note, Revision: 1,
	}
	_, err = r.db.Exec(`INSERT INTO work_effort_entries(
		entry_id,org_id,work_date,user_id,project_id,source_type,source_id,group_id,group_role,role_label,minutes,entry_mode,entered_by,note,revision)
		VALUES(?,?,?,?,?,'manual','',?,?,?,?,?,?,?,1)`,
		e.EntryID, e.OrgID, e.WorkDate, e.UserID, nullIfEmpty(e.ProjectID), nullIfEmpty(e.GroupID), e.GroupRole, e.RoleLabel, e.Minutes, e.EntryMode, e.EnteredBy, e.Note)
	if err != nil {
		return nil, err
	}
	r.effortLog(e.EntryID, a.UserID, "create", map[string]any{"minutes": minutes, "group_role": groupRole})
	return e, nil
}

func (r *WorkGroupRepo) dayMinutes(userID, date string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COALESCE(SUM(minutes),0) FROM work_effort_entries
		WHERE user_id=? AND work_date=? AND voided_at IS NULL`, userID, date).Scan(&n)
	return n, err
}

func (r *WorkGroupRepo) ReviseEffort(a model.WorkGroupActor, entryID string, minutes int, note string) error {
	e, err := r.getEffort(entryID)
	if err != nil || e == nil {
		return errWGNotFound
	}
	if e.VoidedAt != "" {
		return fmt.Errorf("무효인 기록")
	}
	if e.UserID != a.UserID && e.EnteredBy != a.UserID && !a.CanManageOrg(e.OrgID) {
		return errWGForbidden
	}
	sum, err := r.dayMinutes(e.UserID, e.WorkDate)
	if err != nil {
		return err
	}
	if sum-e.Minutes+minutes > 1440 {
		return fmt.Errorf("하루 합계가 1440분을 넘습니다")
	}
	_, err = r.db.Exec(`UPDATE work_effort_entries SET minutes=?, note=?, revision=revision+1, updated_at=? WHERE entry_id=?`,
		minutes, note, time.Now().Format("2006-01-02 15:04:05"), entryID)
	if err != nil {
		return err
	}
	r.effortLog(entryID, a.UserID, "revise", map[string]any{"minutes": minutes})
	return nil
}

func (r *WorkGroupRepo) VoidEffort(a model.WorkGroupActor, entryID string) error {
	e, err := r.getEffort(entryID)
	if err != nil || e == nil {
		return errWGNotFound
	}
	if e.UserID != a.UserID && e.EnteredBy != a.UserID && !a.CanManageOrg(e.OrgID) {
		return errWGForbidden
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`UPDATE work_effort_entries SET voided_at=?, revision=revision+1, updated_at=? WHERE entry_id=?`, now, now, entryID)
	if err != nil {
		return err
	}
	r.effortLog(entryID, a.UserID, "void", nil)
	return nil
}

func (r *WorkGroupRepo) getEffort(id string) (*model.WorkEffortEntry, error) {
	row := r.db.QueryRow(`SELECT entry_id,org_id,work_date,user_id,COALESCE(project_id,''),source_type,COALESCE(source_id,''),
		COALESCE(log_id,''),COALESCE(group_id,''),group_role,role_label,minutes,entry_mode,entered_by,note,revision,COALESCE(voided_at,'')
		FROM work_effort_entries WHERE entry_id=?`, id)
	var e model.WorkEffortEntry
	if err := row.Scan(&e.EntryID, &e.OrgID, &e.WorkDate, &e.UserID, &e.ProjectID, &e.SourceType, &e.SourceID,
		&e.LogID, &e.GroupID, &e.GroupRole, &e.RoleLabel, &e.Minutes, &e.EntryMode, &e.EnteredBy, &e.Note, &e.Revision, &e.VoidedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *WorkGroupRepo) EffortByProject(projectID, from, to string) ([]model.WorkEffortSum, error) {
	return r.sumEffort(`SELECT user_id||':'||COALESCE(group_id,'')||':'||role_label, SUM(minutes)
		FROM work_effort_entries
		WHERE project_id=? AND work_date>=? AND work_date<=? AND voided_at IS NULL
		GROUP BY 1`, projectID, from, to)
}

func (r *WorkGroupRepo) EffortByUser(userID, from, to string) ([]model.WorkEffortSum, error) {
	return r.sumEffort(`SELECT work_date||':'||COALESCE(group_id,''), SUM(minutes)
		FROM work_effort_entries
		WHERE user_id=? AND work_date>=? AND work_date<=? AND voided_at IS NULL
		GROUP BY 1`, userID, from, to)
}

func (r *WorkGroupRepo) sumEffort(q string, args ...any) ([]model.WorkEffortSum, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkEffortSum
	for rows.Next() {
		var s model.WorkEffortSum
		if err := rows.Scan(&s.Key, &s.Minutes); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *WorkGroupRepo) SaveGroupLog(a model.WorkGroupActor, note *model.WorkLogNote) error {
	if note == nil {
		return fmt.Errorf("일지")
	}
	g, err := r.Get(note.GroupID)
	if err != nil || g == nil {
		return errWGNotFound
	}
	if g.ExpiredOn(note.WorkDate) {
		return fmt.Errorf("종료된 그룹")
	}
	ok := a.CanManageOrg(g.OrgID)
	if !ok {
		m := r.memberOn(g.GroupID, a.UserID, note.WorkDate)
		ok = m != nil && m.Role == model.WorkGroupLeader
	}
	if !ok {
		return errWGForbidden
	}
	n, err := NextSeq(r.db, "work_log")
	if err != nil {
		return err
	}
	note.LogID = fmt.Sprintf("WL-%04d", n)
	note.OrgID = g.OrgID
	note.Scope = model.WorkLogScopeGroup
	note.AuthorID = a.UserID
	_, err = r.db.Exec(`INSERT INTO work_log_notes(log_id,org_id,work_date,project_id,group_id,scope,author_id,content,next_plan,status)
		VALUES(?,?,?,?,?,'group',?,?,?,?)`,
		note.LogID, note.OrgID, note.WorkDate, nullIfEmpty(note.ProjectID), note.GroupID, a.UserID, note.Content, note.NextPlan, defaultLogStatus(note.Status))
	return err
}

func defaultLogStatus(s string) string {
	if s == "" {
		return "draft"
	}
	return s
}

func (r *WorkGroupRepo) effortLog(entryID, actor, action string, detail map[string]any) {
	b, _ := json.Marshal(detail)
	if detail == nil {
		b = []byte("{}")
	}
	_, _ = r.db.Exec(`INSERT INTO work_effort_entry_log(entry_id,actor_id,action,detail) VALUES(?,?,?,?)`,
		entryID, actor, action, string(b))
}
