package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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
	OwnOrgID           string
	PreferYear         int
	EmptyListHint      string
	AlwaysInclude      string
	HideOther          bool
	ShowSiteNote       bool
	UseSiteProjects    bool
	SiteCustomerID     string
	SiteCustomerIDs    []string
	SiteOnly           bool
	ShowAllSiteRows    bool
	ConfirmLead        string
	DispatchSelected   bool
}

const siteProjectEmptyHint = "이 사이트가 등록된 사업이 없습니다. 사업 → 사이트 등록에서 이 사이트를 먼저 등록하세요"

const visitSiteProjectEmptyHint = "이 사이트가 등록된 사업이 없습니다. 사업명을 검색해 연결하세요"

const asSiteProjectEmptyHint = "이 기관이 등록된 사업이 없습니다. 사업명을 검색해 연결하세요"

type projectPickerItem struct {
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	ProjectKind string `json:"project_kind,omitempty"`
	PlanYear    int    `json:"plan_year,omitempty"`
	Related     bool   `json:"related"`
	ChildOrg    string `json:"child_org,omitempty"`
	ClosedLabel string `json:"closed_label,omitempty"`
	Current     bool   `json:"current,omitempty"`
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
	EmptyListHint      string              `json:"EmptyListHint"`
	HideOther          bool                `json:"HideOther"`
	ShowSiteNote       bool                `json:"ShowSiteNote"`
	SiteCount          int                 `json:"SiteCount"`
	Site               []projectPickerItem `json:"Site"`
	SiteOnly           bool                `json:"SiteOnly"`
	ShowAllSiteRows    bool                `json:"ShowAllSiteRows"`
	PreferYear         int                 `json:"PreferYear,omitempty"`
	ConfirmLead        string              `json:"ConfirmLead,omitempty"`
	DispatchSelected   bool                `json:"DispatchSelected,omitempty"`
}

func pickerAccessOrg(c echo.Context) string {
	if model.NormalizeRole(currentRole(c)) == model.RoleVisionAdmin {
		return "*"
	}
	org := pickerHomeOrg(c)
	if org == repository.OrgAll {
		return "*"
	}
	return org
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
		PlanYear:    p.PlanYear,
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

// pickerProjectsCached 한 요청 안에서 같은 사업 목록을 두 번 불러오지 않는다.
// 정기점검 화면은 사업 선택기를 둘(일반·방문 편집) 만들고, 95-144C 부터 방문 편집 선택기도 전체 목록을
// 쓰기 때문에 같은 쿼리가 두 번 나갔다(Test95_139 쿼리 상한). 목록은 선택기 옵션과 무관하다.
func pickerProjectsCached(c echo.Context, h *WorkboardHandler, includeClosed bool) []model.WorkProject {
	key := "picker.projects.open"
	if includeClosed {
		key = "picker.projects.all"
	}
	if c != nil {
		if v, ok := c.Get(key).([]model.WorkProject); ok {
			return append([]model.WorkProject(nil), v...)
		}
	}
	list := loadPickerProjects(h, includeClosed)
	if c != nil {
		c.Set(key, append([]model.WorkProject(nil), list...))
	}
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
	loginHome := pickerHomeOrg(c)
	extra := pickerExtraOrgIDs(c)
	ownHome := strings.TrimSpace(opts.OwnOrgID)
	ownExtra := extra
	if ownHome != "" {
		ownExtra = nil
	} else {
		ownHome = loginHome
	}
	role := currentRole(c)
	canOther := model.CanPickOtherProjects(role)
	all := filterPickerStatus(pickerProjectsCached(c, h, opts.IncludeClosed), opts.IncludeClosed)
	var ownSrc, otherSrc []model.WorkProject
	var recency map[string]int64
	siteCount := -1
	var siteSrc []model.WorkProject
	if opts.UseSiteProjects {
		cids := append([]string{}, opts.SiteCustomerIDs...)
		if cid := strings.TrimSpace(opts.SiteCustomerID); cid != "" {
			cids = append(cids, cid)
		}
		if len(cids) > 0 && h != nil && h.repo != nil {
			ownSrc, recency, _ = repository.NewProjectRepo(h.repo.DB()).ListBySiteCustomers(cids, firstNonEmptyID(opts.AlwaysInclude, opts.Selected))
		}
		ownSrc = sortPickerOwnRecency(ownSrc, opts.PreferYear, opts.Selected, recency)
		siteSrc = append([]model.WorkProject(nil), ownSrc...)
		siteN := len(siteSrc)
		ownSrc = ensureSelectedProject(h, ownSrc, opts.AlwaysInclude, opts.Selected)
		ownSrc = sortPickerOwnRecency(ownSrc, opts.PreferYear, opts.Selected, recency)
		if opts.SiteOnly {
			otherSrc = nil
			canOther = false
		} else {
			var orgOwn []model.WorkProject
			orgOwn, otherSrc = splitPickerOwnOther(all, loginHome, extra)
			ownIDs := map[string]bool{}
			for _, p := range ownSrc {
				ownIDs[p.ProjectID] = true
			}
			for _, p := range orgOwn {
				if !ownIDs[p.ProjectID] {
					ownSrc = append(ownSrc, p)
					ownIDs[p.ProjectID] = true
				}
			}
			var rest []model.WorkProject
			for _, p := range otherSrc {
				if !ownIDs[p.ProjectID] {
					rest = append(rest, p)
				}
			}
			otherSrc = rest
		}
		siteCount = siteN
	} else if strings.TrimSpace(opts.OwnOrgID) != "" {
		ownSrc, otherSrc = splitPickerHostOrg(all, ownHome)
		ownSrc = ensureSelectedProject(h, ownSrc, opts.AlwaysInclude, opts.Selected)
		ownSrc = sortPickerOwn(ownSrc, opts.PreferYear, opts.Selected)
	} else {
		ownSrc, otherSrc = splitPickerOwnOther(all, ownHome, ownExtra)
		ownSrc = ensureSelectedProject(h, ownSrc, opts.AlwaysInclude, opts.Selected)
		ownSrc = sortPickerOwn(ownSrc, opts.PreferYear, opts.Selected)
	}
	if opts.HideOther {
		canOther = false
		otherSrc = nil
	}
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
		EmptyListHint:      strings.TrimSpace(opts.EmptyListHint),
		HideOther:          opts.HideOther,
		ShowSiteNote:       opts.ShowSiteNote,
		SiteOnly:           opts.SiteOnly,
		ShowAllSiteRows:    opts.ShowAllSiteRows,
		PreferYear:         opts.PreferYear,
		ConfirmLead:        strings.TrimSpace(opts.ConfirmLead),
		DispatchSelected:   opts.DispatchSelected,
	}
	if siteCount >= 0 {
		view.SiteCount = siteCount
		for _, p := range siteSrc {
			view.Site = append(view.Site, toPickerItem(p, ownHome, ownExtra))
		}
		if view.Site == nil {
			view.Site = []projectPickerItem{}
		}
	}
	for _, p := range ownSrc {
		view.Own = append(view.Own, toPickerItem(p, ownHome, ownExtra))
	}
	if opts.SiteOnly {
		inSite := map[string]bool{}
		for _, p := range view.Site {
			inSite[p.ProjectID] = true
		}
		sel := strings.TrimSpace(opts.Selected)
		for i := range view.Own {
			id := view.Own[i].ProjectID
			if id == "" {
				continue
			}
			if sel != "" && id == sel {
				view.Own[i].Current = true
				continue
			}
			if !inSite[id] {
				view.Own[i].Current = true
			}
		}
	}
	if canOther {
		view.OtherCount = len(otherSrc)
	}
	if view.Selected != "" && view.SelectedName == "" {
		for _, p := range append(ownSrc, all...) {
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
	ownHome := strings.TrimSpace(opts.OwnOrgID)
	extra := pickerExtraOrgIDs(c)
	if ownHome != "" {
		extra = nil
	} else {
		ownHome = pickerHomeOrg(c)
	}
	viewOwn := view.Own
	var own []model.WorkProject
	if opts.UseSiteProjects {
		for _, it := range viewOwn {
			own = append(own, model.WorkProject{ProjectID: it.ProjectID, Name: it.Name, ProjectKind: it.ProjectKind})
		}
	} else {
		all := filterPickerStatus(loadPickerProjects(h, opts.IncludeClosed), opts.IncludeClosed)
		if strings.TrimSpace(opts.OwnOrgID) != "" {
			own, _ = splitPickerHostOrg(all, ownHome)
		} else {
			own, _ = splitPickerOwnOther(all, ownHome, extra)
		}
		own = ensureSelectedProject(h, own, opts.AlwaysInclude, opts.Selected)
		own = sortPickerOwn(own, opts.PreferYear, opts.Selected)
	}
	data["WorkProjects"] = own
}

func splitPickerHostOrg(all []model.WorkProject, org string) (own, other []model.WorkProject) {
	org = strings.TrimSpace(org)
	for _, p := range all {
		if org != "" && strings.TrimSpace(p.OrgID) == org {
			own = append(own, p)
			continue
		}
		other = append(other, p)
	}
	return own, other
}

func ensureSelectedProject(h *WorkboardHandler, own []model.WorkProject, always, selected string) []model.WorkProject {
	id := strings.TrimSpace(always)
	if id == "" {
		id = strings.TrimSpace(selected)
	}
	if id == "" || h == nil || h.repo == nil {
		return own
	}
	for _, p := range own {
		if p.ProjectID == id {
			return own
		}
	}
	p, err := h.repo.GetProject(id)
	if err != nil || p == nil {
		return own
	}
	return append([]model.WorkProject{*p}, own...)
}

func sortPickerOwn(own []model.WorkProject, year int, selected string) []model.WorkProject {
	return sortPickerOwnRecency(own, year, selected, nil)
}

func sortPickerOwnRecency(own []model.WorkProject, year int, selected string, recency map[string]int64) []model.WorkProject {
	sel := strings.TrimSpace(selected)
	sort.SliceStable(own, func(i, j int) bool {
		if sel != "" {
			if own[i].ProjectID == sel {
				return true
			}
			if own[j].ProjectID == sel {
				return false
			}
		}
		if year > 0 {
			yi := own[i].PlanYear == year
			yj := own[j].PlanYear == year
			if yi != yj {
				return yi
			}
		}
		if len(recency) > 0 {
			ri, rj := recency[own[i].ProjectID], recency[own[j].ProjectID]
			if ri != rj {
				return ri > rj
			}
		}
		return strings.TrimSpace(own[i].Name) < strings.TrimSpace(own[j].Name)
	})
	return own
}

func firstNonEmptyID(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func visitPlanYear(visitDate string) int {
	visitDate = strings.TrimSpace(visitDate)
	if len(visitDate) < 4 {
		return 0
	}
	y, err := strconv.Atoi(visitDate[:4])
	if err != nil {
		return 0
	}
	return y
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

func (h *WorkboardHandler) ListProjectsForSite(c echo.Context) error {
	if strings.TrimSpace(currentUserID(c)) == "" {
		return c.JSON(http.StatusForbidden, map[string]interface{}{"own": []projectPickerItem{}, "count": 0})
	}
	var cids []string
	for _, id := range strings.Split(c.QueryParam("customer_id"), ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			cids = append(cids, id)
		}
	}
	include := strings.TrimSpace(c.QueryParam("include"))
	year, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("year")))
	var own []model.WorkProject
	var recency map[string]int64
	if len(cids) > 0 && h != nil && h.repo != nil {
		own, recency, _ = repository.NewProjectRepo(h.repo.DB()).ListBySiteCustomers(cids, include)
	}
	own = sortPickerOwnRecency(own, year, include, recency)
	siteItems := make([]projectPickerItem, 0, len(own))
	for _, p := range own {
		siteItems = append(siteItems, toPickerItem(p, "", nil))
	}
	own = ensureSelectedProject(h, own, include, include)
	own = sortPickerOwnRecency(own, year, include, recency)
	items := make([]projectPickerItem, 0, len(own))
	for _, p := range own {
		items = append(items, toPickerItem(p, "", nil))
	}
	inSite := map[string]bool{}
	for _, it := range siteItems {
		inSite[it.ProjectID] = true
	}
	for i := range items {
		if items[i].ProjectID != "" && !inSite[items[i].ProjectID] {
			items[i].Current = true
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"own": items, "site": siteItems, "count": len(siteItems)})
}

func (h *WorkboardHandler) SearchWorkProjects(c echo.Context) error {
	if strings.TrimSpace(currentUserID(c)) == "" {
		return c.JSON(http.StatusForbidden, map[string]interface{}{"items": []projectPickerItem{}})
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	items := []projectPickerItem{}
	if q == "" || utf8.RuneCountInString(q) < 2 {
		return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
	}
	year, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("year")))
	home := pickerHomeOrg(c)
	extra := pickerExtraOrgIDs(c)
	role := currentRole(c)
	all := filterPickerStatus(loadPickerProjects(h, false), false)
	for _, p := range all {
		if !model.CanLinkWorkProject(p, home, role, extra, false) {
			continue
		}
		if !matchProjectQuery(p, q) {
			continue
		}
		items = append(items, toPickerItem(p, home, extra))
	}
	sortSearchHits(items, year)
	if len(items) > 30 {
		items = items[:30]
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

func sortSearchHits(items []projectPickerItem, year int) {
	sort.SliceStable(items, func(i, j int) bool {
		if year > 0 {
			yi := items[i].PlanYear == year
			yj := items[j].PlanYear == year
			if yi != yj {
				return yi
			}
		}
		if items[i].PlanYear != items[j].PlanYear {
			return items[i].PlanYear > items[j].PlanYear
		}
		return strings.TrimSpace(items[i].Name) < strings.TrimSpace(items[j].Name)
	})
}

func (h *WorkboardHandler) LinkProjectSiteCustomers(c echo.Context) error {
	if strings.TrimSpace(currentUserID(c)) == "" {
		return echo.NewHTTPError(http.StatusForbidden, "로그인하세요")
	}
	if strings.TrimSpace(c.FormValue("confirm")) != "1" {
		return echo.NewHTTPError(http.StatusBadRequest, "확인 후에만 등록합니다")
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if pid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하세요")
	}
	if err := requireProjectLink(c, h.repo.GetProject, pid, false); err != nil {
		return err
	}
	p, err := h.repo.GetProject(pid)
	if err != nil || p == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	_ = c.Request().ParseForm()
	ids := append([]string{}, c.Request().Form["customer_id"]...)
	if len(ids) == 1 && strings.Contains(ids[0], ",") {
		ids = strings.Split(ids[0], ",")
	}
	if len(ids) == 0 {
		if v := strings.TrimSpace(c.FormValue("customer_id")); v != "" {
			ids = strings.Split(v, ",")
		}
	}
	res, err := repository.NewProjectEquipRepo(h.repo.DB()).AddSitesFromCustomers(pid, strings.TrimSpace(p.OrgID), currentUserID(c), ids)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"added": res.Added, "skipped": res.Skipped})
}

func isGeneralWorkEdit(t *model.WorkTask, editMode bool) bool {
	if !editMode || t == nil {
		return false
	}
	if strings.TrimSpace(t.SourceType) != "" {
		return false
	}
	switch t.WorkType {
	case model.WBWorkAS, model.WBWorkMaintenance, model.WBWorkProjectWork:
		return false
	default:
		return true
	}
}

func projectPickerOptsForTask(t *model.WorkTask, sites []repository.WorkTaskSite, editMode, pending bool) ProjectPickerOpts {
	opts := ProjectPickerOpts{AllowPending: pending}
	if t == nil {
		return opts
	}
	opts.Selected = t.ProjectID
	opts.SelectedName = t.ProjectName
	opts.Pending = t.ProjectMatchPending
	if !isGeneralWorkEdit(t, editMode) {
		return opts
	}
	opts.UseSiteProjects = true
	opts.SiteOnly = true
	opts.HideOther = true
	opts.ShowSiteNote = true
	opts.ShowAllSiteRows = true
	opts.AlwaysInclude = t.ProjectID
	opts.SiteCustomerID = t.CustomerID
	opts.AllowQuickRegister = false
	opts.EmptyListHint = "연결된 사업이 없습니다. 사업명을 검색해 이 거래처를 사이트로 등록하세요"
	for _, s := range sites {
		if id := strings.TrimSpace(s.SiteID); id != "" {
			opts.SiteCustomerIDs = append(opts.SiteCustomerIDs, id)
		}
	}
	return opts
}

func projectPickerOptsForVisit(customerID, siteName, selectedID, selectedName, visitDate string) ProjectPickerOpts {
	lead := "이 사이트를 이 사업의 사이트로 등록하고 이 방문을 연결합니다"
	if n := strings.TrimSpace(siteName); n != "" {
		lead = "「" + n + "」를 이 사업의 사이트로 등록하고 이 방문을 연결합니다"
	}
	return ProjectPickerOpts{
		Selected:        selectedID,
		SelectedName:    selectedName,
		PreferYear:      visitPlanYear(visitDate),
		AlwaysInclude:   selectedID,
		EmptyListHint:   visitSiteProjectEmptyHint,
		ShowSiteNote:    true,
		UseSiteProjects: true,
		SiteCustomerID:  customerID,
		SiteOnly:        true,
		HideOther:       true,
		ShowAllSiteRows: true,
		ConfirmLead:     lead,
	}
}

func projectPickerOptsForAS(customerID, orgName, selectedID, selectedName string, preferYear int) ProjectPickerOpts {
	lead := "이 기관을 이 사업의 사이트로 등록하고 이 AS를 연결합니다"
	if n := strings.TrimSpace(orgName); n != "" {
		lead = "「" + n + "」을 이 사업의 사이트로 등록하고 이 AS를 연결합니다"
	}
	return ProjectPickerOpts{
		Selected:        selectedID,
		SelectedName:    selectedName,
		PreferYear:      preferYear,
		AlwaysInclude:   selectedID,
		EmptyListHint:   asSiteProjectEmptyHint,
		ShowSiteNote:    true,
		UseSiteProjects: true,
		SiteCustomerID:  customerID,
		SiteOnly:        true,
		HideOther:       true,
		ShowAllSiteRows: true,
		ConfirmLead:     lead,
	}
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
