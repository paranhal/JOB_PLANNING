package handler

import (
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/service/leaveapp"
	"customer-support/internal/service/leavecalc"
)

type weeklyLeaveRow struct {
	ID     string
	DocNo  string
	Type   string
	Period string
	Status string
	Href   string
}

type weeklyInboxRow struct {
	ID     string
	Title  string
	Status string
	Href   string
}

func (h *MeetingHandler) fillWeeklyPersonalCards(c echo.Context, mon, fri time.Time, data map[string]interface{}) {
	if h == nil || h.leave == nil {
		return
	}
	uid := strings.TrimSpace(identityUserID(c))
	org := currentOrg(c)
	from := mon.Format("2006-01-02")
	to := fri.Format("2006-01-02")
	if h.leave.svc != nil && uid != "" {
		data["ShowLeave"] = true
		mine, err := h.leave.svc.ListMine(org, uid)
		if err == nil {
			var rows []weeklyLeaveRow
			for _, r := range mine {
				if !leaveOverlapsWeek(r, from, to) {
					continue
				}
				rows = append(rows, weeklyLeaveRow{
					ID: r.LeaveReqID, DocNo: r.DocNo, Type: leaveTypeLabel(r.TypeKey),
					Period: leavePeriodLabel(r), Status: r.Status, Href: "/leaves/" + r.LeaveReqID,
				})
			}
			data["LeaveRows"] = rows
			data["LeaveN"] = len(rows)
		}
		data["LeaveRemain"] = "—"
	}
	if h.leave.eng != nil && uid != "" {
		data["ShowInbox"] = true
		items, err := h.leave.eng.ListInbox(org, uid, "")
		if err == nil {
			n := len(items)
			data["InboxN"] = n
			top := items
			if len(top) > 8 {
				top = top[:8]
			}
			var rows []weeklyInboxRow
			for _, it := range top {
				rows = append(rows, weeklyInboxRow{
					ID: it.RequestID, Title: it.Title, Status: it.Status,
					Href: "/approvals/" + it.RequestID,
				})
			}
			data["InboxRows"] = rows
			if n == 0 {
				data["InboxNLabel"] = "—"
			}
		}
	}
}

func leaveOverlapsWeek(r leaveapp.Request, weekFrom, weekTo string) bool {
	s := strings.TrimSpace(r.StartDate)
	e := strings.TrimSpace(r.EndDate)
	if e == "" {
		e = s
	}
	if s == "" {
		return false
	}
	return s <= weekTo && e >= weekFrom
}

func leaveTypeLabel(key string) string {
	if td, ok := leavecalc.TypeByKey(strings.TrimSpace(key)); ok {
		return td.Label
	}
	if key == "" {
		return "—"
	}
	return key
}

func leavePeriodLabel(r leaveapp.Request) string {
	s := strings.TrimSpace(r.StartDate)
	e := strings.TrimSpace(r.EndDate)
	if s == "" {
		return "—"
	}
	if e == "" || e == s {
		return s
	}
	return s + " ~ " + e
}
