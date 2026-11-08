package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	boardViewPeople  = "people"
	boardViewProject = "project"
	boardViewWeekly  = "weekly"
)

type boardMntStat struct {
	Month       int
	Target      int
	Done        int
	Incomplete  int
	Overdue     int
	Unassigned  int
	RateLabel   string
	RateValue   float64
}

type boardLeavePerson struct {
	Name string
	Kind string
	Date string
}

type meetingProjectGroup struct {
	ProjectID    string
	Label        string
	Assignees    []string
	AssigneeLine string
	Count        int
	Unlinked     bool
	SiteBased    bool
	Items        []model.WorkListItem
}

type boardPage struct {
	View          string
	From          string
	To            string
	WeekStart     string
	Kind          []string
	WithWeekly    bool
	Print         bool
	CanTeam       bool
	Role          string
	ShowOrgTabs   bool
	ShowOrgSelect bool
	OrgLocked     bool
	OrgLabel      string
	ViewOrgID     string
	Orgs          []model.Org
	Items         []model.WorkListItem
	PeopleGroups  []meetingAssigneeGroup
	ProjectGroups []meetingProjectGroup
	PeopleN       int
	ProjectN      int
	ItemN         int
	SchedN        int
	Mnt           boardMntStat
	Leaves        []boardLeavePerson
	LeaveNames    map[string]bool
	NeedSelectN   int
	SiteBasedN    int
	DoneGroups    []meetingAssigneeGroup
	PlanGroups    []meetingAssigneeGroup
	OverdueGroups []meetingAssigneeGroup
	ShowLeave     bool
	ShowMnt       bool
	ShowWork      bool
	ReadOnly      bool
}

func (h *MeetingHandler) Board(c echo.Context) error {
	p, err := h.loadBoard(c)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "meeting/board.html", p.tplData(c))
}

func (h *MeetingHandler) BoardWrite(c echo.Context) error {
	return echo.NewHTTPError(http.StatusForbidden, "업무 회의 화면은 조회만 할 수 있습니다")
}

func (h *MeetingHandler) loadBoard(c echo.Context) (*boardPage, error) {
	from, to := parseBoardRange(c)
	view := parseMeetingBoardView(c.QueryParam("view"))
	kinds := parseBoardKinds(c)
	withWeekly := c.QueryParam("with_weekly") == "1" || view == boardViewWeekly
	printMode := c.QueryParam("print") == "1"

	opts, users, canTeam, orgScope, showOrgPick, orgs := h.meetingPicker(c)
	_ = opts
	selected := resolveMeetingAssignee(c, users)
	var mineUID string
	var mineKeys []string
	if canTeam {
		mineUID, mineKeys = meetingAssigneeKeys(selected, users)
	} else {
		mineUID, mineKeys = meetingSelfKeys(c, users)
	}

	if h.groups != nil {
		injectGroupMaps(c, h.groups)
	}

	items, err := h.collectBoardItems(from, to, mineUID, mineKeys)
	if err != nil {
		return nil, err
	}
	items = h.filterMeetingOrg(c, items)
	items = filterBoardKind(items, kinds)
	items = uniqueWorkItems(items)
	if !canTeam && !viewGroupInclude(c) {
		items = filterPersonalOnlyWork(items, mineUID, mineKeys)
	}

	sched, err := h.collectScheduledRange(from, to, mineUID, mineKeys)
	if err != nil {
		return nil, err
	}
	sched = uniqueWorkItems(h.filterMeetingOrg(c, filterBoardKind(sched, kinds)))
	if !canTeam && !viewGroupInclude(c) {
		sched = filterPersonalOnlyWork(sched, mineUID, mineKeys)
	}

	order := assignableNameOrder(users)
	if !canTeam {
		if n := meetingSelfName(c); n != "" {
			order = []string{n}
		}
	}
	people := groupWorkItemsByAssignee(items, order)
	projects, needSel, siteN := h.groupWorkItemsByProject(items)

	lastFrom, lastTo := model.LastISOWeekRange(from)
	lastDone, err := h.collectCompletedRange(lastFrom, lastTo, mineUID, mineKeys)
	if err != nil {
		return nil, err
	}
	mon, fri := meetingWeekMonFri(from)
	planned, err := h.collectScheduledRange(mon, fri, mineUID, mineKeys)
	if err != nil {
		return nil, err
	}
	overdue, err := h.collectWeekOverdue(mon, mineUID, mineKeys)
	if err != nil {
		return nil, err
	}
	lastDone = uniqueWorkItems(h.filterMeetingOrg(c, lastDone))
	planned = uniqueWorkItems(h.filterMeetingOrg(c, planned))
	overdue = uniqueWorkItems(h.filterMeetingOrg(c, overdue))

	role := currentRole(c)
	p := &boardPage{
		View:          view,
		From:          from.Format("2006-01-02"),
		To:            to.Format("2006-01-02"),
		WeekStart:     mon.Format("2006-01-02"),
		Kind:          kinds,
		WithWeekly:    withWeekly,
		Print:         printMode,
		CanTeam:       canTeam,
		Role:          role,
		ShowOrgTabs:   role == model.RoleCLevel,
		ShowOrgSelect: showOrgPick && role != model.RoleCLevel,
		OrgLocked:     !showOrgPick && canTeam && role != model.RoleCLevel,
		OrgLabel:      orgViewLabel(c, orgs),
		ViewOrgID:     orgScope,
		Orgs:          orgs,
		Items:         items,
		PeopleGroups:  people,
		ProjectGroups: projects,
		PeopleN:       meetingGroupCount(people),
		ProjectN:      meetingProjectCount(projects),
		ItemN:         len(items),
		SchedN:        len(sched),
		DoneGroups:    groupWorkItemsByAssignee(lastDone, order),
		PlanGroups:    groupWorkItemsByAssignee(planned, order),
		OverdueGroups: groupWorkItemsByAssignee(overdue, order),
		ShowLeave:     boardPanelOn(c, "show_leave"),
		ShowMnt:       boardPanelOn(c, "show_mnt"),
		ShowWork:      boardPanelOn(c, "show_work"),
		ReadOnly:      role == model.RoleCLevel || isReadOnly(c),
		NeedSelectN:   needSel,
		SiteBasedN:    siteN,
	}
	if p.OrgLocked && p.OrgLabel == "" {
		p.OrgLabel = model.OrgNameLibrary
	}
	p.Mnt = h.boardMntStats(c, items, from)
	p.Leaves, p.LeaveNames = h.boardLeaves(c, from, to)
	return p, nil
}

func (p *boardPage) tplData(c echo.Context) map[string]interface{} {
	if p == nil {
		p = &boardPage{}
	}
	q := p.navQuery()
	data := map[string]interface{}{
		"Title":           "업무 회의",
		"Active":          NavMeetingBoard,
		"View":            p.View,
		"From":            p.From,
		"To":              p.To,
		"WeekStart":       p.WeekStart,
		"Kind":            p.Kind,
		"KindAll":         len(p.Kind) == 0,
		"WithWeekly":      p.WithWeekly,
		"Print":           p.Print,
		"CanTeam":         p.CanTeam,
		"Role":            p.Role,
		"ShowOrgTabs":     p.ShowOrgTabs,
		"ShowOrgSelect":   p.ShowOrgSelect,
		"OrgLocked":       p.OrgLocked,
		"OrgLabel":        p.OrgLabel,
		"ViewOrgID":       p.ViewOrgID,
		"Orgs":            p.Orgs,
		"PeopleGroups":    p.PeopleGroups,
		"ProjectGroups":   p.ProjectGroups,
		"PeopleN":         p.PeopleN,
		"ProjectN":        p.ProjectN,
		"ItemN":           p.ItemN,
		"SchedN":          p.SchedN,
		"Mnt":             p.Mnt,
		"Leaves":          p.Leaves,
		"LeaveNames":      p.LeaveNames,
		"DoneGroups":      p.DoneGroups,
		"PlanGroups":      p.PlanGroups,
		"OverdueGroups":   p.OverdueGroups,
		"ShowLeave":       p.ShowLeave,
		"ShowMnt":         p.ShowMnt,
		"ShowWork":        p.ShowWork,
		"ReadOnly":        p.ReadOnly,
		"NavQ":            template.URL(q),
		"ExcelHref":       "/meeting/board.xlsx?" + q,
		"PrintHref":       "/meeting/board?" + boardQueryWith(q, "print", "1"),
		"PeopleHref":      "/meeting/board?" + boardQueryWith(q, "view", boardViewPeople),
		"NeedSelectN":     p.NeedSelectN,
		"SiteBasedN":      p.SiteBasedN,
		"ProjectHref":     "/meeting/board?" + boardQueryWith(q, "view", boardViewProject),
		"WeeklyHref":      "/meeting/board?" + boardQueryWith(q, "view", boardViewWeekly),
		"ScopeNote":       boardScopeNote(p),
		"KindAS":          boardKindOn(p.Kind, model.WorkPrefixAS),
		"KindMnt":         boardKindOn(p.Kind, model.WorkPrefixMaintenance),
		"KindWork":        boardKindOn(p.Kind, model.WorkPrefixProjectWork),
		"KindSales":       boardKindOn(p.Kind, model.WorkPrefixSales),
		"KindGeneral":     boardKindOn(p.Kind, model.WorkPrefixGeneral),
		"KindSupport":     boardKindOn(p.Kind, model.WorkPrefixSupport),
	}
	return attachViewGroup(c, data)
}

func boardScopeNote(p *boardPage) string {
	if p == nil {
		return "전체 업무 기준"
	}
	if !p.CanTeam {
		return "내 담당 업무"
	}
	if p.ShowOrgTabs || p.ShowOrgSelect {
		if p.ViewOrgID == "" || p.ViewOrgID == repository.OrgAll {
			return "전체 조직 · 조회 전용"
		}
		return p.OrgLabel + " · 조회 전용"
	}
	if p.OrgLocked {
		return p.OrgLabel
	}
	return "전체 업무 기준"
}

func (p *boardPage) navQuery() string {
	v := url.Values{}
	if p == nil {
		return ""
	}
	v.Set("from", p.From)
	v.Set("to", p.To)
	v.Set("view", p.View)
	if p.ViewOrgID != "" && (p.ShowOrgTabs || p.ShowOrgSelect) {
		v.Set("view_org_id", p.ViewOrgID)
	}
	for _, k := range p.Kind {
		v.Add("kind", k)
	}
	if p.WithWeekly && p.View != boardViewWeekly {
		v.Set("with_weekly", "1")
	}
	if !p.ShowLeave {
		v.Set("show_leave", "0")
	}
	if !p.ShowMnt {
		v.Set("show_mnt", "0")
	}
	if !p.ShowWork {
		v.Set("show_work", "0")
	}
	return v.Encode()
}

func boardQueryWith(base, key, val string) string {
	v, _ := url.ParseQuery(base)
	if val == "" {
		v.Del(key)
	} else {
		v.Set(key, val)
	}
	return v.Encode()
}

func parseMeetingBoardView(s string) string {
	switch strings.TrimSpace(s) {
	case boardViewProject:
		return boardViewProject
	case boardViewWeekly:
		return boardViewWeekly
	default:
		return boardViewPeople
	}
}

func parseBoardKinds(c echo.Context) []string {
	raw := c.QueryParams()["kind"]
	if len(raw) == 0 {
		s := strings.TrimSpace(c.QueryParam("kind"))
		if s != "" {
			raw = strings.Split(s, ",")
		}
	}
	allow := map[string]bool{
		model.WorkPrefixAS: true, model.WorkPrefixMaintenance: true,
		model.WorkPrefixProjectWork: true, model.WorkPrefixSales: true,
		model.WorkPrefixGeneral: true, model.WorkPrefixSupport: true,
		model.WorkPrefixConfirm: true,
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range raw {
		k = strings.TrimSpace(k)
		if k == "" || k == "all" || seen[k] || !allow[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

func boardKindOn(kinds []string, prefix string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, k := range kinds {
		if k == prefix {
			return true
		}
	}
	return false
}

func boardPanelOn(c echo.Context, key string) bool {
	v := strings.TrimSpace(c.QueryParam(key))
	if v == "0" || v == "off" || v == "false" {
		return false
	}
	return true
}

func parseBoardRange(c echo.Context) (time.Time, time.Time) {
	fromS := strings.TrimSpace(c.QueryParam("from"))
	toS := strings.TrimSpace(c.QueryParam("to"))
	if fromS == "" && toS == "" {
		return meetingWeekMonFri(parseMeetingAnchor(c.QueryParam("date")))
	}
	if fromS == "" {
		fromS = toS
	}
	if toS == "" {
		toS = fromS
	}
	from := parseMeetingAnchor(fromS)
	to := parseMeetingAnchor(toS)
	if to.Before(from) {
		from, to = to, from
	}
	if to.Sub(from) > 93*24*time.Hour {
		to = from.AddDate(0, 0, 92)
	}
	return from, to
}

func (h *MeetingHandler) collectBoardItems(from, to time.Time, uid string, keys []string) ([]model.WorkListItem, error) {
	sched, err := h.collectScheduledRange(from, to, uid, keys)
	if err != nil {
		return nil, err
	}
	done, err := h.collectCompletedRange(from.Format("2006-01-02"), to.Format("2006-01-02"), uid, keys)
	if err != nil {
		return nil, err
	}
	return uniqueWorkItems(append(sched, done...)), nil
}

func uniqueWorkItems(items []model.WorkListItem) []model.WorkListItem {
	seen := map[string]bool{}
	var out []model.WorkListItem
	for _, it := range items {
		k := model.WorkListItemKey(it)
		if k == ":" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, it)
	}
	return out
}

func filterBoardKind(items []model.WorkListItem, kinds []string) []model.WorkListItem {
	if len(kinds) == 0 {
		return items
	}
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var out []model.WorkListItem
	for _, it := range items {
		if want[it.Prefix] {
			out = append(out, it)
		}
	}
	return out
}

func (h *MeetingHandler) groupWorkItemsByProject(items []model.WorkListItem) (groups []meetingProjectGroup, needSelect, siteBased int) {
	seen := map[string]bool{}
	order := []string{}
	bags := map[string]*meetingProjectGroup{}
	assignSeen := map[string]map[string]bool{}
	var custIDs []string
	for _, it := range items {
		if strings.TrimSpace(it.ProjectID) == "" {
			custIDs = append(custIDs, it.CustomerID)
		}
	}
	siteMap := map[string][]string{}
	if h != nil && h.wb != nil {
		siteMap = h.wb.ListActiveSiteProjectIDsByCustomers(custIDs)
	}
	var nameIDs []string
	for _, it := range items {
		k := model.WorkListItemKey(it)
		if seen[k] {
			continue
		}
		seen[k] = true
		pid, viaSite, need := repository.ResolveMeetingDisplayProject(it, siteMap)
		if need {
			needSelect++
		}
		if viaSite {
			siteBased++
		}
		key := pid
		if key == "" {
			key = "_"
		}
		g, ok := bags[key]
		if !ok {
			g = &meetingProjectGroup{ProjectID: pid, Label: "사업 미연결", Unlinked: pid == ""}
			bags[key] = g
			order = append(order, key)
			assignSeen[key] = map[string]bool{}
			if pid != "" {
				nameIDs = append(nameIDs, pid)
			}
		}
		if viaSite {
			g.SiteBased = true
		}
		g.Items = append(g.Items, it)
		g.Count++
		if a := strings.TrimSpace(it.Assignee); a != "" && !assignSeen[key][a] {
			assignSeen[key][a] = true
			g.Assignees = append(g.Assignees, a)
		}
	}
	names := map[string]string{}
	if h != nil && h.wb != nil {
		names = h.wb.WorkProjectNames(nameIDs)
	}
	for _, key := range order {
		g := bags[key]
		if g.ProjectID != "" {
			if n := names[g.ProjectID]; n != "" {
				g.Label = n
			} else {
				g.Label = g.ProjectID
			}
		}
		g.AssigneeLine = strings.Join(g.Assignees, ", ")
		groups = append(groups, *g)
	}
	return groups, needSelect, siteBased
}

func meetingProjectCount(groups []meetingProjectGroup) int {
	n := 0
	for _, g := range groups {
		n += g.Count
	}
	return n
}

func (h *MeetingHandler) boardMntStats(c echo.Context, items []model.WorkListItem, from time.Time) boardMntStat {
	st := boardMntStat{RateLabel: "—"}
	if h == nil || h.stats == nil {
		return st
	}
	org := currentOrg(c)
	quota, cum, month, err := h.stats.CompanyMntMonthProgress(org, from)
	if err != nil {
		return st
	}
	st.Month = month
	st.Target = quota
	st.Done = cum
	st.Incomplete = quota - cum
	if st.Incomplete < 0 {
		st.Incomplete = 0
	}
	if quota > 0 {
		st.RateValue = repository.ProgressPct(cum, quota)
		st.RateLabel = strconv.FormatFloat(st.RateValue, 'f', 1, 64) + "%"
	}
	today := time.Now().In(from.Location()).Format("2006-01-02")
	seen := map[string]bool{}
	for _, it := range items {
		if it.Prefix != model.WorkPrefixMaintenance {
			continue
		}
		k := model.WorkListItemKey(it)
		if seen[k] {
			continue
		}
		seen[k] = true
		if meetingItemComplete(it) {
			continue
		}
		d := model.WorkItemScheduledDate(it)
		if d != "" && d < today {
			st.Overdue++
		}
		if strings.TrimSpace(it.Assignee) == "" {
			st.Unassigned++
		}
	}
	st.Unassigned += h.countOpenMonthTargets(org, from.Year(), month)
	return st
}

func (h *MeetingHandler) countOpenMonthTargets(org string, year, month int) int {
	if h == nil || h.stats == nil || h.stats.DB() == nil || year < 1 || month < 1 {
		return 0
	}
	q := `SELECT COUNT(*) FROM maintenance_month_targets t
		WHERE t.year=? AND t.month=? AND (t.cancelled_at IS NULL OR TRIM(t.cancelled_at)='')
		  AND NOT EXISTS (
		    SELECT 1 FROM maintenance_visits v
		     WHERE v.customer_id=t.customer_id
		       AND CAST(strftime('%Y', v.visit_date) AS INTEGER)=t.year
		       AND CAST(strftime('%m', v.visit_date) AS INTEGER)=t.month
		  )`
	args := []interface{}{year, month}
	if org != "" && org != repository.OrgAll {
		q += ` AND (t.org_id=? OR t.org_id='')`
		args = append(args, org)
	}
	var n int
	if err := h.stats.DB().QueryRow(q, args...).Scan(&n); err != nil {
		return 0
	}
	return n
}

func (h *MeetingHandler) boardLeaves(c echo.Context, from, to time.Time) ([]boardLeavePerson, map[string]bool) {
	names := map[string]bool{}
	if h == nil || h.leaves == nil {
		return nil, names
	}
	items, err := h.leaves.ListInRange(from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil || len(items) == 0 {
		return nil, names
	}
	allow := map[string]bool{}
	org := currentOrg(c)
	if org != "" && org != repository.OrgAll && h.userRepo != nil {
		if list, err := h.userRepo.ListAssignable(org); err == nil {
			for _, u := range list {
				if n := strings.TrimSpace(u.FullName); n != "" {
					allow[n] = true
				}
				if id := strings.TrimSpace(u.UserID); id != "" {
					allow[id] = true
				}
			}
		}
	}
	var out []boardLeavePerson
	seen := map[string]bool{}
	for _, it := range items {
		n := strings.TrimSpace(it.UserName)
		if n == "" {
			n = strings.TrimSpace(it.UserID)
		}
		if n == "" {
			continue
		}
		if len(allow) > 0 && !allow[n] && !allow[strings.TrimSpace(it.UserID)] {
			continue
		}
		key := n + "|" + it.Date
		if seen[key] {
			continue
		}
		seen[key] = true
		names[n] = true
		out = append(out, boardLeavePerson{Name: n, Kind: model.LeaveBadgeText(n, it.Kind), Date: it.Date})
	}
	return out, names
}

func boardReceiptText(it model.WorkListItem) string {
	d := model.WorkItemScheduledDate(it)
	body := strings.TrimSpace(it.Title)
	if body == "" {
		body = strings.TrimSpace(it.Content)
	}
	return strings.TrimSpace(d + " " + body)
}

func boardActionText(it model.WorkListItem) string {
	d := strings.TrimSpace(it.CompleteDate)
	lab := strings.TrimSpace(it.StatusLabel)
	if lab == "" {
		lab = strings.TrimSpace(it.Status)
	}
	return strings.TrimSpace(d + " " + lab)
}

func boardItemHref(it model.WorkListItem) string {
	if strings.TrimSpace(it.Href) != "" {
		return it.Href
	}
	if it.Prefix == model.WorkPrefixAS && it.RefID != "" {
		return model.ASOpenHref(it.RefID)
	}
	return "#"
}
