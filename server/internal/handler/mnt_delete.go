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

// GenerateConfirm 자동 배정 전 기존 자동 생성분 삭제 건수를 보여 준다.
func (h *MaintenanceHandler) GenerateConfirm(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	del, keep, err := h.repo.AutoVisitDeleteStats(plan.PlanID)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "maintenance/plan_generate.html", map[string]interface{}{
		"Title": "자동 배정 확인", "Active": NavMaintenance, "Plan": plan,
		"DeleteCount": del, "KeepCount": keep,
		"HolidayMissing": holidayMissingBanner(h.holidayRepo, plan.PlanYear),
	})
}

func (h *MaintenanceHandler) GenerateAuto(c echo.Context) error {
	id := c.Param("id")
	if c.FormValue("confirm") != "1" {
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/generate")
	}
	del, _, _ := h.repo.AutoVisitDeleteStats(id)
	plan, _ := h.repo.GetPlan(id)
	if err := service.AutoGenerateMaintenance(h.repo, id); err != nil {
		return h.planErrRedirect(c, id, err.Error())
	}
	if del > 0 {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionBulkDelete,
			TargetTable: "maintenance_visits",
			TargetID:    id,
			SubjectType: "maintenance_plan",
			SubjectID:   id,
			SubjectName: planConfirmTitle(plan),
			Detail:      fmt.Sprintf("자동배정 전 자동생성 방문 %d건 삭제", del),
			Reason:      "자동배정",
			Result:      auditlog.ResultOK,
		})
	}
	if h.wbRepo != nil {
		h.wbRepo.SyncMaintenanceBoard()
	}
	return h.planOKRedirect(c, id, "자동 배정을 반영했습니다")
}

// DeletePlanPage 계획 삭제 3단계 확인 (§23.11).
func (h *MaintenanceHandler) DeletePlanPage(c echo.Context) error {
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	st, err := h.repo.PlanDeleteStats(plan.PlanID)
	if err != nil {
		return err
	}
	block := ""
	now := time.Now()
	confirmTitle := planConfirmTitle(plan)
	if plan.PlanYear < now.Year() {
		block = fmt.Sprintf("%d년 계획은 삭제할 수 없습니다. 보관으로만 내릴 수 있습니다.", plan.PlanYear)
	} else if st.Completed > 0 {
		block = fmt.Sprintf("완료된 방문 %d건이 있어 계획을 삭제할 수 없습니다. 먼저 보관 상태로 바꾸세요.", st.Completed)
	} else if st.Protected > 0 {
		block = model.ProtectedVisitRangeMessage(now, st.Protected)
	}
	return c.Render(http.StatusOK, "maintenance/plan_delete.html", map[string]interface{}{
		"Title": "계획 삭제", "Active": NavMaintenance, "Plan": plan,
		"Stats": st, "Block": block, "Error": c.QueryParam("err"),
		"ConfirmTitle": confirmTitle,
	})
}

func (h *MaintenanceHandler) DeletePlan(c echo.Context) error {
	id := c.Param("id")
	plan, err := h.repo.GetPlan(id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	reason := accessReason(c, "계획 삭제")
	logPlan := func(result, detail string) {
		visits, _ := h.repo.ListVisits(id)
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "maintenance_plans",
			TargetID:    plan.PlanID,
			SubjectType: "maintenance_plan",
			SubjectID:   plan.PlanID,
			SubjectName: planConfirmTitle(plan),
			Detail:      detail,
			Reason:      reason,
			Result:      result,
			BeforeJSON:  toJSON(map[string]interface{}{"plan": plan, "visits": visits}),
		})
	}
	if strings.TrimSpace(c.FormValue("confirm_title")) != planConfirmTitle(plan) {
		logPlan(auditlog.ResultDeny, "계획 제목이 일치하지 않습니다")
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape("계획 제목이 일치하지 않습니다"))
	}
	pw := strings.TrimSpace(c.FormValue("delete_password"))
	if !verifyAndUpgradeSetting(h.settingsRepo, repository.SettingMaintenanceDeletePassword, pw) {
		logPlan(auditlog.ResultDeny, "비밀번호오류")
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape("삭제 비밀번호가 올바르지 않습니다"))
	}
	if err := h.repo.GuardPlanDelete(id); err != nil {
		logPlan(auditlog.ResultDeny, err.Error())
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape(err.Error()))
	}
	visits, _ := h.repo.ListVisits(id)
	if _, err := backupPlanDeleteJSON(h.dataDir, plan, visits); err != nil {
		logPlan(auditlog.ResultDeny, "삭제 전 백업에 실패했습니다")
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape("삭제 전 백업에 실패했습니다: "+err.Error()))
	}
	if err := h.repo.DeletePlan(id); err != nil {
		logPlan(auditlog.ResultDeny, err.Error())
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/delete?err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		TargetTable: "maintenance_plans",
		TargetID:    plan.PlanID,
		SubjectType: "maintenance_plan",
		SubjectID:   plan.PlanID,
		SubjectName: planConfirmTitle(plan),
		Detail:      fmt.Sprintf("계획 삭제 (방문 %d건 포함)", len(visits)),
		Reason:      reason,
		Result:      auditlog.ResultOK,
		BeforeJSON:  toJSON(map[string]interface{}{"plan": plan, "visits": visits}),
	})
	return c.Redirect(http.StatusSeeOther, "/maintenance")
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
