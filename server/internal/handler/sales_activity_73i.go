package handler

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	actBizUnset     = "unset"
	actProjectNone  = "none"
	actMaxProjectID = 100
	actPageSize     = 8
)

type actProjectOpt struct {
	SalesID  string
	Name     string
	SalesNo  string
	BizType  string
	LastDate string
	Checked  bool
}

type actBizChip struct {
	Code    string
	Label   string
	Count   int
	Checked bool
	Href    string
}

type actPageLink struct {
	Label    string
	Href     string
	Current  bool
	Disabled bool
}

func parseCSVLimited(raw string, max int) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func parseActBizTypes(c echo.Context, valid map[string]bool) (codes []string, all bool, none bool) {
	raw := strings.TrimSpace(c.QueryParam("biz_types"))
	if raw == "" {
		return nil, true, false
	}
	if raw == actProjectNone {
		return nil, false, true
	}
	for _, p := range parseCSVLimited(raw, 50) {
		if p == actBizUnset || valid[p] {
			codes = append(codes, p)
		}
	}
	if len(codes) == 0 {
		return nil, false, true
	}
	return codes, false, false
}

func parseActProjectIDs(c echo.Context) (ids []string, all bool, none bool) {
	if _, ok := c.QueryParams()["project_ids"]; !ok {
		return nil, true, false
	}
	raw := strings.TrimSpace(c.QueryParam("project_ids"))
	if raw == "" || raw == actProjectNone {
		return nil, false, true
	}
	ids = parseCSVLimited(raw, actMaxProjectID)
	if len(ids) == 0 {
		return nil, false, true
	}
	return ids, false, false
}

func actScopeBizOf(p model.SalesProject) string {
	b := strings.TrimSpace(p.BizType)
	if b == "" {
		return actBizUnset
	}
	return b
}

func applyActProjectScope(visActs []model.SalesActivity, projects []model.SalesProject, typeAll bool, types []string, idAll bool, wantIDs []string) (visible []actProjectOpt, selectedIDs []string, emptyClass bool, emptyProj bool) {
	last := map[string]string{}
	hasAct := map[string]bool{}
	for _, a := range visActs {
		hasAct[a.SalesID] = true
		if a.ActivityDate > last[a.SalesID] {
			last[a.SalesID] = a.ActivityDate
		}
	}
	byID := map[string]model.SalesProject{}
	for i := range projects {
		byID[projects[i].SalesID] = projects[i]
	}
	typeSet := map[string]bool{}
	for _, t := range types {
		typeSet[t] = true
	}
	if !typeAll && len(types) == 0 {
		return nil, nil, true, true
	}
	for id := range hasAct {
		p := byID[id]
		bt := actScopeBizOf(p)
		if !typeAll && !typeSet[bt] {
			continue
		}
		name := p.Name
		if name == "" {
			name = id
		}
		visible = append(visible, actProjectOpt{
			SalesID: id, Name: name, SalesNo: p.SalesNo, BizType: bt, LastDate: last[id],
		})
	}
	sortActProjects(visible)
	visSet := map[string]bool{}
	for _, v := range visible {
		visSet[v.SalesID] = true
	}
	if idAll {
		for i := range visible {
			visible[i].Checked = true
			selectedIDs = append(selectedIDs, visible[i].SalesID)
		}
		return visible, selectedIDs, false, false
	}
	for _, id := range wantIDs {
		if visSet[id] {
			selectedIDs = append(selectedIDs, id)
		}
	}
	sel := map[string]bool{}
	for _, id := range selectedIDs {
		sel[id] = true
	}
	for i := range visible {
		visible[i].Checked = sel[visible[i].SalesID]
	}
	if len(selectedIDs) == 0 {
		emptyProj = true
	}
	return visible, selectedIDs, false, emptyProj
}

func sortActProjects(rows []actProjectOpt) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].LastDate > rows[i].LastDate || (rows[j].LastDate == rows[i].LastDate && rows[j].Name < rows[i].Name) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func filterActsBySales(acts []model.SalesActivity, ids []string) []model.SalesActivity {
	if ids == nil {
		return acts
	}
	ok := map[string]bool{}
	for _, id := range ids {
		ok[id] = true
	}
	var out []model.SalesActivity
	for _, a := range acts {
		if ok[a.SalesID] {
			out = append(out, a)
		}
	}
	return out
}

func paginateActs(acts []model.SalesActivity, page, size int) (slice []model.SalesActivity, pageN, pages int) {
	n := len(acts)
	pages = (n + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	from := (page - 1) * size
	if from >= n {
		return nil, page, pages
	}
	to := from + size
	if to > n {
		to = n
	}
	return acts[from:to], page, pages
}

func actPageLinks(base url.Values, page, pages int) []actPageLink {
	mk := func(p int, label string, dis, cur bool) actPageLink {
		q := cloneValues(base)
		q.Set("page", strconv.Itoa(p))
		return actPageLink{Label: label, Href: "/sales/activities?" + q.Encode(), Current: cur, Disabled: dis}
	}
	var out []actPageLink
	out = append(out, mk(page-1, "이전", page <= 1, false))
	start, end := page-2, page+2
	if start < 1 {
		start = 1
	}
	if end > pages {
		end = pages
	}
	if start > 1 {
		out = append(out, mk(1, "1", false, page == 1))
		if start > 2 {
			out = append(out, actPageLink{Label: "…", Disabled: true})
		}
	}
	for i := start; i <= end; i++ {
		out = append(out, mk(i, strconv.Itoa(i), false, i == page))
	}
	if end < pages {
		if end < pages-1 {
			out = append(out, actPageLink{Label: "…", Disabled: true})
		}
		out = append(out, mk(pages, strconv.Itoa(pages), false, page == pages))
	}
	out = append(out, mk(page+1, "다음", page >= pages, false))
	return out
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string{}, vs...)
	}
	return out
}

func salesActQuery(month, q, period, status, biz, pids, owner string, types []string, page int) string {
	v := url.Values{}
	if month != "" {
		v.Set("month", month)
	}
	if q != "" {
		v.Set("q", q)
	}
	if period != "" {
		v.Set("period", period)
	}
	if status != "" && status != "all" {
		v.Set("status", status)
	}
	if biz != "" {
		v.Set("biz_types", biz)
	}
	if pids != "" {
		v.Set("project_ids", pids)
	}
	if strings.TrimSpace(owner) != "" {
		v.Set("owner", owner)
	}
	for _, t := range types {
		v.Add("type", t)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	return v.Encode()
}

func encodeCSV(ids []string) string {
	return strings.Join(ids, ",")
}

func todayForAct(c echo.Context) string {
	if s := strings.TrimSpace(c.QueryParam("today")); len(s) == 10 {
		return s
	}
	return time.Now().Format("2006-01-02")
}

func csvOfOpts(opts []actProjectOpt) string {
	ids := make([]string, 0, len(opts))
	for _, o := range opts {
		ids = append(ids, o.SalesID)
	}
	return encodeCSV(ids)
}

func buildActBizChips(c echo.Context, vis []model.SalesActivity, proj map[string]model.SalesProject, codes []model.Code, hasUnset bool, sel []string, all, none bool, month, q, period, status, pidCSV, owner string, types []string) []actBizChip {
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, a := range vis {
		if seen[a.SalesID] {
			continue
		}
		seen[a.SalesID] = true
		bt := actBizUnset
		if p, ok := proj[a.SalesID]; ok {
			bt = actScopeBizOf(p)
		}
		counts[bt]++
	}
	checked := map[string]bool{}
	if all {
		for _, b := range codes {
			checked[b.CodeValue] = true
		}
		if hasUnset {
			checked[actBizUnset] = true
		}
	} else {
		for _, s := range sel {
			checked[s] = true
		}
	}
	var chips []actBizChip
	add := func(code, label string) {
		on := checked[code]
		next := []string{}
		if all {
			for _, b := range codes {
				if b.CodeValue != code {
					next = append(next, b.CodeValue)
				}
			}
			if hasUnset && code != actBizUnset {
				next = append(next, actBizUnset)
			}
		} else if on {
			for _, s := range sel {
				if s != code {
					next = append(next, s)
				}
			}
		} else {
			next = append(append([]string{}, sel...), code)
		}
		bizCSV := encodeCSV(next)
		allOn := true
		wantN := len(codes)
		if hasUnset {
			wantN++
		}
		if len(next) != wantN {
			allOn = false
		}
		if len(next) == 0 {
			bizCSV = actProjectNone
		} else if allOn {
			bizCSV = ""
		}
		href := "/sales/activities?" + salesActQuery(month, q, period, status, bizCSV, "", owner, types, 0)
		chips = append(chips, actBizChip{Code: code, Label: label, Count: counts[code], Checked: on && !none, Href: href})
	}
	for _, b := range codes {
		add(b.CodeValue, b.CodeName)
	}
	if hasUnset {
		add(actBizUnset, "미지정")
	}
	_ = c
	_ = pidCSV
	return chips
}
