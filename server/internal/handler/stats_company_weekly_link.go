package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const weeklyTaskLinkLimit = 200

const (
	weeklyKindTask = "work_task"
	weeklyKindAS   = "as_receipt"
	weeklyKindMnt  = "maintenance_visit"
)

type weeklyLinkRef struct {
	Kind string
	ID   string
}

func (h *StatsHandler) LinkWeeklyTasksProject(c echo.Context) error {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	orgID, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	if weeklyLinkRawCount(c) > weeklyTaskLinkLimit {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 200건까지 연결할 수 있습니다")
	}
	refs := parseWeeklyLinkRefs(c)
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if pid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하세요")
	}
	proj, ok, err := h.weeklyLinkProject(orgID, pid)
	if err != nil {
		return err
	}
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "목록에 없는 사업입니다")
	}
	if proj.OrgID != "" && proj.OrgID != orgID && c.FormValue("confirm_foreign") != "1" {
		return echo.NewHTTPError(http.StatusBadRequest, "타 조직 사업 확인이 필요합니다")
	}

	wantAssignee := weeklyWantAssigneeChange(c)
	var primary *model.User
	if wantAssignee {
		uid := strings.TrimSpace(c.FormValue("primary_user_id"))
		if uid == "" {
			ids := c.Request().Form["assignee_user_id"]
			if len(ids) > 0 {
				uid = strings.TrimSpace(ids[0])
			}
		}
		if uid == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "담당자를 선택하세요")
		}
		if err := checkAssigneeInOrg(h.userRepo, orgID, "", uid, ""); err != nil {
			return err
		}
		primary = h.userRepo.FindAssignable(orgID, uid)
		if primary == nil {
			return errAssigneeOrg()
		}
	}

	asRepo := repository.NewASRepo(h.repo.DB())
	unlock := repository.NewASUnlockRepo(h.repo.DB())
	var okN int
	var warns []string
	var linked []string
	var assigneeNames []string
	for _, ref := range refs {
		switch ref.Kind {
		case weeklyKindMnt:
			href := "/maintenance"
			warns = append(warns, ref.ID+": 점검 일정에서 사업 확인 ("+href+")")
		case weeklyKindAS:
			n, err := h.linkWeeklyAS(c, asRepo, unlock, orgID, ref.ID, pid, primary)
			if err == echo.ErrNotFound {
				return err
			}
			if err != nil {
				warns = append(warns, ref.ID+": "+err.Error())
				continue
			}
			if n != "" {
				assigneeNames = append(assigneeNames, n)
			}
			okN++
			linked = append(linked, weeklyKindAS+":"+ref.ID)
		default:
			n, err := h.linkWeeklyWorkTask(c, unlock, orgID, ref.ID, pid, primary)
			if err == echo.ErrNotFound {
				return err
			}
			if err != nil {
				warns = append(warns, ref.ID+": "+err.Error())
				continue
			}
			if n != "" {
				assigneeNames = append(assigneeNames, n)
			}
			okN++
			linked = append(linked, weeklyKindTask+":"+ref.ID)
		}
	}
	if okN > 0 {
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, TargetTable: "work_tasks,as_receipts", TargetID: strings.Join(linked, ","),
			Reason: fmt.Sprintf("주간보고 사업 연결 %s → %s", strings.Join(linked, ","), pid), Result: auditlog.ResultOK,
		})
	}
	q := url.Values{}
	if okN > 0 {
		who := strings.Join(uniqNonEmpty(assigneeNames), ", ")
		msg := fmt.Sprintf("연결했습니다(%s", proj.Name)
		if who != "" {
			msg += " · " + who
		}
		msg += ")"
		if okN > 1 {
			msg = fmt.Sprintf("연결했습니다(%d건 · %s)", okN, proj.Name)
		}
		q.Set("ok", msg)
	}
	if len(warns) > 0 {
		q.Set("err", strings.Join(warns, "; "))
	}
	if has, _ := h.repo.HasWeeklyReportRowForProject(orgID, pid); !has {
		q.Set("need_row", pid)
	}
	return c.Redirect(http.StatusSeeOther, weeklyAppendQuery(weeklySafeNext(c, orgID), q))
}

func (h *StatsHandler) linkWeeklyWorkTask(c echo.Context, unlock *repository.ASUnlockRepo, orgID, tid, pid string, primary *model.User) (string, error) {
	t, err := h.wbRepo.GetTask(tid)
	if err != nil || t == nil {
		return "", fmt.Errorf("업무가 없습니다")
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return "", echo.ErrNotFound
	}
	if t.OrgID != orgID {
		return "", echo.ErrNotFound
	}
	if !h.canLinkWeeklyTask(c, t) {
		return "", fmt.Errorf("권한이 없습니다")
	}
	if isProjectWorkClosed(t) || t.Status == model.WBTaskComplete || t.Status == model.WBTaskCancelled {
		unlocked, _, _ := unlock.HasActive(t.TaskID, currentUserID(c))
		if !unlocked {
			return "", fmt.Errorf("완료·취소 건은 수정 비밀번호로 잠금 해제한 뒤 연결하세요")
		}
	}
	before, after, err := h.wbRepo.SetTaskProject(t.OrgID, t.TaskID, pid)
	if err != nil {
		return "", err
	}
	line := projectLinkLineHandler(before, after)
	_, _ = h.wbRepo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), WorkContent: line,
	})
	an := ""
	if primary != nil {
		prev := t.Assignee
		if err := h.wbRepo.SetTaskAssignee(t.TaskID, primary.FullName); err != nil {
			return "", err
		}
		an = primary.FullName
		_ = prev
	}
	return an, nil
}

func (h *StatsHandler) linkWeeklyAS(c echo.Context, asRepo *repository.ASRepo, unlock *repository.ASUnlockRepo, orgID, asID, pid string, primary *model.User) (string, error) {
	rowOrg, assignedTo, assignedUID, status, err := asRepo.ASReceiptOrgAndAssignee(asID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rowOrg) != orgID {
		return "", echo.ErrNotFound
	}
	if !h.canLinkWeeklyAS(c, assignedTo, assignedUID, rowOrg) {
		return "", fmt.Errorf("권한이 없습니다")
	}
	if model.IsStatsCompletedStatus(status) || strings.EqualFold(status, "cancelled") {
		unlocked, _, _ := unlock.HasActive(asID, currentUserID(c))
		if !unlocked {
			return "", fmt.Errorf("완료·취소 건은 수정 비밀번호로 잠금 해제한 뒤 연결하세요")
		}
	}
	before, after, err := asRepo.SetASReceiptProject(orgID, asID, pid)
	if err != nil {
		return "", err
	}
	_ = asRepo.InsertASProjectProcess(asID, orgID, ctxString(c, "user_name"), currentUserID(c), projectLinkLineHandler(before, after))
	an := ""
	if primary != nil {
		if err := asRepo.SetASPrimaryAssignee(asID, primary.FullName, primary.UserID); err != nil {
			return "", err
		}
		an = primary.FullName
	}
	return an, nil
}

func projectLinkLineHandler(before, after string) string {
	line := "사업 연결: "
	if before == "" {
		line += "없음"
	} else {
		line += before
	}
	line += " → "
	if after == "" {
		line += "없음"
	} else {
		line += after
	}
	return line
}

func weeklyWantAssigneeChange(c echo.Context) bool {
	if c.FormValue("assignee_keep") == "1" || c.FormValue("assignee_mode") == "keep" {
		return false
	}
	if strings.TrimSpace(c.FormValue("primary_user_id")) != "" {
		return true
	}
	ids := c.Request().Form["assignee_user_id"]
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			return true
		}
	}
	return false
}

func weeklyLinkRawCount(c echo.Context) int {
	n := len(c.Request().Form["task_id"]) + len(c.Request().Form["item"])
	k, ids := c.Request().Form["kind"], c.Request().Form["id"]
	m := len(k)
	if len(ids) < m {
		m = len(ids)
	}
	return n + m
}

func parseWeeklyLinkRefs(c echo.Context) []weeklyLinkRef {
	seen := map[string]bool{}
	var out []weeklyLinkRef
	add := func(kind, id string) {
		kind = mapWeeklyLinkKind(kind)
		id = strings.TrimSpace(id)
		if kind == "" || id == "" {
			return
		}
		key := kind + ":" + id
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, weeklyLinkRef{Kind: kind, ID: id})
	}
	for _, raw := range c.Request().Form["item"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		kind, id, ok := strings.Cut(raw, ":")
		if !ok {
			add(weeklyKindTask, raw)
			continue
		}
		add(kind, id)
	}
	kinds := c.Request().Form["kind"]
	ids := c.Request().Form["id"]
	n := len(kinds)
	if len(ids) < n {
		n = len(ids)
	}
	for i := 0; i < n; i++ {
		add(kinds[i], ids[i])
	}
	for _, tid := range c.Request().Form["task_id"] {
		add(weeklyKindTask, tid)
	}
	if len(out) == 0 {
		if one := strings.TrimSpace(c.FormValue("task_id")); one != "" {
			add(weeklyKindTask, one)
		}
		if one := strings.TrimSpace(c.FormValue("id")); one != "" {
			add(c.FormValue("kind"), one)
		}
	}
	return out
}

func mapWeeklyLinkKind(s string) string {
	switch strings.TrimSpace(s) {
	case weeklyKindTask, model.ScopeWorkAdmin, "task":
		return weeklyKindTask
	case weeklyKindAS, model.ScopeWorkAS:
		return weeklyKindAS
	case weeklyKindMnt, model.ScopeWorkMaintenance, "visit":
		return weeklyKindMnt
	default:
		return ""
	}
}

func uniqNonEmpty(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (h *StatsHandler) weeklyLinkProject(orgID, projectID string) (model.WorkProject, bool, error) {
	list, err := h.repo.ListWeeklyRowProjects(orgID)
	if err != nil {
		return model.WorkProject{}, false, err
	}
	for _, p := range list {
		if p.ProjectID == projectID {
			return p, true, nil
		}
	}
	return model.WorkProject{}, false, nil
}

func (h *StatsHandler) canLinkWeeklyTask(c echo.Context, t *model.WorkTask) bool {
	if t == nil || isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		org := strings.TrimSpace(nz(c.FormValue("org"), currentOrg(c)))
		return org != "" && org != repository.OrgAll && org == t.OrgID
	}
	if currentRole(c) == model.RoleOrgAdmin && loginHomeOrg(c) == t.OrgID {
		return true
	}
	if canEditTask(c, t.Assignee, t.AssigneeUserID) {
		return true
	}
	if h.wbRepo == nil {
		return false
	}
	members, _ := h.wbRepo.ListMembers(t.TaskID)
	for _, m := range members {
		if assigneeIsMine(c, m.Assignee, m.UserID) {
			return true
		}
	}
	return false
}

func (h *StatsHandler) canLinkWeeklyAS(c echo.Context, assignedTo, assignedUID, orgID string) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		org := strings.TrimSpace(nz(c.FormValue("org"), currentOrg(c)))
		return org != "" && org != repository.OrgAll && org == orgID
	}
	if currentRole(c) == model.RoleOrgAdmin && loginHomeOrg(c) == orgID {
		return true
	}
	return assigneeIsMine(c, assignedTo, assignedUID)
}
