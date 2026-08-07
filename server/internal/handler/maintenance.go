package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

type MaintenanceHandler struct {
	repo         *repository.MaintenanceRepo
	customerRepo *repository.CustomerRepo
	userRepo     *repository.UserRepo
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
			"Active":  "maintenance",
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
		"Active": "maintenance",
		"Year":   time.Now().Year(),
	})
}

func (h *MaintenanceHandler) CreatePlan(c echo.Context) error {
	year, _ := strconv.Atoi(c.FormValue("plan_year"))
	if year < 2000 || year > 2100 {
		return echo.NewHTTPError(http.StatusBadRequest, "연도를 입력하세요")
	}
	title := c.FormValue("title")
	p, err := h.repo.CreatePlan(year, title)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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
	// 기술담당: 기본은 본인 배정만. all=1 이면 전체(타 담당자 포함).
	scopeAll := mntScopeAll(c)
	showScopeToggle := isTechRole(c)
	if showScopeToggle && !scopeAll {
		visits = filterVisitsByAssignee(visits, assigneeKeys(c))
	}
	customers, _, err := h.customerRepo.List("", "", "", "", "", 1, 2000, false)
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
	return c.Render(200, "maintenance/plan_show.html", map[string]interface{}{
		"Title":           fmt.Sprintf("정기점검 %d년", plan.PlanYear),
		"Active":          "maintenance",
		"Plan":            plan,
		"Plans":           plans,
		"YearQuery":       yearQuery,
		"Visits":          visits,
		"Customers":       customers,
		"Assignees":       assignees,
		"View":            view,
		"Month":           month,
		"MonthVisits":     shown,
		"Weeks":           buildVisitCalendar(plan.PlanYear, month, shown, today),
		"Board":           board,
		"DoneCount":       doneCount,
		"OpenCount":       len(visits) - doneCount,
		"FlashDone":       c.QueryParam("done"),
		"FlashOK":         c.QueryParam("ok"),
		"Today":           today,
		"CanEdit":         canEditMaintenanceSchedule(c),
		"IsAdmin":         isAdminRole(c),
		"ScopeAll":        scopeAll,
		"ShowScopeToggle": showScopeToggle,
		"ScopeNote":       mntScopeNote(showScopeToggle, scopeAll),
	})
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

func (h *MaintenanceHandler) GenerateAuto(c echo.Context) error {
	id := c.Param("id")
	if err := service.AutoGenerateMaintenance(h.repo, id); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id)
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

func (h *MaintenanceHandler) DeletePlan(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.DeletePlan(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance")
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
	if isTechRole(c) && !mntScopeAll(c) {
		visits = filterVisitsByAssignee(visits, assigneeKeys(c))
	}
	f, err := BuildMaintenanceExcelWorkbook(plan.PlanYear, visits)
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
	h.repo.TouchPlanUpdated(planID)
	return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
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
	h.repo.TouchPlanUpdated(planID)
	back := planURLFrom(c, planID, "")
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"ok=edit")
}

func parseVisitForm(c echo.Context, planID, visitID string) (model.MaintenanceVisit, error) {
	date := strings.TrimSpace(c.FormValue("visit_date"))
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
	if err := h.repo.DeleteVisit(vid); err != nil {
		return err
	}
	if planID != "" {
		h.repo.TouchPlanUpdated(planID)
		return c.Redirect(http.StatusSeeOther, planURLFrom(c, planID, ""))
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance")
}

// --- 사이트 설정 ---

func (h *MaintenanceHandler) ListSiteConfigs(c echo.Context) error {
	items, err := h.repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	return c.Render(200, "maintenance/site_list.html", map[string]interface{}{
		"Title":   "정기점검 사이트 설정",
		"Active":  "maintenance_sites",
		"Items":   items,
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
		"Title":   "점검 사이트 등록",
		"Active":  "maintenance_sites",
		"Pending": pending,
		"Config":  (*model.MaintenanceSiteConfig)(nil),
	})
}

func (h *MaintenanceHandler) EditSiteConfigPage(c echo.Context) error {
	cid := c.Param("customer_id")
	cfg, err := h.repo.GetSiteConfig(cid)
	if err != nil || cfg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "설정이 없습니다")
	}
	return c.Render(200, "maintenance/site_form.html", map[string]interface{}{
		"Title":  "점검 사이트 수정",
		"Active": "maintenance_sites",
		"Config": cfg,
	})
}

func (h *MaintenanceHandler) SaveSiteConfig(c echo.Context) error {
	cfg := &model.MaintenanceSiteConfig{
		CustomerID:      c.FormValue("customer_id"),
		ShortName:       c.FormValue("short_name"),
		Region:          c.FormValue("region"),
		HasKlas:         c.FormValue("has_klas") == "1",
		HasRfid:         c.FormValue("has_rfid") == "1",
		InspectionCycle: c.FormValue("inspection_cycle"),
		EntryCategory:   c.FormValue("entry_category"),
		FixedRule:       c.FormValue("fixed_rule"),
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
