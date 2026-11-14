package repository

import (
	"sort"
	"strings"

	"customer-support/internal/model"
)

// 95-194: 사업 연결 창(모달)의 사업 목록에서 같은 이름의 사업을 1개만 보인다.
// 같은 조직 안에서 이름(공백·괄호 무시)이 같은 사업이 여럿이면 대표 1개만 남긴다.
// 대표: 실적(업무·점검·AS 건수)이 많은 쪽 → 영업 연결이 있는 쪽 → 생성일이 이른 쪽 → 사업ID.
// 목록 표시용이다. 연결 저장·검증은 전체 목록(ListWeeklyRowProjects)을 그대로 쓴다.

// ListWeeklyModalProjects 사업 연결 창에 보이는 사업 목록(동명 중복 제거).
func (r *StatsRepo) ListWeeklyModalProjects(orgID string) ([]model.WorkProject, error) {
	list, err := r.ListWeeklyRowProjects(orgID)
	if err != nil {
		return nil, err
	}
	return r.dedupeWeeklyProjects(list), nil
}

type weeklyProjectRank struct {
	activity int
	sales    bool
	created  string
}

func (r *StatsRepo) dedupeWeeklyProjects(list []model.WorkProject) []model.WorkProject {
	if r == nil || r.db == nil || len(list) < 2 {
		return list
	}
	groups := map[string][]int{}
	for i, p := range list {
		k := p.OrgID + "|" + model.NormalizePlanMergeName(p.Name)
		groups[k] = append(groups[k], i)
	}
	var dupIDs []string
	for _, idx := range groups {
		if len(idx) > 1 {
			for _, i := range idx {
				dupIDs = append(dupIDs, list[i].ProjectID)
			}
		}
	}
	if len(dupIDs) == 0 {
		return list
	}
	rank := r.weeklyProjectRanks(dupIDs)
	drop := map[int]bool{}
	for _, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		best := idx[0]
		for _, i := range idx[1:] {
			if weeklyRankBetter(rank[list[i].ProjectID], list[i].ProjectID, rank[list[best].ProjectID], list[best].ProjectID) {
				best = i
			}
		}
		for _, i := range idx {
			if i != best {
				drop[i] = true
			}
		}
	}
	out := make([]model.WorkProject, 0, len(list)-len(drop))
	for i, p := range list {
		if !drop[i] {
			out = append(out, p)
		}
	}
	return out
}

func weeklyRankBetter(a weeklyProjectRank, aid string, b weeklyProjectRank, bid string) bool {
	if a.activity != b.activity {
		return a.activity > b.activity
	}
	if a.sales != b.sales {
		return a.sales
	}
	if a.created != b.created {
		if a.created == "" {
			return false
		}
		if b.created == "" {
			return true
		}
		return a.created < b.created
	}
	return aid < bid
}

// weeklyProjectRanks 중복 후보 사업들의 실적·영업 연결·생성일을 종류별로 한 번에 조회한다(N+1 없음).
func (r *StatsRepo) weeklyProjectRanks(ids []string) map[string]weeklyProjectRank {
	out := map[string]weeklyProjectRank{}
	sort.Strings(ids)
	for _, ch := range chunkIDs(ids, 300) {
		in, args := inClause(ch)
		rows, err := r.db.Query(`SELECT project_id, COALESCE(sales_project_id,''), COALESCE(created_at,'') FROM work_projects WHERE project_id IN (`+in+`)`, args...)
		if err == nil {
			for rows.Next() {
				var id, sp, cr string
				if rows.Scan(&id, &sp, &cr) == nil {
					out[id] = weeklyProjectRank{sales: strings.TrimSpace(sp) != "", created: cr}
				}
			}
			rows.Close()
		}
		for _, q := range []string{
			`SELECT project_id, COUNT(*) FROM work_tasks WHERE project_id IN (` + in + `) GROUP BY project_id`,
			`SELECT project_id, COUNT(*) FROM maintenance_visits WHERE project_id IN (` + in + `) GROUP BY project_id`,
			`SELECT project_id, COUNT(*) FROM as_receipts WHERE project_id IN (` + in + `) GROUP BY project_id`,
		} {
			rs, err := r.db.Query(q, args...)
			if err != nil {
				continue
			}
			for rs.Next() {
				var id string
				var n int
				if rs.Scan(&id, &n) == nil {
					rk := out[id]
					rk.activity += n
					out[id] = rk
				}
			}
			rs.Close()
		}
	}
	return out
}
