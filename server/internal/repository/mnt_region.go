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
	invalidateOpenUnassignedSlotsCache()
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
	invalidateOpenUnassignedSlotsCache()
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

func (r *MaintenanceRepo) userDisplayMap(ids []string) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	var uniq []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out
	}
	args := make([]any, len(uniq))
	ph := make([]string, len(uniq))
	for i, id := range uniq {
		args[i] = id
		ph[i] = "?"
	}
	q := `SELECT user_id, COALESCE(full_name,'') FROM users WHERE user_id IN (` + strings.Join(ph, ",") + `)`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		for _, id := range uniq {
			out[id] = r.userDisplay(id)
		}
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out[id] = name
	}
	return out
}

func (r *MaintenanceRepo) loadSiteAssigneesByCustomer() (map[string][]model.MaintenanceSiteAssignee, error) {
	out := map[string][]model.MaintenanceSiteAssignee{}
	rows, err := r.db.Query(`
		SELECT customer_id, COALESCE(user_id,''), COALESCE(work_group_id,''), role, COALESCE(sort_order,0)
		FROM maintenance_site_assignees ORDER BY customer_id, sort_order, role`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	var raw []model.MaintenanceSiteAssignee
	for rows.Next() {
		var a model.MaintenanceSiteAssignee
		if err := rows.Scan(&a.CustomerID, &a.UserID, &a.WorkGroupID, &a.Role, &a.SortOrder); err != nil {
			return nil, err
		}
		raw = append(raw, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(raw))
	for _, a := range raw {
		ids = append(ids, a.UserID)
	}
	names := r.userDisplayMap(ids)
	for _, a := range raw {
		a.Name = names[a.UserID]
		out[a.CustomerID] = append(out[a.CustomerID], a)
	}
	return out, nil
}

func (r *MaintenanceRepo) loadRegionAssigneesByKey() (map[string][]model.MaintenanceRegionAssignee, error) {
	out := map[string][]model.MaintenanceRegionAssignee{}
	rows, err := r.db.Query(`
		SELECT org_id, region, COALESCE(user_id,''), COALESCE(work_group_id,''), role, COALESCE(sort_order,0)
		FROM maintenance_region_assignees ORDER BY org_id, region, sort_order, role`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	var raw []model.MaintenanceRegionAssignee
	for rows.Next() {
		var a model.MaintenanceRegionAssignee
		if err := rows.Scan(&a.OrgID, &a.Region, &a.UserID, &a.WorkGroupID, &a.Role, &a.SortOrder); err != nil {
			return nil, err
		}
		raw = append(raw, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(raw))
	for _, a := range raw {
		ids = append(ids, a.UserID)
	}
	names := r.userDisplayMap(ids)
	for _, a := range raw {
		a.Name = names[a.UserID]
		key := strings.TrimSpace(a.OrgID) + "|" + strings.TrimSpace(a.Region)
		out[key] = append(out[key], a)
	}
	return out, nil
}

func defaultOwnerFromMaps(customerID string, cfg *model.MaintenanceSiteConfig, siteBy map[string][]model.MaintenanceSiteAssignee, regionBy map[string][]model.MaintenanceRegionAssignee) string {
	if items := siteBy[customerID]; len(items) > 0 {
		n, _ := firstOwner(items)
		return n
	}
	if cfg == nil || strings.TrimSpace(cfg.Region) == "" {
		return ""
	}
	org := strings.TrimSpace(cfg.OrgID)
	n, _ := firstRegionOwner(regionBy[org+"|"+strings.TrimSpace(cfg.Region)])
	return n
}

func (r *MaintenanceRepo) annotateUnassignedRegionSlow(orgID string, slots []model.UnassignedMonthSlot) {
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
		_ = org
		name, _ := r.defaultOwnerForSite(slots[i].CustomerID)
		if name != "" && strings.TrimSpace(slots[i].LastAssignee) == "" {
			slots[i].RegionPending = true
			slots[i].RegionAssigneeName = name
			slots[i].LastAssignee = name
		}
	}
}

func (r *MaintenanceRepo) annotateUnassignedRegion(orgID string, slots []model.UnassignedMonthSlot) {
	configs, err := r.ListSiteConfigs()
	if err != nil {
		r.annotateUnassignedRegionSlow(orgID, slots)
		return
	}
	r.annotateUnassignedRegionFrom(orgID, slots, configs)
}

func (r *MaintenanceRepo) annotateUnassignedRegionFrom(orgID string, slots []model.UnassignedMonthSlot, configs []model.MaintenanceSiteConfig) {
	_ = orgID
	if len(slots) == 0 {
		return
	}
	cfgBy := map[string]model.MaintenanceSiteConfig{}
	for _, c := range configs {
		cfgBy[c.CustomerID] = c
	}
	siteBy, err := r.loadSiteAssigneesByCustomer()
	if err != nil {
		r.annotateUnassignedRegionSlow(orgID, slots)
		return
	}
	regionBy, err := r.loadRegionAssigneesByKey()
	if err != nil {
		r.annotateUnassignedRegionSlow(orgID, slots)
		return
	}
	for i := range slots {
		cfg, ok := cfgBy[slots[i].CustomerID]
		if !ok {
			continue
		}
		slots[i].Region = strings.TrimSpace(cfg.Region)
		if slots[i].Region == "" {
			continue
		}
		name := defaultOwnerFromMaps(slots[i].CustomerID, &cfg, siteBy, regionBy)
		if name != "" && strings.TrimSpace(slots[i].LastAssignee) == "" {
			slots[i].RegionPending = true
			slots[i].RegionAssigneeName = name
			slots[i].LastAssignee = name
		}
	}
}
