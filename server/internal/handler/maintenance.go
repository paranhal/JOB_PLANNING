package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

type MaintenanceHandler struct {
	repo         *repository.MaintenanceRepo
	customerRepo *repository.CustomerRepo
}

func (h *MaintenanceHandler) ListPlans(c echo.Context) error {
	plans, err := h.repo.ListPlans()
	if err != nil {
		return err
	}
	return c.Render(200, "maintenance/list.html", map[string]interface{}{
		"Title":  "정기점검 일정",
		"Active": "maintenance",
		"Plans":  plans,
	})
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
	customers, _, err := h.customerRepo.List("", 1, 2000)
	if err != nil {
		return err
	}
	month := 0
	if m := c.QueryParam("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}
	var monthVisits []model.MaintenanceVisit
	if month >= 1 && month <= 12 {
		monthVisits, err = h.repo.ListVisitsByMonth(plan.PlanID, plan.PlanYear, month)
		if err != nil {
			return err
		}
	}
	return c.Render(200, "maintenance/plan_show.html", map[string]interface{}{
		"Title":       fmt.Sprintf("정기점검 %d년", plan.PlanYear),
		"Active":      "maintenance",
		"Plan":        plan,
		"Visits":      visits,
		"Customers":   customers,
		"Month":       month,
		"MonthVisits": monthVisits,
	})
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
	f, err := BuildMaintenanceExcelWorkbook(plan.PlanYear, visits)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	fn := time.Now().Format("20060102") + "유지보수계획.xlsx"
	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, fn))
	c.Response().WriteHeader(http.StatusOK)
	return f.Write(c.Response())
}

func (h *MaintenanceHandler) AddVisit(c echo.Context) error {
	planID := c.Param("id")
	date := c.FormValue("visit_date")
	cust := c.FormValue("customer_id")
	cat := c.FormValue("entry_category")
	if cat == "" {
		cat = "normal"
	}
	if date == "" || cust == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "날짜와 고객을 선택하세요")
	}
	if err := h.repo.InsertVisit(planID, date, cust, 0, 0, cat, c.FormValue("notes")); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	h.repo.TouchPlanUpdated(planID)
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+planID)
}

func (h *MaintenanceHandler) DeleteVisit(c echo.Context) error {
	vid := c.Param("vid")
	planID := c.QueryParam("plan_id")
	if err := h.repo.DeleteVisit(vid); err != nil {
		return err
	}
	if planID != "" {
		h.repo.TouchPlanUpdated(planID)
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+planID)
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
		"Title":  "정기점검 사이트 설정",
		"Active": "maintenance_sites",
		"Items":  items,
	})
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
		CustomerID:    c.FormValue("customer_id"),
		ShortName:     c.FormValue("short_name"),
		Region:        c.FormValue("region"),
		HasKlas:       c.FormValue("has_klas") == "1",
		HasRfid:       c.FormValue("has_rfid") == "1",
		EntryCategory: c.FormValue("entry_category"),
		FixedRule:     c.FormValue("fixed_rule"),
	}
	if cfg.CustomerID == "" || cfg.ShortName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "고객과 표시명은 필수입니다")
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
