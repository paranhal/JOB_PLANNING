package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type WorkHandler struct {
	repo     *repository.WorkBoardRepo
	asRepo   *repository.ASRepo
	mntRepo  *repository.MaintenanceRepo
	wbRepo   *repository.WBRepo
	userRepo *repository.UserRepo
}

func NewWorkHandler(
	repo *repository.WorkBoardRepo,
	asRepo *repository.ASRepo,
	mntRepo *repository.MaintenanceRepo,
	wbRepo *repository.WBRepo,
	userRepo *repository.UserRepo,
) *WorkHandler {
	return &WorkHandler{repo: repo, asRepo: asRepo, mntRepo: mntRepo, wbRepo: wbRepo, userRepo: userRepo}
}

func (h *WorkHandler) List(c echo.Context) error {
	role := currentRole(c)
	roleUnresolved := !model.IsKnownRole(role)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	view := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	sortKey, dir := model.NormalizeAdminWorkSort(c.QueryParam("sort"), c.QueryParam("dir"))
	assigneeFilter := strings.TrimSpace(c.QueryParam("assignee"))
	mineParam := c.QueryParam("mine")

	mineUID, mineKeys := "", []string(nil)
	scopeAll := true
	includeAllSales := false
	if roleUnresolved {
		scopeAll = true
	} else if role == model.RoleTech {
		if mineParam == "0" {
			scopeAll = true
		} else {
			scopeAll = false
			mineUID, mineKeys = uid, keys
		}
	} else if role == model.RoleSales {
		scopeAll = false
		mineUID, mineKeys = uid, keys
		includeAllSales = true
		if mineParam == "0" {
			scopeAll = true
			mineUID, mineKeys = "", nil
			includeAllSales = true
		}
	} else if assigneeFilter != "" && (role == model.RoleAdmin || role == model.RoleOffice) {
		scopeAll = false
		mineKeys = []string{assigneeFilter}
	} else if mineParam == "1" {
		scopeAll = false
		mineUID, mineKeys = uid, keys
	}

	today := time.Now().Format("2006-01-02")
	raw, err := h.repo.ListUnified(mineUID, mineKeys, includeAllSales, today)
	if err != nil {
		return err
	}

	var items []model.WorkListItem
	for _, it := range raw {
		if !it.ApplyKanbanMapping() {
			continue
		}
		items = append(items, it)
	}
	model.SortWorkListItems(items, sortKey, dir)
	kanban := model.FillWorkKanban(model.ExecKanbanColumnDefs(), items)

	var assignees []model.User
	if h.userRepo != nil && (role == model.RoleAdmin || role == model.RoleOffice) {
		assignees, _ = h.userRepo.ListAssignable()
	}

	filterQ := workFilterQuery(view, mineParam, assigneeFilter, sortKey, dir, role, scopeAll)
	kanbanQ := workFilterQuery("kanban", mineParam, assigneeFilter, sortKey, dir, role, scopeAll)
	listQ := workFilterQuery("list", mineParam, assigneeFilter, sortKey, dir, role, scopeAll)
	teamQ := workFilterQuery(view, "0", assigneeFilter, sortKey, dir, role, true)
	mineQ := workFilterQuery(view, "1", assigneeFilter, sortKey, dir, role, false)
	return c.Render(http.StatusOK, "work/list.html", map[string]interface{}{
		"Title":          "오늘 내 업무",
		"Active":         NavWork,
		"View":           view,
		"Items":          items,
		"KanbanColumns":  kanban.Columns,
		"KanbanTotal":    kanban.Total,
		"KanbanDrag":     false,
		"KanbanDrop":     "",
		"KanbanHint":     "보류·이관·회신대기는 진행중 열에 뱃지로 구분합니다. 행정업무 칸반과 같은 분류입니다.",
		"Total":          len(items),
		"Sort":           sortKey,
		"Dir":            dir,
		"SortHref":       workSortHrefs(view, mineParam, assigneeFilter, sortKey, dir, role, scopeAll),
		"FilterQ":        filterQ,
		"KanbanHref":     "/work?" + kanbanQ,
		"ListHref":       "/work?" + listQ,
		"TeamHref":       "/work?" + teamQ,
		"MineHref":       "/work?" + mineQ,
		"Role":           role,
		"RoleUnresolved": roleUnresolved,
		"Mine":           !scopeAll && assigneeFilter == "",
		"ScopeAll":       scopeAll,
		"AssigneeFilter": assigneeFilter,
		"Assignees":      assignees,
		"ShowAssignee":   role == model.RoleAdmin || role == model.RoleOffice || scopeAll,
		"ScopeNote":      workTodayScopeNote(role, scopeAll, assigneeFilter, roleUnresolved),
		"CanWrite":       canWriteWorkboard(c),
	})
}

func workTodayScopeNote(role string, scopeAll bool, assignee string, unresolved bool) string {
	if unresolved {
		return "역할을 확인하지 못해 팀 전체로 표시합니다"
	}
	if assignee != "" {
		return assignee + " 배정 업무"
	}
	if role == model.RoleSales && !scopeAll {
		return "내 배정 + 영업 활동"
	}
	if role == model.RoleTech && !scopeAll {
		return "내 배정 업무"
	}
	return "팀 전체"
}

func workFilterQuery(view, mineParam, assignee, sortKey, dir, role string, scopeAll bool) string {
	v := url.Values{}
	if view == "kanban" {
		v.Set("display", "kanban")
		v.Set("view", "kanban")
	} else if view == "list" {
		v.Set("display", "list")
		v.Set("view", "list")
	}
	if role == model.RoleTech || role == model.RoleSales {
		if scopeAll {
			v.Set("mine", "0")
		} else {
			v.Set("mine", "1")
		}
	} else if mineParam == "1" {
		v.Set("mine", "1")
	}
	if assignee != "" {
		v.Set("assignee", assignee)
	}
	sortKey, dir = model.NormalizeAdminWorkSort(sortKey, dir)
	if sortKey != "due_date" || dir != "asc" {
		v.Set("sort", sortKey)
		v.Set("dir", dir)
	}
	return v.Encode()
}

func workSortHrefs(view, mineParam, assignee, curSort, curDir, role string, scopeAll bool) map[string]string {
	curSort, curDir = model.NormalizeAdminWorkSort(curSort, curDir)
	out := map[string]string{}
	for _, col := range []string{"task_id", "title", "customer", "assignee", "work_date", "due_date", "status"} {
		dir := "asc"
		if col == curSort && curDir == "asc" {
			dir = "desc"
		}
		out[col] = "/work?" + workFilterQuery(view, mineParam, assignee, col, dir, role, scopeAll)
	}
	return out
}

func workListURL(bucket string, mine bool, role string) string {
	v := url.Values{}
	v.Set("bucket", bucket)
	if role == model.RoleTech {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	}
	return "/work?" + v.Encode()
}

func planUnplannedURL(mine bool, role, kind string) string {
	return planUnplannedURLDisp(mine, role, kind, "")
}

func planUnplannedURLDisp(mine bool, role, kind, display string) string {
	v := url.Values{}
	if role == model.RoleTech {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	} else if mine {
		v.Set("mine", "1")
	}
	if kind != "" {
		v.Set("kind", kind)
	}
	if model.ParseDisplay(display, "") == "kanban" {
		v.Set("display", "kanban")
	}
	s := v.Encode()
	if s == "" {
		return "/plan/unplanned"
	}
	return "/plan/unplanned?" + s
}
