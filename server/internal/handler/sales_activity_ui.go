package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type salesTypeChip struct {
	Code     string
	Label    string
	Count    int
	Selected bool
	Href     string
}

func salesActivitiesPageView(c echo.Context) string {
	v := strings.TrimSpace(c.QueryParam("view"))
	switch v {
	case "kanban":
		return "kanban"
	case "day":
		return "day"
	case "project":
		return "project"
	case "log", "list":
		return "log"
	}
	hasDate := strings.TrimSpace(c.QueryParam("date")) != ""
	q := strings.TrimSpace(c.QueryParam("q"))
	month := strings.TrimSpace(c.QueryParam("month"))
	types := parseSalesActivityTypeFilter(c)
	methods := parseSalesActivityMethodFilter(c)
	if hasDate && q == "" && month == "" && len(types) == 0 && len(methods) == 0 {
		return "day"
	}
	return "log"
}

func parseSalesActivityTypeFilter(c echo.Context) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range c.QueryParams()["type"] {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" || p == "all" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func parseSalesActivityMethodFilter(c echo.Context) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range c.QueryParams()["method"] {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" || p == "all" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func salesActivityKanbanColumns(acts []model.SalesActivity, group string, types []model.Code, stages []model.SalesStageDef) []model.KanbanColumn {
	raw := salesActivityKanban(acts, group, types, stages)
	out := make([]model.KanbanColumn, 0, len(raw))
	for _, c := range raw {
		col := model.KanbanColumn{Key: c.Key, Title: c.Label, Sort: c.Sort, Border: "border-slate-200"}
		for _, a := range c.Items {
			title := strings.TrimSpace(a.Title)
			if title == "" {
				title = a.TypeLabel
			}
			col.Items = append(col.Items, model.KanbanCard{
				ID: a.ActivityID, RefID: a.ActivityID, Title: title,
				Href: "/sales/" + a.SalesID, OrgName: a.SalesName,
				Assignee: a.OurMembers, TypeLabel: a.TypeLabel,
				Extra:  strings.TrimSpace(a.ActivityDate + " " + a.StartTime),
				Bucket: c.Key, Stage: c.Key,
			})
		}
		col.Count = len(col.Items)
		out = append(out, col)
	}
	return out
}

func salesActLogQuery(month, q string, types []string) string {
	return salesActLogQueryOwner(month, q, types, "")
}

func salesActLogQueryOwner(month, q string, types []string, owner string) string {
	return salesActLogQueryOwner2(month, q, types, nil, owner)
}

func salesActLogQueryOwner2(month, q string, types, methods []string, owner string) string {
	v := url.Values{}
	if month != "" {
		v.Set("month", month)
	}
	if q != "" {
		v.Set("q", q)
	}
	if strings.TrimSpace(owner) != "" {
		v.Set("owner", owner)
	}
	for _, t := range types {
		v.Add("type", t)
	}
	for _, m := range methods {
		v.Add("method", m)
	}
	return v.Encode()
}

func toggleTypeFilter(cur []string, code string) []string {
	var out []string
	found := false
	for _, t := range cur {
		if t == code {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		out = append(out, code)
	}
	return out
}

func decorateActivityDayGroups(groups []model.SalesActivityDayGroup) {
	for i := range groups {
		tn := toneForDate(groups[i].Date, "")
		groups[i].DayStyle = tn.DayStyle
		groups[i].DayClass = tn.DayClass
	}
}

func salesMonthEmptyLabel(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return month + "에 등록된 활동이 없습니다"
	}
	return t.Format("2006년 1월") + "에 등록된 활동이 없습니다"
}

func (h *SalesHandler) renderActivitiesLogData(c echo.Context) (map[string]interface{}, error) {
	monthParam := strings.TrimSpace(c.QueryParam("month"))
	allPeriod := monthParam == "all"
	month := monthParam
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if !allPeriod && len(month) > 7 {
		month = month[:7]
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	sel := parseSalesActivityTypeFilter(c)
	selM := parseSalesActivityMethodFilter(c)
	types, _ := h.repo.ActivityTypes()
	methods, _ := h.repo.ActivityMethods()
	filterMonth := month
	if allPeriod {
		filterMonth = ""
	}
	period := strings.TrimSpace(c.QueryParam("period"))
	if period == "" {
		if allPeriod {
			period = "all"
		} else {
			period = "month"
		}
	}
	today := todayForAct(c)
	baseF := visibleActivityFilter(c)
	visActs, err := h.repo.ListActivitiesFilter(baseF)
	if err != nil {
		return nil, err
	}
	workF := baseF
	workF.Query = q
	if period == "30" {
		from, to := model.PeriodRange("30", today)
		workF.DateFrom, workF.DateTo = from, to
	} else if !allPeriod && period != "all" {
		workF.Month = filterMonth
	}
	workActs, err := h.repo.ListActivitiesFilter(workF)
	if err != nil {
		return nil, err
	}
	chipActs := workActs
	actF := workF
	actF.Types = sel
	actF.Methods = selM
	acts, err := h.repo.ListActivitiesFilter(actF)
	if err != nil {
		return nil, err
	}
	allActs := visActs
	typeCounts := map[string]int{}
	methodCounts := map[string]int{}
	for _, a := range chipActs {
		typeCounts[a.ActivityType]++
		methodCounts[a.ActivityMethod]++
	}
	var chips []salesTypeChip
	var methodChips []salesTypeChip
	chipMonth := month
	if allPeriod {
		chipMonth = "all"
	}
	owner := SalesVisibilityScope(c).Owner
	for _, code := range model.SalesActivityTypeChipCodes() {
		label := model.SalesActTypeDisplayName(code)
		for _, t := range types {
			if t.CodeValue == code {
				label = t.CodeName
				break
			}
		}
		chips = append(chips, salesTypeChip{
			Code:     code,
			Label:    label,
			Count:    typeCounts[code],
			Selected: containsStr(sel, code),
			Href:     "/sales/activities?" + salesActLogQueryOwner2(chipMonth, q, toggleTypeFilter(sel, code), selM, owner),
		})
	}
	for _, code := range model.SalesActivityMethodChipCodes() {
		label := model.SalesActMethodDisplayName(code)
		for _, m := range methods {
			if m.CodeValue == code {
				label = m.CodeName
				break
			}
		}
		methodChips = append(methodChips, salesTypeChip{
			Code:     code,
			Label:    label,
			Count:    methodCounts[code],
			Selected: containsStr(selM, code),
			Href:     "/sales/activities?" + salesActLogQueryOwner2(chipMonth, q, sel, toggleTypeFilter(selM, code), owner),
		})
	}
	optF := repository.SalesListFilter{IncludeClosed: true, IncludeDormant: true}
	applySalesVisibilityToList(c, &optF)
	projects, _ := h.repo.ListFilter(optF)
	bizCodes, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupBizType)
	validBiz := map[string]bool{}
	for _, b := range bizCodes {
		validBiz[b.CodeValue] = true
	}
	hasUnset := false
	projByID := map[string]model.SalesProject{}
	for i := range projects {
		projByID[projects[i].SalesID] = projects[i]
		if strings.TrimSpace(projects[i].BizType) == "" {
			hasUnset = true
		}
	}
	selBiz, bizAll, bizNone := parseActBizTypes(c, validBiz)
	wantIDs, idAll, idNone := parseActProjectIDs(c)
	if idNone {
		wantIDs = nil
		idAll = false
	}
	visibleOpts, selectedIDs, emptyClass, emptyProj := applyActProjectScope(visActs, projects, bizAll, selBiz, idAll && !idNone, wantIDs)
	kpiActs := filterActsBySales(workActs, selectedIDs)
	if emptyClass || emptyProj {
		kpiActs = nil
	}
	latest := model.LatestActivityIDBySales(visActs)
	kpi := model.BuildSalesActivityKPI(kpiActs, latest, today)
	statusF := strings.TrimSpace(c.QueryParam("status"))
	tableActs := kpiActs
	if len(sel) > 0 {
		var typed []model.SalesActivity
		for _, a := range tableActs {
			if containsStr(sel, a.ActivityType) {
				typed = append(typed, a)
			}
		}
		tableActs = typed
	}
	if len(selM) > 0 {
		var typed []model.SalesActivity
		for _, a := range tableActs {
			if containsStr(selM, a.ActivityMethod) {
				typed = append(typed, a)
			}
		}
		tableActs = typed
	}
	if statusF != "" && statusF != "all" {
		var st []model.SalesActivity
		for _, a := range tableActs {
			if model.DeriveSalesActivityStatus(a, latest, today) == statusF {
				st = append(st, a)
			}
		}
		tableActs = st
	}
	pageN, _ := strconv.Atoi(c.QueryParam("page"))
	pageSlice, pageN, pages := paginateActs(tableActs, pageN, actPageSize)
	groups := model.GroupSalesActivitiesByDate(pageSlice)
	decorateActivityDayGroups(groups)
	rows := make([]map[string]interface{}, 0, len(pageSlice))
	for _, a := range pageSlice {
		st := model.DeriveSalesActivityStatus(a, latest, today)
		ownerName := strings.TrimSpace(a.OurMembers)
		if ownerName == "" {
			ownerName = a.CreatedBy
		}
		rows = append(rows, map[string]interface{}{
			"Activity":    a,
			"Status":      st,
			"StatusLabel": model.SalesActivityStatusLabel(st),
			"TypeKO":      model.SalesActivityTypeKO(a.ActivityType, a.TypeLabel),
			"MethodKO":    model.SalesActMethodDisplayName(a.ActivityMethod),
			"Owner":       ownerName,
		})
	}
	bizCSV := ""
	if !bizAll && !bizNone {
		bizCSV = encodeCSV(selBiz)
	}
	if bizNone {
		bizCSV = actProjectNone
	}
	pidCSV := ""
	if emptyProj || idNone {
		pidCSV = actProjectNone
	} else if !idAll && len(selectedIDs) > 0 && len(selectedIDs) < len(visibleOpts) {
		pidCSV = encodeCSV(selectedIDs)
	}
	qvals, _ := url.ParseQuery(salesActQuery2(chipMonth, q, period, statusF, bizCSV, pidCSV, owner, sel, selM, 0))
	pageLinks := actPageLinks(qvals, pageN, pages)
	bizChips := buildActBizChips(c, visActs, projByID, bizCodes, hasUnset, selBiz, bizAll, bizNone, chipMonth, q, period, statusF, pidCSV, owner, sel)
	var users []model.User
	if h.userRepo != nil {
		users = listAssignableForPicker(h.userRepo, c)
	}
	typeQ := salesActLogQueryOwner2("", q, sel, selM, owner)
	if typeQ != "" {
		typeQ = "&" + typeQ
	}
	hasAny := len(allActs) > 0
	showHeaderAdd := canWriteSales(c) && hasAny
	appliedIDs := selectedIDs
	if idAll {
		appliedIDs = nil
		for _, o := range visibleOpts {
			appliedIDs = append(appliedIDs, o.SalesID)
		}
	}
	badge := fmt.Sprintf("%d개 사업", len(visibleOpts))
	if !idAll || emptyProj {
		badge = fmt.Sprintf("%d / %d개 사업 선택", len(selectedIDs), len(visibleOpts))
	}
	resetHref := "/sales/activities?" + salesActQuery2(chipMonth, q, period, statusF, "", "", owner, nil, nil, 0)
	data := map[string]interface{}{
		"Title":           "영업 활동",
		"Active":          NavSalesActivities,
		"View":            "log",
		"Month":           chipMonth,
		"Period":          period,
		"Query":           q,
		"StatusFilter":    statusF,
		"SelectedTypes":   sel,
		"SelectedMethods": selM,
		"Summary":         model.BuildSalesActivitySummary(acts, types),
		"KPI":             kpi,
		"TypeChips":       chips,
		"MethodChips":     methodChips,
		"ChipTotal":       len(chipActs),
		"DayGroups":       groups,
		"Activities":      pageSlice,
		"ActRows":         rows,
		"HasAnyActivity":  hasAny,
		"ShowHeaderAdd":   showHeaderAdd,
		"MonthEmptyLabel": salesMonthEmptyLabel(month),
		"AllPeriodHref":   "/sales/activities?" + salesActLogQueryOwner2("all", q, sel, selM, owner),
		"LogQuery":        salesActQuery2(chipMonth, q, period, statusF, bizCSV, pidCSV, owner, sel, selM, 0),
		"TypeQuery":       typeQ,
		"ActivityTypes":   types,
		"ActivityMethods": methods,
		"TypeGroups":      model.SalesActTypeGroups("", types),
		"SalesOptions":    projects,
		"Today":           today,
		"CanWrite":        canWriteSales(c),
		"CanEditOthers":   isAdminRole(c) || isOfficeRole(c),
		"CurrentUser":     ctxString(c, "user_name"),
		"FormAction":      "/sales/activities",
		"FixedSalesID":    "",
		"DefaultOwner":    ctxString(c, "user_name"),
		"Users":           users,
		"ShowOwnerFilter": SalesVisibilityScope(c).ShowOwnerFilter,
		"Owner":           SalesVisibilityScope(c).Owner,
		"PartyHints":      []string{},
		"ActGroup":        salesActGroup(c.QueryParam("group")),
		"KanbanDrop":      "act",
		"KanbanDrag":      canWriteSales(c),
		"KanbanHint":      "열 = 활동 유형. 카드를 끌어 옮깁니다.",
		"FilterQ":         "",
		"BizChips":        bizChips,
		"ProjectOpts":     visibleOpts,
		"ProjectBadge":    badge,
		"EmptyClass":      emptyClass,
		"EmptyProject":    emptyProj,
		"PartialSelect":   !idAll && !emptyProj && len(selectedIDs) > 0,
		"ResetHref":       resetHref,
		"BizCSV":          bizCSV,
		"ProjectCSV":      pidCSV,
		"PageLinks":       pageLinks,
		"Page":            pageN,
		"Pages":           pages,
		"TableTotal":      len(tableActs),
		"AppliedIDs":      strings.Join(appliedIDs, ","),
		"VisibleIDs":      csvOfOpts(visibleOpts),
	}
	return data, nil
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (h *SalesHandler) renderSalesActMain(c echo.Context, data map[string]interface{}) error {
	files := []string{}
	if partials, err := filepath.Glob("web/templates/sales/_*.html"); err == nil {
		files = append(files, partials...)
	}
	if len(files) == 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "활동 템플릿이 없습니다")
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(c.Response().Writer, "sales_activity_log", data)
}
