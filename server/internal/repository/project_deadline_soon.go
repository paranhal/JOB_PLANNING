package repository

import (
	"strings"
	"time"
)

// 95-179E 대시보드 「마감 임박」 칩. 일자가 없는 대상은 알림에 나오지 않는다.
//
// 사업별로 미완료 1단계·하부 업무·작업 계획 중 plan_end 가 가장 이른 것 하나만 돌려준다.
// 완료 판정: 1단계 = status 'done', 하부 업무·작업 계획 = status 'done' 또는 (사이트가 있고 전부 done/excluded).

// DeadlineSoon 사업 하나의 가장 가까운 미완료 마감.
type DeadlineSoon struct {
	Level   string // phase | package | plan
	Name    string
	PlanEnd string // YYYY-MM-DD
	Days    int    // 마감까지 남은 일수. 0=오늘, 음수=지남
}

// DeadlineSoonByProject 마감이 오늘+withinDays 이내이거나 이미 지난 미완료 대상. 한 번의 쿼리로 묶어서 조회한다.
func (r *ProjectRepo) DeadlineSoonByProject(projectIDs []string, today string, withinDays int) (map[string]DeadlineSoon, error) {
	out := map[string]DeadlineSoon{}
	if len(projectIDs) == 0 || r == nil || r.db == nil {
		return out, nil
	}
	t0, err := time.Parse("2006-01-02", today)
	if err != nil {
		return out, nil
	}
	limit := t0.AddDate(0, 0, withinDays).Format("2006-01-02")
	ph, args := inArgs(projectIDs)
	q := `
	SELECT project_id, level, name, plan_end FROM (
	  SELECT ph.project_id AS project_id, 'phase' AS level, ph.name AS name, substr(ph.plan_end,1,10) AS plan_end
	  FROM project_phases ph
	  WHERE ph.is_active=1 AND ph.status<>'done' AND ph.plan_end<>''
	  UNION ALL
	  SELECT ph.project_id, 'package', pk.name, substr(pk.plan_end,1,10)
	  FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
	  WHERE ph.is_active=1 AND pk.is_active=1 AND pk.status<>'done' AND pk.plan_end<>''
	    AND (NOT EXISTS (SELECT 1 FROM package_sites s WHERE s.package_id=pk.package_id)
	      OR EXISTS (SELECT 1 FROM package_sites s WHERE s.package_id=pk.package_id AND s.status NOT IN ('done','excluded')))
	  UNION ALL
	  SELECT ph.project_id, 'plan', pl.name, substr(pl.plan_end,1,10)
	  FROM package_plans pl
	  JOIN phase_packages pk ON pk.package_id=pl.package_id
	  JOIN project_phases ph ON ph.phase_id=pk.phase_id
	  WHERE ph.is_active=1 AND pk.is_active=1 AND pl.is_active=1 AND pl.status<>'done' AND pl.plan_end<>''
	    AND EXISTS (SELECT 1 FROM package_sites s WHERE s.plan_id=pl.plan_id AND s.status NOT IN ('done','excluded'))
	) t
	WHERE project_id IN (` + ph + `) AND plan_end<=?
	ORDER BY project_id, plan_end, CASE level WHEN 'phase' THEN 0 WHEN 'package' THEN 1 ELSE 2 END, name`
	rows, err := r.db.Query(q, append(args, limit)...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, level, name, end string
		if err := rows.Scan(&pid, &level, &name, &end); err != nil {
			return out, err
		}
		if _, ok := out[pid]; ok {
			continue // 정렬상 첫 행이 가장 이른 마감
		}
		te, err := time.Parse("2006-01-02", end)
		if err != nil {
			continue
		}
		out[pid] = DeadlineSoon{Level: level, Name: name, PlanEnd: end, Days: int(te.Sub(t0).Hours() / 24)}
	}
	return out, rows.Err()
}

// RegisteredSiteCount 95-179F 사업에 등록된 사이트 수(제외 상태 뺌). 「등록 N곳 중 연결 M곳」 안내용.
func (r *ProjectRepo) RegisteredSiteCount(projectID string) int {
	if r == nil || r.db == nil {
		return 0
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id=? AND is_active=1 AND status<>'excluded'`,
		strings.TrimSpace(projectID)).Scan(&n); err != nil {
		return 0
	}
	return n
}
