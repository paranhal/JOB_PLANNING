package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-182 작업·행정/지원 등록 화면: 수행 구조가 있는 사업에서 하부 업무를 고르고, 그 하부 업무에 등록된 사이트만 후보로 보여 준다.
// 사이트 ID 는 고객 ID(작업의 대상 사이트 ID 와 같다)다. 쓰기는 하지 않는다.

// PackageChoice 등록 화면의 하부 업무 선택지.
type PackageChoice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phase string `json:"phase"`
	Sites int    `json:"sites"` // 그 하부 업무에 등록된 사이트 수(제외 뺌)
}

// PackageChoices 사업의 사이트형 하부 업무 목록(수량만 하부 업무는 사이트가 없어 뺀다). 없으면 빈 목록.
func (r *ProjectRepo) PackageChoices(projectID string) ([]PackageChoice, error) {
	out := []PackageChoice{}
	if r == nil || r.db == nil || strings.TrimSpace(projectID) == "" {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT pk.package_id, pk.name, ph.name,
		       (SELECT COUNT(*) FROM package_sites s WHERE s.package_id=pk.package_id AND s.status<>'excluded')
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND ph.is_active=1 AND pk.is_active=1 AND pk.track_mode<>'phase'
		ORDER BY ph.seq, pk.seq, pk.package_id`, strings.TrimSpace(projectID))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c PackageChoice
		if err := rows.Scan(&c.ID, &c.Name, &c.Phase, &c.Sites); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PackageSiteItem 하부 업무에 등록된 사이트 한 곳. Type 은 고객 업종(school|public|library|private).
type PackageSiteItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Sido    string `json:"sido"`
	Sigungu string `json:"sigungu"`
	Type    string `json:"type"`
}

// PackageSiteResult GET /workboard/package-sites 본문. 시도·시군 목록은 화면이 유형별로 항목에서 만든다.
type PackageSiteResult struct {
	Items []PackageSiteItem `json:"items"`
	Total int               `json:"total"`
}

// siteTypeOfIndustry 고객 업종을 사업 사이트 등록 화면의 유형 코드로. 학교·공공기관·도서관 외는 민간업체다.
func siteTypeOfIndustry(industry string) string {
	switch model.IndustryKey(industry) {
	case "학교":
		return "school"
	case "공공기관":
		return "public"
	case "도서관":
		return "library"
	default:
		return "private"
	}
}

// PackageSiteChoices 하부 업무에 등록된 사이트(고객이 있는 것만). 사업 사이트 등록 화면과 같은 유형·시도·시군 기준이다.
func (r *ProjectRepo) PackageSiteChoices(projectID, packageID string) (PackageSiteResult, error) {
	out := PackageSiteResult{Items: []PackageSiteItem{}}
	if r == nil || r.db == nil {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT s.customer_id, s.site_name, COALESCE(s.region,''),
		       COALESCE(c.addr_sido,''), COALESCE(c.addr_sigungu,''), COALESCE(c.industry,'')
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		JOIN project_sites s ON s.id=ps.project_site_id
		LEFT JOIN customers c ON c.customer_id=s.customer_id
		WHERE ps.package_id=? AND ph.project_id=? AND pk.is_active=1 AND ps.status<>'excluded'
		  AND COALESCE(s.customer_id,'')<>''
		ORDER BY s.site_name, s.customer_id`, strings.TrimSpace(packageID), strings.TrimSpace(projectID))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, name, region, asido, asig, ind string
		if err := rows.Scan(&cid, &name, &region, &asido, &asig, &ind); err != nil {
			return out, err
		}
		out.Items = append(out.Items, PackageSiteItem{
			ID: cid, Name: name, Sido: schoolSidoLabel(asido, asig, region, ""), Sigungu: schoolSigunguLabel(asig), Type: siteTypeOfIndustry(ind),
		})
	}
	out.Total = len(out.Items)
	return out, rows.Err()
}

// PackageSitesAllowed 고른 사이트(고객 ID)가 모두 그 하부 업무에 등록돼 있는지 확인한다. 하나라도 아니면 오류.
func (r *ProjectRepo) PackageSitesAllowed(projectID, packageID string, customerIDs []string) error {
	seen := map[string]bool{}
	var ids []string
	for _, id := range customerIDs {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph, args := inArgs(ids)
	args = append(args, strings.TrimSpace(packageID), strings.TrimSpace(projectID))
	var n int
	if err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT s.customer_id)
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		JOIN project_sites s ON s.id=ps.project_site_id
		WHERE s.customer_id IN (`+ph+`) AND ps.package_id=? AND ph.project_id=? AND ps.status<>'excluded'`, args...).Scan(&n); err != nil {
		return err
	}
	if n != len(ids) {
		return fmt.Errorf("선택한 하부 업무에 등록되지 않은 사이트가 있습니다")
	}
	return nil
}
