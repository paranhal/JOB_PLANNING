package handler

import (
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
	redirect := safeWorkboardRedirect(c.QueryParam("redirect"), "/workboard/board")
	org := currentOrg(c)
	wgItems, _ := h.workGroupPicks(c)
	data := map[string]interface{}{
		"Date":          dateStr,
		"Today":         today,
		"ModalRedirect": redirect,
		"KindBackLabel": "← 업무 유형 선택",
		"DailyKind":     kind,
		"CanWrite":      true,
		"Assignees":     listAssignableForPicker(h.userRepo, c),
		"WorkGroups":    wgItems,
		"ASCards":       []model.WBCard{},
		"MntCards":      []model.WBCard{},
		"Customers":     []model.Customer{},
		"Projects":      []model.WorkProject{},
		"WorkProjects":  []model.WorkProject{},
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
		customers, _ := h.customerRepo.ListAllInOrg(org)
		data["Projects"] = projects
		data["Customers"] = customers
	}
	c.Request().Header.Set("HX-Request", "true")
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
			SourceHref:   "/as/" + it.ASID,
			Assignee:     it.AssignedTo,
			PlannedDay:   it.VisitScheduledDate,
			DurationMin:  30,
			Status:       it.Status,
		})
	}
	return as, nil
}

func (h *WorkboardHandler) newWorkMntCards(orgID string) ([]model.WBCard, error) {
	if h.mntRepo == nil {
		return nil, nil
	}
	now := time.Now()
	visits, err := h.mntRepo.ListVisitsBetween(
		now.AddDate(0, 0, -7).Format(dateLayout), now.AddDate(0, 0, 45).Format(dateLayout))
	if err != nil {
		return nil, err
	}
	usedSource, err := h.repo.PlacedSourceKeys()
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	filterOrg := orgID != "" && orgID != repository.OrgAll
	if filterOrg && h.customerRepo != nil {
		custs, err := h.customerRepo.ListAllInOrg(orgID)
		if err != nil {
			return nil, err
		}
		for _, cu := range custs {
			allowed[cu.CustomerID] = true
		}
	}
	var mnt []model.WBCard
	for _, v := range visits {
		if usedSource[model.WBSourceMaintenance+":"+v.VisitID] || v.Completed {
			continue
		}
		if filterOrg && !allowed[v.CustomerID] {
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
	return mnt, nil
}
