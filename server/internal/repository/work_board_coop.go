package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *WorkBoardRepo) fillCoopWorkItems(items []model.WorkListItem) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	taskIDs, asIDs, visitIDs := collectCoopRefIDs(items)
	byTask := r.coopHits(`SELECT t.task_id, COALESCE(t.cooperation_id,''), COALESCE(w.req_org_id,''), COALESCE(g.name,'')
		FROM work_tasks t
		LEFT JOIN work_cooperations w ON w.coop_id=t.cooperation_id
		LEFT JOIN work_groups g ON g.group_id=w.work_group_id
		WHERE t.task_id IN`, taskIDs)
	byAS := r.coopHits(`SELECT ar.as_id, COALESCE(ar.cooperation_id,''), COALESCE(w.req_org_id,''), COALESCE(g.name,'')
		FROM as_receipts ar
		LEFT JOIN work_cooperations w ON w.coop_id=ar.cooperation_id
		LEFT JOIN work_groups g ON g.group_id=w.work_group_id
		WHERE ar.as_id IN`, asIDs)
	byVisit := r.coopHits(`SELECT mv.visit_id, COALESCE(mv.cooperation_id,''), COALESCE(w.req_org_id,''), COALESCE(g.name,'')
		FROM maintenance_visits mv
		LEFT JOIN work_cooperations w ON w.coop_id=mv.cooperation_id
		LEFT JOIN work_groups g ON g.group_id=w.work_group_id
		WHERE mv.visit_id IN`, visitIDs)
	orgName := map[string]string{}
	apply := func(i int, h coopHit) {
		if h.coop == "" {
			return
		}
		items[i].CoopID = h.coop
		if h.from != "" {
			if _, ok := orgName[h.from]; !ok {
				var nm string
				_ = r.db.QueryRow(`SELECT org_name FROM orgs WHERE org_id=?`, h.from).Scan(&nm)
				orgName[h.from] = nm
			}
			if orgName[h.from] != "" {
				items[i].CoopFromOrg = orgName[h.from]
			} else {
				items[i].CoopFromOrg = h.from
			}
		}
		items[i].WorkGroupName = h.gname
	}
	for i, it := range items {
		switch it.Prefix {
		case model.WorkPrefixMaintenance:
			apply(i, byVisit[it.RefID])
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			apply(i, byAS[it.RefID])
		default:
			apply(i, byTask[it.RefID])
		}
	}
}

type coopHit struct{ coop, from, gname string }

func collectCoopRefIDs(items []model.WorkListItem) (tasks, as, visits []string) {
	for _, it := range items {
		switch it.Prefix {
		case model.WorkPrefixMaintenance:
			visits = append(visits, it.RefID)
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			as = append(as, it.RefID)
		default:
			tasks = append(tasks, it.RefID)
		}
	}
	return
}

func (r *WorkBoardRepo) coopHits(q string, ids []string) map[string]coopHit {
	out := map[string]coopHit{}
	if len(ids) == 0 {
		return out
	}
	ph := strings.Repeat("?,", len(ids))
	ph = strings.TrimSuffix(ph, ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(q+" ("+ph+")", args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var h coopHit
		if err := rows.Scan(&id, &h.coop, &h.from, &h.gname); err != nil {
			continue
		}
		out[id] = h
	}
	return out
}
