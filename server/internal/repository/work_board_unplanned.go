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
	return r.listUnplanned(mineUserID, mineKeys, kind, nil, OrgAll)
}

func (r *WorkBoardRepo) ListUnplannedForOrg(mineUserID string, mineKeys []string, kind, orgID string) ([]model.UnplannedItem, model.UnplannedKindCounts, error) {
	return r.listUnplanned(mineUserID, mineKeys, kind, nil, orgID)
}

func (r *WorkBoardRepo) listUnplanned(mineUserID string, mineKeys []string, kind string, reuse *unplannedBase, orgID string) ([]model.UnplannedItem, model.UnplannedKindCounts, error) {
	start := time.Now()
	defer logSlowSQL("work.ListUnplanned", start)
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
		if k == model.UnplannedDelayed && base.Started {
			cur.Started = true
		}
		cur.Kinds = append(cur.Kinds, k)
		cur.Badges = append(cur.Badges, model.UnplannedBadgeOfStarted(k, cur.DaysOverdue, cur.Started && k == model.UnplannedDelayed))
	}

	var delayed, pending, open, unassigned []model.WorkListItem
	var err error
	if reuse != nil {
		delayed, pending, open, unassigned = reuse.Delayed, reuse.Pending, reuse.Open, reuse.Unassigned
	} else {
		delayed, err = r.collectDelayed(mineUserID, mineKeys, today)
		if err != nil {
			return nil, model.UnplannedKindCounts{}, err
		}
		pending, err = r.collectSchedulePending(mineUserID, mineKeys)
		if err != nil {
			return nil, model.UnplannedKindCounts{}, err
		}
		open, err = r.collectOpen(mineUserID, mineKeys)
		if err != nil {
			return nil, model.UnplannedKindCounts{}, err
		}
		unassigned, err = r.CollectUnassigned(OrgAll)
		if err != nil {
			return nil, model.UnplannedKindCounts{}, err
		}
	}
	started := r.asStartedSet(delayedIDs(delayed))
	for _, it := range delayed {
		u := toUnplannedItem(it)
		if it.Prefix == model.WorkPrefixAS {
			u.Started = started[it.RefID]
		}
		if it.SubLabel == "회신 대기" && it.Status == model.WBActionWaiting {
			u.ItemKey = "wa:" + it.RefID
			u.CanAssignDate = false
		}
		if it.SubLabel == "미완료" {
			u.CanAssignDate = false
		}
		add(u, model.UnplannedDelayed)
	}

	for _, it := range pending {
		d := strings.TrimSpace(it.ScheduledDate)
		if d != "" && d < today {
			add(toUnplannedItem(it), model.UnplannedNext)
		}
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
		`t.work_type IN ('admin','support','sales') AND t.status != 'complete'
		 AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')=''`,
		mineUserID, mineKeys, nil)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range tasks {
		add(toUnplannedItem(it), model.UnplannedNoDate)
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

	slots, err := r.listUnassignedSlotsAsItems(orgID)
	if err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	for _, it := range slots {
		add(it, model.UnplannedNoDate)
	}

	if err := r.addSalesUnplanned(add, mineUserID, mineKeys); err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	if err := r.addSalesNextYMUnplanned(add, mineUserID, mineKeys); err != nil {
		return nil, model.UnplannedKindCounts{}, err
	}
	if err := r.addAssetSerialUnplanned(add); err != nil {
		return nil, model.UnplannedKindCounts{}, err
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

type unplannedBase struct {
	Delayed, Pending, Open, Unassigned []model.WorkListItem
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

func (r *WorkBoardRepo) listUnassignedSlotsAsItems(orgID string) ([]model.UnplannedItem, error) {
	mnt := NewMaintenanceRepo(r.db)
	planID, year, month, err := r.currentMntPlanYM()
	if err != nil || planID == "" {
		return nil, nil
	}
	slots, err := mnt.ListOpenUnassignedSlots(planID, year, month)
	if err != nil {
		return nil, err
	}
	slots = FilterUnassignedSlotsByOrg(slots, orgID)
	out := make([]model.UnplannedItem, 0, len(slots))
	for _, s := range slots {
		out = append(out, unplannedFromMonthSlot(planID, s))
	}
	return out, nil
}

func unplannedFromMonthSlot(planID string, s model.UnassignedMonthSlot) model.UnplannedItem {
	title := s.ProductType
	if title == "" {
		title = "점검"
	}
	slotLabel := "일정 배정안됨"
	if s.UnconfirmedQuarter {
		if p, ok := model.CyclePeriodOf(s.Cycle, 12); ok {
			slotLabel = p.UnconfirmedLabel()
		} else {
			slotLabel = "분기 점검 미확정"
		}
	}
	return model.UnplannedItem{
		WorkListItem: model.WorkListItem{
			Prefix:      model.WorkPrefixMaintenance,
			RefID:       s.CustomerID + ":" + s.SlotKey,
			RefNumber:   s.ShortName,
			Title:       title + " · " + slotLabel,
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
}

// CollectQuarterUnassignedAlerts 분기 마지막 달의 「분기 점검 미확정」만. 알람·받은편지함 재사용(D-76).
func (r *WorkBoardRepo) CollectQuarterUnassignedAlerts(orgID string, year, month int) ([]model.WorkListItem, error) {
	if r == nil || !model.IsLastMonthOfQuarter(month) {
		return nil, nil
	}
	mnt := NewMaintenanceRepo(r.db)
	p, err := mnt.GetPlanByYear(year)
	if err != nil || p == nil {
		return nil, err
	}
	slots, err := mnt.ListOpenUnassignedSlots(p.PlanID, year, month)
	if err != nil {
		return nil, err
	}
	slots = FilterUnassignedSlotsByOrg(slots, orgID)
	var out []model.WorkListItem
	for _, s := range slots {
		if !s.UnconfirmedQuarter {
			continue
		}
		out = append(out, unplannedFromMonthSlot(p.PlanID, s).WorkListItem)
	}
	return out, nil
}

func (r *WorkBoardRepo) addAssetSerialUnplanned(add func(model.UnplannedItem, string)) error {
	rows, err := r.db.Query(`
		SELECT a.asset_id, COALESCE(a.product_name,''), COALESCE(cu.org_name,''), COALESCE(a.sales_order_id,'')
		FROM assets a
		LEFT JOIN customers cu ON cu.customer_id = a.customer_id
		WHERE TRIM(COALESCE(a.sales_order_id,'')) != ''
		  AND TRIM(COALESCE(a.serial_number,'')) = ''`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, org, orderID string
		if err := rows.Scan(&id, &name, &org, &orderID); err != nil {
			return err
		}
		title := strings.TrimSpace(name)
		if title == "" {
			title = "납품 자산"
		}
		add(model.UnplannedItem{
			WorkListItem: model.WorkListItem{
				Prefix:      model.WorkPrefixGeneral,
				RefID:       id,
				RefNumber:   id,
				Title:       title + " · " + model.UnplannedAssetSerialTitle,
				OrgName:     org,
				Href:        "/assets/" + id,
				Status:      "serial",
				StatusLabel: model.UnplannedAssetSerialTitle,
			},
			ItemKey:       "asset-serial:" + id,
			CanAssignDate: false,
		}, model.UnplannedAssetSerial)
	}
	return rows.Err()
}

func (r *WorkBoardRepo) addSalesUnplanned(add func(model.UnplannedItem, string), mineUserID string, mineKeys []string) error {
	gaps, kinds, err := NewSalesRepo(r.db).ListFollowupGaps(time.Now().Format("2006-01"))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	mine := strings.TrimSpace(mineUserID) != "" || len(mineKeys) > 0
	mineSet := map[string]bool{}
	for _, k := range mineKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			mineSet[k] = true
		}
	}
	for _, p := range gaps {
		if mine && !mineSet[strings.TrimSpace(p.SalesOwner)] && !mineSet[strings.TrimSpace(p.SalesOwnerID)] {
			continue
		}
		org := p.CustomerValue()
		it := model.UnplannedItem{
			WorkListItem: model.WorkListItem{
				Prefix:        model.WorkPrefixSales,
				RefID:         p.SalesID,
				RefNumber:     p.SalesID,
				Title:         p.Name,
				OrgName:       org,
				Assignee:      p.SalesOwner,
				ScheduledDate: p.ExpectedYM,
				Href:          "/sales/" + p.SalesID,
			},
			ItemKey:       "sales:" + p.SalesID,
			CanAssignDate: false,
			CanNoDate:     false,
		}
		for _, k := range kinds[p.SalesID] {
			add(it, k)
		}
	}
	return nil
}

func (r *WorkBoardRepo) addSalesNextYMUnplanned(add func(model.UnplannedItem, string), mineUserID string, mineKeys []string) error {
	todayYM := time.Now().Format("2006-01")
	gaps, err := NewSalesRepo(r.db).ListNextActionYMGaps(todayYM)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	mine := strings.TrimSpace(mineUserID) != "" || len(mineKeys) > 0
	mineSet := map[string]bool{}
	for _, k := range mineKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			mineSet[k] = true
		}
	}
	for _, g := range gaps {
		if mine && !mineSet[strings.TrimSpace(g.OwnerName)] {
			continue
		}
		it := model.UnplannedItem{
			WorkListItem: model.WorkListItem{
				Prefix:        model.WorkPrefixSales,
				RefID:         g.ActivityID,
				RefNumber:     g.ActivityID,
				Title:         g.NextAction,
				OrgName:       g.CustomerName,
				Assignee:      g.OwnerName,
				ScheduledDate: g.NextActionYM,
				Href:          "/sales/" + g.SalesID,
			},
			ItemKey:       "salesact:" + g.ActivityID,
			CanAssignDate: true,
			CanNoDate:     false,
			CustomerID:    g.CustomerID,
		}
		add(it, model.UnplannedSalesNext)
		if g.NextActionYM < todayYM {
			add(it, model.UnplannedReview)
		}
	}
	return nil
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

// CountPlanning §8.4 계획 수립률.
// 분자는 「계획이 있는 건」(예정일 또는 미정+사유). 밀린 건(경과·다음일정·재검토)은 계획이 있으므로 분자에 둔다(§8.2.1).
// 담당자미배정은 계획이 없는 건이므로 분자에서 뺀다. 정기점검 배정안됨은 분모에만 더한다.
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
		         WHEN TRIM(COALESCE(assigned_to,''))='' AND TRIM(COALESCE(assigned_user_id,''))='' THEN 0
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

	slots, err := r.listUnassignedSlotsAsItems(OrgAll)
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
	if p.Planned < 0 {
		p.Planned = 0
	}
	if p.Planned > p.Open {
		p.Planned = p.Open
	}
	p.NeedPlan = p.Open - p.Planned
	if p.NeedPlan < 0 {
		p.NeedPlan = 0
	}
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
	if strings.HasPrefix(it.Href, "/sales/") || it.Prefix == model.WorkPrefixSales {
		u.CanAssignDate = false
		u.CanNoDate = false
		u.Href = "/sales/" + it.RefID
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
	case strings.HasPrefix(href, "/sales/"):
		return "sales:" + it.RefID
	case it.Prefix == model.WorkPrefixMaintenance:
		return "mnt:" + it.RefID
	default:
		return "gen:" + it.RefID
	}
}

func delayedIDs(items []model.WorkListItem) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		if it.Prefix != model.WorkPrefixAS || it.RefID == "" || seen[it.RefID] {
			continue
		}
		seen[it.RefID] = true
		out = append(out, it.RefID)
	}
	return out
}

// asStartedSet §8.2.2 착수 = start_datetime 또는 as_processes.
func (r *WorkBoardRepo) asStartedSet(ids []string) map[string]bool {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `SELECT ar.as_id FROM as_receipts ar
		WHERE ar.as_id IN (` + ph + `)
		  AND (TRIM(COALESCE(ar.start_datetime,'')) != ''
		       OR EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = ar.as_id))`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out[id] = true
		}
	}
	return out
}

func sortUnplanned(items []model.UnplannedItem) {
	rank := func(it model.UnplannedItem) int {
		switch {
		case it.HasKind(model.UnplannedDelayed) && !it.Started:
			return 0 // 빨강 D+n 먼저 (§8.2.2)
		case it.HasKind(model.UnplannedDelayed) && it.Started:
			return 1
		case it.HasKind(model.UnplannedNoDate):
			return 2
		case it.HasKind(model.UnplannedReview):
			return 3
		case it.HasKind(model.UnplannedNext):
			return 4
		default:
			return 5
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
	date, err := model.ParseAppDate(date)
	if err != nil {
		return err
	}
	if date == "" {
		return fmt.Errorf("날짜가 필요합니다")
	}
	kind, id := parseUnplannedKey(key)
	switch kind {
	case "as":
		return NewASRepo(r.db).AssignUnplanned(id, date, assignee, assigneeUID)
	case "wi":
		if err := NewASWorkRepo(r.db).SetScheduledDate(id, date); err != nil {
			return err
		}
		if strings.TrimSpace(assignee) == "" && strings.TrimSpace(assigneeUID) == "" {
			return nil
		}
		return NewASWorkRepo(r.db).SetAssignee(id, assignee, assigneeUID)
	case "mnt":
		return NewMaintenanceRepo(r.db).AssignOpenVisit(id, date, assignee)
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
		oldName, oldUID := readAssigneePair(r.db, "work_tasks", "task_id", id, "assignee", "assignee_user_id")
		if err := wb.SetTaskDueDate(id, date); err != nil {
			return err
		}
		name, uid := bindStaff(r.db, assignee, assigneeUID)
		_, err := r.db.Exec(`UPDATE work_tasks SET work_date=CASE WHEN TRIM(COALESCE(work_date,''))='' THEN ? ELSE work_date END,
			assignee=CASE WHEN TRIM(?)!='' AND TRIM(COALESCE(assignee,''))='' THEN ? ELSE assignee END,
			assignee_user_id=CASE WHEN TRIM(?)!='' AND TRIM(COALESCE(assignee_user_id,''))='' THEN ? ELSE assignee_user_id END,
			updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, date, name, name, uid, uid, id)
		if err == nil && strings.TrimSpace(name) != "" {
			stampAssignedIfChanged(r.db, "work_tasks", "task_id", id, oldName, oldUID, name, uid)
		}
		return err
	case "gen":
		_, err := r.db.Exec(`UPDATE work_other SET work_date=? WHERE other_id=?`, date, id)
		return err
	case "salesact":
		return NewSalesRepo(r.db).CreateNextActionTask(id, date, assignee, assigneeUID)
	default:
		return fmt.Errorf("알 수 없는 업무")
	}
}

const unplannedHoldPrefix = "[보류] "

// TransferUnplanned 미배정·알림 팝업에서 담당자만 바꾼다.
func (r *WorkBoardRepo) TransferUnplanned(key, assignee, assigneeUID, date string) error {
	assignee = strings.TrimSpace(assignee)
	assigneeUID = strings.TrimSpace(assigneeUID)
	if assignee == "" && assigneeUID == "" {
		return fmt.Errorf("담당자가 필요합니다")
	}
	kind, id := parseUnplannedKey(key)
	switch kind {
	case "as":
		name, uid := bindStaff(r.db, assignee, assigneeUID)
		oldName, oldUID := readAssigneePair(r.db, "as_receipts", "as_id", id, "assigned_to", "assigned_user_id")
		now := time.Now().Format("2006-01-02 15:04:05")
		_, err := r.db.Exec(`UPDATE as_receipts SET assigned_to=?, assigned_user_id=?, updated_at=? WHERE as_id=?`,
			name, uid, now, id)
		if err == nil {
			stampAssignedIfChanged(r.db, "as_receipts", "as_id", id, oldName, oldUID, name, uid)
		}
		return err
	case "wi":
		return NewASWorkRepo(r.db).SetAssignee(id, assignee, assigneeUID)
	case "mnt":
		return NewMaintenanceRepo(r.db).AssignOpenVisit(id, date, assignee)
	case "slot":
		if strings.TrimSpace(date) == "" {
			return fmt.Errorf("날짜가 필요합니다")
		}
		return r.AssignUnplannedDate(key, date, assignee, assigneeUID)
	case "task":
		return NewWBRepo(r.db).SetTaskAssignee(id, assignee)
	case "gen":
		return fmt.Errorf("기타 업무는 이관을 지원하지 않습니다")
	case "salesact":
		return NewSalesRepo(r.db).CreateNextActionTask(id, firstFilled(date, time.Now().Format("2006-01-02")), assignee, assigneeUID)
	default:
		return fmt.Errorf("이관을 지원하지 않는 유형입니다")
	}
}

// HoldUnplanned 보류 상태·사유를 남기고 미배정 팝업에서 뺀다.
func (r *WorkBoardRepo) HoldUnplanned(key, reason, date string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("보류 사유가 필요합니다")
	}
	kind, id := parseUnplannedKey(key)
	note := unplannedHoldPrefix + reason
	switch kind {
	case "as":
		return NewASRepo(r.db).SetHold(id, reason, "")
	case "wi":
		return NewASWorkRepo(r.db).Hold(id, reason)
	case "mnt":
		_, err := r.db.Exec(`UPDATE maintenance_visits SET notes=? WHERE visit_id=?`, note, id)
		return err
	case "slot":
		if strings.TrimSpace(date) == "" {
			date = time.Now().Format("2006-01-02")
		}
		planID, cust, product := parseSlotKey(id)
		if planID == "" || cust == "" {
			return fmt.Errorf("슬롯 정보가 없습니다")
		}
		v := model.MaintenanceVisit{
			PlanID: planID, VisitDate: date, CustomerID: cust,
			ProductType: product, Notes: note, EntryCategory: "normal",
		}
		return NewMaintenanceRepo(r.db).InsertVisitFull(v)
	case "task":
		wb := NewWBRepo(r.db)
		t, err := wb.GetTask(id)
		if err != nil || t == nil {
			return fmt.Errorf("업무가 없습니다")
		}
		t.Status = model.WBTaskHold
		t.HoldReason = reason
		return wb.UpdateTask(t)
	case "gen":
		_, err := r.db.Exec(`UPDATE work_other SET notes=? WHERE other_id=?`, note, id)
		return err
	case "salesact":
		if err := NewSalesRepo(r.db).CreateNextActionTask(id, firstFilled(date, time.Now().Format("2006-01-02")), "", ""); err != nil {
			return err
		}
		t, err := NewWBRepo(r.db).GetTaskBySource(model.WBSourceSalesActivity, id, model.WBSourceRoleNext)
		if err != nil || t == nil {
			return fmt.Errorf("영업 활동 업무가 없습니다")
		}
		t.Status = model.WBTaskHold
		t.HoldReason = reason
		return NewWBRepo(r.db).UpdateTask(t)
	default:
		return fmt.Errorf("보류를 지원하지 않는 유형입니다")
	}
}

func firstFilled(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
