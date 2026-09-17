package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// UnplannedList GET /plan/unplanned — §8 미계획 업무함
func (h *WorkHandler) UnplannedList(c echo.Context) error {
	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)
	kind := strings.TrimSpace(c.QueryParam("kind"))

	mineParam := c.QueryParam("mine")
	mineUID, mineKeys := "", []string(nil)
	scopeAll := false
	if role == model.RoleTech {
		if mineParam == "0" {
			scopeAll = true
		} else {
			mineUID, mineKeys = uid, keys
		}
	} else if mineParam == "1" {
		mineUID, mineKeys = uid, keys
	} else {
		scopeAll = true
	}

	items, counts, err := h.repo.ListUnplanned(mineUID, mineKeys, kind)
	if err != nil {
		return err
	}
	sortKey, dir := parseOptionalSort(c.QueryParam("sort"), c.QueryParam("dir"), "assignee,prefix,task_id,customer,due_date")
	if sortKey != "" {
		sortUnplannedByColumn(items, sortKey, dir)
	}

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}

	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	kanban := model.FillUnplannedKanban(items)
	mineOn := mineUID != ""
	showAssignee := role == model.RoleAdmin || role == model.RoleOffice || scopeAll
	canWrite := canWriteUnplanned(c)
	u := func(k, d string) string {
		return planUnplannedURLFull(mineOn, role, k, d, sortKey, dir)
	}
	filter := unplannedFilterQuery(mineOn, role, kind, display)
	hrefs := sortLinkHrefs("/plan/unplanned", filter, []string{"assignee", "prefix", "task_id", "customer", "due_date"}, sortKey, dir)
	return c.Render(http.StatusOK, "plan/unplanned.html", map[string]interface{}{
		"Title":          "미계획 업무함",
		"Active":         NavPlanUnplanned,
		"Items":          items,
		"Total":          len(items),
		"Counts":         counts,
		"Kind":           kind,
		"ShowAssignee":   showAssignee,
		"Role":           role,
		"Mine":           mineOn,
		"ScopeAll":       scopeAll,
		"ScopeNote":      unplannedScopeNote(role, scopeAll),
		"CanWrite":       canWrite,
		"Users":          users,
		"Err":            c.QueryParam("err"),
		"Ok":             c.QueryParam("ok"),
		"MineQ":          unplannedMineQuery(role, mineOn),
		"Display":        display,
		"KanbanColumns":  kanban.Columns,
		"KanbanTotal":    kanban.Total,
		"KanbanDrag":     false,
		"KanbanDrop":     "",
		"KanbanHint":     "열 = §8.1 유형. 한 건이 여러 유형이면 표 위에서 먼저 맞는 열 하나. 유형은 파생값이라 드래그하지 않습니다.",
		"ListHref":       u(kind, "list"),
		"KanbanHref":     u(kind, "kanban"),
		"MineToggle":     planUnplannedURLFull(!mineOn, role, kind, display, sortKey, dir),
		"KindAllHref":    u("", display),
		"KindNoDate":     u(model.UnplannedNoDate, display),
		"KindUnassigned": u(model.UnplannedUnassigned, display),
		"KindSales":      u(model.UnplannedSalesFollow, display),
		"KindUnsigned":   u(model.UnplannedSalesUnsigned, display),
		"KindDelayed":    u(model.UnplannedDelayed, display),
		"KindNext":       u(model.UnplannedNext, display),
		"KindReview":     u(model.UnplannedReview, display),
		"Sort":           sortKey,
		"Dir":            dir,
		"SortHref":       hrefs,
		"SortSelect":     sortSelectOptions(unplannedSortCols(showAssignee), hrefs, sortKey, dir),
	})
}

func sortUnplannedByColumn(items []model.UnplannedItem, sortKey, dir string) {
	if len(items) == 0 || sortKey == "" {
		return
	}
	desc := dir == "desc"
	val := func(it model.UnplannedItem) string {
		switch sortKey {
		case "assignee":
			return strings.ToLower(it.Assignee)
		case "prefix":
			return it.Prefix
		case "task_id":
			return strings.ToLower(it.RefNumber)
		case "customer":
			return strings.ToLower(it.OrgName + "\x00" + it.Title)
		default:
			if it.ScheduledDate != "" {
				return it.ScheduledDate
			}
			if it.DueDate != "" {
				return it.DueDate
			}
			return "9999-99-99"
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := val(items[i]), val(items[j])
		if a != b {
			if desc {
				return a > b
			}
			return a < b
		}
		return false
	})
}

func unplannedScopeNote(role string, scopeAll bool) string {
	if role == model.RoleTech && !scopeAll {
		return "내 배정 업무 기준 · 담당자 미배정·정기점검 배정안됨은 팀 공통"
	}
	return "전체 업무 기준"
}

func unplannedMineQuery(role string, mine bool) string {
	if role == model.RoleTech {
		if mine {
			return "mine=1"
		}
		return "mine=0"
	}
	if mine {
		return "mine=1"
	}
	return ""
}

func canWriteUnplanned(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	role := currentRole(c)
	return role == model.RoleAdmin || role == model.RoleOffice || role == model.RoleTech
}

func unplannedBack(c echo.Context) string {
	kind := strings.TrimSpace(c.FormValue("kind"))
	if kind == "" {
		kind = strings.TrimSpace(c.QueryParam("kind"))
	}
	role := currentRole(c)
	mine := c.FormValue("mine")
	if mine == "" {
		mine = c.QueryParam("mine")
	}
	disp := strings.TrimSpace(c.FormValue("display"))
	if disp == "" {
		disp = c.QueryParam("display")
	}
	return planUnplannedURLDisp(mine == "1" || (role == model.RoleTech && mine != "0"), role, kind, disp)
}

// UnplannedAssign POST /plan/unplanned/assign — 단건·일괄 날짜 배정
func (h *WorkHandler) UnplannedAssign(c echo.Context) error {
	if !canWriteUnplanned(c) {
		return echo.ErrForbidden
	}
	date, err := model.ParseAppDate(c.FormValue("visit_date"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=date_year"))
	}
	if date == "" {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=date"))
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	assigneeUID := strings.TrimSpace(c.FormValue("assignee_user_id"))
	if assigneeUID == "" && h.userRepo != nil && assignee != "" {
		if users, err := h.userRepo.ListAssignable(); err == nil {
			for _, u := range users {
				if u.FullName == assignee || u.Username == assignee {
					assigneeUID = u.UserID
					if assignee == u.Username {
						assignee = u.FullName
					}
					break
				}
			}
		}
	}

	keys := c.Request().Form["item_key"]
	if len(keys) == 0 {
		if params, err := c.FormParams(); err == nil {
			keys = params["item_key"]
		}
	}
	if len(keys) == 0 {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=select"))
	}

	n := 0
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := h.repo.AssignUnplannedDate(key, date, assignee, assigneeUID); err != nil {
			continue
		}
		n++
		h.recordUnplannedAssignNotice(c, key, date, assignee)
	}
	back := unplannedBack(c)
	if n == 0 {
		return c.Redirect(http.StatusSeeOther, back+qjoin(back, "err=assign"))
	}
	return c.Redirect(http.StatusSeeOther, back+qjoin(back, "ok=assign"))
}

// UnplannedNoDate POST /plan/unplanned/no-date — 미정+사유
func (h *WorkHandler) UnplannedNoDate(c echo.Context) error {
	if !canWriteUnplanned(c) {
		return echo.ErrForbidden
	}
	if h.asRepo == nil {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("item_key"))
	if !strings.HasPrefix(key, "as:") {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=nodate"))
	}
	reason := model.FormatNoDateReason(c.FormValue("no_date_reason"), c.FormValue("no_date_detail"))
	if strings.TrimSpace(reason) == "" {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=reason"))
	}
	asID := strings.TrimPrefix(key, "as:")
	if err := h.asRepo.SetScheduleNoDate(asID, reason); err != nil {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=nodate"))
	}
	back := unplannedBack(c)
	return c.Redirect(http.StatusSeeOther, back+qjoin(back, "ok=nodate"))
}

func (h *WorkHandler) recordUnplannedAssignNotice(c echo.Context, key, date, assignee string) {
	if h == nil || h.notices == nil {
		return
	}
	switch {
	case strings.HasPrefix(key, "as:"):
		asID := strings.TrimPrefix(key, "as:")
		h.syncASPlannedFromUnplanned(asID)
		if h.asRepo != nil {
			if as, err := h.asRepo.GetByID(asID); err == nil && as != nil {
				h.notices.Record(c, model.AssignNoticeSourceAS, as.ASID, as.AssignedTo, as.AssignedUserID, "", "")
			}
		}
	case strings.HasPrefix(key, "mnt:"):
		vid := strings.TrimPrefix(key, "mnt:")
		who := assignee
		if h.mntRepo != nil {
			if v, err := h.mntRepo.GetVisit(vid); err == nil && v != nil {
				if who == "" {
					who = v.Assignee
				}
			}
		}
		h.notices.Record(c, model.AssignNoticeSourceMaintenance, vid, who, "", "", "")
	case strings.HasPrefix(key, "slot:"):
		if h.wbRepo != nil {
			_, _ = h.wbRepo.EnsureMaintenanceTasks()
		}
		id := strings.TrimPrefix(key, "slot:")
		parts := strings.SplitN(id, "|", 3)
		if len(parts) >= 2 && h.mntRepo != nil {
			product := ""
			if len(parts) == 3 {
				product = parts[2]
			}
			if got := h.mntRepo.FindVisitBySlot(parts[0], parts[1], product, date); got != nil {
				who := assignee
				if who == "" {
					who = got.Assignee
				}
				h.notices.Record(c, model.AssignNoticeSourceMaintenance, got.VisitID, who, "", "", "")
			}
		}
	case strings.HasPrefix(key, "task:"):
		h.notices.Record(c, model.AssignNoticeSourceTask, strings.TrimPrefix(key, "task:"), assignee, "", "", "")
	}
}

func (h *WorkHandler) syncASPlannedFromUnplanned(asID string) {
	if h.wbRepo == nil || h.asRepo == nil || strings.TrimSpace(asID) == "" {
		return
	}
	as, err := h.asRepo.GetByID(asID)
	if err != nil || as == nil {
		return
	}
	_ = h.wbRepo.SyncASDailyTask(as)
}

func qjoin(url, extra string) string {
	if strings.Contains(url, "?") {
		return "&" + extra
	}
	return "?" + extra
}
