package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *WorkboardHandler) NewWorkForm(c echo.Context) error {
	if currentRole(c) == model.RoleCLevel || isReadOnly(c) {
		return echo.ErrForbidden
	}
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	kind := strings.TrimSpace(c.QueryParam("kind"))
	switch kind {
	case model.WBWorkAS, model.WBWorkMaintenance, model.WBWorkProjectWork, model.WBWorkAdmin, model.WBWorkSales:
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "잘못된 업무 유형입니다")
	}
	dateStr := strings.TrimSpace(c.QueryParam("date"))
	if _, err := time.ParseInLocation(dateLayout, dateStr, time.Local); err != nil {
		dateStr = time.Now().Format(dateLayout)
	}
	today := time.Now().Format(dateLayout)
	redirect := safeWorkboardRedirect(c.QueryParam("redirect"), workboardHomeURL(""))
	org := currentOrg(c)
	wgItems, wgJSON := h.workGroupPicks(c)
	var extraIDs []string
	extraIDs = append(extraIDs, currentUserID(c))
	if h.groups != nil {
		for _, g := range wgItems {
			ms, _ := h.groups.MembersOn(g.GroupID, "")
			for _, m := range ms {
				extraIDs = append(extraIDs, m.UserID)
			}
		}
	}
	assignees := ensurePickerUsers(h.userRepo, listAssignableForPicker(h.userRepo, c), extraIDs...)
	seed := defaultAssigneeSeed(c)
	pickerIDs := seed.IDs
	if len(pickerIDs) == 0 {
		pickerIDs = extraIDs
	}
	data := map[string]interface{}{
		"Date":             dateStr,
		"Today":            today,
		"ModalRedirect":    redirect,
		"KindBackLabel":    "← 업무 유형 선택",
		"DailyKind":        kind,
		"CanWrite":         true,
		"Assignees":        assignees,
		"PickerNames":      pickerNamesFor(h.userRepo, assignees, extraIDs...),
		"PickerOwnerID":    seed.OwnerID,
		"PickerUserIDs":    pickerIDs,
		"AssigneeRequired": seed.Required,
		"AssigneeSeed":     seed,
		"WorkGroups":       wgItems,
		"WorkGroupsJSON":   wgJSON,
		"ASCards":          []model.WBCard{},
		"MntCards":         []model.WBCard{},
		"Customers":        []model.Customer{},
		"Projects":         []model.WorkProject{},
		"WorkProjects":     []model.WorkProject{},
	}
	switch kind {
	case model.WBWorkAS:
		cards, err := h.newWorkASCards(org)
		if err != nil {
			return err
		}
		data["ASCards"] = cards
	case model.WBWorkMaintenance:
		cards, err := h.newWorkMntCards(org)
		if err != nil {
			return err
		}
		data["MntCards"] = cards
	case model.WBWorkProjectWork:
		workProjects, _ := h.repo.ListWorkPickProjects(org)
		data["WorkProjects"] = workProjects
	case model.WBWorkAdmin, model.WBWorkSales:
		projects, _ := h.repo.ListProjects(true)
		customers, _ := h.customerRepo.ListSelectable()
		data["Projects"] = projects
		data["Customers"] = customers
	}
	attachProjectPicker(c, h, data, ProjectPickerOpts{AllowPending: kind == model.WBWorkAdmin || kind == model.WBWorkSales})
	data["ProjectPickerWork"] = buildProjectPicker(c, h, ProjectPickerOpts{})
	c.Request().Header.Set("HX-Request", "true")
	data["ItemOrgID"] = currentOrg(c)
	attachWorkItemView(data, h, nil, false)
	return c.Render(http.StatusOK, "workboard/new_work_form.html", data)
}

func (h *WorkboardHandler) newWorkASCards(orgID string) ([]model.WBCard, error) {
	if h.asRepo == nil {
		return nil, nil
	}
	if strings.TrimSpace(orgID) == "" {
		orgID = repository.OrgAll
	}
	usedSource, err := h.repo.PlacedSourceKeys()
	if err != nil {
		return nil, err
	}
	items, _, err := h.asRepo.ListFiltered(orgID, "open", "", "", nil, "", "", 1, 400)
	if err != nil {
		return nil, err
	}
	var open []model.ASListItem
	for _, it := range items {
		if usedSource[model.WBSourceAS+":"+it.ASID] {
			continue
		}
		open = append(open, it)
	}
	titles := buildASTitleMap(open)
	var as []model.WBCard
	for _, it := range open {
		if strings.TrimSpace(it.AssignedTo) == "" {
			continue
		}
		as = append(as, model.WBCard{
			Kind:         model.WBSourceAS,
			RefID:        it.ASID,
			Category:     model.WBSourceAS,
			Title:        titles[it.ASID],
			SubTitle:     it.Symptom,
			SourceNumber: it.ASNumber,
			SourceHref:   asOpenHref(it.ASID),
			Assignee:     it.AssignedTo,
			PlannedDay:   it.VisitScheduledDate,
			DurationMin:  30,
			Status:       it.Status,
		})
	}
	return as, nil
}

func (h *WorkboardHandler) mntVisitsForPalette(orgID string) ([]model.MaintenanceVisit, error) {
	if h.mntRepo == nil {
		return nil, nil
	}
	now := time.Now()
	visits, err := h.mntRepo.ListVisitsBetween(
		now.AddDate(0, 0, -7).Format(dateLayout), now.AddDate(0, 0, 45).Format(dateLayout))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(orgID) == "" || orgID == repository.OrgAll {
		return visits, nil
	}
	plans, err := h.mntRepo.ListPlansInOrg(orgID)
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	for _, p := range plans {
		ok[p.PlanID] = true
	}
	var out []model.MaintenanceVisit
	for _, v := range visits {
		if ok[v.PlanID] {
			out = append(out, v)
		}
	}
	return out, nil
}

func (h *WorkboardHandler) newWorkMntCards(orgID string) ([]model.WBCard, error) {
	if h.mntRepo == nil {
		return nil, nil
	}
	visits, err := h.mntVisitsForPalette(orgID)
	if err != nil {
		return nil, err
	}
	usedSource, err := h.repo.PlacedSourceKeys()
	if err != nil {
		return nil, err
	}
	var mnt []model.WBCard
	for _, v := range visits {
		if usedSource[model.WBSourceMaintenance+":"+v.VisitID] || v.Completed {
			continue
		}
		name := v.ShortName
		if name == "" {
			name = v.OrgName
		}
		num := mntVisitNumber(v)
		mnt = append(mnt, model.WBCard{
			Kind:         model.WBSourceMaintenance,
			RefID:        v.VisitID,
			Category:     model.WBSourceMaintenance,
			Title:        model.FormatMaintenanceWorkTitle(name, num),
			SubTitle:     model.FormatMaintenanceDescription(v.ProductType, name),
			ProductType:  v.ProductType,
			SourceNumber: num,
			SourceHref:   "/maintenance/visits/" + v.VisitID,
			Assignee:     v.Assignee,
			PlannedDay:   v.VisitDate,
			DurationMin:  30,
			Unassigned:   strings.TrimSpace(v.Assignee) == "",
		})
	}
	if yearPlans, err := h.mntRepo.ListPlansWithCountsInOrg(orgID); err == nil {
		year := time.Now().Year()
		var planID string
		for _, p := range yearPlans {
			if p.PlanYear == year {
				planID = p.PlanID
				break
			}
		}
		if planID != "" {
			targets, _ := h.mntRepo.ListActiveMonthTargets(planID, year)
			haveVisit := map[string]bool{}
			for _, v := range visits {
				if v.Completed {
					continue
				}
				haveVisit[v.CustomerID] = true
			}
			for _, tg := range targets {
				if haveVisit[tg.CustomerID] {
					continue
				}
				cfg, _ := h.mntRepo.GetSiteConfig(tg.CustomerID)
				name := tg.CustomerID
				if cfg != nil {
					name = cfg.ShortName
					if name == "" {
						name = cfg.OrgName
					}
				}
				num := fmt.Sprintf("%d월대상", tg.Month)
				mnt = append(mnt, model.WBCard{
					Kind:         model.WBSourceMaintenance,
					RefID:        "mt:" + tg.TargetID,
					Category:     model.WBSourceMaintenance,
					Title:        model.FormatMaintenanceWorkTitle(name, num),
					SubTitle:     "월 점검 대상",
					SourceNumber: num,
					SourceHref:   "/maintenance/" + planID + "/manage",
					PlannedDay:   "",
					DurationMin:  30,
					Unassigned:   true,
				})
			}
		}
	}
	return mnt, nil
}

func (h *WorkboardHandler) MntCandidates(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	q := strings.ToLower(strings.TrimSpace(c.QueryParam("q")))
	cards, err := h.newWorkMntCards(currentOrg(c))
	if err != nil {
		return err
	}
	type item struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	var out []item
	for _, c0 := range cards {
		blob := strings.ToLower(c0.Title + " " + c0.SourceNumber + " " + c0.ProductType + " " + c0.SubTitle)
		if q != "" && !strings.Contains(blob, q) {
			continue
		}
		out = append(out, item{ID: c0.RefID, Label: c0.Title + " · " + c0.SourceNumber})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": out})
}

func (h *WorkboardHandler) resolveMntSourceID(c echo.Context, sourceID, workDate string, sel Selection) (string, error) {
	sourceID = strings.TrimSpace(sourceID)
	if !strings.HasPrefix(sourceID, "mt:") {
		return sourceID, nil
	}
	if h.mntRepo == nil {
		return "", fmt.Errorf("정기점검 저장소를 쓸 수 없습니다")
	}
	tid := strings.TrimPrefix(sourceID, "mt:")
	tg, err := h.mntRepo.GetMonthTarget(tid)
	if err != nil || tg == nil {
		return "", fmt.Errorf("월 대상을 찾을 수 없습니다")
	}
	v := model.MaintenanceVisit{
		PlanID: tg.PlanID, VisitDate: workDate, CustomerID: tg.CustomerID,
		ProductType: "", EntryCategory: "normal", Assignee: sel.PrimaryName,
	}
	return h.mntRepo.AssignSlotID(v)
}

func (h *WorkboardHandler) ensureMntVisitForCustomer(c echo.Context, customerID, workDate string, sel Selection) (string, error) {
	if h.mntRepo == nil {
		return "", fmt.Errorf("정기점검 저장소를 쓸 수 없습니다")
	}
	cfg, err := h.mntRepo.GetSiteConfig(customerID)
	if err != nil || cfg == nil {
		return "", fmt.Errorf("점검 사이트가 아닙니다")
	}
	org := strings.TrimSpace(cfg.OrgID)
	if org == "" {
		org = currentOrg(c)
	}
	plan, err := h.mntRepo.EnsureYearPlan(org, time.Now().Year())
	if err != nil {
		return "", err
	}
	v := model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: workDate, CustomerID: customerID,
		EntryCategory: "normal", Assignee: sel.PrimaryName,
	}
	return h.mntRepo.AssignSlotID(v)
}
