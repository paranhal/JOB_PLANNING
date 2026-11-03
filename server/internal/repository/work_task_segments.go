package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *WBRepo) ListSegments(taskID string) ([]model.WorkTaskSegment, error) {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT segment_id, task_id, kind, scope, seq, work_date, start_time, end_time, duration_min,
		       travel_before_min, travel_after_min, user_id, assignee, worker_user_id, worker_name,
		       interrupt_task_id, note
		FROM work_task_segments WHERE task_id=? ORDER BY kind, seq, start_time`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskSegment
	for rows.Next() {
		var s model.WorkTaskSegment
		if err := rows.Scan(&s.SegmentID, &s.TaskID, &s.Kind, &s.Scope, &s.Seq, &s.WorkDate,
			&s.StartTime, &s.EndTime, &s.DurationMin, &s.TravelBeforeMin, &s.TravelAfterMin,
			&s.UserID, &s.Assignee, &s.WorkerUserID, &s.WorkerName, &s.InterruptTaskID, &s.Note); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *WBRepo) ListPlanSegmentsOrVirtual(t model.WorkTask) ([]model.WorkTaskSegment, error) {
	all, err := r.ListSegments(t.TaskID)
	if err != nil {
		return nil, err
	}
	var plan []model.WorkTaskSegment
	for _, s := range all {
		if model.NormalizeSegmentKind(s.Kind) == model.SegKindPlan {
			plan = append(plan, s)
		}
	}
	if len(plan) == 0 {
		v := model.VirtualPlanSegment(t)
		if strings.TrimSpace(v.StartTime) != "" {
			return []model.WorkTaskSegment{v}, nil
		}
	}
	return plan, nil
}

func (r *WBRepo) ListActualSegments(taskID string) ([]model.WorkTaskSegment, error) {
	all, err := r.ListSegments(taskID)
	if err != nil {
		return nil, err
	}
	var out []model.WorkTaskSegment
	for _, s := range all {
		if model.NormalizeSegmentKind(s.Kind) == model.SegKindActual {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *WBRepo) ListSegmentsByTaskIDs(ids []string) (map[string][]model.WorkTaskSegment, error) {
	out := map[string][]model.WorkTaskSegment{}
	if r == nil || r.db == nil || len(ids) == 0 {
		return out, nil
	}
	ph := placeholders(len(ids))
	args := iface(ids)
	rows, err := r.db.Query(`
		SELECT segment_id, task_id, kind, scope, seq, work_date, start_time, end_time, duration_min,
		       travel_before_min, travel_after_min, user_id, assignee, worker_user_id, worker_name,
		       interrupt_task_id, note
		FROM work_task_segments WHERE task_id IN (`+ph+`) ORDER BY task_id, kind, seq, start_time`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s model.WorkTaskSegment
		if err := rows.Scan(&s.SegmentID, &s.TaskID, &s.Kind, &s.Scope, &s.Seq, &s.WorkDate,
			&s.StartTime, &s.EndTime, &s.DurationMin, &s.TravelBeforeMin, &s.TravelAfterMin,
			&s.UserID, &s.Assignee, &s.WorkerUserID, &s.WorkerName, &s.InterruptTaskID, &s.Note); err != nil {
			return nil, err
		}
		out[s.TaskID] = append(out[s.TaskID], s)
	}
	return out, rows.Err()
}

func (r *WBRepo) ReplacePlanSegments(taskID string, segs []model.WorkTaskSegment) error {
	return r.replaceSegmentsOfKind(taskID, model.SegKindPlan, segs)
}

func (r *WBRepo) ReplaceActualSegments(taskID string, segs []model.WorkTaskSegment) error {
	return r.replaceSegmentsOfKind(taskID, model.SegKindActual, segs)
}

func (r *WBRepo) replaceSegmentsOfKind(taskID, kind string, segs []model.WorkTaskSegment) error {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	if _, err := r.db.Exec(`DELETE FROM work_task_segments WHERE task_id=? AND kind=?`, taskID, kind); err != nil {
		return err
	}
	for i, s := range segs {
		if err := r.insertSegment(taskID, kind, i+1, s); err != nil {
			return err
		}
	}
	if kind == model.SegKindPlan {
		st, en, dur := model.EnvelopeFromPlanSegments(segs)
		if st != "" {
			if _, err := r.db.Exec(`UPDATE work_tasks SET start_time=?, end_time=?, duration_min=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
				st, en, dur, taskID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *WBRepo) insertSegment(taskID, kind string, seq int, s model.WorkTaskSegment) error {
	id := strings.TrimSpace(s.SegmentID)
	if id == "" || strings.HasPrefix(id, "virtual:") {
		var err error
		id, err = r.nextID("work_segment", "WS")
		if err != nil {
			return err
		}
	}
	dur := s.DurationMin
	if dur <= 0 {
		dur = model.DurationFromTimes(s.StartTime, s.EndTime)
	}
	_, err := r.db.Exec(`
		INSERT INTO work_task_segments (
			segment_id, task_id, kind, scope, seq, work_date, start_time, end_time, duration_min,
			travel_before_min, travel_after_min, user_id, assignee, worker_user_id, worker_name,
			interrupt_task_id, note)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, taskID, kind, model.NormalizeSegmentScope(s.Scope), seq, strings.TrimSpace(s.WorkDate),
		strings.TrimSpace(s.StartTime), strings.TrimSpace(s.EndTime), dur,
		model.ClampTravelMin(s.TravelBeforeMin), model.ClampTravelMin(s.TravelAfterMin),
		strings.TrimSpace(s.UserID), strings.TrimSpace(s.Assignee),
		strings.TrimSpace(s.WorkerUserID), strings.TrimSpace(s.WorkerName),
		strings.TrimSpace(s.InterruptTaskID), strings.TrimSpace(s.Note))
	return err
}

func (r *WBRepo) ApplyInterruptSplit(old *model.WorkTask, newID, date, newStart, newEnd string, travel int, overlappingNames []string) error {
	if r == nil || old == nil {
		return fmt.Errorf("끼워 넣을 업무가 없습니다")
	}
	plan, err := r.ListPlanSegmentsOrVirtual(*old)
	if err != nil {
		return err
	}
	members, _ := r.ListMembers(old.TaskID)
	allNames := participantNames(*old, members)
	overlapSet := map[string]bool{}
	for _, n := range overlappingNames {
		n = strings.TrimSpace(n)
		if n != "" {
			overlapSet[n] = true
		}
	}
	allHit := len(allNames) > 0
	for _, n := range allNames {
		if !overlapSet[n] {
			allHit = false
			break
		}
	}
	if len(allNames) == 0 {
		allHit = true
	}
	if allHit {
		var next []model.WorkTaskSegment
		seq := 1
		splitAny := false
		for _, seg := range plan {
			if model.NormalizeSegmentScope(seg.Scope) != model.SegScopeCommon {
				next = append(next, seg)
				continue
			}
			if !model.RangesOverlapHHMM(seg.StartTime, seg.EndTime, newStart, newEnd) {
				next = append(next, seg)
				continue
			}
			sp, err := model.SplitPlanAround(seg.StartTime, seg.EndTime, newStart, newEnd, travel)
			if err != nil {
				return err
			}
			splitAny = true
			if sp.HasFront {
				s := seg
				s.SegmentID = ""
				s.Virtual = false
				s.StartTime, s.EndTime = sp.FrontStart, sp.FrontEnd
				s.DurationMin = model.DurationFromTimes(s.StartTime, s.EndTime)
				s.TravelAfterMin = travel
				s.InterruptTaskID = newID
				s.Seq = seq
				next = append(next, s)
				seq++
			}
			if sp.HasBack {
				s := seg
				s.SegmentID = ""
				s.Virtual = false
				s.StartTime, s.EndTime = sp.BackStart, sp.BackEnd
				s.DurationMin = model.DurationFromTimes(s.StartTime, s.EndTime)
				s.TravelBeforeMin = travel
				s.InterruptTaskID = newID
				s.Seq = seq
				next = append(next, s)
				seq++
			}
		}
		if !splitAny {
			return fmt.Errorf("끼워 넣을 겹친 구간이 없습니다")
		}
		if err := r.ReplacePlanSegments(old.TaskID, next); err != nil {
			return err
		}
	} else {
		for _, name := range overlappingNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id, err := r.nextID("work_departure", "WD")
			if err != nil {
				return err
			}
			uid := ""
			if strings.TrimSpace(old.Assignee) == name {
				uid = old.AssigneeUserID
			}
			if _, err := r.db.Exec(`
				INSERT INTO work_task_departures (departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status)
				VALUES (?,?,?,?,?,?,?,?,?)`,
				id, old.TaskID, uid, name, date, newStart, newEnd, newID, model.ReturnPending); err != nil {
				return err
			}
		}
	}
	_ = r.CreateActivity(&model.WorkActivity{
		TaskID: old.TaskID, ActivityType: model.WBActivityOther,
		Content: "끼워넣기: " + newID + " " + newStart + "~" + newEnd,
	})
	return nil
}

func (r *WBRepo) ShiftPlanAfter(t *model.WorkTask, afterHHMM, interruptID string) error {
	if t == nil {
		return nil
	}
	plan, err := r.ListPlanSegmentsOrVirtual(*t)
	if err != nil {
		return err
	}
	raw := model.ParseHHMMMinutes(afterHHMM)
	if raw < 0 {
		return nil
	}
	cut := model.SnapUp15(raw)
	if cut <= raw {
		cut += 30
	}
	changed := false
	for i := range plan {
		s := &plan[i]
		if strings.TrimSpace(s.InterruptTaskID) != interruptID {
			continue
		}
		st := model.ParseHHMMMinutes(s.StartTime)
		if st >= 0 && st < cut {
			s.StartTime = model.FormatHHMMMinutes(cut)
			s.DurationMin = model.DurationFromTimes(s.StartTime, s.EndTime)
			s.SegmentID = ""
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return r.ReplacePlanSegments(t.TaskID, plan)
}

func (r *WBRepo) MergeInterruptSegments(taskID, interruptID string) error {
	plan, err := r.ListPlanSegmentsOrVirtual(model.WorkTask{TaskID: taskID})
	if err != nil {
		return err
	}
	t, err := r.GetTask(taskID)
	if err != nil || t == nil {
		return fmt.Errorf("업무를 찾을 수 없습니다")
	}
	if len(plan) == 1 && plan[0].Virtual {
		return nil
	}
	var kept []model.WorkTaskSegment
	var merge []model.WorkTaskSegment
	for _, s := range plan {
		if strings.TrimSpace(s.InterruptTaskID) == interruptID && model.NormalizeSegmentScope(s.Scope) == model.SegScopeCommon {
			merge = append(merge, s)
			continue
		}
		kept = append(kept, s)
	}
	if len(merge) == 0 {
		return nil
	}
	combined := merge[0]
	combined.SegmentID = ""
	combined.InterruptTaskID = ""
	combined.TravelBeforeMin = 0
	combined.TravelAfterMin = 0
	st, en := combined.StartTime, combined.EndTime
	for _, s := range merge[1:] {
		if s.StartTime < st {
			st = s.StartTime
		}
		if s.EndTime > en {
			en = s.EndTime
		}
	}
	combined.StartTime, combined.EndTime = st, en
	combined.DurationMin = model.DurationFromTimes(st, en)
	kept = append(kept, combined)
	return r.ReplacePlanSegments(taskID, kept)
}

func (r *WBRepo) TasksInterruptedBy(interruptID string) ([]string, error) {
	if r == nil || r.db == nil || strings.TrimSpace(interruptID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT DISTINCT task_id FROM work_task_segments WHERE interrupt_task_id=?
		UNION SELECT DISTINCT task_id FROM work_task_departures WHERE interrupt_task_id=?`, interruptID, interruptID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func (r *WBRepo) ListDepartures(taskID string) ([]model.WorkTaskDeparture, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status
		FROM work_task_departures WHERE task_id=? ORDER BY start_time`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskDeparture
	for rows.Next() {
		var d model.WorkTaskDeparture
		if err := rows.Scan(&d.DepartureID, &d.TaskID, &d.UserID, &d.Assignee, &d.WorkDate, &d.StartTime, &d.EndTime, &d.InterruptTaskID, &d.ReturnStatus); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *WBRepo) ListPendingReturns() ([]model.WorkTaskDeparture, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status
		FROM work_task_departures WHERE return_status=?`, model.ReturnPending)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskDeparture
	for rows.Next() {
		var d model.WorkTaskDeparture
		if err := rows.Scan(&d.DepartureID, &d.TaskID, &d.UserID, &d.Assignee, &d.WorkDate, &d.StartTime, &d.EndTime, &d.InterruptTaskID, &d.ReturnStatus); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *WBRepo) SetDepartureReturn(id, status string) error {
	if status != model.ReturnReturned && status != model.ReturnNone {
		status = model.ReturnNone
	}
	_, err := r.db.Exec(`UPDATE work_task_departures SET return_status=?, updated_at=CURRENT_TIMESTAMP WHERE departure_id=?`, status, id)
	return err
}

func (r *WBRepo) ListItems(taskID string) ([]model.WorkTaskItem, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT item_id, task_id, seq, title, status, note, copied_from_item_id,
		COALESCE(kind,'etc'), COALESCE(product_name,''), COALESCE(model_name,''), COALESCE(qty,1), COALESCE(as_id,''),
		COALESCE(template_id,''), COALESCE(set_main_item_id,''), COALESCE(unit,'')
		FROM work_task_items WHERE task_id=? ORDER BY seq`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return r.listItemsV97(taskID)
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskItem
	for rows.Next() {
		var it model.WorkTaskItem
		if err := rows.Scan(&it.ItemID, &it.TaskID, &it.Seq, &it.Title, &it.Status, &it.Note, &it.CopiedFromItemID,
			&it.Kind, &it.ProductName, &it.ModelName, &it.Qty, &it.ASID,
			&it.TemplateID, &it.SetMainItemID, &it.Unit); err != nil {
			return nil, err
		}
		it.Kind = model.NormalizeItemKind(it.Kind)
		if it.Qty < 1 {
			it.Qty = 1
		}
		it.SetIncluded = true
		it.ClientKey = it.ItemID
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.fillItemExtras(out)
	markSetMains(out)
	return out, nil
}

func markSetMains(items []model.WorkTaskItem) {
	mains := map[string]bool{}
	for _, it := range items {
		if id := strings.TrimSpace(it.SetMainItemID); id != "" {
			mains[id] = true
		}
	}
	for i := range items {
		if mains[items[i].ItemID] {
			items[i].IsSetMain = true
		}
	}
}

func (r *WBRepo) listItemsV97(taskID string) ([]model.WorkTaskItem, error) {
	rows, err := r.db.Query(`SELECT item_id, task_id, seq, title, status, note, copied_from_item_id,
		COALESCE(kind,'etc'), COALESCE(product_name,''), COALESCE(model_name,''), COALESCE(qty,1), COALESCE(as_id,'')
		FROM work_task_items WHERE task_id=? ORDER BY seq`, taskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return r.listItemsLegacy(taskID)
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskItem
	for rows.Next() {
		var it model.WorkTaskItem
		if err := rows.Scan(&it.ItemID, &it.TaskID, &it.Seq, &it.Title, &it.Status, &it.Note, &it.CopiedFromItemID,
			&it.Kind, &it.ProductName, &it.ModelName, &it.Qty, &it.ASID); err != nil {
			return nil, err
		}
		it.Kind = model.NormalizeItemKind(it.Kind)
		if it.Qty < 1 {
			it.Qty = 1
		}
		it.SetIncluded = true
		it.ClientKey = it.ItemID
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.fillItemExtras(out)
	markSetMains(out)
	return out, nil
}

func (r *WBRepo) listItemsLegacy(taskID string) ([]model.WorkTaskItem, error) {
	rows, err := r.db.Query(`SELECT item_id, task_id, seq, title, status, note, copied_from_item_id FROM work_task_items WHERE task_id=? ORDER BY seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskItem
	for rows.Next() {
		var it model.WorkTaskItem
		if err := rows.Scan(&it.ItemID, &it.TaskID, &it.Seq, &it.Title, &it.Status, &it.Note, &it.CopiedFromItemID); err != nil {
			return nil, err
		}
		it.Kind = model.ItemKindEtc
		it.Qty = 1
		it.SetIncluded = true
		it.ClientKey = it.ItemID
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *WBRepo) fillItemExtras(items []model.WorkTaskItem) {
	if len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	asIDs := []string{}
	idx := map[string]int{}
	for i, it := range items {
		ids = append(ids, it.ItemID)
		idx[it.ItemID] = i
		if strings.TrimSpace(it.ASID) != "" {
			asIDs = append(asIDs, it.ASID)
		}
	}
	if assigns, err := r.listItemAssignees(ids); err == nil {
		for id, list := range assigns {
			if i, ok := idx[id]; ok {
				items[i].Assignees = list
			}
		}
	}
	if assets, roles, missing, err := r.listItemAssets(ids, items); err == nil {
		for id, list := range assets {
			if i, ok := idx[id]; ok {
				items[i].AssetIDs = list
				items[i].AssetRole = roles[id]
			}
		}
		for id, miss := range missing {
			if i, ok := idx[id]; ok {
				items[i].AssetMissing = miss
			}
		}
	}
	r.overlayItemAS(items, asIDs)
}

func (r *WBRepo) overlayItemAS(items []model.WorkTaskItem, asIDs []string) {
	if len(asIDs) == 0 {
		return
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(asIDs)), ",")
	args := make([]interface{}, len(asIDs))
	for i, id := range asIDs {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT as_id, COALESCE(as_number,''), COALESCE(status,'') FROM as_receipts WHERE as_id IN (`+ph+`)`, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	type asRow struct{ num, st string }
	m := map[string]asRow{}
	for rows.Next() {
		var id, num, st string
		if rows.Scan(&id, &num, &st) == nil {
			m[id] = asRow{num, st}
		}
	}
	for i := range items {
		if row, ok := m[items[i].ASID]; ok {
			items[i].ASNumber = row.num
			items[i].ASLocked = true
			items[i].Status = model.ItemStatusFromAS(row.st)
		}
	}
}

func (r *WBRepo) ReplaceItems(taskID string, items []model.WorkTaskItem) error {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	old, err := r.ListItems(taskID)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, it := range items {
		if id := strings.TrimSpace(it.ItemID); id != "" {
			keep[id] = true
		}
	}
	for _, it := range old {
		if keep[it.ItemID] {
			continue
		}
		_, _ = r.db.Exec(`DELETE FROM work_task_item_segments WHERE item_id=?`, it.ItemID)
		_, _ = r.db.Exec(`DELETE FROM work_task_item_assignees WHERE item_id=?`, it.ItemID)
		_, _ = r.db.Exec(`DELETE FROM work_task_item_assets WHERE item_id=?`, it.ItemID)
		if _, err := r.db.Exec(`DELETE FROM work_task_items WHERE item_id=?`, it.ItemID); err != nil {
			return err
		}
	}
	oldByID := map[string]model.WorkTaskItem{}
	for _, it := range old {
		oldByID[it.ItemID] = it
	}
	keyToID := map[string]string{}
	for i := range items {
		id := strings.TrimSpace(items[i].ItemID)
		if id == "" {
			nid, err := r.nextID("work_item", "WI")
			if err != nil {
				return err
			}
			id = nid
			items[i].ItemID = id
		}
		keyToID[id] = id
		if k := strings.TrimSpace(items[i].ClientKey); k != "" {
			keyToID[k] = id
		}
	}
	for i := range items {
		sk := strings.TrimSpace(items[i].SetMainKey)
		if sk == "" {
			sk = strings.TrimSpace(items[i].SetMainItemID)
		}
		if sk == "" {
			items[i].SetMainItemID = ""
			continue
		}
		if nid, ok := keyToID[sk]; ok {
			items[i].SetMainItemID = nid
		} else {
			items[i].SetMainItemID = ""
		}
	}
	for i, it := range items {
		title := strings.TrimSpace(it.Title)
		product := strings.TrimSpace(it.ProductName)
		if title == "" {
			title = product
		}
		if title == "" {
			continue
		}
		if product == "" {
			product = title
		}
		id := strings.TrimSpace(it.ItemID)
		qty := it.Qty
		if qty < 1 {
			qty = 1
		}
		kind := model.NormalizeItemKind(it.Kind)
		status := model.NormalizeItemStatus(it.Status)
		asID := strings.TrimSpace(it.ASID)
		if prev, ok := oldByID[id]; ok && strings.TrimSpace(prev.ASID) != "" {
			asID = prev.ASID
			status = prev.Status
		}
		unit := strings.TrimSpace(it.Unit)
		tplID := strings.TrimSpace(it.TemplateID)
		setMain := strings.TrimSpace(it.SetMainItemID)
		if _, err := r.db.Exec(`
			INSERT INTO work_task_items (item_id, task_id, seq, title, status, note, copied_from_item_id, kind, product_name, model_name, qty, as_id, template_id, set_main_item_id, unit)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(item_id) DO UPDATE SET
			  task_id=excluded.task_id, seq=excluded.seq, title=excluded.title, status=excluded.status,
			  note=excluded.note, kind=excluded.kind, product_name=excluded.product_name,
			  model_name=excluded.model_name, qty=excluded.qty, as_id=excluded.as_id,
			  template_id=excluded.template_id, set_main_item_id=excluded.set_main_item_id, unit=excluded.unit,
			  updated_at=CURRENT_TIMESTAMP`,
			id, taskID, i+1, title, status, strings.TrimSpace(it.Note), strings.TrimSpace(it.CopiedFromItemID),
			kind, product, strings.TrimSpace(it.ModelName), qty, asID, tplID, setMain, unit); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such column") {
				if _, err2 := r.db.Exec(`INSERT INTO work_task_items (item_id, task_id, seq, title, status, note, copied_from_item_id) VALUES (?,?,?,?,?,?,?)`,
					id, taskID, i+1, title, status, strings.TrimSpace(it.Note), strings.TrimSpace(it.CopiedFromItemID)); err2 != nil {
					return err2
				}
			} else {
				return err
			}
		}
		if err := r.replaceItemAssignees(id, it.Assignees); err != nil {
			return err
		}
		items[i].ItemID = id
	}
	return nil
}

func (r *WBRepo) CopyIncompleteItems(fromTask, toTask string) error {
	items, err := r.ListItems(fromTask)
	if err != nil {
		return err
	}
	var copy []model.WorkTaskItem
	for _, it := range items {
		if model.ItemIncomplete(it.Status) {
			from := it.ItemID
			it.ItemID = ""
			it.CopiedFromItemID = from
			it.Status = model.ItemTodo
			it.ASID = ""
			it.ASNumber = ""
			it.ASLocked = false
			it.AssetIDs = nil
			it.AssetMissing = false
			it.SetMainItemID = ""
			it.SetMainKey = ""
			it.IsSetMain = false
			it.ClientKey = ""
			copy = append(copy, it)
		}
	}
	if len(copy) == 0 {
		return nil
	}
	return r.ReplaceItems(toTask, copy)
}

func (r *WBRepo) HasIncompleteItems(taskID string) (bool, error) {
	items, err := r.ListItems(taskID)
	if err != nil {
		return false, err
	}
	for _, it := range items {
		if model.ItemIncomplete(it.Status) {
			return true, nil
		}
	}
	return false, nil
}

func personOccupiesAt(t model.WorkTask, person string, members []model.WorkTaskMember, segs []model.WorkTaskSegment, deps []model.WorkTaskDeparture, start, end string) bool {
	memMap := map[string][]model.WorkTaskMember{t.TaskID: members}
	if !personOnTask(t, person, memMap) {
		return false
	}
	for _, d := range deps {
		if (strings.TrimSpace(d.Assignee) == person || strings.TrimSpace(d.UserID) == person) &&
			model.RangesOverlapHHMM(d.StartTime, d.EndTime, start, end) {
			return false
		}
	}
	plan := segs
	if len(plan) == 0 {
		v := model.VirtualPlanSegment(t)
		if v.StartTime != "" {
			plan = []model.WorkTaskSegment{v}
		}
	}
	for _, s := range plan {
		if model.NormalizeSegmentKind(s.Kind) != model.SegKindPlan {
			continue
		}
		if model.NormalizeSegmentScope(s.Scope) == model.SegScopePerson {
			if strings.TrimSpace(s.Assignee) != person && strings.TrimSpace(s.UserID) != person {
				continue
			}
		}
		if model.RangesOverlapHHMM(s.StartTime, s.EndTime, start, end) {
			return true
		}
	}
	return false
}

func (r *WBRepo) CountStoredPlanSegments(taskID string) (int, error) {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM work_task_segments WHERE task_id=? AND kind=?`, taskID, model.SegKindPlan).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WBRepo) CountPersonScopedPlan(taskID string) (int, error) {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM work_task_segments WHERE task_id=? AND kind=? AND scope=?`,
		taskID, model.SegKindPlan, model.SegScopePerson).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WBRepo) MaxPlanSegmentUpdatedAt(taskID string) string {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return ""
	}
	var at string
	err := r.db.QueryRow(`SELECT COALESCE(MAX(updated_at),'') FROM work_task_segments WHERE task_id=? AND kind=?`, taskID, model.SegKindPlan).Scan(&at)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(at)
}

func (r *WBRepo) ShiftSegmentDates(taskID, oldDate, newDate string) error {
	taskID = strings.TrimSpace(taskID)
	oldDate, newDate = strings.TrimSpace(oldDate), strings.TrimSpace(newDate)
	if r == nil || r.db == nil || taskID == "" || oldDate == "" || newDate == "" || oldDate == newDate {
		return nil
	}
	delta, err := daysBetweenYMD(oldDate, newDate)
	if err != nil {
		return err
	}
	sign := "+"
	n := delta
	if n < 0 {
		sign = "-"
		n = -n
	}
	mod := fmt.Sprintf("%s%d days", sign, n)
	if _, err := r.db.Exec(`UPDATE work_task_segments SET work_date=date(work_date, ?), updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND TRIM(work_date)!=''`, mod, taskID); err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			return err
		}
	}
	if _, err := r.db.Exec(`UPDATE work_task_segments SET work_date=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND TRIM(work_date)=''`, newDate, taskID); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	if _, err := r.db.Exec(`UPDATE work_task_departures SET work_date=date(work_date, ?), updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND TRIM(work_date)!=''`, mod, taskID); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	return nil
}

func daysBetweenYMD(from, to string) (int, error) {
	a, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return 0, err
	}
	b, err := time.ParseInLocation("2006-01-02", to, time.Local)
	if err != nil {
		return 0, err
	}
	return int(b.Sub(a).Hours() / 24), nil
}

func (r *WBRepo) CollapsePlanToSingle(t *model.WorkTask, date, start, end string) error {
	if t == nil {
		return nil
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = strings.TrimSpace(t.WorkDate)
	}
	if start == "" {
		start = t.StartTime
	}
	if end == "" {
		end = t.EndTime
	}
	return r.ReplacePlanSegments(t.TaskID, []model.WorkTaskSegment{{
		Kind: model.SegKindPlan, Scope: model.SegScopeCommon, Seq: 1,
		WorkDate: date, StartTime: start, EndTime: end,
		DurationMin: model.DurationFromTimes(start, end),
		Note:        "방문 시간 수정으로 구간 합침",
	}})
}

func clockHHMM(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func (r *WBRepo) PushEnvelopeToVisit(visitID, start, end string, dur int) error {
	visitID = strings.TrimSpace(visitID)
	if r == nil || r.db == nil || visitID == "" {
		return nil
	}
	start, end = clockHHMM(start), clockHHMM(end)
	if dur <= 0 {
		dur = model.DurationFromTimes(start, end)
	}
	_, err := r.db.Exec(`
		UPDATE maintenance_visits
		SET start_time=?, end_time=?, duration_min=?,
		    notes=TRIM(COALESCE(notes,'') || CASE WHEN COALESCE(notes,'')='' THEN '' ELSE char(10) END || '끼워넣기로 시간 조정')
		WHERE visit_id=?`, start, end, dur, visitID)
	return err
}

func (r *WBRepo) VisitUpdatedAt(visitID string) string {
	var at string
	if r == nil || r.db == nil {
		return ""
	}
	_ = r.db.QueryRow(`SELECT COALESCE(updated_at,'') FROM maintenance_visits WHERE visit_id=?`, visitID).Scan(&at)
	return strings.TrimSpace(at)
}

func participantNames(t model.WorkTask, members []model.WorkTaskMember) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	add(t.Assignee)
	for _, m := range members {
		add(m.Assignee)
	}
	return out
}
