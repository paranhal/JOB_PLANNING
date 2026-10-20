package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type WorkGroupHandler struct {
	repo  *repository.WorkGroupRepo
	users *repository.UserRepo
}

func NewWorkGroupHandler(repo *repository.WorkGroupRepo, users *repository.UserRepo) *WorkGroupHandler {
	return &WorkGroupHandler{repo: repo, users: users}
}

func workGroupActor(c echo.Context) model.WorkGroupActor {
	org := strings.TrimSpace(ctxString(c, "org_id"))
	if org == "" {
		org = model.OrgIDLibrary
	}
	return model.WorkGroupActor{
		UserID: currentUserID(c), OrgID: org, Role: currentRole(c),
		ReadOnly: isReadOnly(c), ViewAs: viewAsActive(c),
	}
}

func (h *WorkGroupHandler) mapErr(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	if repository.IsWorkGroupForbidden(err) {
		return echo.ErrForbidden
	}
	if repository.IsWorkGroupNotFound(err) {
		return echo.ErrNotFound
	}
	return err
}

func (h *WorkGroupHandler) List(c echo.Context) error {
	a := workGroupActor(c)
	status := strings.TrimSpace(c.QueryParam("status"))
	view := currentOrg(c)
	var items []model.WorkGroup
	var err error
	rle := model.NormalizeRole(a.Role)
	if rle == model.RoleVisionAdmin || rle == model.RoleCLevel || rle == model.RoleOrgAdmin || rle == model.RoleAdmin {
		org := view
		if rle == model.RoleOrgAdmin || rle == model.RoleAdmin {
			org = a.HomeOrg()
		}
		items, err = h.repo.ListGroups(org, status)
	} else {
		items, err = h.repo.ListMyGroups(a.UserID, "")
	}
	if err != nil {
		return err
	}
	canManage := a.CanManageOrg(a.HomeOrg()) || (rle == model.RoleVisionAdmin && !a.ReadOnly && !a.ViewAs)
	return c.Render(http.StatusOK, "work_group/list.html", map[string]interface{}{
		"Title":     "업무 그룹",
		"Active":    NavWorkGroups,
		"Items":     items,
		"CanManage": canManage,
		"ShowOrg":   canSwitchOrg(c),
		"Status":    status,
		"OK":        c.QueryParam("ok"),
		"Err":       c.QueryParam("err"),
	})
}

func (h *WorkGroupHandler) Form(c echo.Context) error {
	a := workGroupActor(c)
	id := strings.TrimSpace(c.Param("id"))
	var g *model.WorkGroup
	var members []model.WorkGroupPerson
	if id != "" {
		got, err := h.repo.Get(id)
		if err != nil {
			return err
		}
		if got == nil || !h.repo.CanView(a, got) {
			return echo.ErrNotFound
		}
		g = got
		members, _ = h.repo.MembersOn(id, "")
	} else if !a.CanManageOrg(a.HomeOrg()) && currentRole(c) != model.RoleVisionAdmin {
		return echo.ErrForbidden
	}
	org := a.HomeOrg()
	if g != nil {
		org = g.OrgID
	} else if v := currentOrg(c); v != "" && v != repository.OrgAll {
		org = v
	}
	cands, _ := h.users.ListAssignable(org)
	canManage := g != nil && a.CanManageOrg(g.OrgID) || (g == nil && (a.CanManageOrg(org) || currentRole(c) == model.RoleVisionAdmin && a.CanWrite()))
	if g != nil {
		canManage = a.CanManageOrg(g.OrgID)
	}
	canMembers := g != nil && h.repo.CanEditMembers(a, g)
	title := "업무 그룹"
	if g != nil {
		title = g.Name
	}
	return RenderPartial(c, "work_group/_form.html", map[string]interface{}{
		"Title": title, "Group": g, "Members": members, "Candidates": cands,
		"CanManage": canManage, "CanMembers": canMembers, "ActorID": a.UserID,
		"ShowOrgPick": showInsertOrgPick(c) && g == nil, "InsertOrgs": insertOrgChoices(c),
		"FormOrgID": org, "Err": c.QueryParam("err"),
	})
}

func (h *WorkGroupHandler) Create(c echo.Context) error {
	a := workGroupActor(c)
	if !a.CanWrite() {
		return echo.ErrForbidden
	}
	org, err := resolveInsertOrg(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/work-groups?err="+err.Error())
	}
	g := &model.WorkGroup{
		OrgID: org, Name: c.FormValue("name"), Purpose: c.FormValue("purpose"),
		ProjectID: c.FormValue("project_id"), StartDate: c.FormValue("start_date"), EndDate: c.FormValue("end_date"),
	}
	if err := h.repo.Create(a, g); err != nil {
		if repository.IsWorkGroupForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/work-groups?err="+err.Error())
	}
	leader := strings.TrimSpace(c.FormValue("leader_id"))
	if leader != "" {
		_ = h.repo.SetLeader(a, g.GroupID, leader)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) Update(c echo.Context) error {
	a := workGroupActor(c)
	g := &model.WorkGroup{
		GroupID: c.Param("id"), Name: c.FormValue("name"), Purpose: c.FormValue("purpose"),
		ProjectID: c.FormValue("project_id"), StartDate: c.FormValue("start_date"), EndDate: c.FormValue("end_date"),
	}
	if err := h.repo.Update(a, g); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) Close(c echo.Context) error {
	if err := h.repo.Close(workGroupActor(c), c.Param("id")); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) Reopen(c echo.Context) error {
	if err := h.repo.Reopen(workGroupActor(c), c.Param("id")); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) AddMember(c echo.Context) error {
	err := h.repo.AddMember(workGroupActor(c), c.Param("id"), c.FormValue("user_id"), model.WorkGroupMember, "")
	if err != nil {
		if repository.IsWorkGroupForbidden(err) {
			return echo.ErrForbidden
		}
		return c.Redirect(http.StatusSeeOther, "/work-groups?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) RemoveMember(c echo.Context) error {
	if err := h.repo.RemoveMember(workGroupActor(c), c.Param("id"), c.Param("user_id")); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) SetLeader(c echo.Context) error {
	if err := h.repo.SetLeader(workGroupActor(c), c.Param("id"), c.FormValue("user_id")); err != nil {
		return h.mapErr(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}
