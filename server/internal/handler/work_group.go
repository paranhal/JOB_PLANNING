package handler

import (
	"fmt"
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

func workGroupWantsFragment(c echo.Context) bool {
	xrw := strings.ToLower(strings.TrimSpace(c.Request().Header.Get("X-Requested-With")))
	return xrw == "fetch" && wantsHTML(c)
}

func (h *WorkGroupHandler) renderForm(c echo.Context, id, notice, errMsg string, status int) error {
	a := workGroupActor(c)
	id = strings.TrimSpace(id)
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
		raw, _ := h.repo.MembersOn(id, "")
		for _, m := range raw {
			if strings.TrimSpace(m.LeftOn) == "" {
				members = append(members, m)
			}
		}
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
	in := map[string]bool{}
	for _, m := range members {
		in[m.UserID] = true
	}
	var open []model.User
	for _, u := range cands {
		if !in[u.UserID] {
			open = append(open, u)
		}
	}
	canManage := g == nil && (a.CanManageOrg(org) || (currentRole(c) == model.RoleVisionAdmin && a.CanWrite()))
	if g != nil {
		canManage = a.CanManageOrg(g.OrgID)
	}
	canMembers := g != nil && h.repo.CanEditMembers(a, g)
	title := "업무 그룹"
	if g != nil {
		title = g.Name
	}
	if errMsg == "" {
		errMsg = c.QueryParam("err")
	}
	if status == 0 {
		status = http.StatusOK
	}
	c.Response().Status = status
	return RenderPartial(c, "work_group/_form.html", map[string]interface{}{
		"Title": title, "Group": g, "Members": members, "Candidates": open,
		"CanManage": canManage, "CanMembers": canMembers, "ActorID": a.UserID,
		"ShowOrgPick": showInsertOrgPick(c) && g == nil, "InsertOrgs": insertOrgChoices(c),
		"FormOrgID": org, "Err": errMsg, "Notice": notice,
	})
}

func (h *WorkGroupHandler) finishForm(c echo.Context, id, notice, errMsg string, status int) error {
	if workGroupWantsFragment(c) {
		if status == 0 {
			status = http.StatusOK
		}
		return h.renderForm(c, id, notice, errMsg, status)
	}
	if errMsg != "" {
		return c.Redirect(http.StatusSeeOther, "/work-groups?err="+errMsg)
	}
	return c.Redirect(http.StatusSeeOther, "/work-groups?ok=1")
}

func (h *WorkGroupHandler) Form(c echo.Context) error {
	return h.renderForm(c, c.Param("id"), "", c.QueryParam("err"), http.StatusOK)
}

func (h *WorkGroupHandler) Create(c echo.Context) error {
	a := workGroupActor(c)
	if !a.CanWrite() {
		return echo.ErrForbidden
	}
	org, err := resolveInsertOrg(c)
	if err != nil {
		return h.finishForm(c, "", "", err.Error(), http.StatusBadRequest)
	}
	_ = c.Request().ParseForm()
	g := &model.WorkGroup{
		OrgID: org, Name: c.FormValue("name"), Purpose: c.FormValue("purpose"),
		ProjectID: c.FormValue("project_id"), StartDate: c.FormValue("start_date"), EndDate: c.FormValue("end_date"),
	}
	members := c.Request().Form["member_ids"]
	if len(members) == 0 {
		members = c.Request().Form["member_ids[]"]
	}
	leader := strings.TrimSpace(c.FormValue("leader_id"))
	if err := h.repo.CreateWithMembers(a, g, members, leader); err != nil {
		if repository.IsWorkGroupForbidden(err) {
			return echo.ErrForbidden
		}
		st := http.StatusBadRequest
		return h.finishForm(c, "", "", err.Error(), st)
	}
	return h.finishForm(c, g.GroupID, "그룹을 만들었습니다", "", http.StatusOK)
}

func (h *WorkGroupHandler) Update(c echo.Context) error {
	a := workGroupActor(c)
	g := &model.WorkGroup{
		GroupID: c.Param("id"), Name: c.FormValue("name"), Purpose: c.FormValue("purpose"),
		ProjectID: c.FormValue("project_id"), StartDate: c.FormValue("start_date"), EndDate: c.FormValue("end_date"),
	}
	if err := h.repo.Update(a, g); err != nil {
		if workGroupWantsFragment(c) {
			msg := err.Error()
			if repository.IsWorkGroupForbidden(err) {
				return echo.ErrForbidden
			}
			return h.renderForm(c, g.GroupID, "", msg, http.StatusBadRequest)
		}
		return h.mapErr(c, err)
	}
	return h.finishForm(c, g.GroupID, "저장했습니다", "", http.StatusOK)
}

func (h *WorkGroupHandler) Close(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.Close(workGroupActor(c), id); err != nil {
		if workGroupWantsFragment(c) {
			return h.renderForm(c, id, "", err.Error(), http.StatusBadRequest)
		}
		return h.mapErr(c, err)
	}
	return h.finishForm(c, id, "종료했습니다", "", http.StatusOK)
}

func (h *WorkGroupHandler) Reopen(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.Reopen(workGroupActor(c), id); err != nil {
		if workGroupWantsFragment(c) {
			return h.renderForm(c, id, "", err.Error(), http.StatusBadRequest)
		}
		return h.mapErr(c, err)
	}
	return h.finishForm(c, id, "다시 열었습니다", "", http.StatusOK)
}

func (h *WorkGroupHandler) AddMember(c echo.Context) error {
	id := c.Param("id")
	a := workGroupActor(c)
	_ = c.Request().ParseForm()
	ids := append([]string{}, c.Request().Form["member_ids"]...)
	ids = append(ids, c.Request().Form["member_ids[]"]...)
	if u := strings.TrimSpace(c.FormValue("user_id")); u != "" {
		ids = append(ids, u)
	}
	added, skipped := 0, 0
	var last error
	for _, uid := range ids {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		err := h.repo.AddMember(a, id, uid, model.WorkGroupMember, "")
		if err == nil {
			added++
			continue
		}
		if repository.IsWorkGroupForbidden(err) {
			return echo.ErrForbidden
		}
		if repository.IsWorkGroupAlreadyMember(err) {
			skipped++
			continue
		}
		last = err
	}
	notice := ""
	errMsg := ""
	if added > 0 {
		notice = fmt.Sprintf("%d명을 추가했습니다", added)
	}
	if skipped > 0 {
		if notice != "" {
			notice += ". "
		}
		notice += "이미 이 그룹에 있어 건너뛴 사람이 있습니다"
	}
	if last != nil && added == 0 {
		errMsg = last.Error()
		return h.finishForm(c, id, notice, errMsg, http.StatusBadRequest)
	}
	if added == 0 && skipped > 0 {
		notice = "이미 이 그룹에 있습니다"
	}
	return h.finishForm(c, id, notice, "", http.StatusOK)
}

func (h *WorkGroupHandler) RemoveMember(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.RemoveMember(workGroupActor(c), id, c.Param("user_id")); err != nil {
		if workGroupWantsFragment(c) {
			return h.renderForm(c, id, "", err.Error(), http.StatusBadRequest)
		}
		return h.mapErr(c, err)
	}
	return h.finishForm(c, id, "제거했습니다", "", http.StatusOK)
}

func (h *WorkGroupHandler) SetLeader(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.SetLeader(workGroupActor(c), id, c.FormValue("user_id")); err != nil {
		if workGroupWantsFragment(c) {
			if repository.IsWorkGroupForbidden(err) {
				return echo.ErrForbidden
			}
			return h.renderForm(c, id, "", err.Error(), http.StatusBadRequest)
		}
		return h.mapErr(c, err)
	}
	return h.finishForm(c, id, "그룹장을 지정했습니다", "", http.StatusOK)
}
