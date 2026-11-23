package repository

import (
	"customer-support/internal/model"
	"strings"
)

// 95-196: 사업 고객의 하위 기관을 사이트 후보로 보여 주고, 사이트 등록 화면의 기본 유형을 정한다.
// 등록은 기존 AddSitesFromCustomers 를 그대로 쓴다(새 저장 로직 없음).

// ChildSiteCandidate 하위 기관 한 곳.
type ChildSiteCandidate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Industry   string `json:"industry"` // 비어 있으면 「(분류 없음)」으로 묶는다
	Sido       string `json:"sido"`
	Sigungu    string `json:"sigungu"`
	Registered bool   `json:"registered"`
	Depth      int    `json:"depth"`
}

// ChildIndustryChip 업종별 건수.
type ChildIndustryChip struct {
	Industry string `json:"industry"`
	Count    int    `json:"count"`
}

// ChildSiteResult 하위 기관 후보 응답.
type ChildSiteResult struct {
	CustomerID   string               `json:"customerId"`
	CustomerName string               `json:"customerName"`
	Total        int                  `json:"total"`
	Unregistered int                  `json:"unregistered"`
	Chips        []ChildIndustryChip  `json:"chips"`
	Items        []ChildSiteCandidate `json:"items"`
}

const childIndustryNone = "(분류 없음)"
const maxChildDepth = 3

// ProjectSiteCustomerID 사업의 고객(없으면 발주처 고객). 하위 기관의 기준이다.
func (r *ProjectEquipRepo) ProjectSiteCustomerID(projectID string) (string, string) {
	if r == nil || r.db == nil {
		return "", ""
	}
	var cid, oid string
	_ = r.db.QueryRow(`SELECT COALESCE(TRIM(customer_id),''), COALESCE(TRIM(ordering_party_id),'') FROM work_projects WHERE project_id=?`, projectID).Scan(&cid, &oid)
	if cid == "" {
		cid = oid
	}
	if cid == "" {
		return "", ""
	}
	var name string
	_ = r.db.QueryRow(`SELECT COALESCE(NULLIF(TRIM(org_name),''), customer_id) FROM customers WHERE customer_id=?`, cid).Scan(&name)
	return cid, name
}

// ListChildCustomers 고객의 하위 기관(최대 3단계, 활성 party_kind=customer). 이미 이 사업에 등록된 곳은 Registered=true.
// 고객은 전사 공용이라 조직 필터를 걸지 않는다(95-167).
func (r *ProjectEquipRepo) ListChildCustomers(projectID, customerID string) (ChildSiteResult, error) {
	out := ChildSiteResult{CustomerID: customerID, Chips: []ChildIndustryChip{}, Items: []ChildSiteCandidate{}}
	customerID = strings.TrimSpace(customerID)
	if r == nil || r.db == nil || customerID == "" {
		return out, nil
	}
	_ = r.db.QueryRow(`SELECT COALESCE(NULLIF(TRIM(org_name),''), customer_id) FROM customers WHERE customer_id=?`, customerID).Scan(&out.CustomerName)
	reg, err := r.registeredCustomerIDs(projectID)
	if err != nil {
		return out, err
	}
	rows, err := r.db.Query(`
		WITH RECURSIVE ch(customer_id, depth) AS (
			SELECT customer_id, 1 FROM customers WHERE parent_customer_id=?
			UNION
			SELECT c.customer_id, ch.depth+1 FROM customers c JOIN ch ON c.parent_customer_id=ch.customer_id WHERE ch.depth<?
		)
		SELECT c.customer_id, COALESCE(NULLIF(TRIM(c.org_name),''), c.customer_id), COALESCE(TRIM(c.industry),''),
		       COALESCE(c.addr_sido,''), COALESCE(c.addr_sigungu,''), MIN(ch.depth)
		  FROM customers c JOIN ch ON ch.customer_id=c.customer_id
		 WHERE COALESCE(c.is_active,0)=1 AND COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')='customer'
		   AND c.customer_id<>?
		 GROUP BY c.customer_id
		 ORDER BY c.org_name, c.customer_id`, customerID, maxChildDepth, customerID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	chips := map[string]int{}
	var order []string
	for rows.Next() {
		var it ChildSiteCandidate
		var sido, sigungu string
		if err := rows.Scan(&it.ID, &it.Name, &it.Industry, &sido, &sigungu, &it.Depth); err != nil {
			return out, err
		}
		it.Sido = schoolSidoLabel(sido, sigungu, "", "")
		it.Sigungu = schoolSigunguLabel(sigungu)
		it.Registered = reg[it.ID]
		// 95-219: 업종이 여러 개인 하위 기관은 각 업종 칩에 1번씩 센다.
		labels := model.ParseIndustries(it.Industry)
		if len(labels) == 0 {
			labels = []string{childIndustryNone}
		}
		for _, label := range labels {
			if _, ok := chips[label]; !ok {
				order = append(order, label)
			}
			chips[label]++
		}
		out.Items = append(out.Items, it)
		if !it.Registered {
			out.Unregistered++
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.Total = len(out.Items)
	for _, k := range order {
		out.Chips = append(out.Chips, ChildIndustryChip{Industry: k, Count: chips[k]})
	}
	// 많은 업종부터
	for i := 1; i < len(out.Chips); i++ {
		for j := i; j > 0 && out.Chips[j].Count > out.Chips[j-1].Count; j-- {
			out.Chips[j], out.Chips[j-1] = out.Chips[j-1], out.Chips[j]
		}
	}
	return out, nil
}

// childSiteTypeOf 업종 → 유형 탭(기존 siteTypeOfIndustry 재사용). 업종이 비면 빈 문자열(기본 유형 판정에서 제외).
func childSiteTypeOf(industry string) string {
	if strings.TrimSpace(industry) == "" {
		return ""
	}
	return siteTypeOfIndustry(industry)
}

// childSiteTypesOf 업종이 여러 개면 유형도 여러 개다(「도서관,학교」는 도서관이자 학교). 업종이 비면 없다.
func childSiteTypesOf(industry string) []string {
	if strings.TrimSpace(industry) == "" {
		return nil
	}
	return siteTypesOfIndustry(industry)
}

// mostCommonSiteType 업종별 건수에서 가장 많은 유형. 동수면 school → public → library → private 순.
func mostCommonSiteType(counts map[string]int) string {
	best, bestN := "", 0
	for _, t := range []string{"school", "public", "library", "private"} {
		if counts[t] > bestN {
			best, bestN = t, counts[t]
		}
	}
	return best
}

// DefaultSiteType 사이트 등록 화면이 열릴 기본 유형(95-196). 쿼리 type 이 있으면 호출하는 쪽에서 그것을 먼저 쓴다.
//  1. 이 사업의 기존 사이트 고객 업종 중 최다 → 2. 사업 고객 하위 기관 업종 중 최다 → 3. school.
func (r *ProjectEquipRepo) DefaultSiteType(projectID string) string {
	if r == nil || r.db == nil {
		return "school"
	}
	counts := map[string]int{}
	rows, err := r.db.Query(`SELECT COALESCE(TRIM(c.industry),''), COUNT(DISTINCT s.customer_id)
		FROM project_sites s JOIN customers c ON c.customer_id=s.customer_id
		WHERE s.project_id=? AND s.is_active=1 GROUP BY 1`, projectID)
	if err == nil {
		for rows.Next() {
			var ind string
			var n int
			if rows.Scan(&ind, &n) == nil {
				for _, t := range childSiteTypesOf(ind) { // 95-219: 업종마다 한 번씩
					counts[t] += n
				}
			}
		}
		rows.Close()
	}
	if t := mostCommonSiteType(counts); t != "" {
		return t
	}
	cid, _ := r.ProjectSiteCustomerID(projectID)
	if cid != "" {
		if res, err := r.ListChildCustomers(projectID, cid); err == nil {
			counts = map[string]int{}
			for _, ch := range res.Chips {
				for _, t := range childSiteTypesOf(strings.TrimPrefix(ch.Industry, childIndustryNone)) {
					counts[t] += ch.Count
				}
			}
			if t := mostCommonSiteType(counts); t != "" {
				return t
			}
		}
	}
	return "school"
}

// CountUnregisteredChildren 사업 고객의 하위 기관 중 이 사업에 아직 없는 곳 수(활성 party_kind=customer, 3단계까지).
func (r *ProjectEquipRepo) CountUnregisteredChildren(projectID, customerID string) int {
	customerID = strings.TrimSpace(customerID)
	if r == nil || r.db == nil || customerID == "" {
		return 0
	}
	var n int
	_ = r.db.QueryRow(`
		WITH RECURSIVE ch(customer_id, depth) AS (
			SELECT customer_id, 1 FROM customers WHERE parent_customer_id=?
			UNION
			SELECT c.customer_id, ch.depth+1 FROM customers c JOIN ch ON c.parent_customer_id=ch.customer_id WHERE ch.depth<?
		)
		SELECT COUNT(DISTINCT c.customer_id) FROM customers c JOIN ch ON ch.customer_id=c.customer_id
		 WHERE COALESCE(c.is_active,0)=1 AND COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')='customer'
		   AND c.customer_id<>?
		   AND c.customer_id NOT IN (SELECT customer_id FROM project_sites WHERE project_id=? AND is_active=1 AND TRIM(COALESCE(customer_id,''))!='')`,
		customerID, maxChildDepth, customerID, projectID).Scan(&n)
	return n
}
