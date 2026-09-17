package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	workKindAS          = "as"
	workKindMaintenance = "maintenance"
	workKindAdmin       = "admin"
	workKindSales       = "sales"
	workPeriodDefault   = "3m"
	workStatusWaiting   = "waiting"
	workStatusProgress  = "in_progress"
	workStatusComplete  = "complete"
	workStatusDelayed   = "delayed"
	workStatusHold      = "hold"
)

type workFilterChip struct {
	Label string
	Href  string
}

func queryCSV(c echo.Context, key string) []string {
	var raw []string
	if c != nil && c.QueryParams() != nil {
		raw = append(raw, c.QueryParams()[key]...)
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range raw {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func parseWorkAllPeriod(c echo.Context, now time.Time) (from, to, period, rangeParam string) {
	period = strings.TrimSpace(c.QueryParam("period"))
	rangeQ := strings.TrimSpace(c.QueryParam("range"))
	fromQ := strings.TrimSpace(c.QueryParam("from"))
	toQ := strings.TrimSpace(c.QueryParam("to"))
	if period == "" {
		switch rangeQ {
		case "1d":
			period = "today"
		case "1w":
			period = "week"
		case "1m":
			period = "month"
		case "3m":
			period = "3m"
		case "":
			if fromQ != "" && toQ != "" {
				period = "custom"
			} else {
				period = "3m"
				rangeQ = "3m"
			}
		default:
			period = "custom"
		}
		if period == "custom" && fromQ != "" {
			y := fmt.Sprintf("%d-01-01", now.Year())
			if fromQ == y && (toQ == now.Format("2006-01-02") || toQ == "") {
				period = "year"
			}
		}
	}
	switch period {
	case "today":
		rangeQ = "1d"
		fromQ, toQ = "", ""
	case "week":
		rangeQ = "1w"
		fromQ, toQ = "", ""
	case "month":
		rangeQ = "1m"
		fromQ, toQ = "", ""
	case "3m", "":
		period = "3m"
		rangeQ = "3m"
		fromQ, toQ = "", ""
	case "year":
		fromQ = fmt.Sprintf("%d-01-01", now.Year())
		toQ = now.Format("2006-01-02")
		rangeQ = ""
	case "custom":
		rangeQ = ""
		if fromQ == "" && toQ == "" {
			fromQ = now.AddDate(0, -3, 0).Format("2006-01-02")
			toQ = now.Format("2006-01-02")
		}
	}
	lb := model.ParseStatsLookback(rangeQ, fromQ, toQ, "", "", "", now)
	return lb.From, lb.To, period, lb.RangeParam
}

func workPeriodLabel(period, from, to string) string {
	switch period {
	case "today":
		return "오늘"
	case "week":
		return "이번 주"
	case "month":
		return "이번 달"
	case "year":
		return "올해"
	case "custom":
		return from + " ~ " + to
	default:
		return "최근 3개월"
	}
}

func filterWorkItemsKinds(items []model.WorkListItem, kinds []string) []model.WorkListItem {
	if len(kinds) == 0 {
		return items
	}
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		if want[workItemKind(it)] {
			out = append(out, it)
		}
	}
	return out
}

func workItemKind(it model.WorkListItem) string {
	switch it.Prefix {
	case model.WorkPrefixAS, model.WorkPrefixConfirm:
		return workKindAS
	case model.WorkPrefixMaintenance:
		return workKindMaintenance
	case model.WorkPrefixSales:
		return workKindSales
	default:
		return workKindAdmin
	}
}

func filterWorkItemsAssignees(items []model.WorkListItem, names []string) []model.WorkListItem {
	if len(names) == 0 {
		return items
	}
	want := map[string]bool{}
	none := false
	for _, n := range names {
		if n == workAssigneeNone {
			none = true
			continue
		}
		want[n] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		name := strings.TrimSpace(it.Assignee)
		if none && name == "" {
			out = append(out, it)
			continue
		}
		if name != "" && want[name] {
			out = append(out, it)
		}
	}
	return out
}

func filterWorkItemsStatuses(items []model.WorkListItem, statuses []string, today string) []model.WorkListItem {
	if len(statuses) == 0 {
		return items
	}
	want := map[string]bool{}
	for _, s := range statuses {
		want[s] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		if workItemMatchesStatus(it, want, today) {
			out = append(out, it)
		}
	}
	return out
}

// workItemMatchesStatus §35 매핑. 대기=할 일 열, 진행중=진행중 열(보류 제외), 보류=뱃지, 완료, 지연=예정일 지남.
func workItemMatchesStatus(it model.WorkListItem, want map[string]bool, today string) bool {
	mapped := it.MappedStatus
	if mapped == "" {
		mapped = model.MapASStatusToWB(it.Status)
	}
	bucket := model.WBKanbanBucket(mapped)
	if want[workStatusWaiting] && bucket == model.WBTaskWaiting {
		return true
	}
	if want[workStatusProgress] && bucket == model.WBTaskInProgress && mapped != model.WBTaskHold {
		return true
	}
	if want[workStatusHold] && mapped == model.WBTaskHold {
		return true
	}
	if want[workStatusComplete] && mapped == model.WBTaskComplete {
		return true
	}
	if want[workStatusDelayed] {
		if mapped != model.WBTaskComplete {
			sched := model.WorkItemScheduledDate(it)
			if sched != "" && sched < today {
				return true
			}
		}
	}
	return false
}

func filterWorkItemsCustomers(items []model.WorkListItem, ids []string, names map[string]string) []model.WorkListItem {
	if len(ids) == 0 {
		return items
	}
	wantID := map[string]bool{}
	wantName := map[string]bool{}
	for _, id := range ids {
		wantID[id] = true
		if n := strings.TrimSpace(names[id]); n != "" {
			wantName[n] = true
		}
		wantName[id] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		if it.CustomerID != "" && wantID[it.CustomerID] {
			out = append(out, it)
			continue
		}
		if it.OrgName != "" && wantName[it.OrgName] {
			out = append(out, it)
		}
	}
	return out
}

func filterWorkItemsProjects(items []model.WorkListItem, ids []string) []model.WorkListItem {
	if len(ids) == 0 {
		return items
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		if it.ProjectID != "" && want[it.ProjectID] {
			out = append(out, it)
		}
	}
	return out
}

func filterWorkItemsKeyword(items []model.WorkListItem, q string, asIDs map[string]bool) []model.WorkListItem {
	q = strings.TrimSpace(q)
	if q == "" {
		return items
	}
	ql := strings.ToLower(q)
	var out []model.WorkListItem
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title), ql) ||
			strings.Contains(strings.ToLower(it.OrgName), ql) ||
			strings.Contains(strings.ToLower(it.Content), ql) ||
			strings.Contains(strings.ToLower(it.RefNumber), ql) {
			out = append(out, it)
			continue
		}
		if asIDs[it.RefID] && (it.Prefix == model.WorkPrefixAS || it.Prefix == model.WorkPrefixConfirm) {
			out = append(out, it)
		}
	}
	return out
}

func csvJoin(v []string) string {
	return strings.Join(v, ",")
}

func addCSV(v url.Values, key string, vals []string) {
	if len(vals) == 0 {
		return
	}
	v.Set(key, csvJoin(vals))
}

func workHasExtraFilters(q workAllQuery) bool {
	return len(q.Assignees) > 0 || len(q.Kinds) > 0 || len(q.Statuses) > 0 ||
		len(q.Customers) > 0 || len(q.Projects) > 0 || strings.TrimSpace(q.Q) != ""
}

func workStatusLabel(code string) string {
	switch code {
	case workStatusWaiting:
		return "대기"
	case workStatusProgress:
		return "진행중"
	case workStatusComplete:
		return "완료"
	case workStatusDelayed:
		return "지연"
	case workStatusHold:
		return "보류"
	default:
		return code
	}
}

func workKindLabel(code string) string {
	switch code {
	case workKindAS:
		return "AS"
	case workKindMaintenance:
		return "정기점검"
	case workKindAdmin:
		return "행정·지원"
	case workKindSales:
		return "영업 활동"
	default:
		return code
	}
}

func workFilterChips(q workAllQuery, users []model.User, customers []model.Customer, projects []model.WorkProject) []workFilterChip {
	base := q
	var chips []workFilterChip
	nameOf := map[string]string{}
	for _, u := range users {
		nameOf[u.FullName] = u.FullName
	}
	orgOf := map[string]string{}
	for _, c := range customers {
		orgOf[c.CustomerID] = c.OrgName
	}
	projOf := map[string]string{}
	for _, p := range projects {
		n := p.Name
		if p.ShortName != "" {
			n = p.ShortName
		}
		projOf[p.ProjectID] = n
	}
	drop := func(param, value string) string {
		nq := base
		switch param {
		case "assignee":
			nq.Assignees = omitValue(nq.Assignees, value)
		case "kind":
			nq.Kinds = omitValue(nq.Kinds, value)
		case "status":
			nq.Statuses = omitValue(nq.Statuses, value)
		case "customer":
			nq.Customers = omitValue(nq.Customers, value)
		case "project":
			nq.Projects = omitValue(nq.Projects, value)
		case "q":
			nq.Q = ""
		case "period":
			nq.Period = "3m"
			nq.Range = "3m"
			nq.From, nq.To = "", ""
		}
		nq.Offset = 0
		enc := nq.Encode()
		if enc == "" {
			return "/work/all"
		}
		return "/work/all?" + enc
	}
	for _, a := range q.Assignees {
		label := a
		if a == workAssigneeNone {
			label = "미배정"
		}
		chips = append(chips, workFilterChip{Label: label, Href: drop("assignee", a)})
	}
	for _, k := range q.Kinds {
		chips = append(chips, workFilterChip{Label: workKindLabel(k), Href: drop("kind", k)})
	}
	for _, s := range q.Statuses {
		chips = append(chips, workFilterChip{Label: workStatusLabel(s), Href: drop("status", s)})
	}
	for _, id := range q.Customers {
		label := orgOf[id]
		if label == "" {
			label = id
		}
		chips = append(chips, workFilterChip{Label: label, Href: drop("customer", id)})
	}
	for _, id := range q.Projects {
		label := projOf[id]
		if label == "" {
			label = id
		}
		chips = append(chips, workFilterChip{Label: label, Href: drop("project", id)})
	}
	if strings.TrimSpace(q.Q) != "" {
		chips = append(chips, workFilterChip{Label: "검색: " + q.Q, Href: drop("q", "")})
	}
	if q.Period != "" && q.Period != "3m" {
		chips = append(chips, workFilterChip{Label: workPeriodLabel(q.Period, q.From, q.To), Href: drop("period", "")})
	}
	return chips
}

func omitValue(in []string, drop string) []string {
	var out []string
	for _, v := range in {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}

func selectedSet(vals []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func jsonJS(v interface{}) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return template.JS(b)
}

type workCustomerOpt struct {
	ID        string `json:"id"`
	OrgName   string `json:"org_name"`
	ShortName string `json:"short_name"`
	Region    string `json:"region"`
}

func workCustomerOpts(list []model.Customer) []workCustomerOpt {
	out := make([]workCustomerOpt, 0, len(list))
	for _, c := range list {
		out = append(out, workCustomerOpt{
			ID: c.CustomerID, OrgName: c.OrgName, ShortName: c.ShortName, Region: c.Region,
		})
	}
	return out
}
