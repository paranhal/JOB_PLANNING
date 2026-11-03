package repository

import (
	"strings"

	"customer-support/internal/model"
)

type importSiteRow struct {
	ID        string
	Name      string
	Official  string
	Short     string
	OrgID     string
	OrgLabel  string
}

type importSiteIndex struct {
	rows []importSiteRow
}

func (r *CustomerRepo) LoadImportSiteIndex() (*importSiteIndex, error) {
	idx := &importSiteIndex{}
	if r == nil || r.db == nil {
		return idx, nil
	}
	q := `
		SELECT c.customer_id,
		       COALESCE(c.org_name,''),
		       COALESCE(c.official_name,''),
		       COALESCE(cfg.short_name,''),
		       TRIM(COALESCE(c.org_id,'')),
		       COALESCE(NULLIF(TRIM(o.org_name),''), NULLIF(TRIM(o.short_name),''), TRIM(COALESCE(c.org_id,'')))
		  FROM customers c
		  LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = c.customer_id
		  LEFT JOIN orgs o ON o.org_id = c.org_id
		 WHERE COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')='customer'
		   AND c.is_active=1`
	rows, err := r.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it importSiteRow
		if err := rows.Scan(&it.ID, &it.Name, &it.Official, &it.Short, &it.OrgID, &it.OrgLabel); err != nil {
			return nil, err
		}
		idx.rows = append(idx.rows, it)
	}
	return idx, rows.Err()
}

func (idx *importSiteIndex) matches(name string) []importSiteRow {
	key := model.NormalizeOrgLookupName(name)
	if key == "" || idx == nil {
		return nil
	}
	var out []importSiteRow
	for _, it := range idx.rows {
		if model.NormalizeOrgLookupName(it.Name) == key ||
			model.NormalizeOrgLookupName(it.Official) == key ||
			model.NormalizeOrgLookupName(it.Short) == key {
			out = append(out, it)
		}
	}
	return out
}

type importSiteDecision struct {
	Hit  CustomerOption
	Kind string
	Msg  string
}

// Decide 95-2E: 고객은 전사 공용. customers.org_id 로 거절하지 않는다.
// 업로더 조직은 자산 행 org_id 에만 쓴다. Lookup SQL 은 건드리지 않는다.
func (idx *importSiteIndex) Decide(_, name string) importSiteDecision {
	name = strings.TrimSpace(name)
	if name == "" {
		return importSiteDecision{Kind: model.AssetImportErrMissing, Msg: "기관명이 없습니다"}
	}
	all := idx.matches(name)
	if len(all) == 0 {
		return importSiteDecision{
			Kind: model.AssetImportErrMissing,
			Msg:  "등록된 기관 없음: " + name,
		}
	}
	if len(all) > 1 {
		return importSiteDecision{
			Kind: model.AssetImportErrDup,
			Msg:  "동명 여러 개입니다: " + name,
		}
	}
	it := all[0]
	return importSiteDecision{
		Hit: CustomerOption{ID: it.ID, Name: it.Name, ShortName: it.Short},
	}
}

// LookupNormalizedInOrg 이름 정규화 일치 1건. 선택 목록 Lookup SQL 은 그대로 둔다.
func (r *CustomerRepo) LookupNormalizedInOrg(orgID, name string) ([]CustomerOption, error) {
	idx, err := r.LoadImportSiteIndex()
	if err != nil {
		return nil, err
	}
	d := idx.Decide(orgID, name)
	if d.Kind == "" && d.Hit.ID != "" {
		return []CustomerOption{d.Hit}, nil
	}
	return []CustomerOption{}, nil
}
