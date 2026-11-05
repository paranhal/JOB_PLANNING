package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type pendingInterrupt struct {
	Hits   []overlapHit
	Travel int
	Date   string
	Start  string
	End    string
	Names  []string
}

func parseSplitTravel(c echo.Context) int {
	n, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("split_travel_min")))
	return model.ClampTravelMin(n)
}

func splitBlockLabel(code string) string {
	switch code {
	case "read_only":
		return "읽기 전용에서는 나눌 수 없습니다"
	case "other_org":
		return "다른 조직의 업무는 나눌 수 없습니다"
	case "completed":
		return "완료된 업무는 나눌 수 없습니다"
	case "cancelled":
		return "취소된 업무는 나눌 수 없습니다"
	case "not_assignee":
		return "다른 사람 업무는 조직관리자만 나눌 수 있습니다"
	case "no_segments":
		return "나눌 계획 구간이 없습니다"
	case "too_short":
		return "남는 구간이 15분 미만입니다"
	default:
		return "이 계획을 나눌 수 없습니다"
	}
}

// canSplitPlan 완료·취소·타 조직·읽기 전용이 아니면 나눌 수 있다. AS·작업·정기점검·행정/지원 등 유형은 막지 않는다.
func canSplitPlan(c echo.Context, t *model.WorkTask, members []model.WorkTaskMember) bool {
	ok, _, _ := canSplitPlanReason(c, t, members)
	return ok
}

func canSplitPlanReason(c echo.Context, t *model.WorkTask, members []model.WorkTaskMember) (bool, string, string) {
	if t == nil {
		return false, "no_segments", splitBlockLabel("no_segments")
	}
	if isReadOnly(c) || !canWriteWorkboard(c) {
		return false, "read_only", splitBlockLabel("read_only")
	}
	st := strings.TrimSpace(t.Status)
	if st == model.WBTaskComplete {
		return false, "completed", splitBlockLabel("completed")
	}
	if st == model.WBTaskCancelled {
		return false, "cancelled", splitBlockLabel("cancelled")
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return false, "other_org", splitBlockLabel("other_org")
	}
	uid := currentUserID(c)
	name := currentUserDisplayName(c)
	if taskAssignedToUser(*t, members, uid, name) {
		return true, "", ""
	}
	if memberOfGroup(c, t.WorkGroupID) {
		return true, "", ""
	}
	role := currentRole(c)
	if role == model.RoleVisionAdmin {
		return true, "", ""
	}
	if role == model.RoleOrgAdmin {
		org := currentOrg(c)
		if org != "" && (org == t.OrgID || org == repository.OrgAll) {
			return true, "", ""
		}
	}
	return false, "not_assignee", splitBlockLabel("not_assignee")
}

func (h *WorkboardHandler) applyPendingInterrupt(c echo.Context, newID string) error {
	h.seedPendingFromLiveHost(c)
	return h.applyLiveInterrupt(c, newID)
}

func (h *WorkboardHandler) seedPendingFromLiveHost(c echo.Context) {
	if c == nil {
		return
	}
	if v, _ := c.Get("pending_interrupt").(*pendingInterrupt); v != nil && len(v.Hits) > 0 {
		return
	}
	hostID := strings.TrimSpace(firstNonEmpty(c.FormValue("live_host_id"), c.FormValue("interrupt_host_id")))
	if hostID == "" {
		return
	}
	host, err := h.repo.GetTask(hostID)
	if err != nil || host == nil {
		return
	}
	day, hhmm, _ := parseLiveClock(c, firstNonEmpty(c.FormValue("work_date"), host.WorkDate))
	start := firstNonEmpty(c.FormValue("start_time"), hhmm)
	dur := atoiSafe(c.FormValue("duration_min"))
	if dur <= 0 {
		dur = 60
	}
	end := firstNonEmpty(c.FormValue("end_time"), model.FormatHHMMMinutes(model.ParseHHMMMinutes(start)+dur))
	c.Set("pending_interrupt", &pendingInterrupt{
		Hits:   []overlapHit{{Task: *host, Date: day}},
		Travel: atoiSafe(c.FormValue("travel_before_min")),
		Date:   day, Start: start, End: end,
		Names: occupancyNames(host.Assignee, nil),
	})
	if fr := strings.TrimSpace(c.FormValue("force_reason")); fr != "" {
		c.Set("live_force_reason", fr)
	}
}

func (h *WorkboardHandler) commitPendingInterrupt(c echo.Context, newID string, rollbackNew bool) error {
	v, _ := c.Get("pending_interrupt").(*pendingInterrupt)
	if v == nil || strings.TrimSpace(newID) == "" {
		return nil
	}
	var firstErr error
	applied := []string{}
	splitOK := map[string]bool{}
	for _, hit := range v.Hits {
		t, err := h.repo.GetTask(hit.Task.TaskID)
		if err != nil || t == nil {
			firstErr = err
			break
		}
		mems, _ := h.repo.ListMembers(t.TaskID)
		ok, _, label := canSplitPlanReason(c, t, mems)
		if !ok {
			if label == "" {
				label = "이 계획을 나눌 권한이 없습니다"
			}
			firstErr = overlapActionErr(c, label)
			break
		}
		names := []string{hit.Assignee}
		if len(v.Names) > 0 {
			names = v.Names
		}
		day := strings.TrimSpace(hit.Date)
		if day == "" {
			day = v.Date
		}
		var err2 error
		for i := 0; i < 5; i++ {
			err2 = h.repo.ApplyInterruptSplit(t, newID, day, v.Start, v.End, v.Travel, names)
			if err2 == nil || !strings.Contains(err2.Error(), "SQLITE_BUSY") {
				break
			}
		}
		if err2 != nil {
			if splitOK[t.TaskID] && strings.Contains(err2.Error(), "끼워 넣을 겹친 구간이 없습니다") {
				continue
			}
			firstErr = overlapActionErr(c, err2.Error())
			break
		}
		splitOK[t.TaskID] = true
		applied = append(applied, t.TaskID)
	}
	if firstErr != nil {
		if rollbackNew {
			_ = h.repo.DeleteTaskLogged(newID, "끼워넣기 실패 롤백")
		}
		for _, id := range applied {
			_ = h.repo.MergeInterruptSegments(id, newID)
		}
		return firstErr
	}
	return nil
}

func (h *WorkboardHandler) MergeInterrupt(c echo.Context) error {
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	interrupt := strings.TrimSpace(c.FormValue("interrupt_task_id"))
	if interrupt == "" {
		return overlapActionErr(c, "합칠 끼워넣은 업무가 없습니다")
	}
	if err := h.repo.MergeInterruptSegments(id, interrupt); err != nil {
		return err
	}
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		back = "/workboard/board"
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=merge")
}

func (h *WorkboardHandler) ConfirmReturn(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("departure_id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "복귀 대상을 확인하세요")
	}
	st := strings.TrimSpace(c.FormValue("return_status"))
	if err := h.repo.SetDepartureReturn(id, st); err != nil {
		return err
	}
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		back = "/workboard/board"
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=return")
}

func (h *WorkboardHandler) saveWorkItemsFromForm(c echo.Context, taskID string) error {
	if h == nil || h.repo == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	if _, ok := c.Request().PostForm["item_title"]; !ok {
		return nil
	}
	allowed := itemAssigneeAllowed(c, h, taskID)
	items, excludedIDs, err := parseWorkItemsForm(c, allowed)
	if err != nil {
		return err
	}
	old, _ := h.repo.ListItems(taskID)
	oldByID := map[string]model.WorkTaskItem{}
	for _, o := range old {
		oldByID[o.ItemID] = o
	}
	for _, id := range excludedIDs {
		if o, ok := oldByID[id]; ok && itemHistoryLocked(o) {
			return fmt.Errorf("완료·연결 항목은 제외할 수 없습니다")
		}
	}
	return h.repo.ReplaceItems(taskID, items)
}

func itemHistoryLocked(it model.WorkTaskItem) bool {
	if model.NormalizeItemStatus(it.Status) == model.ItemDone {
		return true
	}
	if strings.TrimSpace(it.ASID) != "" || it.ASLocked {
		return true
	}
	return len(it.AssetIDs) > 0
}

func itemAssigneeAllowed(c echo.Context, h *WorkboardHandler, taskID string) map[string]string {
	out := map[string]string{}
	if t, _ := h.repo.GetTask(taskID); t != nil {
		if n := strings.TrimSpace(t.Assignee); n != "" {
			out[n] = strings.TrimSpace(t.AssigneeUserID)
		}
	}
	if mems, err := h.repo.ListMembers(taskID); err == nil {
		for _, m := range mems {
			if n := strings.TrimSpace(m.Assignee); n != "" {
				out[n] = strings.TrimSpace(m.UserID)
			}
		}
	}
	for _, n := range occupancyNames(c.FormValue("assignee"), parseSupportMembers(c, c.FormValue("assignee"))) {
		n = strings.TrimSpace(n)
		if n != "" {
			if _, ok := out[n]; !ok {
				out[n] = ""
			}
		}
	}
	return out
}

func parseWorkItemsForm(c echo.Context, allowed map[string]string) ([]model.WorkTaskItem, []string, error) {
	_ = c.Request().ParseForm()
	titles := c.Request().PostForm["item_title"]
	statuses := c.Request().PostForm["item_status"]
	notes := c.Request().PostForm["item_note"]
	ids := c.Request().PostForm["item_id"]
	kinds := c.Request().PostForm["item_kind"]
	products := c.Request().PostForm["item_product"]
	models := c.Request().PostForm["item_model"]
	qtys := c.Request().PostForm["item_qty"]
	asIDs := c.Request().PostForm["item_as_id"]
	assigns := c.Request().PostForm["item_assignees"]
	tplIDs := c.Request().PostForm["item_template_id"]
	units := c.Request().PostForm["item_unit"]
	keys := c.Request().PostForm["item_client_key"]
	setMains := c.Request().PostForm["item_set_main_key"]
	included := c.Request().PostForm["item_included"]
	var out []model.WorkTaskItem
	var excluded []string
	for i := range titles {
		title := strings.TrimSpace(titles[i])
		product := ""
		if i < len(products) {
			product = strings.TrimSpace(products[i])
		}
		if title == "" {
			title = product
		}
		if title == "" && product == "" {
			continue
		}
		inc := "1"
		if i < len(included) && strings.TrimSpace(included[i]) != "" {
			inc = strings.TrimSpace(included[i])
		}
		itemID := ""
		if i < len(ids) {
			itemID = strings.TrimSpace(ids[i])
		}
		if inc == "0" || inc == "false" {
			if itemID != "" {
				excluded = append(excluded, itemID)
			}
			st := ""
			if i < len(statuses) {
				st = model.NormalizeItemStatus(statuses[i])
			}
			as := ""
			if i < len(asIDs) {
				as = strings.TrimSpace(asIDs[i])
			}
			if st == model.ItemDone || as != "" {
				return nil, nil, fmt.Errorf("완료·연결 항목은 제외할 수 없습니다")
			}
			continue
		}
		it := model.WorkTaskItem{Title: title, ProductName: product, Status: model.ItemTodo, Qty: 1, Kind: model.ItemKindEtc, SetIncluded: true, ItemID: itemID}
		if i < len(statuses) {
			it.Status = model.NormalizeItemStatus(statuses[i])
		}
		if i < len(notes) {
			it.Note = strings.TrimSpace(notes[i])
		}
		if i < len(kinds) {
			raw := strings.TrimSpace(kinds[i])
			if raw != "" && raw != model.ItemKindInstall && raw != model.ItemKindWork && raw != model.ItemKindAS && raw != model.ItemKindEtc {
				return nil, nil, fmt.Errorf("구분 값이 올바르지 않습니다")
			}
			it.Kind = model.NormalizeItemKind(raw)
		}
		if i < len(models) {
			it.ModelName = strings.TrimSpace(models[i])
		}
		if i < len(qtys) {
			q := strings.TrimSpace(qtys[i])
			if q != "" {
				n, err := strconv.Atoi(q)
				if err != nil || n < 1 {
					return nil, nil, fmt.Errorf("수량은 1 이상이어야 합니다")
				}
				it.Qty = n
			}
		}
		if i < len(asIDs) {
			it.ASID = strings.TrimSpace(asIDs[i])
		}
		if product == "" {
			it.ProductName = title
		} else if title == "" {
			it.Title = product
		}
		if i < len(assigns) {
			for _, name := range strings.Split(assigns[i], ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				if allowed != nil {
					if _, ok := allowed[name]; !ok {
						return nil, nil, fmt.Errorf("항목 담당자는 상위 참여자 안에서만 지정할 수 있습니다")
					}
				}
				uid := ""
				if allowed != nil {
					uid = allowed[name]
				}
				it.Assignees = append(it.Assignees, model.WorkTaskItemAssignee{Name: name, UserID: uid})
			}
		}
		if i < len(tplIDs) {
			it.TemplateID = strings.TrimSpace(tplIDs[i])
		}
		if i < len(units) {
			u := strings.TrimSpace(units[i])
			if u != "" && !model.ValidItemUnitCode(u, nil) {
				return nil, nil, fmt.Errorf("단위 값이 올바르지 않습니다")
			}
			it.Unit = u
		}
		if i < len(keys) {
			it.ClientKey = strings.TrimSpace(keys[i])
		}
		if i < len(setMains) {
			it.SetMainKey = strings.TrimSpace(setMains[i])
		}
		out = append(out, it)
	}
	return out, excluded, nil
}

func parseActualSegmentsForm(c echo.Context, t *model.WorkTask) []model.WorkTaskSegment {
	_ = c.Request().ParseForm()
	starts := c.Request().PostForm["seg_start"]
	ends := c.Request().PostForm["seg_end"]
	workers := c.Request().PostForm["seg_worker"]
	dates := c.Request().PostForm["seg_date"]
	var out []model.WorkTaskSegment
	for i := range starts {
		st := strings.TrimSpace(starts[i])
		if st == "" {
			continue
		}
		en := ""
		if i < len(ends) {
			en = strings.TrimSpace(ends[i])
		}
		day := strings.TrimSpace(t.WorkDate)
		if i < len(dates) && strings.TrimSpace(dates[i]) != "" {
			day = strings.TrimSpace(dates[i])
		}
		worker := ""
		if i < len(workers) {
			worker = strings.TrimSpace(workers[i])
		}
		out = append(out, model.WorkTaskSegment{
			Kind: model.SegKindActual, Scope: model.SegScopeCommon, Seq: i + 1,
			WorkDate: day, StartTime: clockFromLocal(st), EndTime: clockFromLocal(en),
			WorkerName: worker, DurationMin: model.DurationFromTimes(clockFromLocal(st), clockFromLocal(en)),
		})
	}
	return out
}

func clockFromLocal(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "T", " ")
	if len(s) >= 16 {
		return s[11:16]
	}
	if len(s) >= 5 && strings.Contains(s, ":") {
		if i := strings.LastIndex(s, " "); i >= 0 && len(s) >= i+6 {
			return s[i+1 : i+6]
		}
		return s[:5]
	}
	return s
}

func datetimeLocalFromSeg(date, hhmm string) string {
	date = strings.TrimSpace(date)
	hhmm = strings.TrimSpace(hhmm)
	if date == "" || hhmm == "" {
		return ""
	}
	if len(hhmm) >= 5 {
		hhmm = hhmm[:5]
	}
	return date + "T" + hhmm
}

func actualOverrunsNextPlan(actual []model.WorkTaskSegment, plan []model.WorkTaskSegment) bool {
	if len(actual) == 0 || len(plan) < 2 {
		return false
	}
	lastAct := actual[len(actual)-1]
	ae := model.ParseHHMMMinutes(lastAct.EndTime)
	if ae < 0 {
		return false
	}
	for i, p := range plan {
		if i == 0 {
			continue
		}
		ps := model.ParseHHMMMinutes(p.StartTime)
		if ps >= 0 && ae > ps {
			return true
		}
	}
	return false
}

func (h *WorkboardHandler) offerMergeAfterDelete(c echo.Context, deletedID string) bool {
	ids, _ := h.repo.TasksInterruptedBy(deletedID)
	if len(ids) == 0 {
		return false
	}
	if strings.TrimSpace(c.FormValue("merge_interrupt")) == "1" {
		for _, id := range ids {
			_ = h.repo.MergeInterruptSegments(id, deletedID)
		}
		return false
	}
	if strings.TrimSpace(c.FormValue("merge_interrupt")) == "0" {
		return false
	}
	msg := "끼워 넣은 업무를 없앱니다. 나뉜 계획을 다시 합칠까요?"
	if wantsJSONError(c) {
		_ = c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "merge_prompt": true, "message": msg, "host_task_ids": ids, "interrupt_task_id": deletedID,
		})
		return true
	}
	return false
}

func boardItemIsExec(it boardItem) bool {
	return strings.Contains(it.DedupKey, "|act")
}

func boardCalItems(placed, overdue, exec []boardItem, showPlan, showAct bool, filterHit func(boardItem) bool) []boardItem {
	var calItems []boardItem
	if showPlan {
		for _, it := range placed {
			it.PlaceDate = model.TaskPlanDate(it.Task)
			calItems = append(calItems, it)
		}
		for _, it := range overdue {
			it.PlaceDate = model.TaskPlanDate(it.Task)
			calItems = append(calItems, it)
		}
	}
	if showAct {
		same := map[string]bool{}
		if showPlan {
			for _, it := range calItems {
				d := it.PlaceDate
				if d == "" {
					d = model.TaskPlanDate(it.Task)
				}
				if d != "" && d == model.TaskExecDate(it.Task) {
					same[taskDedupKey(it.Task)] = true
				}
			}
		}
		for _, it := range exec {
			if filterHit != nil && !filterHit(it) {
				continue
			}
			ed := strings.TrimSpace(it.PlaceDate)
			if ed == "" {
				ed = model.TaskExecDate(it.Task)
			}
			if showPlan && same[taskDedupKey(it.Task)] {
				continue
			}
			it.PlaceDate = ed
			calItems = append(calItems, it)
		}
	}
	return calItems
}

func boardListedItems(view, listRange string, base time.Time, calItems []boardItem, showPlan bool) []boardItem {
	if view != boardViewList {
		return calItems
	}
	if listRange != boardRangeDay {
		return append([]boardItem{}, calItems...)
	}
	day := base.Format(dateLayout)
	var items []boardItem
	for _, it := range calItems {
		if it.Overdue && showPlan {
			items = append(items, it)
			continue
		}
		if it.PlaceDate == day {
			items = append(items, it)
			continue
		}
		if showPlan && !boardItemIsExec(it) && model.TaskSpansDate(it.Task.WorkDate, it.Task.DueDate, day) {
			items = append(items, it)
		}
	}
	return items
}

func uniqueTaskCount(items []boardItem) int {
	seen := map[string]bool{}
	n := 0
	for _, it := range items {
		id := strings.TrimSpace(it.Task.TaskID)
		if id == "" || seen[id] || it.Fragment {
			continue
		}
		seen[id] = true
		n++
	}
	return n
}
