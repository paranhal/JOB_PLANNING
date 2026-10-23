package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"customer-support/internal/model"
)

func industryMatchSQL(alias, key string) (string, []any) {
	k := model.IndustryKey(key)
	col := alias + ".industry"
	if k == model.IndustryUnspecified {
		return `TRIM(COALESCE(` + col + `,''))=''`, nil
	}
	raws := model.IndustryRawValues(k)
	if len(raws) == 0 {
		return `1=0`, nil
	}
	ph := make([]string, len(raws))
	args := make([]any, len(raws))
	for i, v := range raws {
		ph[i] = "?"
		args[i] = v
	}
	return `TRIM(COALESCE(` + col + `,'')) IN (` + strings.Join(ph, ",") + `)`, args
}

func (r *MaintenanceRepo) ListSiteBulkIndustries() ([]model.SiteBulkIndustry, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(industry,''), COUNT(*)
		FROM customers
		WHERE is_active=1 AND COALESCE(party_kind,'customer')='customer'
		GROUP BY COALESCE(industry,'')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	merged := map[string]*model.SiteBulkIndustry{}
	for rows.Next() {
		var raw string
		var n int
		if err := rows.Scan(&raw, &n); err != nil {
			return nil, err
		}
		key := model.IndustryKey(raw)
		if b, ok := merged[key]; ok {
			b.Count += n
			continue
		}
		merged[key] = &model.SiteBulkIndustry{Key: key, Label: key, Count: n}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]model.SiteBulkIndustry, 0, len(merged))
	for _, b := range merged {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == model.IndustryUnspecified {
			return false
		}
		if out[j].Key == model.IndustryUnspecified {
			return true
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func (r *MaintenanceRepo) ListSiteBulkCandidates(industryKey, workOrgID string) ([]model.SiteBulkCandidate, error) {
	clause, args := industryMatchSQL("c", industryKey)
	q := `
		SELECT c.customer_id, COALESCE(c.org_name,''), COALESCE(p.org_name,''),
		       COALESCE(c.addr_sido,''), COALESCE(c.addr_sigungu,''), COALESCE(c.address,''),
		       COALESCE(c.main_phone,''), COALESCE(s.customer_id,''), COALESCE(s.org_id,''),
		       COALESCE(o.org_name,''), COALESCE(s.short_name,'')
		FROM customers c
		LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
		LEFT JOIN maintenance_site_config s ON s.customer_id = c.customer_id
		LEFT JOIN orgs o ON o.org_id = NULLIF(s.org_id,'')
		WHERE c.is_active=1 AND COALESCE(c.party_kind,'customer')='customer'
		  AND ` + clause + `
		ORDER BY c.org_name COLLATE NOCASE`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type raw struct {
		id, name, parent, sido, sigungu, addr, phone, siteID, siteOrg, orgName, short string
	}
	var list []raw
	for rows.Next() {
		var x raw
		if err := rows.Scan(&x.id, &x.name, &x.parent, &x.sido, &x.sigungu, &x.addr, &x.phone, &x.siteID, &x.siteOrg, &x.orgName, &x.short); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	existingFold := r.siteNameFolds(workOrgID)
	out := make([]model.SiteBulkCandidate, 0, len(list))
	for _, x := range list {
		if x.siteID != "" && workOrgID != "" && x.siteOrg == workOrgID {
			continue
		}
		region := ExtractKoreaRegion(strings.TrimSpace(x.sido + " " + x.sigungu + " " + x.addr))
		row := model.SiteBulkCandidate{
			CustomerID:    x.id,
			OrgName:       x.name,
			ParentName:    x.parent,
			RegionPreview: region,
			ShortName:     x.name,
			Selectable:    x.siteID == "",
			WarnNoRegion:  region == "",
			WarnEmpty:     strings.TrimSpace(x.phone) == "" || strings.TrimSpace(x.addr+x.sido+x.sigungu) == "",
		}
		if x.siteID != "" {
			row.Selectable = false
			row.ShortName = ""
			row.OtherOrgName = x.orgName
			if row.OtherOrgName == "" {
				row.OtherOrgName = x.siteOrg
			}
		} else if existingFold[foldSiteName(x.name)] {
			row.WarnSimilar = true
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *MaintenanceRepo) siteNameFolds(orgID string) map[string]bool {
	out := map[string]bool{}
	rows, err := r.db.Query(`
		SELECT COALESCE(s.short_name,''), COALESCE(c.org_name,'')
		FROM maintenance_site_config s
		JOIN customers c ON c.customer_id=s.customer_id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var short, name string
		if err := rows.Scan(&short, &name); err != nil {
			return out
		}
		if k := foldSiteName(short); k != "" {
			out[k] = true
		}
		if k := foldSiteName(name); k != "" {
			out[k] = true
		}
	}
	return out
}

func foldSiteName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (r *MaintenanceRepo) BulkRegisterSites(in model.SiteBulkRegisterInput) (*model.SiteBulkRegisterResult, error) {
	if len(in.CustomerIDs) > model.SiteBulkMax {
		return nil, fmt.Errorf("한 번에 최대 %d곳까지 등록할 수 있습니다", model.SiteBulkMax)
	}
	if !model.ValidInspectionCycle(in.Cycle) {
		return nil, fmt.Errorf("점검주기를 선택하세요")
	}
	org := strings.TrimSpace(in.OrgID)
	if org == "" || org == OrgAll {
		return nil, fmt.Errorf("조직을 선택하세요")
	}
	rounds := in.Rounds
	if rounds < 1 {
		rounds = 1
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range in.CustomerIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	res := &model.SiteBulkRegisterResult{}
	if len(ids) == 0 {
		return res, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	batchID := randomID("b15")
	fallback := strings.TrimSpace(in.FallbackRegion)
	ins, err := tx.Prepare(`
		INSERT INTO maintenance_site_config
		(customer_id, short_name, region, has_klas, has_rfid, inspection_cycle, entry_category, fixed_rule, rounds_per_period, org_id, bulk_batch_id, updated_at)
		VALUES (?,?,?,0,0,?,?,?,?,?,?, CURRENT_TIMESTAMP)`)
	if err != nil {
		return nil, err
	}
	defer ins.Close()
	prodOK := map[string]bool{}
	for _, pid := range in.ProductIDs {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		var pOrg string
		err := tx.QueryRow(`SELECT org_id FROM inspection_products WHERE product_id=? AND is_active=1`, pid).Scan(&pOrg)
		if err == nil && pOrg == org {
			prodOK[pid] = true
		}
	}
	industry := model.IndustryKey(in.IndustryKey)
	for _, id := range ids {
		skip, name := r.bulkValidateOne(tx, id, org, industry)
		if skip != "" {
			res.Skipped = append(res.Skipped, model.SiteBulkSkip{CustomerID: id, Name: name, Reason: skip})
			continue
		}
		short := strings.TrimSpace(in.ShortNames[id])
		if short == "" {
			short = name
		}
		if short == "" {
			short = id
		}
		region := r.customerRegionPreview(tx, id)
		if region == "" && fallback != "" {
			region = fallback
		}
		if _, err := ins.Exec(id, short, region, in.Cycle, "normal", nil, rounds, org, batchID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(region) == "" {
			res.NoRegion++
		}
		if _, err := tx.Exec(`DELETE FROM site_inspection_products WHERE customer_id=?`, id); err != nil {
			return nil, err
		}
		for pid := range prodOK {
			if _, err := tx.Exec(`INSERT INTO site_inspection_products (customer_id, product_id) VALUES (?,?)`, id, pid); err != nil {
				return nil, err
			}
		}
		res.Registered++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if res.Registered > 0 {
		res.BatchID = batchID
	}
	label := in.ActorLabel
	if label == "" {
		label = in.IndustryKey
	}
	logCreateWithReason(r.db, "maintenance_site_config", "customer_id", "bulk:"+org,
		label, fmt.Sprintf("업종별 일괄등록 업종=%s 주기=%s 회차=%d 등록=%d 건너뜀=%d",
			industry, in.Cycle, rounds, res.Registered, len(res.Skipped)))
	if res.Registered > 0 {
		invalidateOpenUnassignedSlotsCache()
	}
	return res, nil
}

func (r *MaintenanceRepo) bulkValidateOne(tx *sql.Tx, customerID, org, industryKey string) (reason, name string) {
	var active int
	var kind, rawInd, cname string
	err := tx.QueryRow(`
		SELECT is_active, COALESCE(party_kind,'customer'), COALESCE(industry,''), COALESCE(org_name,'')
		FROM customers WHERE customer_id=?`, customerID).Scan(&active, &kind, &rawInd, &cname)
	if err == sql.ErrNoRows {
		return model.SiteBulkSkipNotFound, customerID
	}
	if err != nil {
		return model.SiteBulkSkipNotFound, customerID
	}
	if active != 1 {
		return model.SiteBulkSkipInactive, cname
	}
	if kind != "customer" {
		return model.SiteBulkSkipPartner, cname
	}
	if model.IndustryKey(rawInd) != industryKey {
		return model.SiteBulkSkipIndustry, cname
	}
	var siteOrg string
	err = tx.QueryRow(`SELECT COALESCE(org_id,'') FROM maintenance_site_config WHERE customer_id=?`, customerID).Scan(&siteOrg)
	if err == nil {
		if siteOrg == org || siteOrg == "" {
			return model.SiteBulkSkipAlready, cname
		}
		return model.SiteBulkSkipOtherOrg, cname
	}
	if err != sql.ErrNoRows {
		return model.SiteBulkSkipAlready, cname
	}
	return "", cname
}

func (r *MaintenanceRepo) customerRegionPreview(tx *sql.Tx, customerID string) string {
	var sido, sigungu, addr string
	_ = tx.QueryRow(`SELECT COALESCE(addr_sido,''), COALESCE(addr_sigungu,''), COALESCE(address,'') FROM customers WHERE customer_id=?`,
		customerID).Scan(&sido, &sigungu, &addr)
	return ExtractKoreaRegion(strings.TrimSpace(sido + " " + sigungu + " " + addr))
}
