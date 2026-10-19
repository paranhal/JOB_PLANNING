package repository

import (
	"fmt"
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
	var salesIDs []string
	for _, p := range works {
		if sid := strings.TrimSpace(p.SalesProjectID); sid != "" {
			linked[sid]++
			salesIDs = append(salesIDs, sid)
		}
	}
	linkedSales, err := r.salesByIDs(salesIDs)
	if err != nil {
		return nil, err
	}

	candidates, err := r.listUnlinkedContractedSales(ContractUnionFilter{OrgID: f.OrgID})
	if err != nil {
		return nil, err
	}
	byName := map[string][]model.SalesProject{}
	for _, s := range candidates {
		k := model.NormalizeProjectName(s.Name)
		if k == "" {
			continue
		}
		byName[k] = append(byName[k], s)
	}
	exclude := map[string]bool{}

	var rows []model.ContractRow
	for _, p := range works {
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
			StartDate:      p.StartDate,
			EndDate:        p.EndDate,
			Status:         p.Status,
			ProjectKind:    p.ProjectKind,
			ProjectKindSet: p.ProjectKindSet,
			SalesBizType:   p.SalesBizType,
		}
		if sid := strings.TrimSpace(p.SalesProjectID); sid != "" {
			if linked[sid] > 1 {
				row.DupLinked = true
			}
			if sp, ok := linkedSales[sid]; ok {
				row.SalesAmount = sp.ContractAmount
				if sp.ContractAmount != amt {
					row.AmountMismatch = true
				}
			}
		} else {
			cands := byName[model.NormalizeProjectName(p.Name)]
			if len(cands) == 1 {
				row.LinkNeed = cands[0].DisplayNo()
				row.LinkSalesID = cands[0].SalesID
				row.SalesAmount = cands[0].ContractAmount
				if cands[0].ContractAmount != amt {
					row.AmountMismatch = true
				}
				exclude[cands[0].SalesID] = true
			} else if len(cands) > 1 {
				row.LinkAmbiguous = true
			}
		}
		rows = append(rows, row)
	}

	if strings.TrimSpace(f.Status) != "" {
		return rows, nil
	}

	for _, s := range candidates {
		if linked[s.SalesID] > 0 || exclude[s.SalesID] {
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
			SalesBizType:   s.BizType,
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
			COALESCE(s.created_at,''), COALESCE(s.org_id,''), COALESCE(s.status,''), COALESCE(s.biz_type,''),
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
			&p.ContractAmount, &p.ExpectedAmount, &p.ContractedAt, &p.CreatedAt, &p.OrgID, &p.Status, &p.BizType, &p.CustomerName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepo) salesByIDs(ids []string) (map[string]model.SalesProject, error) {
	out := map[string]model.SalesProject{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	q := `SELECT sales_id, COALESCE(sales_no,''), COALESCE(name,''), COALESCE(contract_amount,0)
		FROM sales_projects WHERE sales_id IN (` + strings.Join(ph, ",") + `)`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p model.SalesProject
		if err := rows.Scan(&p.SalesID, &p.SalesNo, &p.Name, &p.ContractAmount); err != nil {
			return nil, err
		}
		out[p.SalesID] = p
	}
	return out, rows.Err()
}

func (r *ProjectRepo) LinkSalesProject(projectID, salesID, by string) error {
	projectID = strings.TrimSpace(projectID)
	salesID = strings.TrimSpace(salesID)
	if projectID == "" || salesID == "" {
		return fmt.Errorf("연결할 사업이 없습니다")
	}
	p, err := r.Get(projectID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.SalesProjectID) != "" {
		return fmt.Errorf("이미 영업 사업과 연결되어 있습니다")
	}
	old := strings.TrimSpace(p.SalesProjectID)
	p.SalesProjectID = salesID
	return touchUpdate(r.db, "work_projects", "project_id", projectID, p.Name, func() error {
		_, err := r.db.Exec(`UPDATE work_projects SET sales_project_id=?, updated_at=CURRENT_TIMESTAMP WHERE project_id=?`, salesID, projectID)
		if err == nil {
			_ = by
			_ = old
		}
		return err
	})
}

func ParseYearQuery(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
