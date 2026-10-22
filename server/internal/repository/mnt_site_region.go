package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func FilterMaintenanceSites(items []model.MaintenanceSiteConfig, region, cycle, q, batch string) []model.MaintenanceSiteConfig {
	region = strings.TrimSpace(region)
	cycle = strings.TrimSpace(cycle)
	q = strings.ToLower(strings.TrimSpace(q))
	batch = strings.TrimSpace(batch)
	var out []model.MaintenanceSiteConfig
	for _, it := range items {
		reg := strings.TrimSpace(it.Region)
		if region == "none" {
			if reg != "" {
				continue
			}
		} else if region != "" && reg != region {
			continue
		}
		if cycle != "" && strings.TrimSpace(it.InspectionCycle) != cycle {
			continue
		}
		if batch != "" && strings.TrimSpace(it.BulkBatchID) != batch {
			continue
		}
		if q != "" {
			hay := strings.ToLower(it.ShortName + " " + it.OrgName + " " + it.CustomerID + " " + reg)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func DistinctSiteRegions(items []model.MaintenanceSiteConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		r := strings.TrimSpace(it.Region)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

func (r *MaintenanceRepo) SiteDefaultOwnerName(customerID string) string {
	n, _ := r.defaultOwnerForSite(customerID)
	return n
}

func (r *MaintenanceRepo) BulkSetSiteRegions(orgID string, ids []string, region string) (*model.SiteRegionBulkResult, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, fmt.Errorf("지역을 입력하세요")
	}
	return r.applySiteRegions(orgID, ids, func(cur string, customerID string) (string, string) {
		if cur == region {
			return "", ""
		}
		return region, ""
	})
}

func (r *MaintenanceRepo) FillSiteRegionsFromAddress(orgID string, ids []string) (*model.SiteRegionBulkResult, error) {
	return r.applySiteRegions(orgID, ids, func(cur string, customerID string) (string, string) {
		if strings.TrimSpace(cur) != "" {
			return "", ""
		}
		got := r.customerRegionFromDB(customerID)
		if got == "" {
			return "", "주소에서 지역을 알 수 없음"
		}
		return got, ""
	})
}

func (r *MaintenanceRepo) customerRegionFromDB(customerID string) string {
	var sido, sigungu, addr string
	_ = r.db.QueryRow(`SELECT COALESCE(addr_sido,''), COALESCE(addr_sigungu,''), COALESCE(address,'') FROM customers WHERE customer_id=?`,
		customerID).Scan(&sido, &sigungu, &addr)
	return ExtractKoreaRegion(strings.TrimSpace(sido + " " + sigungu + " " + addr))
}

func (r *MaintenanceRepo) applySiteRegions(orgID string, ids []string, next func(cur, customerID string) (region, skipReason string)) (*model.SiteRegionBulkResult, error) {
	if len(ids) > model.SiteBulkMax {
		return nil, fmt.Errorf("한 번에 최대 %d곳까지 지정할 수 있습니다", model.SiteBulkMax)
	}
	org := strings.TrimSpace(orgID)
	if org == "" || org == OrgAll {
		return nil, fmt.Errorf("조직을 선택하세요")
	}
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
	res := &model.SiteRegionBulkResult{}
	if len(uniq) == 0 {
		return res, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range uniq {
		var cur, siteOrg, name string
		err := tx.QueryRow(`
			SELECT COALESCE(s.region,''), COALESCE(NULLIF(TRIM(s.org_id),''), COALESCE(c.org_id,'')), COALESCE(s.short_name,c.org_name,'')
			FROM maintenance_site_config s
			LEFT JOIN customers c ON c.customer_id=s.customer_id
			WHERE s.customer_id=?`, id).Scan(&cur, &siteOrg, &name)
		if err == sql.ErrNoRows {
			res.Skipped = append(res.Skipped, model.SiteBulkSkip{CustomerID: id, Name: id, Reason: model.SiteBulkSkipNotFound})
			continue
		}
		if err != nil {
			return nil, err
		}
		if siteOrg != org && !(siteOrg == "" && org == model.OrgIDLibrary) {
			res.Skipped = append(res.Skipped, model.SiteBulkSkip{CustomerID: id, Name: name, Reason: model.SiteBulkSkipOtherOrg})
			continue
		}
		want, skip := next(cur, id)
		if skip != "" {
			res.Skipped = append(res.Skipped, model.SiteBulkSkip{CustomerID: id, Name: name, Reason: skip})
			continue
		}
		if want == "" || want == strings.TrimSpace(cur) {
			continue
		}
		if _, err := tx.Exec(`UPDATE maintenance_site_config SET region=?, updated_at=CURRENT_TIMESTAMP WHERE customer_id=?`, want, id); err != nil {
			return nil, err
		}
		res.Changed++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
