package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/hwpx"
	"customer-support/internal/model"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leaveapp"
	"customer-support/internal/service/leavecalc"
)

type LeaveHandler struct {
	svc *leaveapp.Service
	eng *approval.Engine
}

func (h *LeaveHandler) actor(c echo.Context) approval.Actor {
	return approval.Actor{
		UserID: currentUserID(c), Username: ctxString(c, "username"),
		Name: currentUserDisplayName(c), OrgID: currentOrg(c), Role: currentRole(c),
		ReadOnly: isReadOnly(c), ViewAs: viewAsActive(c),
	}
}

func (h *LeaveHandler) requireLeave(c echo.Context) error {
	if currentUserID(c) == "" {
		return echo.ErrForbidden
	}
	if !hasPerm(c, model.PermSelfLeave) {
		return echo.ErrForbidden
	}
	return nil
}

func (h *LeaveHandler) requireLeaveWrite(c echo.Context) error {
	if err := h.requireLeave(c); err != nil {
		return err
	}
	if isReadOnly(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	if currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	return nil
}

func (h *LeaveHandler) List(c echo.Context) error {
	if err := h.requireLeave(c); err != nil {
		return err
	}
	items, err := h.svc.ListMine(currentOrg(c), currentUserID(c))
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "leave/list.html", map[string]interface{}{
		"Title":  "내 휴가",
		"Active": NavLeaves,
		"Items":  items,
		"OK":     strings.TrimSpace(c.QueryParam("ok")),
		"Error":  strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *LeaveHandler) New(c echo.Context) error {
	if err := h.requireLeaveWrite(c); err != nil {
		return err
	}
	a := h.actor(c)
	head, _ := h.eng.OrgHead(a.OrgID)
	p, _ := h.eng.GetPolicy(a.OrgID)
	members, _ := h.svc.OrgMembers(a.OrgID)
	phone := ""
	_ = h.svc.DB.QueryRow(`SELECT COALESCE(phone,'') FROM employee_profiles WHERE user_id=?`, a.UserID).Scan(&phone)
	return c.Render(http.StatusOK, "leave/form.html", map[string]interface{}{
		"Title":       "근태허가원",
		"Active":      NavLeaves,
		"Types":       leavecalc.Types(),
		"Members":     members,
		"Phone":       phone,
		"NeedDeputy":  approval.RequireLeaveDeputy(p, head == a.UserID),
		"IsHead":      head == a.UserID,
		"Error":       strings.TrimSpace(c.QueryParam("err")),
		"PeriodDays":  hwpx.LeavePeriodDays,
		"PeriodHours": hwpx.LeavePeriodHours,
		"PeriodSwap":  hwpx.LeavePeriodSwap,
	})
}

func (h *LeaveHandler) Create(c echo.Context) error {
	if err := h.requireLeaveWrite(c); err != nil {
		return err
	}
	in := leaveapp.Input{
		TypeKey: c.FormValue("type_key"), TypeNote: c.FormValue("type_note"),
		PeriodMode: c.FormValue("period_mode"),
		StartDate:  c.FormValue("start_date"), EndDate: c.FormValue("end_date"),
		StartTime: c.FormValue("start_time"), EndTime: c.FormValue("end_time"),
		HalfPart: c.FormValue("half_part"),
		SwapDate: c.FormValue("swap_date"), SwapState: c.FormValue("swap_state"),
		Reason: c.FormValue("reason"), Destination: c.FormValue("destination"),
		Phone: c.FormValue("contact_phone"), DeputyUserID: c.FormValue("deputy_user_id"),
		ApproverDeputyUserID: c.FormValue("approver_deputy_user_id"),
	}
	if in.EndDate == "" {
		in.EndDate = in.StartDate
	}
	id, err := h.svc.Submit(h.actor(c), in)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/leaves/new?err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, Result: auditlog.ResultOK,
		TargetTable: "leave_requests", SubjectName: id, Detail: "근태허가원 상신",
	})
	return c.Redirect(http.StatusSeeOther, "/leaves/"+id+"?ok="+url.QueryEscape("상신했습니다."))
}

func (h *LeaveHandler) Show(c echo.Context) error {
	if err := h.requireLeave(c); err != nil {
		return err
	}
	req, err := h.svc.Get(c.Param("id"))
	if err != nil || req == nil {
		return echo.ErrNotFound
	}
	a := h.actor(c)
	if req.UserID != a.UserID && !isAdminRole(c) && !h.isOrgHead(a) {
		return echo.ErrForbidden
	}
	var steps []approval.Step
	var ar approval.Request
	if req.ApprovalRequestID != "" {
		ar, _ = h.eng.GetRequest(req.ApprovalRequestID)
		steps, _ = h.eng.ListSteps(req.ApprovalRequestID)
	}
	return c.Render(http.StatusOK, "leave/show.html", map[string]interface{}{
		"Title":       "근태허가원 " + req.DocNo,
		"Active":      NavLeaves,
		"Item":        req,
		"Type":        typeLabel(req.TypeKey),
		"AR":          ar,
		"Steps":       steps,
		"Stamps":      h.svc.StampCells(req),
		"OK":          strings.TrimSpace(c.QueryParam("ok")),
		"Error":       strings.TrimSpace(c.QueryParam("err")),
		"CanWithdraw": req.UserID == a.UserID && req.Status == "pending" && !a.ReadOnly && !a.ViewAs && currentRole(c) != model.RoleCLevel,
	})
}

func (h *LeaveHandler) Download(c echo.Context) error {
	if err := h.requireLeave(c); err != nil {
		return err
	}
	req, err := h.svc.Get(c.Param("id"))
	if err != nil || req == nil {
		return echo.ErrNotFound
	}
	a := h.actor(c)
	if req.UserID != a.UserID && !isAdminRole(c) && !h.isOrgHead(a) {
		return echo.ErrForbidden
	}
	doc, name, err := h.svc.FillHWPX(req.LeaveReqID)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/leaves/"+req.LeaveReqID+"?err="+url.QueryEscape(err.Error()))
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(http.StatusOK, "application/hwp+zip", doc)
}

func (h *LeaveHandler) Withdraw(c echo.Context) error {
	if err := h.requireLeaveWrite(c); err != nil {
		return err
	}
	req, err := h.svc.Get(c.Param("id"))
	if err != nil || req == nil {
		return echo.ErrNotFound
	}
	if req.ApprovalRequestID == "" {
		return c.Redirect(http.StatusSeeOther, "/leaves/"+req.LeaveReqID+"?err="+url.QueryEscape("결재 요청이 없다"))
	}
	if err := h.eng.Withdraw(h.actor(c), req.ApprovalRequestID); err != nil {
		return c.Redirect(http.StatusSeeOther, "/leaves/"+req.LeaveReqID+"?err="+url.QueryEscape(err.Error()))
	}
	_ = h.svc.SyncStatus(req.LeaveReqID, "withdrawn")
	return c.Redirect(http.StatusSeeOther, "/leaves/"+req.LeaveReqID+"?ok="+url.QueryEscape("회수했습니다."))
}

func (h *LeaveHandler) Inbox(c echo.Context) error {
	if !hasPerm(c, model.PermSelfApproval) {
		return echo.ErrForbidden
	}
	a := h.actor(c)
	items, err := h.eng.ListInbox(a.OrgID, a.UserID, "")
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "leave/inbox.html", map[string]interface{}{
		"Title":  "결재할 문서",
		"Active": NavApprovalsInbox,
		"Items":  items,
		"OK":     strings.TrimSpace(c.QueryParam("ok")),
		"Error":  strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *LeaveHandler) ApprovalShow(c echo.Context) error {
	if !hasPerm(c, model.PermSelfApproval) {
		return echo.ErrForbidden
	}
	ar, err := h.eng.GetRequest(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	a := h.actor(c)
	steps, _ := h.eng.ListSteps(ar.RequestID)
	acc := approval.AccessFor(a, ar, steps, delegateID(h.eng, steps))
	actLabel := ""
	if st := pendingStep(steps); st.ApproverID != "" {
		dec := h.eng.ResolveActor(mustPolicy(h.eng, ar.OrgID), st, ar.DrafterID, ar.OrgID, a, "")
		actLabel = dec.Label
	}
	if acc&approval.PermView == 0 {
		return echo.ErrForbidden
	}
	var leave *leaveapp.Request
	var stamps []leaveapp.StampCell
	if ar.DocType == approval.DocLeave {
		leave, _ = h.svc.Get(ar.DocID)
		stamps = h.svc.StampCells(leave)
	}
	return c.Render(http.StatusOK, "leave/approval_show.html", map[string]interface{}{
		"Title":    ar.Title,
		"Active":   NavApprovalsInbox,
		"AR":       ar,
		"Steps":    steps,
		"Leave":    leave,
		"Stamps":   stamps,
		"CanAct":   acc&approval.PermAct != 0 && ar.Status == approval.StatusInProgress,
		"ActLabel": actLabel,
		"OK":       strings.TrimSpace(c.QueryParam("ok")),
		"Error":    strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *LeaveHandler) Approve(c echo.Context) error {
	if !hasPerm(c, model.PermSelfApproval) || isReadOnly(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if err := h.eng.Approve(h.actor(c), id, c.FormValue("comment"), c.FormValue("emergency_reason")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/approvals/"+id+"?err="+url.QueryEscape(err.Error()))
	}
	ar, _ := h.eng.GetRequest(id)
	if ar.DocType == approval.DocLeave {
		_ = h.svc.SyncStatus(ar.DocID, "approved")
	}
	accessLog(c, auditlog.Record{Action: auditlog.ActionUpdate, Result: auditlog.ResultOK, TargetTable: "approval_requests", SubjectName: id, Detail: "승인"})
	return c.Redirect(http.StatusSeeOther, "/approvals/inbox?ok="+url.QueryEscape("승인했습니다."))
}

func (h *LeaveHandler) Reject(c echo.Context) error {
	if !hasPerm(c, model.PermSelfApproval) || isReadOnly(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if err := h.eng.Reject(h.actor(c), id, c.FormValue("comment")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/approvals/"+id+"?err="+url.QueryEscape(err.Error()))
	}
	ar, _ := h.eng.GetRequest(id)
	if ar.DocType == approval.DocLeave {
		_ = h.svc.SyncStatus(ar.DocID, "rejected")
	}
	return c.Redirect(http.StatusSeeOther, "/approvals/inbox?ok="+url.QueryEscape("반려했습니다."))
}

func (h *LeaveHandler) isOrgHead(a approval.Actor) bool {
	head, _ := h.eng.OrgHead(a.OrgID)
	return head != "" && head == a.UserID
}

func typeLabel(key string) string {
	t, ok := leavecalc.TypeByKey(key)
	if !ok {
		return key
	}
	return t.Label
}

func mustPolicy(e *approval.Engine, org string) approval.Policy {
	p, err := e.GetPolicy(org)
	if err != nil {
		return approval.DefaultPolicy(org)
	}
	return p
}

func pendingStep(steps []approval.Step) approval.Step {
	for _, s := range steps {
		if s.Status == approval.StepPending && s.StepType == approval.TypeApprove {
			return s
		}
	}
	return approval.Step{}
}

func delegateID(e *approval.Engine, steps []approval.Step) string {
	st := pendingStep(steps)
	if st.ApproverID == "" || e == nil {
		return ""
	}
	var to string
	_ = e.DB.QueryRow(`SELECT to_user_id FROM approval_delegations
		WHERE from_user_id=? AND revoked_at IS NULL AND start_date<=date('now') AND end_date>=date('now')
		ORDER BY created_at DESC LIMIT 1`, st.ApproverID).Scan(&to)
	return to
}
