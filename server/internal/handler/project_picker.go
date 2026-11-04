package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const projectMatchNeededMsg = "이 업무는 사업명 매칭이 필요합니다. 사업을 선택한 뒤 진행하세요"

type ProjectPickerOpts struct {
	Name               string
	ID                 string
	Required           bool
	EmptyLabel         string
	Selected           string
	SelectedName       string
	IncludeClosed      bool
	AllowPending       bool
	AllowQuickRegister bool
	Placeholder        string
	Pending            bool
}

type projectPickerItem struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	ProjectKind string `json:"project_kind,omitempty"`
	Related     bool   `json:"related"`
	ChildOrg    string `json:"child_org,omitempty"`
	ClosedLabel string `json:"closed_label,omitempty"`
}

type ProjectPickerView struct {
	Name               string              `json:"Name"`
	ID                 string              `json:"ID"`
	Required           bool                `json:"Required"`
	EmptyLabel         string              `json:"EmptyLabel"`
	Selected           string              `json:"Selected"`
	SelectedName       string              `json:"SelectedName"`
	Placeholder        string              `json:"Placeholder"`
	Own                []projectPickerItem `json:"Own"`
	OtherCount         int                 `json:"OtherCount"`
	CanPickOther       bool                `json:"CanPickOther"`
	OtherURL           string              `json:"OtherURL"`
	IncludeClosed      bool                `json:"IncludeClosed"`
	AllowPending       bool                `json:"AllowPending"`
	AllowQuickRegister bool                `json:"AllowQuickRegister"`
	Pending            bool                `json:"Pending"`
	CanCreateProject   bool                `json:"CanCreateProject"`
}

func pickerHomeOrg(c echo.Context) string {
	org := strings.TrimSpace(currentOrg(c))
	if org == "" || org == repository.OrgAll {
		if home := strings.TrimSpace(loginHomeOrg(c)); home != "" && home != repository.OrgAll {
			return home
		}
	}
	if org == repository.OrgAll {
		return repository.OrgAll
	}
	return org
}

func pickerExtraOrgIDs(c echo.Context) []string {
	if model.NormalizeRole(currentRole(c)) != model.RoleOrgAdmin {
		return nil
	}
	home := strings.TrimSpace(loginHomeOrg(c))
	if home == "" {
		home = strings.TrimSpace(currentOrg(c))
	}
	if home == "" || home == repository.OrgAll {
		return nil
	}
	return divisionChildOrgIDs(c, home)
}

func closedProjectLabel(status string) string {
	switch strings.TrimSpace(status) {
	case model.WBProjectArchived:
		return "(보관)"
	case model.WBProjectComplete:
		return "(완료)"
	case "cancelled":
		return "(취소)"
	default:
		return ""
	}
}

func toPickerItem(p model.WorkProject, home string, extra []string) projectPickerItem {
	it := projectPickerItem{
		ProjectID:   p.ProjectID,
		Name:        strings.TrimSpace(p.Name),
		ProjectKind: p.ProjectKind,
		ClosedLabel: closedProjectLabel(p.Status),
	}
	host := strings.TrimSpace(p.OrgID)
	if home != "" && home != repository.OrgAll && host != "" && host != home {
		if orgIDInList(host, extra) {
			it.ChildOrg = strings.TrimSpace(p.OrgLabel)
			if it.ChildOrg == "" {
				it.ChildOrg = host
			}
		} else if p.Related || model.ProjectAccess(p, home) == model.ProjectAccessRelated {
			it.Related = true
		}
	} else if p.Related {
		it.Related = true
	}
	return it
}

func orgIDInList(id string, ids []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, x := range ids {
		if strings.TrimSpace(x) == id {
			return true
		}
	}
	return false
}

func splitPickerOwnOther(all []model.WorkProject, home string, extra []string) (own, other []model.WorkProject) {
	for _, p := range all {
		if model.ProjectInOwnPickScope(p, home, extra) {
			if home != "" && home != repository.OrgAll && strings.TrimSpace(p.OrgID) != home && !orgIDInList(p.OrgID, extra) {
				p.Related = true
			}
			own = append(own, p)
			continue
		}
		other = append(other, p)
	}
	return own, other
}

func filterPickerStatus(all []model.WorkProject, includeClosed bool) []model.WorkProject {
	var out []model.WorkProject
	for _, p := range all {
		if !includeClosed && projectStatusClosed(p.Status) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func projectStatusClosed(status string) bool {
	switch strings.TrimSpace(status) {
	case model.WBProjectArchived, model.WBProjectComplete, "cancelled":
		return true
	default:
		return false
	}
}

func loadPickerProjects(h *WorkboardHandler, includeClosed bool) []model.WorkProject {
	if h == nil || h.repo == nil {
		return nil
	}
	if includeClosed {
		list, _ := h.repo.ListProjects(false)
		pr := repository.NewProjectRepo(h.repo.DB())
		if pr != nil {
			labeled, err := pr.ListFiltered(repository.OrgAll, "", 0, "")
			if err == nil {
				return labeled
			}
		}
		return list
	}
	list, _ := h.repo.ListActiveProjectsLabeled()
	return list
}

func buildProjectPicker(c echo.Context, h *WorkboardHandler, opts ProjectPickerOpts) ProjectPickerView {
	if strings.TrimSpace(opts.Name) == "" {
		opts.Name = "project_id"
	}
	if strings.TrimSpace(opts.EmptyLabel) == "" {
		opts.EmptyLabel = "— 선택 (해당 시) —"
	}
	if strings.TrimSpace(opts.Placeholder) == "" {
		opts.Placeholder = "사업명으로 찾기"
	}
	home := pickerHomeOrg(c)
	extra := pickerExtraOrgIDs(c)
	role := currentRole(c)
	canOther := model.CanPickOtherProjects(role)
	all := filterPickerStatus(loadPickerProjects(h, opts.IncludeClosed), opts.IncludeClosed)
	ownSrc, otherSrc := splitPickerOwnOther(all, home, extra)
	view := ProjectPickerView{
		Name:               opts.Name,
		ID:                 opts.ID,
		Required:           opts.Required,
		EmptyLabel:         opts.EmptyLabel,
		Selected:           strings.TrimSpace(opts.Selected),
		SelectedName:       strings.TrimSpace(opts.SelectedName),
		Placeholder:        opts.Placeholder,
		CanPickOther:       canOther,
		OtherURL:           "/workboard/projects/picker-other",
		IncludeClosed:      opts.IncludeClosed,
		AllowPending:       opts.AllowPending,
		AllowQuickRegister: opts.AllowQuickRegister,
		Pending:            opts.Pending,
		CanCreateProject:   canWriteProjects(c),
	}
	for _, p := range ownSrc {
		view.Own = append(view.Own, toPickerItem(p, home, extra))
	}
	if canOther {
		view.OtherCount = len(otherSrc)
	}
	if view.Selected != "" && view.SelectedName == "" {
		for _, p := range all {
			if p.ProjectID == view.Selected {
				view.SelectedName = p.Name
				break
			}
		}
	}
	return view
}

func pickerFromRepo(repo *repository.WBRepo) *WorkboardHandler {
	return &WorkboardHandler{repo: repo}
}

func attachProjectPicker(c echo.Context, h *WorkboardHandler, data map[string]interface{}, opts ProjectPickerOpts) {
	if data == nil {
		return
	}
	view := buildProjectPicker(c, h, opts)
	data["ProjectPicker"] = view
	var wp []model.WorkProject
	all := filterPickerStatus(loadPickerProjects(h, opts.IncludeClosed), opts.IncludeClosed)
	own, _ := splitPickerOwnOther(all, pickerHomeOrg(c), pickerExtraOrgIDs(c))
	wp = own
	data["WorkProjects"] = wp
}

func (h *WorkboardHandler) ListPickerOtherProjects(c echo.Context) error {
	if strings.TrimSpace(currentUserID(c)) == "" {
		return c.JSON(http.StatusForbidden, map[string]interface{}{"groups": []projectOrgGroup{}, "total": 0})
	}
	if !model.CanPickOtherProjects(currentRole(c)) {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":  "타 부서 사업을 선택할 권한이 없습니다",
			"groups": []projectOrgGroup{},
			"total":  0,
		})
	}
	includeClosed := c.QueryParam("closed") == "1"
	all := filterPickerStatus(loadPickerProjects(h, includeClosed), includeClosed)
	_, other := splitPickerOwnOther(all, pickerHomeOrg(c), pickerExtraOrgIDs(c))
	q := strings.TrimSpace(c.QueryParam("q"))
	var matched []model.WorkProject
	for _, p := range other {
		if matchProjectQuery(p, q) {
			matched = append(matched, p)
		}
	}
	groups := groupProjectsByOrg(matched)
	if groups == nil {
		groups = []projectOrgGroup{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"groups": groups, "total": len(matched)})
}

func requireProjectLink(c echo.Context, get func(string) (*model.WorkProject, error), projectID string, includeClosed bool) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if get == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	p, err := get(projectID)
	if err != nil || p == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	if c == nil {
		return nil
	}
	org := pickerHomeOrg(c)
	if org == "" {
		return nil
	}
	if !model.CanLinkWorkProject(*p, org, currentRole(c), pickerExtraOrgIDs(c), includeClosed) {
		return echo.NewHTTPError(http.StatusForbidden, "이 사업에 업무를 등록할 수 없습니다")
	}
	return nil
}

func applySupportProjectForm(c echo.Context, t *model.WorkTask) string {
	if t == nil {
		return ""
	}
	pending := c.FormValue("project_match_pending") == "1"
	if t.WorkType != model.WBWorkSupport {
		t.ProjectMatchPending = false
		return ""
	}
	if pending {
		t.ProjectID = ""
		t.ProjectMatchPending = true
		return ""
	}
	if strings.TrimSpace(t.ProjectID) == "" {
		return "project_required"
	}
	t.ProjectMatchPending = false
	return ""
}

func denyIfProjectMatchPending(t *model.WorkTask) error {
	if t != nil && t.ProjectMatchPending {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, projectMatchNeededMsg)
	}
	return nil
}

func (h *WorkboardHandler) MatchTaskProject(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	if !canWriteWorkboard(c) || isReadOnly(c) {
		return echo.ErrForbidden
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if pid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하세요")
	}
	if err := requireProjectLink(c, h.repo.GetProject, pid, false); err != nil {
		return err
	}
	t.ProjectID = pid
	t.ProjectMatchPending = false
	if err := h.repo.UpdateTask(t); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=project"))
}
