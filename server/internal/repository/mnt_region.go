package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *MaintenanceRepo) ListRegionAssignees(orgID, region string) ([]model.MaintenanceRegionAssignee, error) {
	orgID, region = strings.TrimSpace(orgID), strings.TrimSpace(region)
	if orgID == "" || orgID == OrgAll || region == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT org_id, region, COALESCE(user_id,''), COALESCE(work_group_id,''), role, COALESCE(sort_order,0)
		FROM maintenance_region_assignees WHERE org_id=? AND region=? ORDER BY sort_order, role`, orgID, region)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceRegionAssignee
	for rows.Next() {
		var a model.MaintenanceRegionAssignee
		if err := rows.Scan(&a.OrgID, &a.Region, &a.UserID, &a.WorkGroupID, &a.Role, &a.SortOrder); err != nil {
			return nil, err
		}
		a.Name = r.userDisplay(a.UserID)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *MaintenanceRepo) ReplaceRegionAssignees(orgID, region string, items []model.MaintenanceRegionAssignee) error {
	orgID, region = strings.TrimSpace(orgID), strings.TrimSpace(region)
	if orgID == "" || orgID == OrgAll || region == "" {
		return fmt.Errorf("조직과 지역이 필요합니다")
	}
	if _, err := r.db.Exec(`DELETE FROM maintenance_region_assignees WHERE org_id=? AND region=?`, orgID, region); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for i, it := range items {
		uid := strings.TrimSpace(it.UserID)
		gid := strings.TrimSpace(it.WorkGroupID)
		if uid == "" && gid == "" {
			continue
		}
		role := it.Role
		if role != model.WBMemberSupport {
			role = model.WBMemberOwner
		}
		if _, err := r.db.Exec(`
			INSERT INTO maintenance_region_assignees(org_id, region, user_id, work_group_id, role, sort_order)
			VALUES (?,?,?,?,?,?)`, orgID, region, nullIfEmpty(uid), nullIfEmpty(gid), role, i); err != nil {
			return err
		}
	}
	return nil
}

func (r *MaintenanceRepo) ListSiteAssignees(customerID string) ([]model.MaintenanceSiteAssignee, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT customer_id, COALESCE(user_id,''), COALESCE(work_group_id,''), role, COALESCE(sort_order,0)
		FROM maintenance_site_assignees WHERE customer_id=? ORDER BY sort_order, role`, customerID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceSiteAssignee
	for rows.Next() {
		var a model.MaintenanceSiteAssignee
		if err := rows.Scan(&a.CustomerID, &a.UserID, &a.WorkGroupID, &a.Role, &a.SortOrder); err != nil {
			return nil, err
		}
		a.Name = r.userDisplay(a.UserID)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *MaintenanceRepo) ReplaceSiteAssignees(customerID string, items []model.MaintenanceSiteAssignee) error {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil
	}
	if _, err := r.db.Exec(`DELETE FROM maintenance_site_assignees WHERE customer_id=?`, customerID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	for i, it := range items {
		uid := strings.TrimSpace(it.UserID)
		gid := strings.TrimSpace(it.WorkGroupID)
		if uid == "" && gid == "" {
			continue
		}
		role := it.Role
		if role != model.WBMemberSupport {
			role = model.WBMemberOwner
		}
		if _, err := r.db.Exec(`
			INSERT INTO maintenance_site_assignees(customer_id, user_id, work_group_id, role, sort_order)
			VALUES (?,?,?,?,?)`, customerID, nullIfEmpty(uid), nullIfEmpty(gid), role, i); err != nil {
			return err
		}
	}
	return nil
}

func (r *MaintenanceRepo) ApplyRegionDefaultsToOpenVisits(orgID, region string) error {
	orgID, region = strings.TrimSpace(orgID), strings.TrimSpace(region)
	if orgID == "" || orgID == OrgAll || region == "" {
		return nil
	}
	rows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(NULLIF(TRIM(c.org_id),''), '')
		FROM maintenance_visits v
		JOIN maintenance_site_config s ON s.customer_id=v.customer_id
		LEFT JOIN customers c ON c.customer_id=v.customer_id
		WHERE COALESCE(v.completed,0)=0 AND TRIM(COALESCE(s.region,''))=?`, region)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id, rowOrg string
		if err := rows.Scan(&id, &rowOrg); err != nil {
			return err
		}
		if rowOrg != "" && rowOrg != orgID {
			continue
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		v, err := r.GetVisit(id)
		if err != nil || v == nil {
			continue
		}
		v.Assignee = ""
		v.WorkGroupID = ""
		r.applyVisitGroupAndDefaults(v)
		if err := r.UpdateVisit(*v); err != nil {
			return err
		}
	}
	return nil
}

func (r *MaintenanceRepo) userDisplay(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	u, _ := NewUserRepo(r.db).GetByID(userID)
	if u == nil {
		return ""
	}
	return u.FullName
}

func (r *MaintenanceRepo) annotateUnassignedRegion(orgID string, slots []model.UnassignedMonthSlot) {
	for i := range slots {
		cfg, _ := r.GetSiteConfig(slots[i].CustomerID)
		if cfg == nil {
			continue
		}
		slots[i].Region = strings.TrimSpace(cfg.Region)
		if slots[i].Region == "" {
			continue
		}
		org := strings.TrimSpace(cfg.OrgID)
		if org == "" {
			org = orgID
		}
		name, _ := r.defaultOwnerForSite(slots[i].CustomerID)
		if name != "" && strings.TrimSpace(slots[i].LastAssignee) == "" {
			slots[i].RegionPending = true
			slots[i].RegionAssigneeName = name
			slots[i].LastAssignee = name
		}
	}
}
