package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func planConfirmTitle(p *model.MaintenancePlan) string {
	if p == nil {
		return ""
	}
	if t := strings.TrimSpace(p.Title); t != "" {
		return t
	}
	return fmt.Sprintf("%d년 정기점검", p.PlanYear)
}

func (h *MaintenanceHandler) planErrRedirect(c echo.Context, planID, msg string) error {
	back := planURLFrom(c, planID, "")
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"err="+url.QueryEscape(msg))
}

func (h *MaintenanceHandler) planOKRedirect(c echo.Context, planID, msg string) error {
	back := planURLFrom(c, planID, "")
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"ok="+url.QueryEscape(msg))
}

func parseGenerateMonth(c echo.Context, plan *model.MaintenancePlan) int {
	raw := firstNonEmpty(c.FormValue("month"), c.QueryParam("month"))
	m, _ := strconv.Atoi(strings.TrimSpace(raw))
	if m >= 1 && m <= 12 {
		return m
	}
	now := time.Now()
	if plan != nil {
		return defaultPlanMonth(plan.PlanYear, nil, now)
	}
	return int(now.Month())
}

func generateConfirmURL(planID string, month int, order string) string {
	q := url.Values{}
	if month >= 1 && month <= 12 {
		q.Set("month", strconv.Itoa(month))
	}
	if o := strings.TrimSpace(order); o != "" {
		q.Set("assign_order", o)
	}
	loc := "/maintenance/" + planID + "/generate"
	if enc := q.Encode(); enc != "" {
		loc += "?" + enc
	}
	return loc
}

// GenerateConfirm 자동 배정 전 해당 월 삭제·보호 건수를 보여 준다. §34.4.5
func (h *MaintenanceHandler) GenerateConfirm(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	month := parseGenerateMonth(c, plan)
	st, err := h.repo.AutoAssignMonthStats(plan.PlanID, plan.PlanYear, month)
	if err != nil {
		return err
	}
	order := service.NormalizeGenerateOrder(c.QueryParam("assign_order"))
	configs, _ := h.repo.ListSiteConfigs()
	dist, _ := h.repo.ListRegionDistanceOrder()
	unreg := service.UnregisteredRegionCount(configs, dist)
	return c.Render(http.StatusOK, "maintenance/plan_generate.html", map[string]interface{}{
		"Title": "자동 배정 확인", "Active": NavMaintenance, "Plan": plan,
		"Year": plan.PlanYear, "Month": month,
		"DeleteCount": st.DeleteCount, "CompletedKeep": st.CompletedKeep,
		"PastKeep": st.PastKeep, "ManualKeep": st.ManualKeep,
		"PastMonth": st.PastMonth, "Today": st.Today,
		"TodayLabel":              model.FormatMonthDay(st.Today),
		"Visits":                  st.Visits,
		"AssignOrder":             order,
		"UnregisteredRegionCount": unreg,
		"HolidayMissing":          holidayMissingBanner(h.holidayRepo, plan.PlanYear),
		"PastMonthMsg":            model.ErrGeneratePastMonth.Error(),
	})
}

func (h *MaintenanceHandler) GenerateAuto(c echo.Context) error {
	id := c.Param("id")
	plan, _ := h.repo.GetPlan(id)
	month := parseGenerateMonth(c, plan)
	year := 0
	if plan != nil {
		year = plan.PlanYear
	}
	order := service.NormalizeGenerateOrder(c.FormValue("assign_order"))
	if c.FormValue("confirm") != "1" {
		return c.Redirect(http.StatusSeeOther, generateConfirmURL(id, month, order))
	}
	now := time.Now()
	if h.repo != nil && h.repo.Now != nil {
		now = h.repo.Now()
	}
	if model.IsPastGenerateMonth(year, month, now) {
		return h.planErrRedirect(c, id, model.ErrGeneratePastMonth.Error())
	}
	st, _ := h.repo.AutoAssignMonthStats(id, year, month)
	skipped, err := service.AutoGenerateMaintenanceWithOrder(h.repo, id, order, year, month)
	if err != nil {
		return h.planErrRedirect(c, id, err.Error())
	}
	if st.DeleteCount > 0 {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionBulkDelete,
			TargetTable: "maintenance_visits",
			TargetID:    id,
			SubjectType: "maintenance_plan",
			SubjectID:   id,
			SubjectName: planConfirmTitle(plan),
			Detail:      fmt.Sprintf("%d년 %d월 자동배정 전 자동생성 방문 %d건 삭제", year, month, st.DeleteCount),
			Reason:      "자동배정",
			Result:      auditlog.ResultOK,
		})
	}
	if h.wbRepo != nil {
		h.wbRepo.SyncMaintenanceBoard()
	}
	msg := "자동 배정을 반영했습니다"
	if skipped > 0 {
		msg = fmt.Sprintf("자동 배정을 반영했습니다. 이미 그달에 있어 건너뜀 %d건", skipped)
	}
	return h.planOKRedirect(c, id, msg)
}

const planDeleteDisabledMsg = "계획은 삭제할 수 없습니다. 보관만 가능합니다."

// DeletePlanPage 계획 삭제는 진입만 남기고 실제 삭제는 막는다. §34.4.4 · §23.11
func (h *MaintenanceHandler) DeletePlanPage(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	st, err := h.repo.PlanDeleteStats(plan.PlanID)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "maintenance/plan_delete.html", map[string]interface{}{
		"Title": "계획 삭제", "Active": NavMaintenance, "Plan": plan,
		"Stats": st, "Block": planDeleteDisabledMsg, "Error": c.QueryParam("err"),
		"ConfirmTitle": planConfirmTitle(plan),
	})
}

func (h *MaintenanceHandler) DeletePlan(c echo.Context) error {
	id := c.Param("id")
	plan, err := h.repo.GetPlan(id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	reason := accessReason(c, "계획 삭제")
	visits, _ := h.repo.ListVisits(id)
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		TargetTable: "maintenance_plans",
		TargetID:    plan.PlanID,
		SubjectType: "maintenance_plan",
		SubjectID:   plan.PlanID,
		SubjectName: planConfirmTitle(plan),
		Detail:      planDeleteDisabledMsg,
		Reason:      reason,
		Result:      auditlog.ResultDeny,
		BeforeJSON:  toJSON(map[string]interface{}{"plan": plan, "visits": visits}),
	})
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape(planDeleteDisabledMsg))
}

func (h *MaintenanceHandler) ArchivePlan(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.SetPlanStatus(id, "archived"); err != nil {
		return err
	}
	h.repo.TouchPlanUpdated(id)
	return h.planOKRedirect(c, id, "계획을 보관했습니다")
}

// DuplicatesPreview 중복 정리 대상 목록 (§23.12).
func (h *MaintenanceHandler) DuplicatesPreview(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	month, _ := strconv.Atoi(c.QueryParam("month"))
	var drops []repository.DuplicateDrop
	if month >= 1 && month <= 12 {
		drops, err = h.repo.ListDuplicateDrops(plan.PlanID, plan.PlanYear, month)
	} else {
		for m := 1; m <= 12; m++ {
			d, e := h.repo.ListDuplicateDrops(plan.PlanID, plan.PlanYear, m)
			if e != nil {
				return e
			}
			drops = append(drops, d...)
		}
	}
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "maintenance/plan_duplicates.html", map[string]interface{}{
		"Title": "중복 정리", "Active": NavMaintenance, "Plan": plan,
		"Drops": drops, "Month": month, "View": c.QueryParam("view"),
	})
}

func (h *MaintenanceHandler) CollapseDuplicates(c echo.Context) error {
	id := c.Param("id")
	if c.FormValue("confirm") != "1" {
		q := url.Values{}
		q.Set("month", c.FormValue("month"))
		q.Set("view", c.FormValue("view"))
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/duplicates?"+q.Encode())
	}
	plan, err := h.repo.GetPlan(id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	month, _ := strconv.Atoi(c.FormValue("month"))
	deleted, skipped := 0, 0
	if month >= 1 && month <= 12 {
		deleted, skipped, err = h.repo.CollapseDuplicateMonthVisits(id, plan.PlanYear, month)
	} else {
		for m := 1; m <= 12; m++ {
			d, s, e := h.repo.CollapseDuplicateMonthVisits(id, plan.PlanYear, m)
			if e != nil {
				return h.planErrRedirect(c, id, e.Error())
			}
			deleted += d
			skipped += s
		}
	}
	if err != nil {
		return h.planErrRedirect(c, id, err.Error())
	}
	if deleted > 0 {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionBulkDelete,
			TargetTable: "maintenance_visits",
			TargetID:    id,
			SubjectType: "maintenance_plan",
			SubjectID:   id,
			SubjectName: planConfirmTitle(plan),
			Detail:      fmt.Sprintf("중복 방문 %d건 정리 (제외 %d건)", deleted, skipped),
			Reason:      "중복정리",
			Result:      auditlog.ResultOK,
		})
	}
	msg := fmt.Sprintf("중복 %d건을 정리했습니다", deleted)
	if skipped > 0 {
		msg += fmt.Sprintf(" (당월·이전 %d건은 제외)", skipped)
	}
	return h.planOKRedirect(c, id, msg)
}

func backupPlanDeleteJSON(dataDir string, plan *model.MaintenancePlan, visits []model.MaintenanceVisit) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data"
	}
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	name := fmt.Sprintf("계획삭제_%s_%s.json", plan.PlanID, ts)
	path := filepath.Join(dir, name)
	payload := map[string]interface{}{
		"plan": plan, "visits": visits, "backed_up_at": time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		return "", err
	}
	return name, nil
}
