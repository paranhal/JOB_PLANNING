package handler

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	mntStateDone    = "done"
	mntStatePlanned = "planned"
	mntStateCheck   = "check"
)

type MntCalChip struct {
	VisitID string
	Label   string
	State   string
	Title   string
}

type MntCalGroup struct {
	GroupID       string
	Name          string
	OrgLabel      string
	MemberSummary string
	MemberN       int
	Members       []MntCalPerson
}

type MntCalPerson struct {
	UserID   string
	FullName string
}

type MntMonthStats struct {
	Total int
	Done  int
	Open  int // 예정+확인 필요
}

type MntVisitCard struct {
	VisitID       string `json:"visit_id"`
	Date          string `json:"date"`
	Site          string `json:"site"`
	Product       string `json:"product"`
	Region        string `json:"region"`
	AssigneeLabel string `json:"assignee_label"`
	AssigneeTitle string `json:"assignee_title"`
	GroupName     string `json:"group_name"`
	State         string `json:"state"`
	StateLabel    string `json:"state_label"`
	BadgeLabel    string `json:"badge_label"`
	BadgeExtra    string `json:"badge_extra"`
	BadgeClass    string `json:"badge_class"`
	RoundNo       int    `json:"round_no"`
	Dup           bool   `json:"dup"`
	Completed     bool   `json:"completed"`
	CanDelete     bool   `json:"can_delete"`
	CustomerID    string `json:"customer_id"`
	ProductType   string `json:"product_type"`
	Assignee      string `json:"assignee"`
	PeopleIDs     string `json:"people_ids"`
	OwnerID       string `json:"owner_id"`
	WorkGroupID   string `json:"work_group_id"`
	ProjectID     string `json:"project_id"`
	EntryCategory string `json:"entry_category"`
	Notes         string `json:"notes"`
	CompletedDate string `json:"completed_date"`
	CompleteHref  string `json:"complete_href"`
	DeleteHref    string `json:"delete_href"`
}

func mntVisitState(v model.MaintenanceVisit, today string) string {
	today = strings.TrimSpace(today)
	if v.Completed {
		return mntStateDone
	}
	d := strings.TrimSpace(v.VisitDate)
	if len(d) >= 10 {
		d = d[:10]
	}
	if (d != "" && d < today) || !visitHasAssignee(v) || strings.TrimSpace(v.DupReason) != "" {
		return mntStateCheck
	}
	return mntStatePlanned
}

func mntVisitStateLabel(state string) string {
	switch state {
	case mntStateDone:
		return "완료"
	case mntStatePlanned:
		return "예정"
	default:
		return "확인 필요"
	}
}

func visitHasAssignee(v model.MaintenanceVisit) bool {
	if strings.TrimSpace(v.Assignee) != "" {
		return true
	}
	for _, p := range v.People {
		if strings.TrimSpace(p.UserID) != "" || strings.TrimSpace(p.AssigneeName) != "" {
			return true
		}
	}
	return false
}

func visitAssigneeIDs(v model.MaintenanceVisit) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, p := range v.People {
		add(p.UserID)
	}
	return out
}

func visitAssigneeNames(v model.MaintenanceVisit) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(v.Assignee)
	for _, p := range v.People {
		add(p.AssigneeName)
	}
	return out
}

func parseAssigneeFilter(raw string) (kind, id string) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "", raw == "all":
		return "all", ""
	case raw == "none":
		return "none", ""
	case strings.HasPrefix(raw, "group:"):
		return "group", strings.TrimSpace(strings.TrimPrefix(raw, "group:"))
	case strings.HasPrefix(raw, "user:"):
		return "user", strings.TrimSpace(strings.TrimPrefix(raw, "user:"))
	default:
		return "all", ""
	}
}

func filterVisitsByAssigneeKey(visits []model.MaintenanceVisit, key string, groupMembers map[string]map[string]bool, userName string) []model.MaintenanceVisit {
	kind, id := parseAssigneeFilter(key)
	if kind == "all" {
		return visits
	}
	out := make([]model.MaintenanceVisit, 0, len(visits))
	for _, v := range visits {
		switch kind {
		case "none":
			if !visitHasAssignee(v) {
				out = append(out, v)
			}
		case "group":
			if id == "" {
				continue
			}
			if strings.TrimSpace(v.WorkGroupID) == id {
				out = append(out, v)
				continue
			}
			mem := groupMembers[id]
			if len(mem) == 0 {
				continue
			}
			hit := false
			for _, uid := range visitAssigneeIDs(v) {
				if mem[uid] {
					hit = true
					break
				}
			}
			if hit {
				out = append(out, v)
			}
		case "user":
			if id == "" {
				continue
			}
			hit := false
			for _, uid := range visitAssigneeIDs(v) {
				if uid == id {
					hit = true
					break
				}
			}
			if !hit && userName != "" {
				for _, n := range visitAssigneeNames(v) {
					if strings.EqualFold(n, userName) {
						hit = true
						break
					}
				}
			}
			if hit {
				out = append(out, v)
			}
		}
	}
	return out
}

func slotAssigneeName(s model.UnassignedMonthSlot) string {
	n := strings.TrimSpace(s.LastAssignee)
	if n != "" {
		return n
	}
	return strings.TrimSpace(s.RegionAssigneeName)
}

func filterSlotsByAssigneeKey(slots []model.UnassignedMonthSlot, key string, groups []MntCalGroup, userName string) []model.UnassignedMonthSlot {
	kind, id := parseAssigneeFilter(key)
	if kind == "all" {
		return slots
	}
	groupNames := map[string]bool{}
	if kind == "group" && id != "" {
		for _, g := range groups {
			if g.GroupID != id {
				continue
			}
			for _, m := range g.Members {
				if n := strings.TrimSpace(m.FullName); n != "" {
					groupNames[strings.ToLower(n)] = true
				}
			}
		}
	}
	out := make([]model.UnassignedMonthSlot, 0, len(slots))
	for _, s := range slots {
		name := slotAssigneeName(s)
		switch kind {
		case "none":
			if name == "" {
				out = append(out, s)
			}
		case "user":
			if id == "" {
				continue
			}
			if userName != "" && strings.EqualFold(name, userName) {
				out = append(out, s)
			}
		case "group":
			if id == "" || len(groupNames) == 0 {
				continue
			}
			if groupNames[strings.ToLower(name)] {
				out = append(out, s)
			}
		}
	}
	return out
}

func mntMonthStats(visits []model.MaintenanceVisit, today string) MntMonthStats {
	var s MntMonthStats
	s.Total = len(visits)
	for _, v := range visits {
		if mntVisitState(v, today) == mntStateDone {
			s.Done++
		}
	}
	s.Open = s.Total - s.Done
	return s
}

func defaultSelectedDay(year, month int, today string, visits []model.MaintenanceVisit) string {
	prefix := fmt.Sprintf("%04d-%02d-", year, month)
	if strings.HasPrefix(today, prefix) {
		return today
	}
	first := ""
	for _, v := range visits {
		d := strings.TrimSpace(v.VisitDate)
		if len(d) < 10 || !strings.HasPrefix(d, prefix) {
			continue
		}
		d = d[:10]
		if first == "" || d < first {
			first = d
		}
	}
	if first != "" {
		return first
	}
	return fmt.Sprintf("%04d-%02d-01", year, month)
}

func siteNameOf(v model.MaintenanceVisit) string {
	if strings.TrimSpace(v.ShortName) != "" {
		return v.ShortName
	}
	return v.OrgName
}

func buildDayChips(visits []model.MaintenanceVisit, today string, limit int) (chips []MntCalChip, more int) {
	if limit < 1 {
		limit = 3
	}
	for i, v := range visits {
		if i >= limit {
			more = len(visits) - limit
			break
		}
		st := mntVisitState(v, today)
		names := strings.Join(visitAssigneeNames(v), ", ")
		title := siteNameOf(v) + " · " + mntVisitStateLabel(st)
		if names != "" {
			title += " · " + names
		}
		chips = append(chips, MntCalChip{
			VisitID: v.VisitID,
			Label:   siteNameOf(v),
			State:   st,
			Title:   title,
		})
	}
	return chips, more
}

func attachCalendarChips(weeks [][]MntCalDay, today string, selected string) {
	for wi := range weeks {
		for di := range weeks[wi] {
			d := &weeks[wi][di]
			if d.Date == "" {
				continue
			}
			d.Selected = d.Date == selected
			d.Chips, d.More = buildDayChips(d.Visits, today, 3)
		}
	}
}

func buildVisitCards(visits []model.MaintenanceVisit, today, deleteAfter string, regions map[string]string) []MntVisitCard {
	out := make([]MntVisitCard, 0, len(visits))
	for _, v := range visits {
		st := mntVisitState(v, today)
		b := model.VisitStatusBadgeOf(v, today)
		names := visitAssigneeNames(v)
		label, title := formatAssigneeLine(names)
		reg := ""
		if regions != nil {
			reg = strings.TrimSpace(regions[v.CustomerID])
		}
		out = append(out, MntVisitCard{
			VisitID:       v.VisitID,
			Date:          v.VisitDate,
			Site:          siteNameOf(v),
			Product:       v.ProductType,
			Region:        reg,
			AssigneeLabel: label,
			AssigneeTitle: title,
			GroupName:     v.WorkGroupName,
			State:         st,
			StateLabel:    mntVisitStateLabel(st),
			BadgeLabel:    b.Label,
			BadgeExtra:    b.Extra,
			BadgeClass:    b.Class,
			RoundNo:       v.VisitRound(),
			Dup:           strings.TrimSpace(v.DupReason) != "",
			Completed:     v.Completed,
			CanDelete:     !v.Completed && v.VisitDate >= deleteAfter,
			CustomerID:    v.CustomerID,
			ProductType:   v.ProductType,
			Assignee:      v.Assignee,
			PeopleIDs:     visitPeopleIDsCSV(v),
			OwnerID:       visitOwnerUserID(v),
			WorkGroupID:   v.WorkGroupID,
			ProjectID:     v.ProjectID,
			EntryCategory: v.EntryCategory,
			Notes:         v.Notes,
			CompletedDate: v.CompletedDate,
		})
	}
	return out
}

func formatAssigneeLine(names []string) (label, title string) {
	title = strings.Join(names, ", ")
	switch len(names) {
	case 0:
		return "", ""
	case 1:
		return names[0], title
	default:
		return fmt.Sprintf("%s 외 %d", names[0], len(names)-1), title
	}
}

func cardsByDay(cards []MntVisitCard) map[string][]MntVisitCard {
	out := map[string][]MntVisitCard{}
	for _, c := range cards {
		d := c.Date
		if len(d) >= 10 {
			d = d[:10]
		}
		out[d] = append(out[d], c)
	}
	return out
}

func weekdayKo(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return ""
	}
	switch t.Weekday() {
	case time.Sunday:
		return "일요일"
	case time.Monday:
		return "월요일"
	case time.Tuesday:
		return "화요일"
	case time.Wednesday:
		return "수요일"
	case time.Thursday:
		return "목요일"
	case time.Friday:
		return "금요일"
	default:
		return "토요일"
	}
}

func calQuery(planID string, view string, month int, assignee, all, day string) string {
	q := url.Values{}
	if view != "" {
		q.Set("view", view)
	}
	if month > 0 {
		q.Set("month", fmt.Sprintf("%d", month))
	}
	if a := strings.TrimSpace(assignee); a != "" && a != "all" {
		q.Set("assignee", a)
	}
	if all == "1" {
		q.Set("all", "1")
	}
	if strings.TrimSpace(day) != "" {
		q.Set("day", day)
	}
	enc := q.Encode()
	if enc == "" {
		return "/maintenance/" + planID
	}
	return "/maintenance/" + planID + "?" + enc
}

func memberSummary(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	return names[0] + " · " + names[1]
}

func dayInWeeks(weeks [][]MntCalDay, day string) bool {
	day = strings.TrimSpace(day)
	if day == "" {
		return false
	}
	for _, w := range weeks {
		for _, d := range w {
			if d.Date == day {
				return true
			}
		}
	}
	return false
}

func calAssigneeLabel(key string, groups []MntCalGroup, people []MntCalPerson) string {
	kind, id := parseAssigneeFilter(key)
	switch kind {
	case "none":
		return "미배정"
	case "group":
		for _, g := range groups {
			if g.GroupID == id {
				return g.Name
			}
		}
		return "업무 그룹"
	case "user":
		for _, p := range people {
			if p.UserID == id {
				return p.FullName
			}
		}
		for _, g := range groups {
			for _, m := range g.Members {
				if m.UserID == id {
					return m.FullName
				}
			}
		}
		return "담당자"
	default:
		return "전체 담당자"
	}
}

func visitRegionMap(customers []model.CustomerListItem, cfgs []model.MaintenanceSiteConfig) map[string]string {
	out := map[string]string{}
	for _, c := range customers {
		if r := strings.TrimSpace(c.SiteRegion); r != "" {
			out[c.CustomerID] = r
			continue
		}
		if r := strings.TrimSpace(repository.ExtractKoreaRegion(c.AddrSido)); r != "" {
			out[c.CustomerID] = r
		}
	}
	for _, cfg := range cfgs {
		if r := strings.TrimSpace(cfg.Region); r != "" {
			out[cfg.CustomerID] = r
		}
	}
	return out
}

func attachCardHrefs(cards []MntVisitCard, planID, view string, month int, assignee, all, day string) {
	for i := range cards {
		d := cards[i].Date
		if len(d) >= 10 {
			d = d[:10]
		}
		q := strings.TrimPrefix(calQuery(planID, view, month, assignee, all, d), "/maintenance/"+planID)
		q = strings.TrimPrefix(q, "?")
		if q != "" {
			q = "&" + q
		}
		cards[i].CompleteHref = "/maintenance/visits/" + cards[i].VisitID + "/complete?plan_id=" + planID + q
		if cards[i].CanDelete {
			cards[i].DeleteHref = "/maintenance/visits/" + cards[i].VisitID + "/delete?plan_id=" + planID + q
		}
	}
}
