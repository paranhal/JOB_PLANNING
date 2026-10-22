package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *MaintenanceRepo) ListVisitAssignees(visitID string) ([]model.MaintenanceVisitAssignee, error) {
	visitID = strings.TrimSpace(visitID)
	if visitID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT visit_id, COALESCE(user_id,''), COALESCE(assignee_name,''), role,
		       COALESCE(work_group_id,''), COALESCE(sort_order,0)
		FROM maintenance_visit_assignees WHERE visit_id=? ORDER BY sort_order, role`, visitID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceVisitAssignee
	for rows.Next() {
		var a model.MaintenanceVisitAssignee
		if err := rows.Scan(&a.VisitID, &a.UserID, &a.AssigneeName, &a.Role, &a.WorkGroupID, &a.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *MaintenanceRepo) ListVisitAssigneesForVisits(visitIDs []string) (map[string][]model.MaintenanceVisitAssignee, error) {
	out := map[string][]model.MaintenanceVisitAssignee{}
	if r == nil || r.db == nil || len(visitIDs) == 0 {
		return out, nil
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(visitIDs))
	for _, id := range visitIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i += 200 {
		end := i + 200
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		ph := strings.Repeat("?,", len(chunk))
		ph = strings.TrimSuffix(ph, ",")
		q := fmt.Sprintf(`
			SELECT visit_id, COALESCE(user_id,''), COALESCE(assignee_name,''), role,
			       COALESCE(work_group_id,''), COALESCE(sort_order,0)
			FROM maintenance_visit_assignees WHERE visit_id IN (%s)
			ORDER BY visit_id, sort_order, role`, ph)
		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}
		rows, err := r.db.Query(q, args...)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return out, nil
			}
			return out, err
		}
		for rows.Next() {
			var a model.MaintenanceVisitAssignee
			if err := rows.Scan(&a.VisitID, &a.UserID, &a.AssigneeName, &a.Role, &a.WorkGroupID, &a.SortOrder); err != nil {
				rows.Close()
				return out, err
			}
			out[a.VisitID] = append(out[a.VisitID], a)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func AttachVisitPeople(visits []model.MaintenanceVisit, byVisit map[string][]model.MaintenanceVisitAssignee) {
	if len(visits) == 0 {
		return
	}
	for i := range visits {
		visits[i].People = byVisit[visits[i].VisitID]
	}
}

func (r *MaintenanceRepo) ReplaceVisitAssignees(visitID string, items []model.MaintenanceVisitAssignee, groupID string) error {
	visitID = strings.TrimSpace(visitID)
	if visitID == "" {
		return nil
	}
	if _, err := r.db.Exec(`DELETE FROM maintenance_visit_assignees WHERE visit_id=?`, visitID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	var ownerName, ownerUID string
	for i, it := range items {
		name := strings.TrimSpace(it.AssigneeName)
		uid := strings.TrimSpace(it.UserID)
		if name == "" && uid != "" {
			name, uid = bindStaff(r.db, "", uid)
		} else if name != "" {
			name, uid = bindStaff(r.db, name, uid)
		}
		if name == "" && uid == "" {
			continue
		}
		role := strings.TrimSpace(it.Role)
		if role != model.WBMemberSupport {
			role = model.WBMemberOwner
		}
		if ownerName == "" && role == model.WBMemberOwner {
			ownerName, ownerUID = name, uid
		}
		gid := strings.TrimSpace(it.WorkGroupID)
		if gid == "" {
			gid = strings.TrimSpace(groupID)
		}
		if _, err := r.db.Exec(`
			INSERT INTO maintenance_visit_assignees(visit_id, user_id, assignee_name, role, work_group_id, sort_order)
			VALUES (?,?,?,?,?,?)`, visitID, uid, name, role, nullIfEmpty(gid), i); err != nil {
			return err
		}
	}
	if ownerName == "" && len(items) > 0 {
		ownerName = strings.TrimSpace(items[0].AssigneeName)
		_, ownerUID = bindStaff(r.db, ownerName, items[0].UserID)
	}
	if ownerName != "" {
		_, err := r.db.Exec(`UPDATE maintenance_visits SET assignee=?, assignee_user_id=?, work_group_id=COALESCE(NULLIF(?,''), work_group_id) WHERE visit_id=?`,
			ownerName, nullIfEmpty(ownerUID), strings.TrimSpace(groupID), visitID)
		return err
	}
	return nil
}

func (r *MaintenanceRepo) applyVisitGroupAndDefaults(v *model.MaintenanceVisit) {
	if v == nil {
		return
	}
	if strings.TrimSpace(v.WorkGroupID) != "" {
		r.applyWorkGroupToVisit(v)
		return
	}
	if strings.TrimSpace(v.Assignee) != "" {
		return
	}
	owner, groupID := r.defaultOwnerForSite(v.CustomerID)
	if owner != "" {
		v.Assignee = owner
	}
	if v.WorkGroupID == "" {
		v.WorkGroupID = groupID
		if groupID != "" {
			r.applyWorkGroupToVisit(v)
		}
	}
}

func (r *MaintenanceRepo) applyWorkGroupToVisit(v *model.MaintenanceVisit) {
	gid := strings.TrimSpace(v.WorkGroupID)
	if gid == "" {
		return
	}
	wg := NewWorkGroupRepo(r.db)
	g, err := wg.Get(gid)
	if err != nil || g == nil {
		return
	}
	v.WorkGroupName = g.Name
	ms, _ := wg.MembersOn(gid, "")
	var owner string
	for _, m := range ms {
		if m.Role == model.WorkGroupLeader {
			owner = m.FullName
			break
		}
	}
	if strings.TrimSpace(v.Assignee) == "" && owner != "" {
		v.Assignee = owner
	}
}

func (r *MaintenanceRepo) syncVisitAssigneeRows(v model.MaintenanceVisit) error {
	if strings.TrimSpace(v.VisitID) == "" {
		return nil
	}
	existing, _ := r.ListVisitAssignees(v.VisitID)
	var items []model.MaintenanceVisitAssignee
	if gid := strings.TrimSpace(v.WorkGroupID); gid != "" {
		ms, _ := NewWorkGroupRepo(r.db).MembersOn(gid, "")
		for _, m := range ms {
			role := model.WBMemberSupport
			if m.Role == model.WorkGroupLeader {
				role = model.WBMemberOwner
			}
			items = append(items, model.MaintenanceVisitAssignee{
				UserID: m.UserID, AssigneeName: m.FullName, Role: role, WorkGroupID: gid,
			})
		}
	}
	if len(items) == 0 && strings.TrimSpace(v.Assignee) != "" {
		items = append(items, model.MaintenanceVisitAssignee{
			AssigneeName: v.Assignee, Role: model.WBMemberOwner, WorkGroupID: v.WorkGroupID,
		})
		for _, e := range existing {
			if e.Role != model.WBMemberSupport {
				continue
			}
			if strings.TrimSpace(e.AssigneeName) == strings.TrimSpace(v.Assignee) {
				continue
			}
			items = append(items, e)
		}
	}
	if len(items) == 0 {
		if len(existing) == 0 {
			return nil
		}
		items = existing
	}
	return r.ReplaceVisitAssignees(v.VisitID, items, v.WorkGroupID)
}

func (r *MaintenanceRepo) defaultOwnerForSite(customerID string) (owner, groupID string) {
	items, _ := r.ListSiteAssignees(customerID)
	if len(items) > 0 {
		return firstOwner(items)
	}
	cfg, _ := r.GetSiteConfig(customerID)
	if cfg == nil || strings.TrimSpace(cfg.Region) == "" {
		return "", ""
	}
	org := strings.TrimSpace(cfg.OrgID)
	regs, _ := r.ListRegionAssignees(org, cfg.Region)
	return firstRegionOwner(regs)
}

func firstOwner(items []model.MaintenanceSiteAssignee) (string, string) {
	var owner, gid string
	for _, it := range items {
		if it.Role == model.WBMemberOwner || owner == "" {
			if strings.TrimSpace(it.Name) != "" {
				owner = it.Name
			}
			if strings.TrimSpace(it.WorkGroupID) != "" {
				gid = it.WorkGroupID
			}
			if it.Role == model.WBMemberOwner {
				break
			}
		}
	}
	return owner, gid
}

func firstRegionOwner(items []model.MaintenanceRegionAssignee) (string, string) {
	var owner, gid string
	for _, it := range items {
		if it.Role == model.WBMemberOwner || owner == "" {
			if strings.TrimSpace(it.Name) != "" {
				owner = it.Name
			}
			if strings.TrimSpace(it.WorkGroupID) != "" {
				gid = it.WorkGroupID
			}
			if it.Role == model.WBMemberOwner {
				break
			}
		}
	}
	return owner, gid
}
