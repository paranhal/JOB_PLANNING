package handler

import (
	"fmt"
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
	dataDir      string
}

// syncVisitTask 방문 상태·일정·담당자를 연결된 일일업무(work_tasks)에 반영한다.
func (h *MaintenanceHandler) syncVisitTask(visitID string, completed bool) {
	if h == nil || h.wbRepo == nil || visitID == "" {
		return
	}
	_ = h.wbRepo.SyncMaintenanceTaskStatus(visitID, completed)
	_, _ = h.wbRepo.SyncMaintenanceTaskFromVisit(visitID)
}

// ListPlans 연도 목록 화면 없이 현재(또는 최신) 연도 계획으로 바로 이동한다.
func (h *MaintenanceHandler) ListPlans(c echo.Context) error {
	plans, err := h.repo.ListPlans()
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		return c.Render(http.StatusOK, "maintenance/list.html", map[string]interface{}{
			"Title":   "정기점검 일정",
			"Active":  NavMaintenance,
			"Plans":   plans,
			"IsAdmin": isAdminRole(c),
			"Empty":   true,
		})
	}
	nowY := time.Now().Year()
	for _, p := range plans {
		if p.PlanYear == nowY {
			return c.Redirect(http.StatusSeeOther, "/maintenance/"+p.PlanID)
		}
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+plans[0].PlanID)
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
	p, err := h.repo.CreatePlan(year, title)
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
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	visits, err := h.repo.ListVisits(plan.PlanID)
	if err != nil {
		return err
	}
	visits = model.DatedVisits(visits)
	// 기술담당: 기본은 본인 배정만. all=1 이면 전체(타 담당자 포함).
	scopeAll := mntScopeAll(c)
	showScopeToggle := isTechRole(c)
	if showScopeToggle && !scopeAll {
		visits = filterVisitsByAssignee(visits, assigneeKeys(c))
	}
	customers, _, err := h.customerRepo.List("", "", "", "", "", 1, 2000, false, model.PartyKindCustomer)
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

	doneCount := 0
	for _, v := range visits {
		if v.Completed {
			doneCount++
		}
	}
	board := buildVisitBoard(shown, today)
	var assignees []model.User
	if h.userRepo != nil {
		assignees, _ = h.userRepo.ListAssignable()
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
		unassigned, _ = h.repo.ListUnassignedMonthSlots(plan.PlanID, plan.PlanYear, quotaMonth)
	}
	dupMonth := month
	if view != mntViewList {
		dupMonth = quotaMonth
	}
	dupCount, _ := h.repo.CountDuplicateDrops(plan.PlanID, plan.PlanYear, dupMonth)
	return c.Render(200, "maintenance/plan_show.html", map[string]interface{}{
		"Title":           fmt.Sprintf("정기점검 %d년", plan.PlanYear),
		"Active":          NavMaintenance,
		"Plan":            plan,
		"Plans":           plans,
		"YearQuery":       yearQuery,
		"Visits":          visits,
		"Customers":       customers,
		"Assignees":       assignees,
		"Projects":        projects,
		"Unassigned":      unassigned,
		"QuotaMonth":      quotaMonth,
		"View":            view,
		"Month":           month,
		"MonthVisits":     shown,
		"Weeks":           decorateMntWeeks(buildVisitCalendar(plan.PlanYear, month, shown, today), today, h.holidayRepo, h.repo.Leaves(), currentUserID(c)),
		"Board":           board,
		"DoneCount":       doneCount,
		"OpenCount":       len(visits) - doneCount,
		"FlashDone":       c.QueryParam("done"),
		"FlashOK":         c.QueryParam("ok"),
		"FlashErr":        c.QueryParam("err"),
		"DupCount":        dupCount,
		"DeleteAfter":     model.NextMonthStart(time.Now()).Format("2006-01-02"),
		"Today":           today,
		"CanEdit":         canEditMaintenanceSchedule(c),
		"IsAdmin":         isAdminRole(c),
		"ScopeAll":        scopeAll,
		"ShowScopeToggle": showScopeToggle,
		"ScopeNote":       mntScopeNote(showScopeToggle, scopeAll),
		"HolidayMissing":  holidayMissingBanner(h.holidayRepo, plan.PlanYear),
	})
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
	if err := h.repo.SetPlanStatus(id, "approved"); err != nil {
		return err
	}
	h.repo.TouchPlanUpdated(id)
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id)
}

func (h *MaintenanceHandler) UnapprovePlan(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.SetPlanStatus(id, "draft"); err != nil {
		return err
	}
	h.repo.TouchPlanUpdated(id)
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id)
}

func (h *MaintenanceHandler) ExportExcel(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
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
	v, err := parseVisitForm(c, planID, "")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.repo.InsertVisitFull(v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.wbRepo != nil {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
	h.repo.TouchPlanUpdated(planID)
	return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
}

// AssignUnassignedSlot 이번 달 미배정 슬롯에 방문일을 넣어 1건을 만든다.
func (h *MaintenanceHandler) AssignUnassignedSlot(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	planID := c.Param("id")
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
	if err := h.repo.AssignSlot(v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.wbRepo != nil {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
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
	}
	if v.Completed {
		v.CompletedDate = strings.TrimSpace(c.FormValue("completed_date"))
	}
	return v, nil
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
	return planURL(planID, view, month, done, all)
}

// planURL 계획 화면으로 돌아가는 주소 — 보던 보기 방식·달·담당자 범위를 유지한다.
func planURL(planID, view, month, done string, all bool) string {
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
	items, err := h.repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	now := time.Now()
	quota := model.SummarizeMonthVisitQuota(items, now.Year(), int(now.Month()))
	return c.Render(200, "maintenance/site_list.html", map[string]interface{}{
		"Title":   "정기점검 사이트 설정",
		"Active":  NavMaintenanceSites,
		"Items":   items,
		"Quota":   quota,
		"Synced":  c.QueryParam("synced"),
		"Skipped": c.QueryParam("skipped"),
	})
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

func (h *MaintenanceHandler) NewSiteConfigPage(c echo.Context) error {
	pending, err := h.repo.ListCustomersWithoutConfig()
	if err != nil {
		return err
	}
	return c.Render(200, "maintenance/site_form.html", map[string]interface{}{
		"Title":           "점검 사이트 등록",
		"Active":          NavMaintenanceSites,
		"Pending":         pending,
		"Config":          (*model.MaintenanceSiteConfig)(nil),
		"FixedDaySet":     map[int]bool{},
		"FixedLastMonday": false,
	})
}

func (h *MaintenanceHandler) EditSiteConfigPage(c echo.Context) error {
	cid := c.Param("customer_id")
	cfg, err := h.repo.GetSiteConfig(cid)
	if err != nil || cfg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "설정이 없습니다")
	}
	_, lastMon := model.ParseFixedRule(cfg.FixedRule)
	return c.Render(200, "maintenance/site_form.html", map[string]interface{}{
		"Title":           "점검 사이트 수정",
		"Active":          NavMaintenanceSites,
		"Config":          cfg,
		"FixedDaySet":     model.FixedDaySet(cfg.FixedRule),
		"FixedLastMonday": lastMon,
	})
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
		CustomerID:      c.FormValue("customer_id"),
		ShortName:       c.FormValue("short_name"),
		Region:          c.FormValue("region"),
		HasKlas:         c.FormValue("has_klas") == "1",
		HasRfid:         c.FormValue("has_rfid") == "1",
		InspectionCycle: c.FormValue("inspection_cycle"),
		EntryCategory:   c.FormValue("entry_category"),
		FixedRule:       model.FormatFixedRule(days, lastMon),
	}
	if cfg.CustomerID == "" || cfg.ShortName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "고객과 표시명은 필수입니다")
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
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/sites")
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
