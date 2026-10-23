package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type mntWorkGroupPickJSON struct {
	GroupID       string                `json:"id"`
	Name          string                `json:"name"`
	LeaderID      string                `json:"leader_id"`
	OrgLabel      string                `json:"org_label,omitempty"`
	MemberN       int                   `json:"member_n"`
	MemberSummary string                `json:"member_summary,omitempty"`
	Members       []mntWorkGroupMemJSON `json:"members"`
}

type mntWorkGroupMemJSON struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

func (h *MaintenanceHandler) listVisibleWorkGroups(c echo.Context) []model.WorkGroup {
	if h == nil || h.groups == nil || c == nil {
		return nil
	}
	org := currentOrg(c)
	role := model.NormalizeRole(currentRole(c))
	admin := role == model.RoleOrgAdmin || role == model.RoleAdmin || role == model.RoleVisionAdmin || role == model.RoleCLevel
	if org == repository.OrgAll && (role == model.RoleVisionAdmin || role == model.RoleCLevel) {
		groups, _ := h.groups.ListGroups("", model.WorkGroupActive)
		return groups
	}
	if org == "" || org == repository.OrgAll {
		org = workGroupActor(c).HomeOrg()
	}
	groups, _ := h.groups.ListVisibleActiveForWork(org, currentUserID(c), admin)
	return groups
}

func (h *MaintenanceHandler) workGroupPicksForVisit(c echo.Context) ([]model.WorkGroup, template.JS) {
	groups := h.listVisibleWorkGroups(c)
	orgNames := map[string]string{}
	if h != nil && h.orgs != nil {
		if rows, err := h.orgs.ListActive(); err == nil {
			for _, o := range rows {
				orgNames[o.OrgID] = o.OrgName
			}
		}
	}
	showOrg := currentOrg(c) == repository.OrgAll
	out := make([]mntWorkGroupPickJSON, 0, len(groups))
	for _, g := range groups {
		p := mntWorkGroupPickJSON{GroupID: g.GroupID, Name: g.Name, LeaderID: g.LeaderID}
		if showOrg {
			p.OrgLabel = orgNames[g.OrgID]
			if p.OrgLabel == "" {
				p.OrgLabel = g.OrgID
			}
		}
		var names []string
		if h.groups != nil {
			ms, _ := h.groups.MembersOn(g.GroupID, "")
			for _, m := range ms {
				p.Members = append(p.Members, mntWorkGroupMemJSON{UserID: m.UserID, Name: m.FullName, Role: m.Role})
				names = append(names, m.FullName)
				if m.Role == model.WorkGroupLeader && p.LeaderID == "" {
					p.LeaderID = m.UserID
				}
			}
		}
		p.MemberN = len(p.Members)
		if p.MemberN == 0 {
			p.MemberN = g.MemberN
		}
		p.MemberSummary = memberSummary(names)
		out = append(out, p)
	}
	b, err := json.Marshal(out)
	if err != nil || len(b) == 0 {
		b = []byte("[]")
	}
	return groups, template.JS(b)
}

func uniqueTrimmed(vals []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range vals {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func mergeVisitAssigneeIDs(groupIDs, checked, exclude []string) []string {
	ex := map[string]bool{}
	for _, raw := range exclude {
		id := strings.TrimSpace(raw)
		if id != "" {
			ex[id] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] || ex[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range groupIDs {
		add(id)
	}
	for _, id := range checked {
		add(id)
	}
	return out
}

func formStringSlice(c echo.Context, key string) []string {
	if c == nil {
		return nil
	}
	vals, err := c.FormParams()
	if err != nil {
		return nil
	}
	return vals[key]
}

func formVisitGroupIDs(c echo.Context) []string {
	ids := uniqueTrimmed(formStringSlice(c, "assignee_group_id"))
	if wg := strings.TrimSpace(c.FormValue("work_group_id")); wg != "" && !containsUserID(ids, wg) {
		ids = append(ids, wg)
	}
	return ids
}

func formVisitExcludeIDs(c echo.Context) []string {
	raw := append([]string{}, formStringSlice(c, "assignee_exclude_user_id")...)
	raw = append(raw, formStringSlice(c, "assignee_exclude_id")...)
	return uniqueTrimmed(raw)
}

func (h *MaintenanceHandler) visibleGroupByID(c echo.Context, groupID string) *model.WorkGroup {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil
	}
	for _, g := range h.listVisibleWorkGroups(c) {
		if g.GroupID == groupID {
			gg := g
			return &gg
		}
	}
	return nil
}

func (h *MaintenanceHandler) applyVisitAssigneesFromForm(c echo.Context, v *model.MaintenanceVisit) error {
	if v == nil {
		return nil
	}
	groupIDs := formVisitGroupIDs(c)
	checked := uniqueTrimmed(formStringSlice(c, "assignee_user_id"))
	exclude := formVisitExcludeIDs(c)
	ownerID := strings.TrimSpace(c.FormValue("owner_user_id"))
	multi := strings.TrimSpace(c.FormValue("assignee_multi")) == "1"
	if !multi && len(groupIDs) == 0 && len(checked) == 0 && len(exclude) == 0 && ownerID == "" {
		name := strings.TrimSpace(c.FormValue("assignee"))
		kind, _ := parseAssigneeFilter(name)
		if name != "" && kind == "all" && name != "all" {
			v.Assignee = name
		}
		return nil
	}

	ex := map[string]bool{}
	for _, id := range exclude {
		ex[id] = true
	}
	groupMember := map[string]bool{}
	personGroup := map[string]string{}
	var ids []string
	seen := map[string]bool{}
	firstLeader := ""
	for i, gid := range groupIDs {
		g := h.visibleGroupByID(c, gid)
		if g == nil {
			return fmt.Errorf("업무 그룹을 확인할 수 없습니다")
		}
		if h.groups == nil {
			continue
		}
		ms, _ := h.groups.MembersOn(gid, v.VisitDate)
		for _, m := range ms {
			uid := strings.TrimSpace(m.UserID)
			if uid == "" {
				continue
			}
			groupMember[uid] = true
			if i == 0 && m.Role == model.WorkGroupLeader && firstLeader == "" {
				firstLeader = uid
			}
			if ex[uid] || seen[uid] {
				continue
			}
			seen[uid] = true
			ids = append(ids, uid)
			personGroup[uid] = gid
		}
	}
	for _, uid := range checked {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		ids = append(ids, uid)
	}
	for _, id := range ids {
		if !h.assigneeIDAllowed(c, id, groupMember) {
			return fmt.Errorf("%s", errAssigneeSameOrg)
		}
	}
	if ownerID != "" && !containsUserID(ids, ownerID) {
		ownerID = ""
	}
	if ownerID == "" {
		if firstLeader != "" && containsUserID(ids, firstLeader) {
			ownerID = firstLeader
		} else if len(ids) > 0 {
			ownerID = ids[0]
		}
	}

	people := make([]model.MaintenanceVisitAssignee, 0, len(ids))
	var ownerName string
	for i, id := range ids {
		u := h.lookupAssignableUser(c, id)
		name := id
		if u != nil {
			name = strings.TrimSpace(u.FullName)
			if name == "" {
				name = u.Username
			}
		} else if h.userRepo != nil {
			if got, _ := h.userRepo.GetByID(id); got != nil {
				name = strings.TrimSpace(got.FullName)
			}
		}
		role := model.WBMemberSupport
		if id == ownerID || (ownerID == "" && i == 0) {
			role = model.WBMemberOwner
			ownerName = name
		}
		people = append(people, model.MaintenanceVisitAssignee{
			UserID: id, AssigneeName: name, Role: role, WorkGroupID: personGroup[id],
		})
	}
	v.People = people
	v.Assignee = ownerName
	if len(groupIDs) > 0 {
		v.WorkGroupID = groupIDs[0]
	} else {
		v.WorkGroupID = ""
	}
	return nil
}

func (h *MaintenanceHandler) assigneeIDAllowed(c echo.Context, uid string, groupMember map[string]bool) bool {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return false
	}
	if groupMember[uid] {
		return true
	}
	return h.lookupAssignableUser(c, uid) != nil
}

func (h *MaintenanceHandler) lookupAssignableUser(c echo.Context, uid string) *model.User {
	if h == nil || h.userRepo == nil {
		return nil
	}
	org := assignableOrgID(c)
	if org == "" || org == repository.OrgAll {
		return nil
	}
	return h.userRepo.FindAssignable(org, uid)
}

func containsUserID(ids []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, id := range ids {
		if strings.TrimSpace(id) == want {
			return true
		}
	}
	return false
}

func visitPeopleIDsCSV(v model.MaintenanceVisit) string {
	ids := visitAssigneeIDs(v)
	return strings.Join(ids, ",")
}

func visitOwnerUserID(v model.MaintenanceVisit) string {
	for _, p := range v.People {
		if p.Role == model.WBMemberOwner && strings.TrimSpace(p.UserID) != "" {
			return p.UserID
		}
	}
	if len(v.People) > 0 {
		return strings.TrimSpace(v.People[0].UserID)
	}
	return ""
}

func visitGroupIDsCSV(v model.MaintenanceVisit) string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(v.WorkGroupID)
	for _, p := range v.People {
		add(p.WorkGroupID)
	}
	return strings.Join(out, ",")
}
