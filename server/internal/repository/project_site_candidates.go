package repository

import (
	"sort"
	"strings"
)

const schoolSiteUnspecified = "미지정"

// SchoolSiteCandidate 사업 사이트 등록 후보 한 줄.
type SchoolSiteCandidate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Sido       string `json:"sido"`
	Sigungu    string `json:"sigungu"`
	Registered bool   `json:"registered"`
}

// SchoolSiteRegion 시도별 시군 목록. 필터와 무관하게 후보 전체 기준.
type SchoolSiteRegion struct {
	Sido    string   `json:"sido"`
	Sigungu []string `json:"sigungu"`
}

// SchoolSiteCandidateResult GET /sites/candidates 본문.
type SchoolSiteCandidateResult struct {
	Regions []SchoolSiteRegion    `json:"regions"`
	Items   []SchoolSiteCandidate `json:"items"`
	Total   int                   `json:"total"`
}

func schoolSidoLabel(sido, sigungu, region, address string) string {
	blob := strings.TrimSpace(strings.Join([]string{sido, sigungu, region, address}, " "))
	if lab := ExtractKoreaRegion(blob); lab != "" {
		return lab
	}
	if lab := ExtractKoreaRegion(strings.TrimSpace(sido)); lab != "" {
		return lab
	}
	return schoolSiteUnspecified
}

func schoolSigunguLabel(sigungu string) string {
	s := strings.TrimSpace(sigungu)
	if s == "" {
		return schoolSiteUnspecified
	}
	return s
}

func csvSet(vals []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			out[v] = true
		}
	}
	return out
}

func matchSchoolRegionFilter(label string, want map[string]bool) bool {
	if len(want) == 0 {
		return true
	}
	if want[label] {
		return true
	}
	for w := range want {
		if w == "" {
			continue
		}
		if strings.Contains(label, w) || strings.Contains(w, label) {
			return true
		}
	}
	return false
}

func sortRegionKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a == schoolSiteUnspecified {
			return false
		}
		if b == schoolSiteUnspecified {
			return true
		}
		return a < b
	})
}

func (r *ProjectEquipRepo) registeredCustomerIDs(projectID string) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := r.db.Query(`SELECT DISTINCT customer_id FROM project_sites
		WHERE project_id=? AND is_active=1 AND TRIM(COALESCE(customer_id,''))!=''`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func normalizeProjectSiteType(typ string) string {
	switch strings.TrimSpace(typ) {
	case "public", "library", "private", "school":
		return strings.TrimSpace(typ)
	default:
		return "school"
	}
}

func siteCandidateIndustry(typ string) (industry string, orEmpty bool) {
	switch normalizeProjectSiteType(typ) {
	case "public":
		return "공공기관", false
	case "library":
		return "도서관", false
	case "private":
		return "민간", true
	default:
		return "학교", false
	}
}

// ListSchoolSiteCandidates 전사 활성 학교. 상위 없어도 포함. Lookup 공용 경로만 사용.
func (r *ProjectEquipRepo) ListSchoolSiteCandidates(projectID, orgID, q string, sidos, sigungus []string) (SchoolSiteCandidateResult, error) {
	return r.ListSiteCandidates(projectID, orgID, "school", q, sidos, sigungus)
}

// ListSiteCandidates 사업 사이트 후보. type=school|public|library|private.
// 고객은 전사 공용이라 사업 조직으로 거르지 않는다(95-167). orgID는 호출 호환용이다.
func (r *ProjectEquipRepo) ListSiteCandidates(projectID, orgID, typ, q string, sidos, sigungus []string) (SchoolSiteCandidateResult, error) {
	out := SchoolSiteCandidateResult{Regions: []SchoolSiteRegion{}, Items: []SchoolSiteCandidate{}}
	if r == nil || r.db == nil {
		return out, nil
	}
	_ = strings.TrimSpace(orgID)
	ind, orEmpty := siteCandidateIndustry(typ)
	opts, _, err := NewCustomerRepo(r.db).Lookup(CustomerQuery{
		Scope:           ScopeSite,
		Industry:        ind,
		IndustryOrEmpty: orEmpty,
	})
	if err != nil {
		return out, err
	}
	reg, err := r.registeredCustomerIDs(projectID)
	if err != nil {
		return out, err
	}
	q = strings.TrimSpace(q)
	sidoWant := csvSet(sidos)
	sigWant := csvSet(sigungus)
	bySido := map[string]map[string]bool{}
	var all []SchoolSiteCandidate
	for _, o := range opts {
		it := SchoolSiteCandidate{
			ID:         o.ID,
			Name:       o.Name,
			Sido:       schoolSidoLabel(o.AddrSido, o.AddrSigungu, o.Region, ""),
			Sigungu:    schoolSigunguLabel(o.AddrSigungu),
			Registered: reg[o.ID],
		}
		if bySido[it.Sido] == nil {
			bySido[it.Sido] = map[string]bool{}
		}
		bySido[it.Sido][it.Sigungu] = true
		all = append(all, it)
	}
	sidoKeys := make([]string, 0, len(bySido))
	for s := range bySido {
		sidoKeys = append(sidoKeys, s)
	}
	sortRegionKeys(sidoKeys)
	for _, s := range sidoKeys {
		gkeys := make([]string, 0, len(bySido[s]))
		for g := range bySido[s] {
			gkeys = append(gkeys, g)
		}
		sortRegionKeys(gkeys)
		out.Regions = append(out.Regions, SchoolSiteRegion{Sido: s, Sigungu: gkeys})
	}
	for _, it := range all {
		if !matchSchoolRegionFilter(it.Sido, sidoWant) {
			continue
		}
		if !matchSchoolRegionFilter(it.Sigungu, sigWant) {
			continue
		}
		if q != "" {
			blob := it.Name + it.Sido + it.Sigungu
			if !strings.Contains(blob, q) {
				continue
			}
		}
		out.Items = append(out.Items, it)
	}
	out.Total = len(out.Items)
	return out, nil
}
