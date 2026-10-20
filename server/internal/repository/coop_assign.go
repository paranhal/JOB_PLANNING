package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *CoopRepo) Assign(a model.CoopActor, id, planStart, planEnd, instruction, groupID string, userIDs []string) error {
	c, err := r.must(id)
	if err != nil {
		return err
	}
	if !r.CanActInbox(a, c.TargetOrgID) {
		return errCoopForbidden
	}
	if c.Status != model.CoopAccepted && c.Status != model.CoopAssigned {
		return fmt.Errorf("수락·배정됨만 배정할 수 있습니다")
	}
	planStart, planEnd = strings.TrimSpace(planStart), strings.TrimSpace(planEnd)
	if planStart == "" || planEnd == "" {
		return fmt.Errorf("확정 일정을 입력하세요")
	}
	if planEnd < planStart {
		return fmt.Errorf("확정 종료가 시작보다 앞입니다")
	}
	groupID = strings.TrimSpace(groupID)
	people, gid, err := r.resolveAssignees(c.TargetOrgID, groupID, userIDs)
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM cooperation_assignees WHERE cooperation_id=?`, id); err != nil {
		return err
	}
	for _, p := range people {
		if _, err := r.db.Exec(`INSERT INTO cooperation_assignees(cooperation_id,user_id,role,work_group_id) VALUES(?,?,?,?)`,
			id, p.UserID, p.Role, nullIfEmpty(gid)); err != nil {
			return err
		}
	}
	if _, err := r.db.Exec(`UPDATE work_cooperations SET status=?, plan_start=?, plan_end=?, instruction=?, work_group_id=?, updated_at=CURRENT_TIMESTAMP WHERE coop_id=?`,
		model.CoopAssigned, planStart, planEnd, strings.TrimSpace(instruction), nullIfEmpty(gid), id); err != nil {
		return err
	}
	r.event(id, a.UserID, "배정", gid, r.onBehalfOf(a, c.TargetOrgID))
	c.PlanStart, c.PlanEnd, c.Instruction, c.WorkGroupID = planStart, planEnd, instruction, gid
	c.Assignees = people
	c.Status = model.CoopAssigned
	return r.syncLinked(c)
}

func (r *CoopRepo) resolveAssignees(targetOrg, groupID string, userIDs []string) ([]model.CoopAssignee, string, error) {
	allowed, err := r.users.ListAssignable(targetOrg)
	if err != nil {
		return nil, "", err
	}
	ok := map[string]model.User{}
	for _, u := range allowed {
		ok[u.UserID] = u
	}
	var people []model.CoopAssignee
	if groupID != "" {
		g, err := r.groups.Get(groupID)
		if err != nil || g == nil {
			return nil, "", fmt.Errorf("그룹을 찾을 수 없습니다")
		}
		if g.OrgID != targetOrg {
			return nil, "", fmt.Errorf("대상 팀의 그룹만 배정할 수 있습니다")
		}
		ms, err := r.groups.MembersOn(groupID, "")
		if err != nil {
			return nil, "", err
		}
		if len(ms) == 0 {
			return nil, "", fmt.Errorf("그룹 구성원이 없습니다")
		}
		for _, m := range ms {
			if _, found := ok[m.UserID]; !found {
				return nil, "", fmt.Errorf("그룹 구성원이 대상 조직 소속이 아닙니다")
			}
			role := model.CoopAssignMember
			if m.Role == model.WorkGroupLeader {
				role = model.CoopAssignLead
			}
			people = append(people, model.CoopAssignee{UserID: m.UserID, FullName: m.FullName, Role: role})
		}
		return people, groupID, nil
	}
	seen := map[string]bool{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		u, found := ok[id]
		if !found {
			return nil, "", fmt.Errorf("대상 팀 소속만 배정할 수 있습니다")
		}
		seen[id] = true
		role := model.CoopAssignMember
		if len(people) == 0 {
			role = model.CoopAssignLead
		}
		people = append(people, model.CoopAssignee{UserID: u.UserID, FullName: u.FullName, Role: role})
	}
	if len(people) == 0 {
		return nil, "", fmt.Errorf("담당 개인 1명 이상 또는 업무 그룹을 고르세요")
	}
	return people, "", nil
}

func (r *CoopRepo) syncLinked(c *model.Cooperation) error {
	var n int
	_ = r.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM work_tasks WHERE cooperation_id=?) +
		(SELECT COUNT(*) FROM as_receipts WHERE cooperation_id=?) +
		(SELECT COUNT(*) FROM maintenance_visits WHERE cooperation_id=?)`,
		c.CoopID, c.CoopID, c.CoopID).Scan(&n)
	if n == 0 {
		return r.createLinked(c)
	}
	return r.updateLinkedOpen(c)
}

func (r *CoopRepo) leadName(c *model.Cooperation) (string, string) {
	if len(c.Assignees) == 0 {
		return "", ""
	}
	lead := c.Assignees[0]
	for _, a := range c.Assignees {
		if a.Role == model.CoopAssignLead {
			lead = a
			break
		}
	}
	return lead.FullName, lead.UserID
}

func (r *CoopRepo) createLinked(c *model.Cooperation) error {
	name, uid := r.leadName(c)
	switch c.Kind {
	case model.CoopAS:
		return r.createLinkedAS(c, name, uid)
	default:
		return r.createLinkedTask(c, name, uid)
	}
}

func (r *CoopRepo) createLinkedTask(c *model.Cooperation, name, uid string) error {
	wt := model.WBWorkAdmin
	if c.Kind == model.CoopInspection {
		wt = model.WBWorkAdmin
	}
	t := &model.WorkTask{
		WorkType: wt, Title: c.Title, Description: c.Instruction + "\n" + c.Body,
		DueDate: c.PlanEnd, WorkDate: c.PlanStart, Status: model.WBTaskWaiting,
		Assignee: name, AssigneeUserID: uid, OrgID: c.TargetOrgID, ProjectID: c.ProjectID,
	}
	if err := r.wb.CreateTask(t); err != nil {
		return err
	}
	_, _ = r.db.Exec(`UPDATE work_tasks SET cooperation_id=? WHERE task_id=?`, c.CoopID, t.TaskID)
	return r.attachSupports(t.TaskID, c.Assignees, uid)
}

func (r *CoopRepo) attachSupports(taskID string, people []model.CoopAssignee, owner string) error {
	var sup []model.WorkTaskMember
	for _, p := range people {
		if p.UserID == owner {
			continue
		}
		sup = append(sup, model.WorkTaskMember{Assignee: p.FullName, UserID: p.UserID, Role: model.WBMemberSupport})
	}
	if len(sup) == 0 {
		return nil
	}
	return r.wb.ReplaceSupportMembers(taskID, sup)
}

func (r *CoopRepo) createLinkedAS(c *model.Cooperation, name, uid string) error {
	custID := ""
	if c.ProjectID != "" {
		if p, err := r.proj.Get(c.ProjectID); err == nil && p != nil {
			custID = p.CustomerID
		}
	}
	incomplete := 1
	if custID == "" {
		party, _, err := r.cust.CreatePartyNameOnly("협조 "+c.Title, model.PartyKindCustomer)
		if err != nil {
			return err
		}
		custID = party.CustomerID
	}
	as := &model.ASReceipt{
		CustomerID: custID, ReceiptChannel: "협조", Requester: c.ReqUserName, RequesterName: c.ReqUserName,
		RequesterType: "협조", Symptom: c.Body, Urgency: "중", AssignedTo: name, AssignedUserID: uid,
		VisitScheduledDate: c.PlanStart, OrgID: c.TargetOrgID, ProjectID: c.ProjectID, ReceivedBy: c.ReqUserName,
	}
	if as.VisitScheduledDate == "" {
		as.VisitScheduledDate = todayYMD()
	}
	if err := r.as.Create(as); err != nil {
		return err
	}
	_, _ = r.db.Exec(`UPDATE as_receipts SET cooperation_id=?, coop_incomplete=? WHERE as_id=?`, c.CoopID, incomplete, as.ASID)
	return nil
}

func (r *CoopRepo) updateLinkedOpen(c *model.Cooperation) error {
	name, uid := r.leadName(c)
	rows, err := r.db.Query(`SELECT task_id, status FROM work_tasks WHERE cooperation_id=?`, c.CoopID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return err
		}
		if st == model.WBTaskComplete || st == model.WBTaskCancelled {
			continue
		}
		t, err := r.wb.GetTask(id)
		if err != nil || t == nil {
			continue
		}
		t.DueDate, t.WorkDate, t.Description = c.PlanEnd, c.PlanStart, c.Instruction+"\n"+c.Body
		t.Assignee, t.AssigneeUserID = name, uid
		if err := r.wb.UpdateTask(t); err != nil {
			return err
		}
		_ = r.attachSupports(id, c.Assignees, uid)
	}
	asRows, err := r.db.Query(`SELECT as_id, status FROM as_receipts WHERE cooperation_id=?`, c.CoopID)
	if err != nil {
		return err
	}
	defer asRows.Close()
	for asRows.Next() {
		var id, st string
		if err := asRows.Scan(&id, &st); err != nil {
			return err
		}
		if st == "completed" || st == "cancelled" || st == "closed" {
			continue
		}
		_, _ = r.db.Exec(`UPDATE as_receipts SET assigned_to=?, assigned_user_id=?, visit_scheduled_date=?, updated_at=CURRENT_TIMESTAMP WHERE as_id=?`,
			name, uid, c.PlanStart, id)
	}
	return nil
}

func (r *CoopRepo) ProjectView(a model.CoopActor, coopID string) (*model.CoopProjectView, error) {
	c, err := r.must(coopID)
	if err != nil {
		return nil, err
	}
	if !r.CanSee(a, c) {
		return nil, errCoopNotFound
	}
	if c.Status != model.CoopAccepted && c.Status != model.CoopAssigned {
		return nil, errCoopNotFound
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return nil, errCoopNotFound
	}
	p, err := r.proj.Get(c.ProjectID)
	if err != nil || p == nil {
		return nil, errCoopNotFound
	}
	v := &model.CoopProjectView{
		ProjectID: p.ProjectID, Name: p.Name, ShortName: p.ShortName, PlanYear: p.PlanYear,
		Status: p.Status, StartDate: p.StartDate, EndDate: p.EndDate, CustomerName: p.CustomerName,
		Notes: p.Notes, Kind: p.ProjectKind,
	}
	return v, nil
}

func (r *CoopRepo) ListFiles(id string) ([]model.CoopFile, error) {
	rows, err := r.db.Query(`SELECT id, file_name, stored_path, mime, size FROM cooperation_files WHERE cooperation_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CoopFile
	for rows.Next() {
		var f model.CoopFile
		if err := rows.Scan(&f.ID, &f.FileName, &f.Path, &f.MIME, &f.Size); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *CoopRepo) AddFile(id, name, path, mime, by string, size int64) error {
	_, err := r.db.Exec(`INSERT INTO cooperation_files(cooperation_id,file_name,stored_path,mime,size,uploaded_by) VALUES(?,?,?,?,?,?)`,
		id, name, path, mime, size, by)
	return err
}

func (r *CoopRepo) File(id int64) (*model.CoopFile, string, error) {
	var f model.CoopFile
	var coop string
	err := r.db.QueryRow(`SELECT id, cooperation_id, file_name, stored_path, mime, size FROM cooperation_files WHERE id=?`, id).
		Scan(&f.ID, &coop, &f.FileName, &f.Path, &f.MIME, &f.Size)
	if err != nil {
		return nil, "", err
	}
	return &f, coop, nil
}

func (r *CoopRepo) FileCount(id string) int {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM cooperation_files WHERE cooperation_id=?`, id).Scan(&n)
	return n
}
