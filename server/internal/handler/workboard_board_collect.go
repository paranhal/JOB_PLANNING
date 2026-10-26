package handler

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

type boardEntries struct {
	Placed         []boardItem
	Unplaced       []boardItem
	Overdue        []boardItem
	Exec           []boardItem
	PlanN          int
	ActN           int
	OtherUnplacedN int
}

func (e boardEntries) keySet() map[string]bool {
	out := map[string]bool{}
	for _, it := range e.Placed {
		out[it.DedupKey] = true
	}
	for _, it := range e.Unplaced {
		out[it.DedupKey] = true
	}
	for _, it := range e.Overdue {
		out[it.DedupKey] = true
	}
	return out
}

func boardKeepQuery(c echo.Context) url.Values {
	v := url.Values{}
	if m := strings.TrimSpace(c.QueryParam("mine")); m != "" {
		v.Set("mine", m)
	}
	if s := strings.TrimSpace(c.QueryParam("show")); s != "" {
		plan, act := model.ParseScheduleShow(s)
		v.Set("show", model.EncodeScheduleShow(plan, act))
	}
	if a := normalizeAssigneeFilter(c.QueryParam("assignee")); a != "" {
		v.Set("assignee", a)
	}
	if q := strings.TrimSpace(c.QueryParam("q")); q != "" {
		v.Set("q", q)
	}
	if auto := strings.TrimSpace(c.QueryParam("auto")); auto != "" {
		v.Set("auto", auto)
	}
	if p := strings.TrimSpace(c.QueryParam("project")); p != "" {
		v.Set("project", p)
	}
	if g := strings.TrimSpace(c.QueryParam("work_group_id")); g != "" {
		v.Set("work_group_id", g)
	}
	return v
}

func boardURL(view, date string, keep url.Values) string {
	u := "/workboard/board?view=" + url.QueryEscape(view) + "&date=" + url.QueryEscape(date)
	if enc := keep.Encode(); enc != "" {
		u += "&" + enc
	}
	return u
}

func taskDedupKey(t model.WorkTask) string {
	if st, id := strings.TrimSpace(t.SourceType), strings.TrimSpace(t.SourceID); st != "" && id != "" {
		return st + ":" + id
	}
	if id := strings.TrimSpace(t.TaskID); id != "" {
		return "task:" + id
	}
	return "task:" + t.Title
}

func cardDedupKey(card model.WBCard) string {
	if card.Kind == "wi" || card.Category == model.WorkPrefixConfirm || card.Category == model.WorkPrefixSupport {
		cat := strings.TrimSpace(card.Category)
		if cat == "" {
			cat = "wi"
		}
		return cat + ":" + strings.TrimSpace(card.RefID)
	}
	if card.Kind == "task" || strings.TrimSpace(card.TaskID) != "" {
		id := strings.TrimSpace(card.TaskID)
		if id == "" {
			id = strings.TrimSpace(card.RefID)
		}
		return "task:" + id
	}
	kind := strings.TrimSpace(card.Kind)
	if kind == "" {
		kind = strings.TrimSpace(card.Category)
	}
	return kind + ":" + strings.TrimSpace(card.RefID)
}

func formatMonthDay(ymd string) string {
	t, err := time.ParseInLocation(dateLayout, ymd, time.Local)
	if err != nil {
		return ymd
	}
	return fmt.Sprintf("%02d. %02d", int(t.Month()), t.Day())
}

func cardDueTag(planned, today string) (tag string, late bool) {
	planned = strings.TrimSpace(planned)
	if planned == "" {
		return "날짜 미정", false
	}
	if today != "" && planned < today {
		return "예정일 경과 " + formatMonthDay(planned), true
	}
	return "예정일 " + formatMonthDay(planned), false
}

func (h *WorkboardHandler) collectBoardEntries(c echo.Context, from, to string, scope boardScope) (boardEntries, error) {
	return h.collectBoardEntriesAuto(c, from, to, scope, c.QueryParam("auto") != "0")
}

func (h *WorkboardHandler) collectBoardEntriesAuto(c echo.Context, from, to string, scope boardScope, doAuto bool) (boardEntries, error) {
	var out boardEntries
	org := currentOrg(c)
	today := time.Now().Format(dateLayout)
	projectFilter := strings.TrimSpace(c.QueryParam("project"))
	groupFilter := strings.TrimSpace(c.QueryParam("work_group_id"))

	syncFrom, syncTo := registerSyncWindow(from, to)
	h.syncMaintenanceTasks(syncFrom, syncTo, org)

	if doAuto {
		autoFrom := from
		if registerPastDayLockEnabled && autoFrom < today {
			autoFrom = today
		}
		if autoFrom <= to {
			if _, err := h.autoPlacePlanned(autoFrom, to, org); err != nil {
				return out, err
			}
		}
	}

	placed, err := h.repo.ListTasksBetweenForOrg(from, to, org)
	if err != nil {
		return out, err
	}
	placed = ensureTimelineDisplayTimes(placed)

	asIDs, mntIDs := collectASMntIDs(placed)
	asStatus, _ := h.repo.ASStatusesByIDs(asIDs)
	mntDone, _ := h.repo.MaintenanceCompletedByIDs(mntIDs)
	var visible []model.WorkTask
	for _, t := range placed {
		ok := false
		t, ok = applyWorkboardSourceStatus(t, asStatus, mntDone)
		if !ok || t.Status == model.WBTaskCancelled {
			continue
		}
		visible = append(visible, t)
	}
	visible = filterTasksByProject(visible, projectFilter)
	visible = filterTasksByWorkGroup(visible, groupFilter)

	ids := taskIDsOf(visible)
	memberMap := map[string][]model.WorkTaskMember{}
	if len(ids) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}
	allVisible := visible
	visible = filterTasksByParticipants(visible, memberMap, scope.AssigneeFilter)
	visible = applyViewGroupToTasks(c, allVisible, visible, scope)

	siteLines := map[string]string{}
	ids = taskIDsOf(visible)
	if len(ids) > 0 {
		if m, err := h.repo.TaskSiteLinesByTaskIDs(ids); err == nil {
			siteLines = m
		}
	}
	var custIDs []string
	for i := range visible {
		if line := siteLines[visible[i].TaskID]; line != "" {
			visible[i].SiteLine = line
		}
		if cid := strings.TrimSpace(visible[i].CustomerID); cid != "" {
			custIDs = append(custIDs, cid)
		}
	}
	addrMap := map[string]string{}
	if h.customerRepo != nil && len(custIDs) > 0 {
		if m, err := h.customerRepo.AddressesByIDs(custIDs); err == nil {
			addrMap = m
		}
	}

	seen := map[string]bool{}
	write := boardCanWrite(c)
	for _, t := range visible {
		it := buildBoardItem(t, memberMap[t.TaskID], addrMap[strings.TrimSpace(t.CustomerID)], today)
		it.DedupKey = taskDedupKey(t)
		it.Href = t.DetailHref()
		if it.Status.Key == "late" {
			day := strings.TrimSpace(t.WorkDate)
			if day == "" {
				day = strings.TrimSpace(t.DueDate)
			}
			if day != "" && day < today {
				d := model.DaysBetweenDates(day, today)
				if d < 1 {
					d = 1
				}
				it.Overdue = true
				it.OverdueDays = d
			}
		}
		if seen[it.DedupKey] {
			continue
		}
		seen[it.DedupKey] = true
		out.Placed = append(out.Placed, it)
	}
	out.PlanN = len(out.Placed)

	execRaw, err := h.repo.ListTasksCompleteBetweenForOrg(from, to, org)
	if err != nil {
		return out, err
	}
	execRaw = ensureTimelineDisplayTimes(execRaw)
	asIDs2, mntIDs2 := collectASMntIDs(execRaw)
	asStatus2, _ := h.repo.ASStatusesByIDs(asIDs2)
	mntDone2, _ := h.repo.MaintenanceCompletedByIDs(mntIDs2)
	var execVis []model.WorkTask
	for _, t := range execRaw {
		ok := false
		t, ok = applyWorkboardSourceStatus(t, asStatus2, mntDone2)
		if !ok || t.Status == model.WBTaskCancelled {
			continue
		}
		execVis = append(execVis, t)
	}
	execVis = filterTasksByProject(execVis, projectFilter)
	execVis = filterTasksByWorkGroup(execVis, groupFilter)
	execIDs := taskIDsOf(execVis)
	execMem := map[string][]model.WorkTaskMember{}
	if len(execIDs) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(execIDs); err == nil {
			execMem = m
		}
	}
	allExec := execVis
	execVis = filterTasksByParticipants(execVis, execMem, scope.AssigneeFilter)
	execVis = applyViewGroupToTasks(c, allExec, execVis, scope)
	execSeen := map[string]bool{}
	for _, t := range execVis {
		it := buildBoardItem(t, execMem[t.TaskID], addrMap[strings.TrimSpace(t.CustomerID)], today)
		it.DedupKey = taskDedupKey(t) + "|act"
		it.Href = t.DetailHref()
		it.PlaceDate = model.TaskExecDate(t)
		k := it.DedupKey
		if execSeen[k] {
			continue
		}
		execSeen[k] = true
		out.Exec = append(out.Exec, it)
	}
	out.ActN = len(out.Exec)

	asCards, mntCards, adminCards, salesCards, unassignedCards, err := h.registerPalette(org, strings.TrimSpace(c.QueryParam("palette")) == "all")
	if err != nil {
		return out, err
	}
	workCards, err := h.paletteWorkCards(org)
	if err != nil {
		return out, err
	}
	allCards := []model.WBCard{}
	allCards = append(allCards, asCards...)
	allCards = append(allCards, mntCards...)
	allCards = append(allCards, adminCards...)
	allCards = append(allCards, salesCards...)
	allCards = append(allCards, workCards...)
	allCards = append(allCards, unassignedCards...)
	allCards = filterCardsByProject(allCards, projectFilter)

	undatedTasks, err := h.repo.ListUndatedASTasksForOrg(org)
	if err != nil {
		return out, err
	}
	undatedCards := undatedASCards(undatedTasks)

	var rawUnplaced []model.WBCard
	rawUnplaced = append(rawUnplaced, allCards...)
	rawUnplaced = append(rawUnplaced, undatedCards...)

	filtered := filterCardsByAssignee(rawUnplaced, scope.AssigneeFilter)
	out.OtherUnplacedN = unplacedOtherCount(rawUnplaced, filtered)

	for _, card := range filtered {
		if card.Dimmed {
			continue
		}
		key := cardDedupKey(card)
		if key == ":" || seen[key] {
			continue
		}
		it := cardToUnplacedItem(c, card, today, write, scope)
		it.DedupKey = key
		seen[key] = true
		out.Unplaced = append(out.Unplaced, it)
	}

	overdue, err := h.boardOverdueItems(c, from, org, scope, today, write, seen)
	if err != nil {
		return out, err
	}
	out.Overdue = overdue
	return out, nil
}

func unplacedOtherCount(all, mine []model.WBCard) int {
	if len(all) <= len(mine) {
		return 0
	}
	hit := map[string]bool{}
	for _, c := range mine {
		hit[cardDedupKey(c)] = true
	}
	n := 0
	for _, c := range all {
		if c.Dimmed {
			continue
		}
		k := cardDedupKey(c)
		if !hit[k] {
			n++
			hit[k] = true
		}
	}
	return n
}

func cardToUnplacedItem(c echo.Context, card model.WBCard, today string, write bool, scope boardScope) boardItem {
	t := model.WBCardAsWaitingTask(card)
	it := buildBoardItem(t, nil, "", today)
	it.Unplaced = true
	it.DateLabel = "날짜 미정"
	href := strings.TrimSpace(card.SourceHref)
	if href == "" {
		href = card.DetailHref()
	}
	if href == "" {
		href = t.DetailHref()
	}
	it.Href = href
	it.PlaceKind = card.Kind
	it.PlaceRef = card.RefID
	it.PlaceAssignee = strings.TrimSpace(card.Assignee)
	it.PlaceDur = card.DurationMin
	if it.PlaceDur <= 0 {
		it.PlaceDur = 30
	}
	it.PlaceDue = strings.TrimSpace(card.PlannedDay)
	it.DueTag, it.DueLate = cardDueTag(it.PlaceDue, today)
	it.CustomerLine = strings.TrimSpace(card.SubTitle)
	if it.CustomerLine == "" {
		it.CustomerLine = strings.TrimSpace(t.OrgName)
	}
	it.CanPlace = write && canPlaceCard(c, card)
	if !scope.ScopeAll {
		it.AssigneeName = ""
		it.AssigneeExtra = ""
	}
	return it
}

func canPlaceCard(c echo.Context, card model.WBCard) bool {
	if strings.TrimSpace(card.Assignee) == "" {
		return isAdminRole(c) || isOfficeRole(c)
	}
	return canEditTask(c, card.Assignee, "")
}

func (h *WorkboardHandler) boardOverdueItems(c echo.Context, from, org string, scope boardScope, today string, write bool, seen map[string]bool) ([]boardItem, error) {
	if h.workBoard == nil {
		return nil, nil
	}
	var mineKeys []string
	if strings.TrimSpace(scope.AssigneeFilter) != "" {
		mineKeys = []string{scope.AssigneeFilter}
	}
	delayed, err := h.workBoard.ListBucketOnForOrg(model.WorkBucketDelayed, from, "", mineKeys, 0, org)
	if err != nil {
		return nil, err
	}
	progress, err := h.workBoard.ListBucketOnForOrg(model.WorkBucketInProgress, from, "", mineKeys, 0, org)
	if err != nil {
		return nil, err
	}
	stampWorkListOverdue(delayed, today)
	stampWorkListOverdue(progress, today)
	var merged []model.WorkListItem
	uSeen := map[string]bool{}
	for _, it := range delayed {
		k := model.WorkListItemKey(it)
		if uSeen[k] {
			continue
		}
		uSeen[k] = true
		merged = append(merged, it)
	}
	for _, it := range progress {
		d := model.WorkItemScheduledDate(it)
		if d == "" || d >= from {
			continue
		}
		k := model.WorkListItemKey(it)
		if uSeen[k] {
			continue
		}
		uSeen[k] = true
		merged = append(merged, it)
	}
	var out []boardItem
	for _, it := range merged {
		k := model.WorkListItemKey(it)
		if seen[k] {
			continue
		}
		if it.MappedStatus == model.WBTaskComplete || it.Status == model.WBTaskComplete {
			continue
		}
		bi := workListToBoardItem(it, today)
		bi.Overdue = true
		if bi.OverdueDays < 1 {
			bi.OverdueDays = 1
		}
		bi.CanPlace = write && canEditTask(c, it.Assignee, "")
		seen[k] = true
		out = append(out, bi)
	}
	return out, nil
}

func workListToBoardItem(it model.WorkListItem, today string) boardItem {
	day := model.WorkItemScheduledDate(it)
	t := model.WorkTask{
		Title:       it.Title,
		Assignee:    it.Assignee,
		Status:      it.MappedStatus,
		WorkDate:    day,
		DueDate:     it.DueDate,
		OrgID:       it.OrgID,
		OrgName:     it.OrgName,
		CustomerID:  it.CustomerID,
		ProjectID:   it.ProjectID,
		SourceID:    it.RefID,
		WorkGroupID: it.WorkGroupID,
	}
	if t.Status == "" {
		t.Status = it.Status
	}
	switch it.Prefix {
	case model.WorkPrefixAS:
		t.SourceType = model.WBSourceAS
		t.WorkType = model.WBWorkAS
	case model.WorkPrefixMaintenance:
		t.SourceType = model.WBSourceMaintenance
		t.WorkType = model.WBWorkMaintenance
	case model.WorkPrefixSales:
		t.SourceType = model.WBSourceSalesActivity
		t.WorkType = model.WBWorkSales
	case model.WorkPrefixProjectWork:
		t.WorkType = model.WBWorkProjectWork
		t.TaskID = it.RefID
	case model.WorkPrefixGeneral:
		t.WorkType = model.WBWorkAdmin
		t.TaskID = it.RefID
	default:
		t.WorkType = model.WBWorkAdmin
		t.TaskID = it.RefID
	}
	bi := buildBoardItem(t, nil, "", today)
	href := strings.TrimSpace(it.Href)
	if href == "" {
		href = t.DetailHref()
	}
	bi.Href = href
	bi.DedupKey = model.WorkListItemKey(it)
	bi.OverdueDays = it.DaysOverdue
	bi.PlaceKind = "task"
	bi.PlaceRef = it.RefID
	if it.Prefix == model.WorkPrefixAS {
		bi.PlaceKind = model.WBSourceAS
	}
	if it.Prefix == model.WorkPrefixMaintenance {
		bi.PlaceKind = model.WBSourceMaintenance
	}
	if it.Prefix == model.WorkPrefixConfirm || it.Prefix == model.WorkPrefixSupport {
		bi.Href = "/as/work/" + it.RefID + "/action"
	}
	if it.Prefix == model.WorkPrefixAS && (bi.Href == "" || bi.Href == "#") {
		bi.Href = asOpenHref(it.RefID)
	}
	bi.DeleteKey, bi.DeleteDisabledReason = deleteKeyForWorkList(it, t)
	return bi
}

func sortListBoardItems(items []boardItem) {
	var late, dated, undated []boardItem
	for _, it := range items {
		if it.Overdue {
			late = append(late, it)
			continue
		}
		d := strings.TrimSpace(it.Task.WorkDate)
		if d == "" {
			d = strings.TrimSpace(it.Task.DueDate)
		}
		if d == "" || it.Unplaced {
			undated = append(undated, it)
			continue
		}
		dated = append(dated, it)
	}
	sort.SliceStable(late, func(i, j int) bool {
		return strings.TrimSpace(late[i].Task.WorkDate) < strings.TrimSpace(late[j].Task.WorkDate)
	})
	sort.SliceStable(dated, func(i, j int) bool {
		di, dj := strings.TrimSpace(dated[i].Task.WorkDate), strings.TrimSpace(dated[j].Task.WorkDate)
		if di != dj {
			return di < dj
		}
		return dated[i].Task.Title < dated[j].Task.Title
	})
	ordered := make([]boardItem, 0, len(items))
	ordered = append(ordered, late...)
	ordered = append(ordered, dated...)
	ordered = append(ordered, undated...)
	copy(items, ordered)
}

func boardMineEmptyAllCount(h *WorkboardHandler, c echo.Context, from, to string) int {
	if h == nil {
		return 0
	}
	allScope := resolveBoardScope(c)
	allScope.ScopeAll = true
	allScope.AssigneeFilter = ""
	allScope.MineParam = "0"
	all, err := h.collectBoardEntriesAuto(c, from, to, allScope, false)
	if err != nil {
		return 0
	}
	return len(all.Placed) + len(all.Unplaced) + len(all.Overdue)
}
