package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type SiteProgress struct {
	Installed int
	Inspected int
	Denom     int
	Excluded  int
	AllDone   bool
}

func (r *ProjectEquipRepo) SiteProgress(projectID string) (SiteProgress, error) {
	var p SiteProgress
	rows, err := r.db.Query(`SELECT status, COUNT(*) FROM project_sites WHERE project_id=? AND is_active=1 GROUP BY status`, projectID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return p, err
		}
		ns := model.NormalizeSiteStatus(st)
		if ns == model.SiteExcluded {
			p.Excluded += n
			continue
		}
		p.Denom += n
		if model.SiteStatusCountsAsInstalled(st) {
			p.Installed += n
		}
		if ns == model.SiteInspected {
			p.Inspected += n
		}
	}
	p.AllDone = p.Denom > 0 && p.Inspected == p.Denom
	return p, rows.Err()
}

type SiteAddSkip struct {
	CustomerID string `json:"customer_id"`
	Reason     string `json:"reason"`
	Name       string `json:"name,omitempty"`
}

type SiteAddResult struct {
	Added        int
	Skipped      int
	AddedIDs     []string
	SkippedItems []SiteAddSkip
}

func siteNameWithRegion(name, sigungu, sido string) string {
	name = strings.TrimSpace(name)
	g := strings.TrimSpace(sigungu)
	if g == "" {
		g = strings.TrimSpace(sido)
	}
	if g == "" || strings.Contains(name, "("+g+")") {
		return name
	}
	return name + " (" + g + ")"
}

func (r *ProjectEquipRepo) siteCustomerOnProject(projectID, customerID string) (bool, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id=? AND customer_id=? AND is_active=1`,
		projectID, customerID).Scan(&n)
	return n > 0, err
}

func (r *ProjectEquipRepo) AddSitesFromCustomers(projectID, orgID, userID string, customerIDs []string) (SiteAddResult, error) {
	var out SiteAddResult
	type row struct {
		id, name, org, sido, sigungu string
	}
	seen := map[string]bool{}
	var rows []row
	for _, cid := range customerIDs {
		cid = strings.TrimSpace(cid)
		if cid == "" || seen[cid] {
			continue
		}
		seen[cid] = true
		var rec row
		rec.id = cid
		err := r.db.QueryRow(`SELECT org_name, org_id, COALESCE(addr_sido,''), COALESCE(addr_sigungu,'') FROM customers WHERE customer_id=?`, cid).
			Scan(&rec.name, &rec.org, &rec.sido, &rec.sigungu)
		if err != nil {
			return out, fmt.Errorf("고객을 찾을 수 없습니다")
		}
		if rec.org != orgID {
			return SiteAddResult{}, fmt.Errorf("다른 조직 고객입니다")
		}
		rows = append(rows, rec)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, rec := range rows {
		ok, err := r.siteCustomerOnProject(projectID, rec.id)
		if err != nil {
			return out, err
		}
		if ok {
			out.Skipped++
			out.SkippedItems = append(out.SkippedItems, SiteAddSkip{CustomerID: rec.id, Name: rec.name, Reason: "이미 이 사업에 등록됨"})
			continue
		}
		s := &model.ProjectSite{
			ProjectID: projectID, CustomerID: rec.id, SiteName: rec.name,
			Region: rec.sigungu, Status: model.SitePlanned, OrgID: orgID,
		}
		if s.Region == "" {
			s.Region = rec.sido
		}
		if err := r.SaveSite(s); err != nil {
			if !strings.Contains(err.Error(), "같은") && !strings.Contains(err.Error(), "이미 대상") {
				return out, err
			}
			alt := siteNameWithRegion(rec.name, rec.sigungu, rec.sido)
			if alt == rec.name {
				out.Skipped++
				out.SkippedItems = append(out.SkippedItems, SiteAddSkip{CustomerID: rec.id, Name: rec.name, Reason: "같은 이름 사이트가 있음"})
				continue
			}
			s.ID = ""
			s.SiteName = alt
			if err2 := r.SaveSite(s); err2 != nil {
				out.Skipped++
				out.SkippedItems = append(out.SkippedItems, SiteAddSkip{CustomerID: rec.id, Name: rec.name, Reason: "같은 이름 사이트가 있음"})
				continue
			}
		}
		_, _ = r.db.Exec(`INSERT INTO project_site_events(site_id,project_id,from_status,to_status,event_date,note,created_by,source)
			VALUES(?,?,?,?,?,?,?,?)`, s.ID, projectID, "", model.SitePlanned, now[:10], "대상 추가", userID, "ui")
		out.Added++
		out.AddedIDs = append(out.AddedIDs, rec.id)
	}
	return out, nil
}

func (r *ProjectEquipRepo) BulkSiteStatus(projectID, userID, to, eventDate, reason string, ids []string) error {
	return r.applyBulkSiteStatus(projectID, userID, to, eventDate, reason, "ui", ids)
}

func (r *ProjectEquipRepo) applyBulkSiteStatus(projectID, userID, to, eventDate, reason, source string, ids []string) error {
	to = model.NormalizeSiteStatus(to)
	if to == "" {
		return fmt.Errorf("상태를 확인하세요")
	}
	if eventDate == "" {
		eventDate = time.Now().Format("2006-01-02")
	}
	if strings.TrimSpace(source) == "" {
		source = "ui"
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cur, err := r.GetSite(id)
		if err != nil || cur.ProjectID != projectID {
			return fmt.Errorf("사이트를 찾을 수 없습니다")
		}
		from := cur.Status
		cur.Status = to
		if to == model.SiteInstalled {
			cur.InstalledAt = eventDate
		}
		if to == model.SiteInspected {
			if cur.InstalledAt == "" {
				cur.InstalledAt = eventDate
			}
			cur.InspectedAt = eventDate
		}
		if to == model.SiteExcluded {
			cur.ExcludedReason = reason
		}
		if err := r.SaveSite(cur); err != nil {
			return err
		}
		if _, err := r.db.Exec(`UPDATE project_sites SET installed_at=?, inspected_at=?, excluded_reason=?, status_changed_at=?, status_changed_by=? WHERE id=?`,
			cur.InstalledAt, cur.InspectedAt, cur.ExcludedReason, now, userID, id); err != nil {
			return err
		}
		_, _ = r.db.Exec(`INSERT INTO project_site_events(site_id,project_id,from_status,to_status,event_date,note,created_by,source)
			VALUES(?,?,?,?,?,?,?,?)`, id, projectID, from, to, eventDate, reason, userID, source)
	}
	return nil
}

type SchoolPick struct {
	CustomerID string
	Name       string
	Region     string
	OrgCode    string
	Already    bool
}

type ParentOrg struct {
	CustomerID string
	Name       string
}

func (r *ProjectEquipRepo) ListParentOrgs(orgID string) ([]ParentOrg, error) {
	rows, err := r.db.Query(`SELECT customer_id, org_name FROM customers
		WHERE org_id=? AND is_active=1 AND (has_parent=0 OR parent_customer_id IS NULL OR TRIM(COALESCE(parent_customer_id,''))='')
		ORDER BY org_name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ParentOrg
	for rows.Next() {
		var p ParentOrg
		if err := rows.Scan(&p.CustomerID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectEquipRepo) ListSchoolPick(projectID, orgID, parentID, q, region string, page, size int) ([]SchoolPick, int, error) {
	if size != 50 {
		size = 100
	}
	if page < 1 {
		page = 1
	}
	where := `c.org_id=? AND c.is_active=1 AND c.parent_customer_id=?`
	args := []interface{}{orgID, parentID}
	if q = strings.TrimSpace(q); q != "" {
		where += ` AND (c.org_name LIKE ? OR COALESCE(c.org_code,'') LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if region = strings.TrimSpace(region); region != "" {
		where += ` AND COALESCE(c.addr_sigungu,'')=?`
		args = append(args, region)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM customers c WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := r.db.Query(`SELECT c.customer_id, c.org_name, COALESCE(c.addr_sigungu,''), COALESCE(c.org_code,''),
		EXISTS(SELECT 1 FROM project_sites s WHERE s.project_id=? AND s.customer_id=c.customer_id AND s.is_active=1)
		FROM customers c WHERE `+where+` ORDER BY c.org_name LIMIT ? OFFSET ?`,
		append([]interface{}{projectID}, args...)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SchoolPick
	for rows.Next() {
		var p SchoolPick
		var already int
		if err := rows.Scan(&p.CustomerID, &p.Name, &p.Region, &p.OrgCode, &already); err != nil {
			return nil, 0, err
		}
		p.Already = already == 1
		out = append(out, p)
	}
	return out, total, rows.Err()
}
