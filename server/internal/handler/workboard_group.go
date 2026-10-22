package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type workGroupPickJSON struct {
	GroupID    string             `json:"id"`
	Name       string             `json:"name"`
	LeaderName string             `json:"leader_name"`
	Members    []workGroupPickMem `json:"members"`
}

type workGroupPickMem struct {
	UserID   string `json:"user_id"`
	FullName string `json:"name"`
	Role     string `json:"role"`
}

func (h *WorkboardHandler) visibleWorkGroups(c echo.Context) []model.WorkGroup {
	if h == nil || h.groups == nil {
		return nil
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		org = workGroupActor(c).HomeOrg()
	}
	role := model.NormalizeRole(currentRole(c))
	admin := role == model.RoleOrgAdmin || role == model.RoleAdmin || role == model.RoleVisionAdmin
	items, err := h.groups.ListVisibleActiveForWork(org, currentUserID(c), admin)
	if err != nil {
		return nil
	}
	return items
}

func (h *WorkboardHandler) workGroupPicks(c echo.Context) ([]model.WorkGroup, string) {
	items := h.visibleWorkGroups(c)
	out := make([]workGroupPickJSON, 0, len(items))
	for _, g := range items {
		p := workGroupPickJSON{GroupID: g.GroupID, Name: g.Name, LeaderName: g.LeaderName}
		ms, _ := h.groups.MembersOn(g.GroupID, "")
		for _, m := range ms {
			p.Members = append(p.Members, workGroupPickMem{UserID: m.UserID, FullName: m.FullName, Role: m.Role})
		}
		out = append(out, p)
	}
	b, _ := json.Marshal(out)
	if len(b) == 0 {
		b = []byte("[]")
	}
	return items, string(b)
}

func (h *WorkboardHandler) applyWorkGroupForm(c echo.Context, t *model.WorkTask) error {
	if t == nil {
		return nil
	}
	gid := strings.TrimSpace(c.FormValue("work_group_id"))
	t.WorkGroupID = gid
	if gid == "" {
		return nil
	}
	if h.groups == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "그룹을 확인할 수 없습니다")
	}
	g, err := h.groups.Get(gid)
	if err != nil || g == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "그룹을 찾을 수 없습니다")
	}
	org := strings.TrimSpace(t.OrgID)
	if org == "" {
		org = currentOrg(c)
	}
	if strings.TrimSpace(g.OrgID) != org {
		return echo.NewHTTPError(http.StatusBadRequest, "다른 조직의 그룹입니다")
	}
	day := time.Now().Format("2006-01-02")
	if g.Status != model.WorkGroupActive || g.ExpiredOn(day) {
		return echo.NewHTTPError(http.StatusBadRequest, "종료되었거나 기간이 지난 그룹입니다")
	}
	if strings.TrimSpace(t.Assignee) != "" {
		t.AssigneeSource = model.WBAssigneeSourceManual
	}
	t.WorkGroupName = g.Name
	return nil
}

func (h *WorkboardHandler) RefillTaskGroup(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	gid := strings.TrimSpace(t.WorkGroupID)
	if gid == "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+id+"?err=group")
	}
	if h.groups == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "그룹을 확인할 수 없습니다")
	}
	ms, err := h.groups.MembersOn(gid, "")
	if err != nil {
		return err
	}
	var owner string
	var supports []model.WorkTaskMember
	for _, m := range ms {
		if m.Role == model.WorkGroupLeader && owner == "" {
			owner = m.FullName
			continue
		}
		supports = append(supports, model.WorkTaskMember{Assignee: m.FullName, Role: model.WBMemberSupport})
	}
	if owner == "" && t.Assignee != "" {
		owner = t.Assignee
	}
	if owner != "" {
		t.Assignee = owner
		t.AssigneeSource = model.WBAssigneeSourceManual
		if err := h.repo.UpdateTask(t); err != nil {
			return err
		}
	}
	if err := h.repo.ReplaceSupportMembers(id, supports); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=group"))
}

func filterTasksByWorkGroup(items []model.WorkTask, gid string) []model.WorkTask {
	gid = strings.TrimSpace(gid)
	if gid == "" {
		return items
	}
	var out []model.WorkTask
	for _, t := range items {
		if strings.TrimSpace(t.WorkGroupID) == gid {
			out = append(out, t)
		}
	}
	return out
}
