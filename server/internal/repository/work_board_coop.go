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
	r.fillWorkGroupNames(items)
}

// fillWorkGroupNames 95-222: 업무그룹에 속한 카드에 그룹명을 채운다(협조로 이미 채운 것은 그대로).
// 접수·업무·점검 방문은 자기 work_group_id, 재방문·확인·지원(RefID=work_id)은 하위업무 담당 행의 그룹, 없으면 접수의 그룹이다.
func (r *WorkBoardRepo) fillWorkGroupNames(items []model.WorkListItem) {
	var taskIDs, asIDs, visitIDs []string
	for _, it := range items {
		if it.WorkGroupName != "" || strings.TrimSpace(it.RefID) == "" {
			continue
		}
		switch it.Prefix {
		case model.WorkPrefixMaintenance:
			visitIDs = append(visitIDs, it.RefID)
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			asIDs = append(asIDs, it.RefID)
		default:
			taskIDs = append(taskIDs, it.RefID)
		}
	}
	if len(taskIDs)+len(asIDs)+len(visitIDs) == 0 {
		return
	}
	pairs := func(q string, ids []string) map[string]string {
		out := map[string]string{}
		if len(ids) == 0 {
			return out
		}
		rows, err := r.db.Query(q+placeholders(len(ids))+`)`, iface(ids)...)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var id, nm string
			if rows.Scan(&id, &nm) == nil && nm != "" {
				out[id] = nm
			}
		}
		return out
	}
	byTask := pairs(`SELECT t.task_id, COALESCE(g.name,'') FROM work_tasks t
		JOIN work_groups g ON g.group_id=t.work_group_id WHERE t.task_id IN (`, taskIDs)
	byVisit := pairs(`SELECT mv.visit_id, COALESCE(g.name,'') FROM maintenance_visits mv
		JOIN work_groups g ON g.group_id=mv.work_group_id WHERE mv.visit_id IN (`, visitIDs)
	byAS := pairs(`SELECT ar.as_id, COALESCE(g.name,'') FROM as_receipts ar
		JOIN work_groups g ON g.group_id=ar.work_group_id WHERE ar.as_id IN (`, asIDs)
	byWork := pairs(`SELECT w.work_id, COALESCE(g.name,'') FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id=w.as_id
		JOIN work_groups g ON g.group_id=COALESCE(
			(SELECT NULLIF(a.work_group_id,'') FROM as_work_item_assignees a WHERE a.work_id=w.work_id AND COALESCE(a.work_group_id,'')<>'' LIMIT 1),
			NULLIF(ar.work_group_id,''))
		WHERE w.work_id IN (`, asIDs)
	for i, it := range items {
		if it.WorkGroupName != "" {
			continue
		}
		switch it.Prefix {
		case model.WorkPrefixMaintenance:
			items[i].WorkGroupName = byVisit[it.RefID]
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			if nm := byWork[it.RefID]; nm != "" {
				items[i].WorkGroupName = nm
			} else {
				items[i].WorkGroupName = byAS[it.RefID]
			}
		default:
			items[i].WorkGroupName = byTask[it.RefID]
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
