package repository

import (
	"strings"

	"customer-support/internal/model"
)

type ProjectActivityCounts struct {
	AS     map[string]int
	Mnt    map[string]int
	Wait   map[string]int
	Unspec int
}

func (r *ProjectRepo) CountProjectActivities() (ProjectActivityCounts, error) {
	out := ProjectActivityCounts{AS: map[string]int{}, Mnt: map[string]int{}, Wait: map[string]int{}}
	asN, err := r.countByProjectCol(`SELECT COALESCE(project_id,''), COUNT(*) FROM as_receipts GROUP BY COALESCE(project_id,'')`)
	if err != nil {
		return out, err
	}
	mntN, err := r.countByProjectCol(`SELECT COALESCE(project_id,''), COUNT(*) FROM maintenance_visits GROUP BY COALESCE(project_id,'')`)
	if err != nil {
		return out, err
	}
	for id, n := range asN {
		if id == "" {
			out.Unspec += n
			continue
		}
		out.AS[id] = n
	}
	for id, n := range mntN {
		if id == "" {
			out.Unspec += n
			continue
		}
		out.Mnt[id] = n
	}
	taskRows, err := r.db.Query(`SELECT COALESCE(project_id,''), COALESCE(work_type,''), COUNT(*) FROM work_tasks WHERE status!=? GROUP BY 1, 2`, model.WBTaskComplete)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return out, nil
		}
		return out, err
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var id, wt string
		var n int
		if err := taskRows.Scan(&id, &wt, &n); err != nil {
			return out, err
		}
		id = strings.TrimSpace(id)
		if id == "" {
			out.Unspec += n
			continue
		}
		switch model.WorkTaskActClass(wt) {
		case model.ActFilterAS:
			out.AS[id] += n
		case model.ActFilterMnt:
			out.Mnt[id] += n
		default:
			out.Wait[id] += n
		}
	}
	return out, taskRows.Err()
}

func (r *ProjectRepo) countByProjectCol(q string, args ...interface{}) (map[string]int, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return map[string]int{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[strings.TrimSpace(id)] = n
	}
	return out, rows.Err()
}
