package handler

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type MaintenanceHandler struct {
	repo         *repository.MaintenanceRepo
	customerRepo *repository.CustomerRepo
	userRepo     *repository.UserRepo
	wbRepo       *repository.WBRepo
	settingsRepo *repository.SettingsRepo
	holidayRepo  *repository.HolidayRepo
	groups       *repository.WorkGroupRepo
	orgs         *repository.OrgRepo
	notices      *assignNoticeHook
	dataDir      string
}

func (h *MaintenanceHandler) requireVisiblePlan(c echo.Context, planID string) (*model.MaintenancePlan, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		planID = c.Param("id")
	}
	plan, err := h.repo.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	if plan == nil || !repository.VisibleInOrg(plan.OrgID, currentOrg(c)) {
		return nil, echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	return plan, nil
}

func (h *MaintenanceHandler) recordVisitAssign(c echo.Context, visitID, assignee, planID, customerID, product, date string) {
	if h == nil || h.notices == nil {
		return
	}
	visitID = strings.TrimSpace(visitID)
	if visitID == "" && h.repo != nil {
		if got := h.repo.FindVisitBySlot(planID, customerID, product, date); got != nil {
			visitID = got.VisitID
			if strings.TrimSpace(assignee) == "" {
				assignee = got.Assignee
			}
		}
	}
	if visitID == "" {
		return
	}
	h.notices.Record(c, model.AssignNoticeSourceMaintenance, visitID, assignee, "", "", "")
}

// syncVisitTask 방문 상태·일정·담당자를 연결된 일일업무(work_tasks)에 반영한다.
func (h *MaintenanceHandler) syncVisitTask(visitID string, completed bool) {
	if h == nil || h.wbRepo == nil || visitID == "" {
		return
	}
	_ = h.wbRepo.SyncMaintenanceTaskStatus(visitID, completed)
	_, _ = h.wbRepo.SyncMaintenanceTaskFromVisit(visitID)
}

// ListPlans 자기 조직의 올해 계획으로 이동한다. 없으면 「올해 점검 계획 만들기」.
func (h *MaintenanceHandler) ListPlans(c echo.Context) error {
	org := currentOrg(c)
	nowY := time.Now().Year()
	if org == repository.OrgAll {
		return c.Render(http.StatusOK, "maintenance/list.html", map[string]interface{}{
			"Title":          "정기점검 일정",
			"Active":         NavMaintenance,
			"IsAdmin":        isAdminRole(c),
			"Empty":          true,
			"NeedOrg":        true,
			"Year":           nowY,
			"CreateThisYear": false,
		})
	}
	plan, err := h.repo.GetPlanByYearInOrg(org, nowY)
	if err != nil {
		return err
	}
	if plan != nil {
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+plan.PlanID)
	}
	return c.Render(http.StatusOK, "maintenance/list.html", map[string]interface{}{
		"Title":          "정기점검 일정",
		"Active":         NavMaintenance,
		"IsAdmin":        isAdminRole(c),
		"Empty":          true,
		"NeedOrg":        false,
		"Year":           nowY,
		"CreateThisYear": true,
	})
}

func (h *MaintenanceHandler) NewPlanPage(c echo.Context) error {
	return c.Render(200, "maintenance/plan_new.html", map[string]interface{}{
		"Title":  "연도 계획 등록",
		"Active": NavMaintenance,
		"Year":   time.Now().Year(),
	})
}

func (h *MaintenanceHandler) CreatePlan(c echo.Context) error {
	year, _ := strconv.Atoi(c.FormValue("plan_year"))
	next := strings.TrimSpace(c.FormValue("next"))
	fromID := strings.TrimSpace(c.FormValue("from_plan_id"))
	if year < 2000 || year > 2100 {
		if next == "manage" && fromID != "" {
			return h.manageErrRedirect(c, fromID, "연도를 입력하세요")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "연도를 입력하세요")
	}
	title := c.FormValue("title")
	org, orgErr := resolveInsertOrg(c)
	if orgErr != nil {
		if next == "manage" && fromID != "" {
			return h.manageErrRedirect(c, fromID, orgErr.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, orgErr.Error())
	}
	p, err := h.repo.CreatePlanForOrg(org, year, title)
	if err != nil {
		if next == "manage" && fromID != "" {
			return h.manageErrRedirect(c, fromID, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if next == "manage" {
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+p.PlanID+"/manage")
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+p.PlanID)
}

func (h *MaintenanceHandler) ShowPlan(c echo.Context) error {
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	visits, err := h.repo.ListVisits(plan.PlanID)
	if err != nil {
		return err
	}
	visits = model.DatedVisits(visits)
	h.hydrateVisitPeople(visits)
	// 기술담당: 기본은 본인 배정만. all=1 이면 전체(타 담당자 포함).
	scopeAll := mntScopeAll(c)
	showScopeToggle := isTechRole(c)
	if showScopeToggle && !scopeAll {
		visits = filterVisitsByAssignee(visits, assigneeKeys(c))
	}
	customers, _, err := h.customerRepo.List(currentOrg(c), "", "", "", "", "", 1, 2000, false, model.PartyKindCustomer, false)
	if err != nil {
		return err
	}
	today := time.Now().Format(dateLayout)
	view := mntView(c.QueryParam("view"))
	month := 0
	if m := c.QueryParam("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}
	if month < 1 || month > 12 {
		month = 0
	}
	// 캘린더·칸반은 한 달을 보는 화면이라 달이 정해져야 한다.
	if month == 0 && view != mntViewList {
		month = defaultPlanMonth(plan.PlanYear, visits, time.Now())
	}

	quotaMonth := month
	if quotaMonth == 0 {
		if plan.PlanYear == time.Now().Year() {
			quotaMonth = int(time.Now().Month())
		} else {
			quotaMonth = 1
		}
	}

	shown := visits
	if month != 0 {
		shown = filterVisitsByMonth(visits, plan.PlanYear, month)
	}
	assigneeKey := strings.TrimSpace(c.QueryParam("assignee"))
	if assigneeKey == "" {
		assigneeKey = "all"
	}
	calGroups, calPeople, groupMembers := h.calFilterOptions(c, shown)
	userName := ""
	if kind, uid := parseAssigneeFilter(assigneeKey); kind == "user" && uid != "" {
		userName = h.userDisplayName(uid)
	}
	shown = filterVisitsByAssigneeKey(shown, assigneeKey, groupMembers, userName)

	doneCount := 0
	var incomplete []model.MaintenanceVisit
	for _, v := range visits {
		if v.Completed {
			doneCount++
		}
	}
	for _, v := range shown {
		if v.Completed {
			continue
		}
		if v.VisitDate != "" && v.VisitDate > today {
			continue
		}
		incomplete = append(incomplete, v)
	}
	board := buildVisitBoard(shown, today)
	var assignees []model.User
	if h.userRepo != nil {
		assignees = listAssignableForPicker(h.userRepo, c)
	}
	plans, _ := h.repo.ListPlans()
	yearQ := url.Values{}
	if v := strings.TrimSpace(c.QueryParam("view")); v != "" {
		yearQ.Set("view", v)
	}
	if month != 0 {
		yearQ.Set("month", strconv.Itoa(month))
	}
	if scopeAll {
		yearQ.Set("all", "1")
	}
	if a := strings.TrimSpace(assigneeKey); a != "" && a != "all" {
		yearQ.Set("assignee", a)
	}
	yearQuery := ""
	if enc := yearQ.Encode(); enc != "" {
		yearQuery = "?" + enc
	}
	var projects []model.WorkProject
	if h.wbRepo != nil {
		projects, _ = h.wbRepo.ListProjects(true)
	}
	var unassigned []model.UnassignedMonthSlot
	if quotaMonth >= 1 && quotaMonth <= 12 {
		unassigned, _ = h.repo.ListOpenUnassignedSlots(plan.PlanID, plan.PlanYear, quotaMonth)
		unassigned = repository.FilterUnassignedSlotsByOrg(unassigned, currentOrg(c))
	}
	dupMonth := month
	if view != mntViewList {
		dupMonth = quotaMonth
	}
	dupCount, _ := h.repo.CountDuplicateDrops(plan.PlanID, plan.PlanYear, dupMonth)
	qNum := model.CalendarQuarter(quotaMonth)
	qFrom, qTo := model.QuarterDateBounds(plan.PlanYear, qNum)
	weeks := decorateMntWeeks(buildVisitCalendar(plan.PlanYear, month, shown, today), today, h.holidayRepo, h.repo.Leaves(), currentUserID(c))
	selDay := strings.TrimSpace(c.QueryParam("day"))
	if !dayInWeeks(weeks, selDay) {
		selDay = defaultSelectedDay(plan.PlanYear, month, today, shown)
	}
	attachCalendarChips(weeks, today, selDay)
	scopeAllFlag := ""
	if scopeAll && showScopeToggle {
		scopeAllFlag = "1"
	}
	cfgs, _ := h.repo.ListSiteConfigs()
	regions := visitRegionMap(customers, cfgs)
	dayCards := buildVisitCards(shown, today, model.NextMonthStart(time.Now()).Format("2006-01-02"), regions)
	attachCardHrefs(dayCards, plan.PlanID, view, month, assigneeKey, scopeAllFlag, selDay)
	stats := mntMonthStats(shown, today)
	qScope := ""
	if scopeAll && showScopeToggle {
		qScope = "&all=1"
	}
	qAssignee := ""
	if a := strings.TrimSpace(assigneeKey); a != "" && a != "all" {
		qAssignee = "&assignee=" + a
	}
	periodGroups := buildPeriodGroups(h.repo, plan.PlanID, plan.PlanYear, quotaMonth, currentOrg(c))
	attachPeriodGroupRegions(h.repo, periodGroups)
	periodGroups = filterPeriodGroupsByAssignee(periodGroups, assigneeKey, calGroups, userName)
	unassigned = filterSlotsByAssigneeKey(unassigned, assigneeKey, calGroups, userName)
	var quarterPending []model.UnassignedMonthSlot
	quarterLast := model.IsLastMonthOfQuarter(quotaMonth)
	for _, g := range periodGroups {
		if g.Cycle == "quarterly" {
			quarterPending = g.Slots
			quarterLast = g.LastMonth
			break
		}
	}
	return c.Render(200, "maintenance/plan_show.html", map[string]interface{}{
		"Title":               fmt.Sprintf("정기점검 %d년", plan.PlanYear),
		"Active":              NavMaintenance,
		"Plan":                plan,
		"Plans":               plans,
		"YearQuery":           yearQuery,
		"Visits":              visits,
		"Customers":           customers,
		"Assignees":           assignees,
		"Projects":            projects,
		"Unassigned":          unassigned,
		"PeriodGroups":        periodGroups,
		"QuarterPending":      quarterPending,
		"QuarterLastMonth":    quarterLast,
		"QuarterAlert":        quarterLast && len(quarterPending) > 0,
		"QuarterNum":          qNum,
		"QuarterFrom":         qFrom,
		"QuarterTo":           qTo,
		"QuotaMonth":          quotaMonth,
		"View":                view,
		"Month":               month,
		"MonthVisits":         shown,
		"Weeks":               weeks,
		"Board":               board,
		"AssigneeKey":         assigneeKey,
		"AssigneeLabel":       calAssigneeLabel(assigneeKey, calGroups, calPeople),
		"CalGroups":           calGroups,
		"CalPeople":           calPeople,
		"SelectedDay":         selDay,
		"SelectedWeekday":     weekdayKo(selDay),
		"DayVisits":           cardsByDay(dayCards),
		"MonthStats":          stats,
		"HasExcel":            true,
		"ExportHref":          template.URL("/maintenance/" + plan.PlanID + "/export.xlsx"),
		"QScope":              template.URL(qScope),
		"QAssignee":           template.URL(qAssignee),
		"TodayHref":           template.URL(calTodayHref(plan.PlanID, plan.PlanYear, today, assigneeKey, scopeAllFlag)),
		"DoneCount":           doneCount,
		"OpenCount":           len(visits) - doneCount,
		"IncompleteVisits":    incomplete,
		"FlashDone":           c.QueryParam("done"),
		"FlashOK":             c.QueryParam("ok"),
		"FlashErr":            c.QueryParam("err"),
		"DupCount":            dupCount,
		"DeleteAfter":         model.NextMonthStart(time.Now()).Format("2006-01-02"),
		"Today":               today,
		"CanEdit":             canEditMaintenanceSchedule(c),
		"CanManageRegion":     canManageMntRegion(c),
		"IsAdmin":             isAdminRole(c),
		"ScopeAll":            scopeAll,
		"ShowScopeToggle":     showScopeToggle,
		"ScopeNote":           mntScopeNote(showScopeToggle, scopeAll),
		"HolidayMissing":      holidayMissingBanner(h.holidayRepo, plan.PlanYear),
		"OrgProducts":         h.orgProductList(plan.OrgID),
		"UseOrgProductLegend": h.useOrgProductLegend(plan.OrgID),
		"WorkGroups":          []model.WorkGroup{},
	})
}

func (h *MaintenanceHandler) hydrateVisitPeople(visits []model.MaintenanceVisit) {
	if h == nil || h.repo == nil || len(visits) == 0 {
		return
	}
	ids := make([]string, 0, len(visits))
	for _, v := range visits {
		ids = append(ids, v.VisitID)
	}
	by, _ := h.repo.ListVisitAssigneesForVisits(ids)
	repository.AttachVisitPeople(visits, by)
}

func (h *MaintenanceHandler) userDisplayName(uid string) string {
	if h == nil || h.userRepo == nil || strings.TrimSpace(uid) == "" {
		return ""
	}
	u, err := h.userRepo.GetByID(uid)
	if err != nil || u == nil {
		return ""
	}
	return strings.TrimSpace(u.FullName)
}

func (h *MaintenanceHandler) calFilterOptions(c echo.Context, visits []model.MaintenanceVisit) ([]MntCalGroup, []MntCalPerson, map[string]map[string]bool) {
	var groups []model.WorkGroup
	if h != nil && h.groups != nil {
		org := currentOrg(c)
		role := model.NormalizeRole(currentRole(c))
		admin := role == model.RoleOrgAdmin || role == model.RoleAdmin || role == model.RoleVisionAdmin || role == model.RoleCLevel
		if org == repository.OrgAll && (role == model.RoleVisionAdmin || role == model.RoleCLevel) {
			groups, _ = h.groups.ListGroups("", model.WorkGroupActive)
		} else {
			if org == "" || org == repository.OrgAll {
				org = workGroupActor(c).HomeOrg()
			}
			groups, _ = h.groups.ListVisibleActiveForWork(org, currentUserID(c), admin)
		}
	}
	orgNames := map[string]string{}
	if h != nil && h.orgs != nil {
		if rows, err := h.orgs.ListActive(); err == nil {
			for _, o := range rows {
				orgNames[o.OrgID] = o.OrgName
			}
		}
	}
	showOrg := currentOrg(c) == repository.OrgAll
	outG := make([]MntCalGroup, 0, len(groups))
	members := map[string]map[string]bool{}
	inGroup := map[string]bool{}
	for _, g := range groups {
		cg := MntCalGroup{GroupID: g.GroupID, Name: g.Name, MemberN: g.MemberN}
		if showOrg {
			cg.OrgLabel = orgNames[g.OrgID]
			if cg.OrgLabel == "" {
				cg.OrgLabel = g.OrgID
			}
		}
		var names []string
		if h != nil && h.groups != nil {
			ms, _ := h.groups.MembersOn(g.GroupID, "")
			mem := map[string]bool{}
			for _, m := range ms {
				mem[m.UserID] = true
				inGroup[m.UserID] = true
				names = append(names, m.FullName)
				cg.Members = append(cg.Members, MntCalPerson{UserID: m.UserID, FullName: m.FullName})
			}
			members[g.GroupID] = mem
			cg.MemberN = len(ms)
		}
		cg.MemberSummary = memberSummary(names)
		outG = append(outG, cg)
	}
	seen := map[string]bool{}
	var people []MntCalPerson
	addP := func(uid, name string) {
		uid = strings.TrimSpace(uid)
		name = strings.TrimSpace(name)
		if uid == "" || inGroup[uid] || seen[uid] {
			return
		}
		seen[uid] = true
		if name == "" {
			name = h.userDisplayName(uid)
		}
		people = append(people, MntCalPerson{UserID: uid, FullName: name})
	}
	for _, v := range visits {
		for _, p := range v.People {
			addP(p.UserID, p.AssigneeName)
		}
	}
	return outG, people, members
}

func calTodayHref(planID string, planYear int, today, assignee, all string) string {
	t, err := time.ParseInLocation("2006-01-02", today, time.Local)
	if err != nil {
		return calQuery(planID, mntViewCalendar, 0, assignee, all, "")
	}
	if t.Year() != planYear {
		return calQuery(planID, mntViewCalendar, 1, assignee, all, fmt.Sprintf("%d-01-01", planYear))
	}
	return calQuery(planID, mntViewCalendar, int(t.Month()), assignee, all, today)
}

func holidayOffSet(repo *repository.HolidayRepo, year int) map[string]bool {
	if repo == nil {
		return nil
	}
	m, err := repo.DatesSet(year)
	if err != nil {
		return nil
	}
	return m
}

func mntScopeNote(showToggle, scopeAll bool) string {
	if !showToggle {
		return "전체 일정"
	}
	if scopeAll {
		return "전체 일정(타 담당자 포함)"
	}
	return "내 점검 일정"
}

func (h *MaintenanceHandler) ApprovePlan(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.requireVisiblePlan(c, id); err != nil {
		return err
	}
	if err := h.repo.SetPlanStatus(id, "approved"); err != nil {
		return err
	}
	h.repo.TouchPlanUpdated(id)
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id)
}

func (h *MaintenanceHandler) UnapprovePlan(c echo.Context) error {
	id := c.Param("id")
	if _, err := h.requireVisiblePlan(c, id); err != nil {
		return err
	}
	if err := h.repo.SetPlanStatus(id, "draft"); err != nil {
		return err
	}
	h.repo.TouchPlanUpdated(id)
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id)
}

func (h *MaintenanceHandler) ExportExcel(c echo.Context) error {
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	visits, err := h.repo.ListVisits(plan.PlanID)
	if err != nil {
		return err
	}
	visits = model.DatedVisits(visits)
	if isTechRole(c) && !mntScopeAll(c) {
		visits = filterVisitsByAssignee(visits, assigneeKeys(c))
	}
	f, err := BuildMaintenanceExcelWorkbook(plan.PlanYear, visits, holidayOffSet(h.holidayRepo, plan.PlanYear), holidayNameMap(h.holidayRepo, plan.PlanYear))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "maintenance_plan")
}

func (h *MaintenanceHandler) AddVisit(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	planID := c.Param("id")
	if _, err := h.requireVisiblePlan(c, planID); err != nil {
		return err
	}
	v, err := parseVisitForm(c, planID, "")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.repo.InsertVisitFull(v); err != nil {
		setFlashErrCookie(c, err.Error())
		return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
	}
	if h.wbRepo != nil {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
	h.recordVisitAssign(c, v.VisitID, v.Assignee, planID, v.CustomerID, v.ProductType, v.VisitDate)
	h.repo.TouchPlanUpdated(planID)
	return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
}

// AssignUnassignedSlot 이번 달 미배정 슬롯에 방문일을 넣어 1건을 만든다.
func (h *MaintenanceHandler) AssignUnassignedSlot(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	planID := c.Param("id")
	if _, err := h.requireVisiblePlan(c, planID); err != nil {
		return err
	}
	date, err := model.ParseAppDate(c.FormValue("visit_date"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	cust := strings.TrimSpace(c.FormValue("customer_id"))
	product := strings.TrimSpace(c.FormValue("product_type"))
	if date == "" || cust == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "날짜와 고객을 선택하세요")
	}
	v := model.MaintenanceVisit{
		PlanID: planID, VisitDate: date, CustomerID: cust,
		ProductType: product, ProjectID: strings.TrimSpace(c.FormValue("project_id")),
		Assignee: strings.TrimSpace(c.FormValue("assignee")), EntryCategory: "normal",
	}
	if err := applyVisitDupForm(c, &v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.repo.AssignSlot(v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.wbRepo != nil {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
	h.recordVisitAssign(c, v.VisitID, v.Assignee, planID, cust, product, date)
	h.repo.TouchPlanUpdated(planID)
	back := planURLFrom(c, planID, "")
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"ok=assign")
}

// UpdateVisit 방문 일정·담당자·점검대상 등을 수정한다.
func (h *MaintenanceHandler) UpdateVisit(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	vid := c.Param("visit_id")
	existing, err := h.repo.GetVisit(vid)
	if err != nil || existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "방문을 찾을 수 없습니다")
	}
	planID := existing.PlanID
	if p := c.FormValue("plan_id"); p != "" {
		planID = p
	}
	v, err := parseVisitForm(c, planID, vid)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// 완료 체크박스가 폼에 없으면 기존 완료 상태를 유지한다(수정 모달에서 끈 경우만 해제).
	if c.FormValue("completed_present") == "1" {
		v.Completed = c.FormValue("completed") != ""
		if v.Completed {
			v.CompletedDate = strings.TrimSpace(c.FormValue("completed_date"))
			if v.CompletedDate == "" {
				v.CompletedDate = v.VisitDate
			}
		} else {
			v.CompletedDate = ""
		}
	} else {
		v.Completed = existing.Completed
		v.CompletedDate = existing.CompletedDate
	}
	if err := h.repo.UpdateVisit(v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	h.syncVisitTask(vid, v.Completed)
	h.recordVisitAssign(c, vid, v.Assignee, "", "", "", "")
	h.repo.TouchPlanUpdated(planID)
	back := planURLFrom(c, planID, "")
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"ok=edit")
}

func parseVisitForm(c echo.Context, planID, visitID string) (model.MaintenanceVisit, error) {
	date, err := model.ParseAppDate(c.FormValue("visit_date"))
	if err != nil {
		return model.MaintenanceVisit{}, err
	}
	cust := strings.TrimSpace(c.FormValue("customer_id"))
	cat := c.FormValue("entry_category")
	if cat == "" {
		cat = "normal"
	}
	if date == "" || cust == "" {
		return model.MaintenanceVisit{}, fmt.Errorf("날짜와 고객을 선택하세요")
	}
	v := model.MaintenanceVisit{
		VisitID: visitID, PlanID: planID, VisitDate: date, CustomerID: cust,
		EntryCategory: cat, Notes: strings.TrimSpace(c.FormValue("notes")),
		Assignee:    strings.TrimSpace(c.FormValue("assignee")),
		ProductType: strings.TrimSpace(c.FormValue("product_type")),
		ProjectID:   strings.TrimSpace(c.FormValue("project_id")),
		Completed:   c.FormValue("completed") != "",
		RoundNo:     parseVisitRound(c.FormValue("round_no")),
		WorkGroupID: strings.TrimSpace(c.FormValue("work_group_id")),
	}
	if v.Completed {
		v.CompletedDate = strings.TrimSpace(c.FormValue("completed_date"))
	}
	if err := applyVisitDupForm(c, &v); err != nil {
		return v, err
	}
	return v, nil
}

func parseVisitRound(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func applyVisitDupForm(c echo.Context, v *model.MaintenanceVisit) error {
	if v == nil {
		return nil
	}
	if strings.TrimSpace(c.FormValue("confirm_dup")) != "1" {
		return nil
	}
	reason := strings.TrimSpace(c.FormValue("dup_reason"))
	if reason == "" {
		return fmt.Errorf("그래도 추가하려면 사유를 입력하세요")
	}
	v.DupReason = reason
	return nil
}

// CompleteVisit 방문 완료 표시를 켜고 끈다.
func (h *MaintenanceHandler) CompleteVisit(c echo.Context) error {
	vid := c.Param("visit_id")
	planID := c.QueryParam("plan_id")
	done := c.FormValue("done") != "0"
	if err := h.repo.SetVisitCompleted(vid, done, strings.TrimSpace(c.FormValue("done_date"))); err != nil {
		return err
	}
	h.syncVisitTask(vid, done)
	if planID != "" {
		h.repo.TouchPlanUpdated(planID)
		return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance")
}

// CompleteVisitsUntil 지정일까지의 방문을 한 번에 완료 처리한다.
func (h *MaintenanceHandler) CompleteVisitsUntil(c echo.Context) error {
	planID := c.Param("id")
	until := strings.TrimSpace(c.FormValue("until"))
	if until == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "기준일을 입력하세요")
	}
	var only []string
	if isTechRole(c) && !mntScopeAll(c) {
		only = assigneeKeys(c)
	}
	n, err := h.repo.CompleteVisitsUntil(planID, until, only)
	if err != nil {
		return err
	}
	if h.wbRepo != nil {
		_ = h.wbRepo.BackfillMaintenanceTaskStatuses()
	}
	h.repo.TouchPlanUpdated(planID)
	return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, strconv.Itoa(n)))
}

// planURLFrom 요청의 보기·달·전체범위를 유지한 계획 화면 주소.
func planURLFrom(c echo.Context, planID, done string) string {
	view := firstNonEmpty(c.FormValue("view"), c.QueryParam("view"))
	month := firstNonEmpty(c.FormValue("month"), c.QueryParam("month"))
	all := mntScopeAll(c) && isTechRole(c)
	assignee := firstNonEmpty(c.FormValue("assignee"), c.QueryParam("assignee"))
	day := firstNonEmpty(c.FormValue("day"), c.QueryParam("day"))
	return planURLExtra(planID, view, month, done, all, assignee, day)
}

// planURL 계획 화면으로 돌아가는 주소 — 보던 보기 방식·달·담당자 범위를 유지한다.
func planURL(planID, view, month, done string, all bool) string {
	return planURLExtra(planID, view, month, done, all, "", "")
}

func planURLExtra(planID, view, month, done string, all bool, assignee, day string) string {
	q := url.Values{}
	if v := mntView(view); v != mntViewCalendar || view != "" {
		q.Set("view", v)
	}
	if month != "" && month != "0" {
		q.Set("month", month)
	}
	if all {
		q.Set("all", "1")
	}
	if a := strings.TrimSpace(assignee); a != "" && a != "all" {
		q.Set("assignee", a)
	}
	if d := strings.TrimSpace(day); d != "" {
		q.Set("day", d)
	}
	if done != "" {
		q.Set("done", done)
	}
	if len(q) == 0 {
		return "/maintenance/" + planID
	}
	return "/maintenance/" + planID + "?" + q.Encode()
}

func (h *MaintenanceHandler) DeleteVisit(c echo.Context) error {
	vid := c.Param("visit_id")
	planID := c.QueryParam("plan_id")
	back := "/maintenance"
	if planID != "" {
		back = planURLFrom(c, planID, "")
	}
	v, _ := h.repo.GetVisit(vid)
	subjectType, subjectID, subjectName := "maintenance_visit", vid, vid
	if v != nil {
		subjectType, subjectID, subjectName = "customer", v.CustomerID, v.OrgName
		if subjectName == "" {
			subjectName = v.ShortName
		}
	}
	if err := h.repo.DeleteVisit(vid); err != nil {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "maintenance_visits",
			TargetID:    vid,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			SubjectName: subjectName,
			Detail:      err.Error(),
			Reason:      accessReason(c, "방문 삭제"),
			Result:      auditlog.ResultDeny,
			BeforeJSON:  toJSON(v),
		})
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, back+sep+"err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		TargetTable: "maintenance_visits",
		TargetID:    vid,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		SubjectName: subjectName,
		Detail:      "방문 삭제",
		Reason:      accessReason(c, "방문 삭제"),
		Result:      auditlog.ResultOK,
		BeforeJSON:  toJSON(v),
	})
	// 원본이 사라지면 일일업무에 유령 카드가 남는다.
	if h.wbRepo != nil {
		_ = h.wbRepo.DeleteTasksBySource(model.WBSourceMaintenance, vid)
	}
	if planID != "" {
		h.repo.TouchPlanUpdated(planID)
		return c.Redirect(http.StatusSeeOther, back)
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance")
}

// --- 사이트 설정 ---

func (h *MaintenanceHandler) ListSiteConfigs(c echo.Context) error {
	return h.renderSiteList(c, "", 0, 0, 0)
}

// SyncSiteConfigsFromAssets 유상 계약 + 점검주기가 있는 자산의 기관을 사이트 설정에 일괄 등록
func (h *MaintenanceHandler) SyncSiteConfigsFromAssets(c echo.Context) error {
	res, err := h.repo.SyncSiteConfigsFromAssets()
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/maintenance/sites?synced=%d&skipped=%d", res.Added, res.Skipped))
}

const (
	errNeedSiteCustomer     = "고객을 목록에서 선택해 주세요"
	errSiteCustomerWrongOrg = "현재 조직의 고객만 등록할 수 있습니다"
	hintSiteCustomerMinQ    = "두 글자 이상 입력하세요"
)

type siteFormDraft struct {
	CustomerID   string
	CustomerName string
	ShortName    string
	Region       string
}

func (h *MaintenanceHandler) NewSiteConfigPage(c echo.Context) error {
	return h.renderSiteForm(c, nil, map[int]bool{}, false, "", http.StatusOK, siteFormDraft{})
}

func (h *MaintenanceHandler) SearchSiteCustomers(c echo.Context) error {
	q := strings.TrimSpace(c.QueryParam("q"))
	if len([]rune(q)) < 2 {
		return c.JSON(http.StatusOK, map[string]any{
			"items": []repository.SiteCustomerHit{},
			"hint":  hintSiteCustomerMinQ,
		})
	}
	items, err := h.repo.SearchCustomersWithoutConfig(currentOrg(c), q, 20)
	if err != nil {
		return err
	}
	if items == nil {
		items = []repository.SiteCustomerHit{}
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "hint": ""})
}

func (h *MaintenanceHandler) renderSiteForm(c echo.Context, cfg *model.MaintenanceSiteConfig, days map[int]bool, lastMon bool, errMsg string, status int, draft siteFormDraft) error {
	title := "점검 사이트 등록"
	if cfg != nil {
		title = "점검 사이트 수정"
	}
	if status == 0 {
		status = http.StatusOK
	}
	org := currentOrg(c)
	if cfg != nil && strings.TrimSpace(cfg.OrgID) != "" {
		org = cfg.OrgID
	}
	products := h.orgProductList(org)
	legacy := products == nil && model.UseLegacyInspectionSlots(org, h.repo.CountInspectionProducts(org))
	if !legacy && products == nil {
		products = []model.InspectionProduct{}
	}
	needOrg := cfg == nil && (org == "" || org == repository.OrgAll)
	if draft.CustomerID != "" && draft.CustomerName == "" && h.customerRepo != nil {
		if cu, _ := h.customerRepo.GetByID(draft.CustomerID); cu != nil {
			draft.CustomerName = cu.OrgName
		}
	}
	var users []model.User
	if h.userRepo != nil {
		users = listAssignableForPicker(h.userRepo, c)
	}
	return c.Render(status, "maintenance/site_form.html", map[string]interface{}{
		"Title":           title,
		"Active":          NavMaintenanceSites,
		"Config":          cfg,
		"Draft":           draft,
		"NeedOrg":         needOrg,
		"Products":        products,
		"Legacy":          legacy,
		"FixedDaySet":     days,
		"FixedLastMonday": lastMon,
		"Err":             errMsg,
		"Users":           users,
	})
}

func (h *MaintenanceHandler) EditSiteConfigPage(c echo.Context) error {
	cid := c.Param("customer_id")
	cfg, err := h.repo.GetSiteConfig(cid)
	if err != nil || cfg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "설정이 없습니다")
	}
	_, lastMon := model.ParseFixedRule(cfg.FixedRule)
	return h.renderSiteForm(c, cfg, model.FixedDaySet(cfg.FixedRule), lastMon, "", http.StatusOK, siteFormDraft{})
}

func parseFixedDayValues(vals []string) []int {
	var days []int
	seen := map[int]bool{}
	for _, s := range vals {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 || n > 31 || seen[n] {
			continue
		}
		seen[n] = true
		days = append(days, n)
	}
	return days
}

func (h *MaintenanceHandler) SaveSiteConfig(c echo.Context) error {
	_ = c.Request().ParseForm()
	days := parseFixedDayValues(c.Request().PostForm["fixed_day"])
	lastMon := c.FormValue("fixed_last_monday") == "1" || c.FormValue("fixed_rule") == model.FixedRuleLastMonday
	cfg := &model.MaintenanceSiteConfig{
		CustomerID:      strings.TrimSpace(c.FormValue("customer_id")),
		ShortName:       strings.TrimSpace(c.FormValue("short_name")),
		Region:          strings.TrimSpace(c.FormValue("region")),
		HasKlas:         c.FormValue("has_klas") == "1",
		HasRfid:         c.FormValue("has_rfid") == "1",
		InspectionCycle: c.FormValue("inspection_cycle"),
		EntryCategory:   c.FormValue("entry_category"),
		FixedRule:       model.FormatFixedRule(days, lastMon),
		RoundsPerPeriod: parseVisitRound(c.FormValue("rounds_per_period")),
	}
	draft := siteFormDraft{
		CustomerID:   cfg.CustomerID,
		CustomerName: strings.TrimSpace(c.FormValue("customer_q")),
		ShortName:    cfg.ShortName,
		Region:       cfg.Region,
	}
	existing, _ := h.repo.GetSiteConfig(cfg.CustomerID)
	creating := existing == nil
	if cfg.CustomerID == "" {
		return h.renderSiteForm(c, nil, model.FixedDaySet(cfg.FixedRule), lastMon, errNeedSiteCustomer, http.StatusBadRequest, draft)
	}
	if cfg.ShortName == "" {
		msg := "표시명은 필수입니다"
		st := http.StatusBadRequest
		if !creating {
			st = http.StatusOK
			msg = "고객과 표시명은 필수입니다"
		}
		show := existing
		if creating {
			show = nil
		}
		return h.renderSiteForm(c, show, model.FixedDaySet(cfg.FixedRule), lastMon, msg, st, draft)
	}
	if creating {
		view := currentOrg(c)
		if view == "" || view == repository.OrgAll {
			return h.renderSiteForm(c, nil, model.FixedDaySet(cfg.FixedRule), lastMon, errNeedInsertOrg, http.StatusBadRequest, draft)
		}
		if h.customerRepo != nil {
			cu, _ := h.customerRepo.GetByID(cfg.CustomerID)
			if cu == nil || !cu.IsActive {
				return h.renderSiteForm(c, nil, model.FixedDaySet(cfg.FixedRule), lastMon, errNeedSiteCustomer, http.StatusBadRequest, draft)
			}
		}
	}
	switch cfg.InspectionCycle {
	case "monthly", "odd_bimonthly", "even_bimonthly", "quarterly", "semi", "yearly":
	default:
		cfg.InspectionCycle = "monthly"
	}
	if cfg.EntryCategory == "" {
		cfg.EntryCategory = "normal"
	}
	if err := h.repo.UpsertSiteConfig(cfg); err != nil {
		log.Printf("site config save: %v", err)
		show := existing
		if creating {
			show = nil
		}
		return h.renderSiteForm(c, show, model.FixedDaySet(cfg.FixedRule), lastMon, err.Error(), http.StatusOK, draft)
	}
	if _, ok := c.Request().PostForm["product_id"]; ok {
		siteOrg := h.repo.CustomerOrgID(cfg.CustomerID)
		if siteOrg == "" {
			siteOrg = currentOrg(c)
		}
		if err := h.repo.ReplaceSiteProducts(cfg.CustomerID, siteOrg, c.Request().PostForm["product_id"]); err != nil {
			if repository.IsOtherOrgInspectionProduct(err) {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			show := cfg
			if creating {
				if got, _ := h.repo.GetSiteConfig(cfg.CustomerID); got != nil {
					show = got
				}
			}
			return h.renderSiteForm(c, show, model.FixedDaySet(cfg.FixedRule), lastMon, err.Error(), http.StatusOK, draft)
		}
	}
	_ = h.repo.ReplaceSiteAssignees(cfg.CustomerID, parseSiteAssigneeForm(c))
	return c.Redirect(http.StatusSeeOther, "/maintenance/sites")
}

func parseSiteAssigneeForm(c echo.Context) []model.MaintenanceSiteAssignee {
	owner := strings.TrimSpace(c.FormValue("site_owner_id"))
	gid := strings.TrimSpace(c.FormValue("site_work_group_id"))
	var out []model.MaintenanceSiteAssignee
	if owner != "" || gid != "" {
		out = append(out, model.MaintenanceSiteAssignee{UserID: owner, WorkGroupID: gid, Role: model.WBMemberOwner})
	}
	for _, id := range c.Request().PostForm["site_support_id"] {
		id = strings.TrimSpace(id)
		if id == "" || id == owner {
			continue
		}
		out = append(out, model.MaintenanceSiteAssignee{UserID: id, Role: model.WBMemberSupport})
	}
	return out
}

func (h *MaintenanceHandler) DeleteSiteConfig(c echo.Context) error {
	if err := h.repo.DeleteSiteConfig(c.Param("customer_id")); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/sites")
}

func (h *MaintenanceHandler) ExportSiteConfigsExcel(c echo.Context) error {
	items, err := h.repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	items = repository.FilterSiteConfigsByOrg(items, currentOrg(c))
	f, err := BuildSiteConfigExcelWorkbook(items)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "sites")
}

func buildPeriodGroups(repo *repository.MaintenanceRepo, planID string, year, month int, orgID string) []model.MaintenancePeriodGroup {
	if repo == nil || month < 1 {
		return nil
	}
	slots, _ := repo.PendingPeriodSlots(planID, year, month)
	slots = repository.FilterUnassignedSlotsByOrg(slots, orgID)
	by := map[string][]model.UnassignedMonthSlot{}
	order := []string{"quarterly", "semi", "yearly"}
	for _, s := range slots {
		by[s.Cycle] = append(by[s.Cycle], s)
	}
	var out []model.MaintenancePeriodGroup
	for _, cycle := range order {
		p, ok := model.CyclePeriodOf(cycle, month)
		if !ok {
			continue
		}
		list := by[cycle]
		if len(list) == 0 {
			continue
		}
		from, to := model.PeriodDateBounds(year, p)
		out = append(out, model.MaintenancePeriodGroup{
			Cycle: cycle, Index: p.Index, EndMonth: p.EndMonth, LastMonth: p.EndMonth == month,
			LabelShort: p.LabelShort(), ConfirmTitle: p.ConfirmTitle(), From: from, To: to, Slots: list,
		})
	}
	return out
}

func attachPeriodGroupRegions(repo *repository.MaintenanceRepo, groups []model.MaintenancePeriodGroup) {
	if repo == nil || len(groups) == 0 {
		return
	}
	cfgs, err := repo.ListSiteConfigs()
	if err != nil {
		return
	}
	by := map[string]string{}
	for _, c := range cfgs {
		by[c.CustomerID] = strings.TrimSpace(c.Region)
	}
	for gi := range groups {
		for si := range groups[gi].Slots {
			if groups[gi].Slots[si].Region == "" {
				groups[gi].Slots[si].Region = by[groups[gi].Slots[si].CustomerID]
			}
		}
	}
}

func filterPeriodGroupsByAssignee(groups []model.MaintenancePeriodGroup, key string, calGroups []MntCalGroup, userName string) []model.MaintenancePeriodGroup {
	if len(groups) == 0 {
		return groups
	}
	kind, _ := parseAssigneeFilter(key)
	if kind == "all" {
		return groups
	}
	var out []model.MaintenancePeriodGroup
	for _, g := range groups {
		g.Slots = filterSlotsByAssigneeKey(g.Slots, key, calGroups, userName)
		if len(g.Slots) == 0 {
			continue
		}
		out = append(out, g)
	}
	return out
}
