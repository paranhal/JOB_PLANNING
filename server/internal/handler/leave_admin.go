package handler

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/leavecalc"
	"customer-support/internal/service/leaveledger"
)

func (h *LeaveHandler) canTeamView(c echo.Context) bool {
	if currentUserID(c) == "" {
		return false
	}
	if hasPerm(c, model.PermLeaveTeamView) || isAdminRole(c) {
		return true
	}
	return h.isOrgHead(h.actor(c))
}

func (h *LeaveHandler) canLeaveAdmin(c echo.Context) bool {
	return hasPerm(c, model.PermLeaveAdmin) || isAdminRole(c)
}

func (h *LeaveHandler) canLeaveAdminWrite(c echo.Context) bool {
	return h.canLeaveAdmin(c) && !isReadOnly(c) && !viewAsActive(c)
}

func (h *LeaveHandler) leaveScopeOrg(c echo.Context) string {
	role := currentRole(c)
	if role == model.RoleVisionAdmin || role == model.RoleCLevel {
		return ""
	}
	org := currentOrg(c)
	if org == repository.OrgAll {
		return ""
	}
	return org
}

func (h *LeaveHandler) ledger() *leaveledger.Service {
	if h.svc != nil && h.svc.Ledger != nil {
		return h.svc.Ledger
	}
	return leaveledger.New(h.svc.DB)
}

func (h *LeaveHandler) Team(c echo.Context) error {
	if !h.canTeamView(c) {
		return echo.ErrForbidden
	}
	asOf := h.nowDate()
	rows, err := h.ledger().OrgMemberRows(h.leaveScopeOrg(c), asOf)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "leave/team.html", map[string]interface{}{
		"Title":    "팀 휴가",
		"Active":   NavLeavesTeam,
		"Rows":     rows,
		"ShowOrg":  h.leaveScopeOrg(c) == "",
		"CanAdmin": h.canLeaveAdmin(c),
	})
}

func (h *LeaveHandler) Admin(c echo.Context) error {
	if !h.canLeaveAdmin(c) {
		return echo.ErrForbidden
	}
	led := h.ledger()
	scope := h.leaveScopeOrg(c)
	month := strings.TrimSpace(c.QueryParam("month"))
	if len(month) < 7 {
		month = h.nowDate()
		if len(month) >= 7 {
			month = month[:7]
		}
	}
	asOf := h.nowDate()
	members, err := led.OrgMemberRows(scope, asOf)
	if err != nil {
		return err
	}
	unreg, nohire, err := led.GapRows(scope)
	if err != nil {
		return err
	}
	abs, err := led.AbsenceRows(scope, month)
	if err != nil {
		return err
	}
	settle, err := led.SettlementRows(scope, asOf)
	if err != nil {
		return err
	}
	histUser := strings.TrimSpace(c.QueryParam("user_id"))
	var hist []leaveledger.HistoryRow
	if histUser != "" {
		if _, err := h.scopedUserOrg(c, histUser); err != nil {
			return err
		}
		hist, _ = led.History(histUser)
	}
	return c.Render(http.StatusOK, "leave/admin.html", map[string]interface{}{
		"Title":    "휴가 관리",
		"Active":   NavLeaveAdmin,
		"Members":  members,
		"Unreg":    unreg,
		"NoHire":   nohire,
		"Absence":  abs,
		"Settle":   settle,
		"Hist":     hist,
		"HistUser": histUser,
		"Month":    month,
		"ShowOrg":  scope == "",
		"CanWrite": h.canLeaveAdminWrite(c),
		"OK":       strings.TrimSpace(c.QueryParam("ok")),
		"Error":    strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *LeaveHandler) AbsenceXLSX(c echo.Context) error {
	if !h.canLeaveAdmin(c) {
		return echo.ErrForbidden
	}
	month := strings.TrimSpace(c.QueryParam("month"))
	rows, err := h.ledger().AbsenceRows(h.leaveScopeOrg(c), month)
	if err != nil {
		return err
	}
	xf := excelize.NewFile()
	sheet := xf.GetSheetName(0)
	_ = xf.SetSheetRow(sheet, "A1", &[]interface{}{"일시", "이름", "조직", "문서", "구분", "일수", "분", "비고"})
	for i, r := range rows {
		cell := fmt.Sprintf("A%d", i+2)
		_ = xf.SetSheetRow(sheet, cell, &[]interface{}{r.CreatedAt, r.Name, r.OrgID, r.LeaveReqID, r.Kind, r.DaysLabel, r.Minutes, r.Note})
	}
	buf, err := xf.WriteToBuffer()
	if err != nil {
		return err
	}
	name := "결근전환.xlsx"
	if len(month) >= 7 {
		name = "결근전환-" + month[:7] + ".xlsx"
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

func (h *LeaveHandler) SetHire(c echo.Context) error {
	if !h.canLeaveAdminWrite(c) {
		return echo.ErrForbidden
	}
	userID := strings.TrimSpace(c.FormValue("user_id"))
	hire := strings.TrimSpace(c.FormValue("hire_date"))
	org, err := h.scopedUserOrg(c, userID)
	if err != nil {
		return err
	}
	if hire == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/leaves?err="+url.QueryEscape("입사일을 입력하세요"))
	}
	if err := h.ledger().SetHireDate(userID, org, hire); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/leaves?err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{Action: auditlog.ActionUpdate, Result: auditlog.ResultOK, TargetTable: "employee_profiles", SubjectName: userID, Detail: "입사일"})
	return c.Redirect(http.StatusSeeOther, "/admin/leaves?ok="+url.QueryEscape("입사일을 저장했습니다.")+"&user_id="+url.QueryEscape(userID))
}

func (h *LeaveHandler) SetAuto(c echo.Context) error {
	if !h.canLeaveAdminWrite(c) {
		return echo.ErrForbidden
	}
	userID := strings.TrimSpace(c.FormValue("user_id"))
	on := c.FormValue("auto") == "1" || c.FormValue("auto") == "on"
	org, err := h.scopedUserOrg(c, userID)
	if err != nil {
		return err
	}
	led := h.ledger()
	if err := led.SetAutoEnabled(userID, org, on); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/leaves?err="+url.QueryEscape(err.Error()))
	}
	if on {
		_, _ = led.Balance(userID, h.nowDate())
	}
	msg := "자동 계산을 껐습니다."
	if on {
		msg = "자동 계산을 켰습니다."
	}
	accessLog(c, auditlog.Record{Action: auditlog.ActionUpdate, Result: auditlog.ResultOK, TargetTable: "employee_profiles", SubjectName: userID, Detail: msg})
	return c.Redirect(http.StatusSeeOther, "/admin/leaves?ok="+url.QueryEscape(msg)+"&user_id="+url.QueryEscape(userID))
}

func (h *LeaveHandler) Adjust(c echo.Context) error {
	if !h.canLeaveAdminWrite(c) {
		return echo.ErrForbidden
	}
	userID := strings.TrimSpace(c.FormValue("user_id"))
	if _, err := h.scopedUserOrg(c, userID); err != nil {
		return err
	}
	min, err := parseLeaveDays(c.FormValue("days"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/leaves?err="+url.QueryEscape(err.Error())+"&user_id="+url.QueryEscape(userID))
	}
	if err := h.ledger().Adjust(userID, currentUserID(c), c.FormValue("note"), c.FormValue("carry_approver"), min, strings.TrimSpace(c.FormValue("expires_on"))); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/leaves?err="+url.QueryEscape(err.Error())+"&user_id="+url.QueryEscape(userID))
	}
	accessLog(c, auditlog.Record{Action: auditlog.ActionUpdate, Result: auditlog.ResultOK, TargetTable: "leave_grants", SubjectName: userID, Detail: "연차 수동 조정"})
	return c.Redirect(http.StatusSeeOther, "/admin/leaves?ok="+url.QueryEscape("조정했습니다.")+"&user_id="+url.QueryEscape(userID))
}

func (h *LeaveHandler) scopedUserOrg(c echo.Context, userID string) (string, error) {
	if userID == "" {
		return "", echo.ErrBadRequest
	}
	var org string
	err := h.svc.DB.QueryRow(`SELECT COALESCE(org_id,'') FROM users WHERE user_id=?`, userID).Scan(&org)
	if err != nil {
		return "", echo.ErrNotFound
	}
	scope := h.leaveScopeOrg(c)
	if scope != "" && org != scope {
		return "", echo.ErrForbidden
	}
	if org == "" {
		org = currentOrg(c)
	}
	return org, nil
}

func parseLeaveDays(s string) (int, error) {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("일수를 입력하세요")
	}
	min := int(math.Round(f * float64(leavecalc.DefaultWorkMinutesPerDay)))
	if min == 0 {
		return 0, fmt.Errorf("조정 분(일수)이 없다")
	}
	return min, nil
}
