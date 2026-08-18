package repository

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// ListUnplanned §8 미계획 업무함. /work 의 collectDelayed·collectSchedulePending·collectUnassigned를 재사용한다.
func (r *WorkBoardRepo) ListUnplanned(mineUserID string, mineKeys []string, kind string) ([]model.UnplannedItem, model.UnplannedKindCounts, error) {
	today := time.Now().Format("2006-01-02")
	bag := map[string]*model.UnplannedItem{}

	add := func(base model.UnplannedItem, k string) {
		if base.RefID == "" && base.Href == "" && base.ItemKey == "" {
			return
		}
		key := base.ItemKey
		if key == "" {
			key = unplannedItemKey(base.WorkListItem)
			base.ItemKey = key
		}
		cur, ok := bag[key]
		if !ok {
			if base.ItemKey == "" {
				u := toUnplannedItem(base.WorkListItem)
				u.ItemKey = key
				u.PlanID = base.PlanID
				u.CustomerID = base.CustomerID
				u.ProductType = base.ProductType
				u.NoDateReason = base.NoDateReason
				u.NoDateAt = base.NoDateAt
				base = u
			}
			base.ItemKey = key
			cur = &base
			bag[key] = cur
		}
		for _, exist := range cur.Kinds {
			if exist == k {
				return
			}
		}
		cur.Kinds = append(cur.Kinds, k)
		cur.Badges = append(cur.Badges, model.UnplannedBadgeOf(k, cur.DaysOverdue))
	}

	delayed, err := r.collectDelayed(mineUserID, mineKeys, today)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range delayed {
		u := toUnplannedItem(it)
		if it.SubLabel == "회신 대기" && it.Status == model.WBActionWaiting {
			u.ItemKey = "wa:" + it.RefID
			u.CanAssignDate = false
		}
		add(u, model.UnplannedDelayed)
	}

	pending, err := r.collectSchedulePending(mineUserID, mineKeys)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range pending {
		d := strings.TrimSpace(it.ScheduledDate)
		if d != "" && d < today {
			add(toUnplannedItem(it), model.UnplannedNext)
		}
	}

	open, err := r.collectOpen(mineUserID, mineKeys)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range open {
		if strings.TrimSpace(it.ScheduledDate) == "" {
			add(toUnplannedItem(it), model.UnplannedNoDate)
		}
	}

	pc, err := r.queryAS(
		`ar.status='partial_complete' AND COALESCE(ar.visit_scheduled_date,'')=''`,
		mineUserID, mineKeys, nil)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range pc {
		add(toUnplannedItem(it), model.UnplannedNoDate)
	}

	tasks, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support') AND t.status != 'complete'
		 AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')=''`,
		mineUserID, mineKeys, nil)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range tasks {
		add(toUnplannedItem(it), model.UnplannedNoDate)
	}

	unassigned, err := r.collectUnassigned()
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range unassigned {
		add(toUnplannedItem(it), model.UnplannedUnassigned)
	}

	meta := r.loadASNoDateMeta()
	for key, it := range bag {
		if !strings.HasPrefix(key, "as:") {
			continue
		}
		m, ok := meta[it.RefID]
		if !ok {
			continue
		}
		it.NoDateReason = m.reason
		it.NoDateAt = m.at
		if m.expired {
			add(*it, model.UnplannedReview)
		}
	}

	slots, err := r.listUnassignedSlotsAsItems()
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range slots {
		add(it, model.UnplannedNoDate)
	}

	var counts model.UnplannedKindCounts
	all := make([]model.UnplannedItem, 0, len(bag))
	for _, it := range bag {
		counts.AddItem(*it)
		all = append(all, *it)
	}

	kind = strings.TrimSpace(kind)
	out := all
	if kind != "" {
		filtered := make([]model.UnplannedItem, 0, len(all))
		for _, it := range all {
			if it.HasKind(kind) {
				filtered = append(filtered, it)
			}
		}
		out = filtered
	}

	sortUnplanned(out)
	return out, counts, nil
}

type noDateMeta struct {
	reason  string
	at      string
	expired bool
}

func (r *WorkBoardRepo) loadASNoDateMeta() map[string]noDateMeta {
	out := map[string]noDateMeta{}
	rows, err := r.db.Query(`
		SELECT as_id, TRIM(COALESCE(schedule_no_date_reason,'')), TRIM(COALESCE(schedule_no_date_at,'')),
		       CASE WHEN TRIM(COALESCE(schedule_no_date_at,'')) != ''
		             AND julianday('now','localtime') - julianday(schedule_no_date_at) >= ?
		            THEN 1 ELSE 0 END
		FROM as_receipts
		WHERE status IN `+model.SQLStatusOpenIncomplete+`
		  AND TRIM(COALESCE(schedule_no_date_reason,'')) != ''`, model.UnplannedNoDateReviewDays)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, reason, at string
		var expired int
		if rows.Scan(&id, &reason, &at, &expired) != nil {
			continue
		}
		out[id] = noDateMeta{reason: reason, at: at, expired: expired == 1}
	}
	return out
}

func (r *WorkBoardRepo) listUnassignedSlotsAsItems() ([]model.UnplannedItem, error) {
	mnt := NewMaintenanceRepo(r.db)
	planID, year, month, err := r.currentMntPlanYM()
	if err != nil || planID == "" {
		return nil, nil
	}
	slots, err := mnt.ListUnassignedMonthSlots(planID, year, month)
	if err != nil {
		return nil, err
	}
	out := make([]model.UnplannedItem, 0, len(slots))
	for _, s := range slots {
		title := s.ProductType
		if title == "" {
			title = "점검"
		}
		it := model.UnplannedItem{
			WorkListItem: model.WorkListItem{
				Prefix:      model.WorkPrefixMaintenance,
				RefID:       s.CustomerID + ":" + s.SlotKey,
				RefNumber:   s.ShortName,
				Title:       title + " · 일정 배정안됨",
				OrgName:     s.OrgName,
				Status:      "unassigned_slot",
				StatusLabel: "배정안됨",
				Href:        "/maintenance/" + planID,
				SubLabel:    s.ProductType,
			},
			ItemKey:       "slot:" + planID + "|" + s.CustomerID + "|" + s.ProductType,
			CanAssignDate: true,
			PlanID:        planID,
			CustomerID:    s.CustomerID,
			ProductType:   s.ProductType,
		}
		out = append(out, it)
	}
	return out, nil
}

func (r *WorkBoardRepo) currentMntPlanYM() (planID string, year, month int, err error) {
	now := time.Now()
	year, month = now.Year(), int(now.Month())
	mnt := NewMaintenanceRepo(r.db)
	p, err := mnt.GetPlanByYear(year)
	if err != nil {
		return "", 0, 0, err
	}
	if p == nil {
		plans, e := mnt.ListPlans()
		if e != nil || len(plans) == 0 {
			return "", year, month, e
		}
		p = &plans[0]
		year = p.PlanYear
	}
	return p.PlanID, year, month, nil
}

// CountPlanning §8.4 계획 수립률. 미정+사유는 분자에 포함한다. 정기점검 배정안됨은 분모에만 더한다.
func (r *WorkBoardRepo) CountPlanning() (model.PlanningCounts, error) {
	var p model.PlanningCounts
	add := func(open, planned int) {
		p.Open += open
		p.Planned += planned
	}

	var open, planned int
	err := r.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE
		         WHEN TRIM(COALESCE(visit_scheduled_date,'')) != ''
		           OR TRIM(COALESCE(schedule_no_date_reason,'')) != '' THEN 1 ELSE 0 END),0)
		FROM as_receipts
		WHERE status IN `+model.SQLStatusOpenIncomplete).Scan(&open, &planned)
	if err != nil {
		return p, err
	}
	add(open, planned)

	open, planned = 0, 0
	err = r.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN TRIM(COALESCE(w.scheduled_date,'')) != '' THEN 1 ELSE 0 END),0)
		FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		WHERE w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')`).Scan(&open, &planned)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return p, err
	}
	add(open, planned)

	monthPrefix := time.Now().Format("2006-01") + "%"
	var mntOpen int
	err = r.db.QueryRow(`
		SELECT COUNT(*) FROM maintenance_visits
		WHERE visit_date LIKE ? AND COALESCE(completed,0)=0`, monthPrefix).Scan(&mntOpen)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return p, err
	}
	add(mntOpen, mntOpen)

	slots, err := r.listUnassignedSlotsAsItems()
	if err != nil {
		return p, err
	}
	add(len(slots), 0)

	open, planned = 0, 0
	err = r.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE
		         WHEN COALESCE(NULLIF(TRIM(work_date),''), NULLIF(TRIM(due_date),''), '') != '' THEN 1 ELSE 0 END),0)
		FROM work_tasks
		WHERE work_type IN ('admin','support') AND status != 'complete'`).Scan(&open, &planned)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return p, err
	}
	add(open, planned)

	open, planned = 0, 0
	err = r.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN TRIM(COALESCE(work_date,'')) != '' THEN 1 ELSE 0 END),0)
		FROM work_other WHERE phase != 'complete'`).Scan(&open, &planned)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return p, err
	}
	add(open, planned)
	return p, nil
}

func toUnplannedItem(it model.WorkListItem) model.UnplannedItem {
	u := model.UnplannedItem{
		WorkListItem:  it,
		CanAssignDate: true,
		CanNoDate:     false,
	}
	key := unplannedItemKey(it)
	if strings.HasPrefix(key, "as:") {
		u.CanNoDate = true
		u.Href = "/as/" + it.RefID + "/action"
	}
	if it.Prefix == model.WorkPrefixMaintenance && !strings.HasPrefix(key, "slot:") {
		u.Href = "/maintenance/visits/" + it.RefID + "/action"
	}
	return u
}

func unplannedItemKey(it model.WorkListItem) string {
	href := it.Href
	switch {
	case strings.Contains(href, "/as/work/"):
		return "wi:" + it.RefID
	case strings.HasPrefix(href, "/as/"):
		return "as:" + it.RefID
	case strings.HasPrefix(href, "/workboard/tasks/"):
		return "task:" + it.RefID
	case it.Prefix == model.WorkPrefixMaintenance:
		return "mnt:" + it.RefID
	default:
		return "gen:" + it.RefID
	}
}

func sortUnplanned(items []model.UnplannedItem) {
	rank := func(it model.UnplannedItem) int {
		switch {
		case it.HasKind(model.UnplannedDelayed):
			return 0
		case it.HasKind(model.UnplannedNoDate):
			return 1
		case it.HasKind(model.UnplannedReview):
			return 2
		case it.HasKind(model.UnplannedNext):
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := rank(items[i]), rank(items[j])
		if ri != rj {
			return ri < rj
		}
		if items[i].DaysOverdue != items[j].DaysOverdue {
			return items[i].DaysOverdue > items[j].DaysOverdue
		}
		return items[i].OrgName < items[j].OrgName
	})
}

func parseUnplannedKey(key string) (kind, id string) {
	key = strings.TrimSpace(key)
	if i := strings.Index(key, ":"); i > 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

func parseSlotKey(id string) (planID, customerID, product string) {
	parts := strings.SplitN(id, "|", 3)
	if len(parts) < 2 {
		return "", "", ""
	}
	planID, customerID = parts[0], parts[1]
	if len(parts) == 3 {
		product = parts[2]
	}
	return planID, customerID, product
}

// AssignUnplannedDate 미계획 목록에서 날짜를 바로 넣는다.
func (r *WorkBoardRepo) AssignUnplannedDate(key, date, assignee, assigneeUID string) error {
	date = strings.TrimSpace(date)
	if date == "" {
		return fmt.Errorf("날짜가 필요합니다")
	}
	kind, id := parseUnplannedKey(key)
	switch kind {
	case "as":
		return NewASRepo(r.db).AssignUnplanned(id, date, assignee, assigneeUID)
	case "wi":
		return NewASWorkRepo(r.db).SetScheduledDate(id, date)
	case "mnt":
		return NewMaintenanceRepo(r.db).SetVisitDate(id, date)
	case "slot":
		planID, cust, product := parseSlotKey(id)
		if planID == "" || cust == "" {
			return fmt.Errorf("슬롯 정보가 없습니다")
		}
		v := model.MaintenanceVisit{
			PlanID: planID, VisitDate: date, CustomerID: cust,
			ProductType: product, Assignee: strings.TrimSpace(assignee), EntryCategory: "normal",
		}
		return NewMaintenanceRepo(r.db).InsertVisitFull(v)
	case "task":
		wb := NewWBRepo(r.db)
		if err := wb.SetTaskDueDate(id, date); err != nil {
			return err
		}
		_, err := r.db.Exec(`UPDATE work_tasks SET work_date=CASE WHEN TRIM(COALESCE(work_date,''))='' THEN ? ELSE work_date END,
			assignee=CASE WHEN TRIM(?)!='' AND TRIM(COALESCE(assignee,''))='' THEN ? ELSE assignee END,
			updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, date, assignee, assignee, id)
		return err
	case "gen":
		_, err := r.db.Exec(`UPDATE work_other SET work_date=? WHERE other_id=?`, date, id)
		return err
	default:
		return fmt.Errorf("알 수 없는 업무")
	}
}
