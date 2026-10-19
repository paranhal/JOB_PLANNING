package repository

import (
	"strconv"
	"strings"

	"customer-support/internal/model"
)

type ContractUnionFilter struct {
	Search     string
	Year       int
	Status     string
	IncludeAll bool
	OrgID      string
}

// ListContractUnion 사업관리 계약 ∪ 미연결 영업 계약 완료. 80-A. 73-E와 공유.
func (r *ProjectRepo) ListContractUnion(f ContractUnionFilter) ([]model.ContractRow, error) {
	works, err := r.ListContracts(f.Search, f.Year, f.Status, f.IncludeAll)
	if err != nil {
		return nil, err
	}
	linked := map[string]int{}
	var rows []model.ContractRow
	for _, p := range works {
		if sid := strings.TrimSpace(p.SalesProjectID); sid != "" {
			linked[sid]++
		}
		amt := p.ContractAmount
		row := model.ContractRow{
			Source:       model.ContractSourceWork,
			ProjectID:    p.ProjectID,
			SalesID:      p.SalesProjectID,
			SalesNo:      p.SalesNo,
			Name:         p.Name,
			Customer:     p.CustomerOrParty(),
			Amount:       amt,
			ContractedAt: firstNonEmptyStr(p.StartDate, p.CreatedAt.Format("2006-01-02")),
			StartDate:    p.StartDate,
			EndDate:      p.EndDate,
			Status:       p.Status,
		}
		rows = append(rows, row)
	}

	// 상태 필터가 있으면 ② 행은 숨김(80-A.5).
	if strings.TrimSpace(f.Status) != "" {
		return rows, nil
	}

	sales, err := r.listUnlinkedContractedSales(f)
	if err != nil {
		return nil, err
	}
	for _, s := range sales {
		if linked[s.SalesID] > 0 {
			continue
		}
		amt := s.ContractAmount
		row := model.ContractRow{
			Source:         model.ContractSourceSales,
			SalesID:        s.SalesID,
			SalesNo:        s.DisplayNo(),
			Name:           s.Name,
			Customer:       firstNonEmptyStr(s.CustomerName, s.ProspectName),
			Amount:         amt,
			ExpectedAmount: s.ExpectedAmount,
			AmountMissing:  amt <= 0,
			ContractedAt:   firstNonEmptyStr(s.ContractedAt, s.CreatedAt),
			OrgID:          s.OrgID,
			Promoted:       s.Status == model.SalesStatusPromoted,
		}
		if f.Year > 0 && row.Year() != f.Year {
			continue
		}
		if q := strings.TrimSpace(f.Search); q != "" && !salesRowMatches(row, q) {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func salesRowMatches(r model.ContractRow, q string) bool {
	q = strings.ToLower(q)
	blob := strings.ToLower(strings.Join([]string{r.Name, r.Customer, r.SalesNo, r.SalesID}, " "))
	return strings.Contains(blob, q)
}

func firstNonEmptyStr(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (r *ProjectRepo) listUnlinkedContractedSales(f ContractUnionFilter) ([]model.SalesProject, error) {
	q := `
		SELECT s.sales_id, COALESCE(s.sales_no,''), COALESCE(s.name,''), COALESCE(s.prospect_name,''),
			COALESCE(s.contract_amount,0), COALESCE(s.expected_amount,0), COALESCE(s.contracted_at,''),
			COALESCE(s.created_at,''), COALESCE(s.org_id,''), COALESCE(s.status,''),
			COALESCE(NULLIF(TRIM(cu.org_name),''), '')
		FROM sales_projects s
		LEFT JOIN customers cu ON cu.customer_id = s.customer_id
		LEFT JOIN work_projects w ON TRIM(COALESCE(w.sales_project_id,'')) = s.sales_id
		WHERE s.stage='closed' AND s.close_reason='contracted'
		  AND w.project_id IS NULL`
	var args []interface{}
	org := strings.TrimSpace(f.OrgID)
	if org == "" {
		org = OrgAll
	}
	q, args = appendOrg(q, args, "s", org)
	q += ` ORDER BY s.contracted_at DESC, s.sales_id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesProject
	for rows.Next() {
		var p model.SalesProject
		if err := rows.Scan(&p.SalesID, &p.SalesNo, &p.Name, &p.ProspectName,
			&p.ContractAmount, &p.ExpectedAmount, &p.ContractedAt, &p.CreatedAt, &p.OrgID, &p.Status, &p.CustomerName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ParseYearQuery(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
