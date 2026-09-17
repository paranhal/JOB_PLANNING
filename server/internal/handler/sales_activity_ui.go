package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
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
	case "log", "list":
		return "log"
	}
	hasDate := strings.TrimSpace(c.QueryParam("date")) != ""
	q := strings.TrimSpace(c.QueryParam("q"))
	month := strings.TrimSpace(c.QueryParam("month"))
	types := parseSalesActivityTypeFilter(c)
	if hasDate && q == "" && month == "" && len(types) == 0 {
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
	v := url.Values{}
	if month != "" {
		v.Set("month", month)
	}
	if q != "" {
		v.Set("q", q)
	}
	for _, t := range types {
		v.Add("type", t)
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
	types, _ := h.repo.ActivityTypes()
	filterMonth := month
	if allPeriod {
		filterMonth = ""
	}
	chipActs, err := h.repo.ListActivitiesFilter(repository.SalesActivityFilter{Month: filterMonth, Query: q})
	if err != nil {
		return nil, err
	}
	acts, err := h.repo.ListActivitiesFilter(repository.SalesActivityFilter{Month: filterMonth, Query: q, Types: sel})
	if err != nil {
		return nil, err
	}
	allActs, err := h.repo.ListActivitiesFilter(repository.SalesActivityFilter{Query: q})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, a := range chipActs {
		counts[a.ActivityType]++
	}
	var chips []salesTypeChip
	chipMonth := month
	if allPeriod {
		chipMonth = "all"
	}
	for _, code := range model.SalesActivityChipCodes() {
		label := code
		for _, t := range types {
			if t.CodeValue == code {
				label = t.CodeName
				break
			}
		}
		chips = append(chips, salesTypeChip{
			Code:     code,
			Label:    label,
			Count:    counts[code],
			Selected: containsStr(sel, code),
			Href:     "/sales/activities?" + salesActLogQuery(chipMonth, q, toggleTypeFilter(sel, code)),
		})
	}
	groups := model.GroupSalesActivitiesByDate(acts)
	decorateActivityDayGroups(groups)
	projects, _ := h.repo.List("", model.SalesStatusActive, "")
	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	typeQ := salesActLogQuery("", q, sel)
	if typeQ != "" {
		typeQ = "&" + typeQ
	}
	logQuery := salesActLogQuery(chipMonth, q, sel)
	hasAny := len(allActs) > 0
	showHeaderAdd := canWriteSales(c) && hasAny
	data := map[string]interface{}{
		"Title":           "영업 활동",
		"Active":          NavSalesActivities,
		"View":            "log",
		"Month":           chipMonth,
		"Query":           q,
		"SelectedTypes":   sel,
		"Summary":         model.BuildSalesActivitySummary(acts, types),
		"TypeChips":       chips,
		"ChipTotal":       len(chipActs),
		"DayGroups":       groups,
		"HasAnyActivity":  hasAny,
		"ShowHeaderAdd":   showHeaderAdd,
		"MonthEmptyLabel": salesMonthEmptyLabel(month),
		"AllPeriodHref":   "/sales/activities?" + salesActLogQuery("all", q, sel),
		"LogQuery":        logQuery,
		"TypeQuery":       typeQ,
		"ActivityTypes":   types,
		"SalesOptions":    projects,
		"Today":           time.Now().Format("2006-01-02"),
		"CanWrite":        canWriteSales(c),
		"CanEditOthers":   isAdminRole(c) || isOfficeRole(c),
		"CurrentUser":     ctxString(c, "user_name"),
		"FormAction":      "/sales/activities",
		"FixedSalesID":    "",
		"DefaultOwner":    ctxString(c, "user_name"),
		"Users":           users,
		"PartyHints":      []string{},
		"ActGroup":        salesActGroup(c.QueryParam("group")),
		"KanbanDrop":      "act",
		"KanbanDrag":      canWriteSales(c),
		"KanbanHint":      "열 = 활동 유형. 카드를 끌어 옮깁니다.",
		"FilterQ":         "",
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
