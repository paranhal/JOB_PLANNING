package repository

import (
	"strings"

	"customer-support/internal/model"
)

type RegisterInputTotals struct {
	Tasks    int
	People   int
	Minutes  int
	Complete int
}

func (t RegisterInputTotals) Hours() float64 {
	if t.Minutes <= 0 {
		return 0
	}
	return float64(t.Minutes) / 60
}

type RegisterPersonStat struct {
	Name    string
	Tasks   int
	Minutes int
}

func (r RegisterPersonStat) Hours() float64 { return float64(r.Minutes) / 60 }

type RegisterProjectStat struct {
	ProjectID string
	Name      string
	Tasks     int
	Minutes   int
}

func (r RegisterProjectStat) Hours() float64 { return float64(r.Minutes) / 60 }

type RegisterTypeStat struct {
	Key     string
	Label   string
	Tasks   int
	Minutes int
}

func (r RegisterTypeStat) Hours() float64 { return float64(r.Minutes) / 60 }

func workTypeLabel(key string) string {
	switch key {
	case model.WBWorkAS:
		return "AS"
	case model.WBWorkMaintenance:
		return "정기점검"
	case model.WBWorkSales:
		return "영업"
	case model.WBWorkSupport:
		return "지원"
	default:
		return "행정·지원"
	}
}

func taskDayExpr() string {
	return `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')`
}

func (r *WBRepo) RegisterInputTotals(day string) (RegisterInputTotals, error) {
	var out RegisterInputTotals
	if r == nil || r.db == nil {
		return out, nil
	}
	day = strings.TrimSpace(day)
	err := r.db.QueryRow(`
		SELECT COUNT(*), COUNT(DISTINCT CASE WHEN TRIM(COALESCE(t.assignee,''))='' THEN '(미배정)' ELSE TRIM(t.assignee) END),
		       COALESCE(SUM(COALESCE(t.duration_min,30)),0),
		       COALESCE(SUM(CASE WHEN t.status='complete' THEN 1 ELSE 0 END),0)
		FROM work_tasks t
		WHERE `+taskDayExpr()+` = ?`, day).Scan(&out.Tasks, &out.People, &out.Minutes, &out.Complete)
	return out, err
}

func (r *WBRepo) RegisterInputByPerson(day string) ([]RegisterPersonStat, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT CASE WHEN TRIM(COALESCE(t.assignee,''))='' THEN '(미배정)' ELSE TRIM(t.assignee) END,
		       COUNT(*), COALESCE(SUM(COALESCE(t.duration_min,30)),0)
		FROM work_tasks t
		WHERE `+taskDayExpr()+` = ?
		GROUP BY 1
		ORDER BY 3 DESC, 1`, strings.TrimSpace(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RegisterPersonStat
	for rows.Next() {
		var it RegisterPersonStat
		if err := rows.Scan(&it.Name, &it.Tasks, &it.Minutes); err != nil {
			continue
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

func (r *WBRepo) RegisterInputByProject(day string) ([]RegisterProjectStat, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT COALESCE(t.project_id,''),
		       CASE WHEN COALESCE(t.project_id,'')='' THEN '(사업 없음)'
		            ELSE COALESCE(NULLIF(TRIM(p.short_name),''), NULLIF(TRIM(p.name),''), t.project_id) END,
		       COUNT(*), COALESCE(SUM(COALESCE(t.duration_min,30)),0)
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id=t.project_id
		WHERE `+taskDayExpr()+` = ?
		GROUP BY 1, 2
		ORDER BY 4 DESC, 2`, strings.TrimSpace(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RegisterProjectStat
	for rows.Next() {
		var it RegisterProjectStat
		if err := rows.Scan(&it.ProjectID, &it.Name, &it.Tasks, &it.Minutes); err != nil {
			continue
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

func (r *WBRepo) RegisterInputByType(day string) ([]RegisterTypeStat, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT COALESCE(t.work_type,'admin'), COUNT(*), COALESCE(SUM(COALESCE(t.duration_min,30)),0)
		FROM work_tasks t
		WHERE `+taskDayExpr()+` = ?
		GROUP BY 1
		ORDER BY 3 DESC`, strings.TrimSpace(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RegisterTypeStat
	for rows.Next() {
		var it RegisterTypeStat
		if err := rows.Scan(&it.Key, &it.Tasks, &it.Minutes); err != nil {
			continue
		}
		it.Label = workTypeLabel(it.Key)
		list = append(list, it)
	}
	return list, rows.Err()
}
