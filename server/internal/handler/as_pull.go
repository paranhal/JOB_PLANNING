package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/config"
	"customer-support/internal/model"
	"customer-support/internal/notify"
	"customer-support/internal/repository"
)

type openAssignFail struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// APIOpenByCustomer 기관의 안 끝난 AS·정기점검. 0건이면 빈 배열. §64.2
func (h *ASHandler) APIOpenByCustomer(c echo.Context) error {
	cid := strings.TrimSpace(c.Param("customer_id"))
	org := currentOrg(c)
	var items []model.CustomerOpenItem
	asList, err := h.repo.ListOpenByCustomer(org, cid)
	if err != nil {
		return err
	}
	for _, it := range asList {
		title := strings.TrimSpace(it.Symptom)
		if title == "" {
			title = it.ProductName
		}
		assignee := strings.TrimSpace(it.AssignedTo)
		if assignee == "" {
			assignee = model.StatsUnassignedLabel
		}
		items = append(items, model.CustomerOpenItem{
			Kind: "as", ItemKey: "as:" + it.ASID, ID: it.ASID, Number: it.ASNumber,
			Title: title, Assignee: assignee, VisitDate: it.VisitScheduledDate,
			Status: model.ASStatusDisplayLabel(it.Status), Href: "/as/" + it.ASID + "?embed=1",
		})
	}
	if h.maintRepo != nil {
		visits, err := h.maintRepo.ListOpenByCustomer(org, cid)
		if err != nil {
			return err
		}
		for _, v := range visits {
			name := v.ShortName
			if name == "" {
				name = v.OrgName
			}
			title := "정기점검"
			if v.ProductType != "" {
				title += " · " + v.ProductType
			}
			assignee := strings.TrimSpace(v.Assignee)
			if assignee == "" {
				assignee = model.StatsUnassignedLabel
			}
			items = append(items, model.CustomerOpenItem{
				Kind: "mnt", ItemKey: "mnt:" + v.VisitID, ID: v.VisitID,
				Number: name, Title: title, Assignee: assignee, VisitDate: v.VisitDate,
				Status: "미완료", Href: "/maintenance/visits/" + v.VisitID + "?embed=1",
			})
		}
	}
	if items == nil {
		items = []model.CustomerOpenItem{}
	}
	return c.JSON(http.StatusOK, items)
}

// PullOpen POST /as/open/pull — 담당=나, 수행일=오늘. §64.4
func (h *ASHandler) PullOpen(c echo.Context) error {
	if isReadOnly(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	keys := c.Request().PostForm["item_key"]
	reason := strings.TrimSpace(c.FormValue("takeover_reason"))
	today := time.Now().Format("2006-01-02")
	me := strings.TrimSpace(ctxString(c, "user_name"))
	meUID := strings.TrimSpace(ctxString(c, "user_id"))
	n, skipped, fails, needReason := h.assignOpenRows(c, keys, repeatStr(today, len(keys)), repeatStr(me, len(keys)), reason, true)
	if needReason {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "need_reason": true, "error": "남의 담당 건은 사유가 필요합니다"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "assigned": n, "skipped_empty": skipped, "failed": fails,
		"ask_close": n > 0, "assignee": me, "date": today, "me_uid": meUID,
	})
}

// AssignOpenDates POST /as/open/assign — 줄마다 날짜·담당자. §64.5
func (h *ASHandler) AssignOpenDates(c echo.Context) error {
	if isReadOnly(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["item_key"]
	dates := c.Request().PostForm["visit_date"]
	who := c.Request().PostForm["assignee"]
	reason := strings.TrimSpace(c.FormValue("takeover_reason"))
	n, skipped, fails, needReason := h.assignOpenRows(c, ids, dates, who, reason, false)
	if needReason {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "need_reason": true, "error": "남의 담당 건은 사유가 필요합니다"})
	}
	msg := ""
	if skipped > 0 {
		msg = fmt.Sprintf("날짜 없는 %d건은 배정하지 않았습니다", skipped)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "assigned": n, "skipped_empty": skipped, "failed": fails, "message": msg,
	})
}

func repeatStr(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func (h *ASHandler) assignOpenRows(c echo.Context, keys, dates, assignees []string, reason string, pull bool) (okN, skipped int, fails []openAssignFail, needReason bool) {
	today := time.Now().Format("2006-01-02")
	me := strings.TrimSpace(ctxString(c, "user_name"))
	meUID := strings.TrimSpace(ctxString(c, "user_id"))
	meUser := strings.TrimSpace(ctxString(c, "username"))
	org := currentOrg(c)

	if reason == "" {
		for i := range keys {
			key := strings.TrimSpace(keys[i])
			if key == "" {
				continue
			}
			date := ""
			if i < len(dates) {
				date = strings.TrimSpace(dates[i])
			}
			if date == "" {
				continue
			}
			oldName, oldUID, _, _, _ := h.openOwner(org, key)
			if oldName != "" && !isSelfAssign(meUID, me, meUser, oldUID, oldName, "") {
				return 0, 0, nil, true
			}
		}
	}

	for i := range keys {
		key := strings.TrimSpace(keys[i])
		if key == "" {
			continue
		}
		date := ""
		if i < len(dates) {
			date = strings.TrimSpace(dates[i])
		}
		who := me
		if i < len(assignees) && strings.TrimSpace(assignees[i]) != "" {
			who = strings.TrimSpace(assignees[i])
		}
		if date == "" {
			skipped++
			continue
		}
		parsed, err := model.ParseAppDate(date)
		if err != nil {
			fails = append(fails, openAssignFail{Key: key, Error: err.Error()})
			continue
		}
		date = parsed
		if date < today {
			fails = append(fails, openAssignFail{Key: key, Error: "지난 날짜는 배정할 수 없습니다"})
			continue
		}
		oldName, oldUID, label, kind, id := h.openOwner(org, key)
		if kind == "" {
			fails = append(fails, openAssignFail{Key: key, Error: "건을 찾지 못했습니다"})
			continue
		}
		foreign := oldName != "" && !isSelfAssign(meUID, me, meUser, oldUID, oldName, "")
		if err := h.applyOpenAssign(kind, id, date, who); err != nil {
			fails = append(fails, openAssignFail{Key: key, Error: err.Error()})
			continue
		}
		okN++
		h.recordOpenAssign(c, kind, id, who, oldName, oldUID, date, reason, pull, label, foreign)
	}
	return
}

func (h *ASHandler) openOwner(org, key string) (name, uid, label, kind, id string) {
	switch {
	case strings.HasPrefix(key, "as:"):
		id = strings.TrimPrefix(key, "as:")
		as, err := h.repo.GetByID(org, id)
		if err != nil || as == nil {
			as, err = h.repo.GetByID(repository.OrgAll, id)
		}
		if err != nil || as == nil {
			return "", "", "", "", ""
		}
		return as.AssignedTo, as.AssignedUserID, as.ASNumber, "as", as.ASID
	case strings.HasPrefix(key, "mnt:"):
		id = strings.TrimPrefix(key, "mnt:")
		if h.maintRepo == nil {
			return "", "", "", "", ""
		}
		v, err := h.maintRepo.GetVisit(id)
		if err != nil || v == nil {
			return "", "", "", "", ""
		}
		return v.Assignee, "", v.VisitID, "mnt", v.VisitID
	default:
		return "", "", "", "", ""
	}
}

func (h *ASHandler) applyOpenAssign(kind, id, date, who string) error {
	switch kind {
	case "as":
		uid := ""
		if h.userRepo != nil {
			if u := h.userRepo.FindAssignable(who); u != nil {
				uid = u.UserID
				who = u.FullName
			}
		}
		return h.repo.AssignUnplanned(id, date, who, uid)
	case "mnt":
		if h.maintRepo == nil {
			return fmt.Errorf("정기점검 저장소가 없습니다")
		}
		return h.maintRepo.AssignOpenVisit(id, date, who)
	default:
		return fmt.Errorf("알 수 없는 업무")
	}
}

func (h *ASHandler) recordOpenAssign(c echo.Context, kind, id, who, oldName, oldUID, date, reason string, pull bool, label string, foreign bool) {
	src := model.AssignNoticeSourceAS
	if kind == "mnt" {
		src = model.AssignNoticeSourceMaintenance
	}
	if h.notices != nil {
		h.notices.Record(c, src, id, who, "", oldName, oldUID)
	}
	if kind == "as" {
		h.syncASPlannedDailyTask(id)
	}
	verb := "배정"
	if pull {
		verb = "가져오기"
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		TargetTable: map[string]string{"as": "as_receipts", "mnt": "maintenance_visits"}[kind],
		TargetID:    id,
		Detail:      fmt.Sprintf("AS 미완료 %s %s → %s %s", verb, label, who, date),
		Reason:      accessReason(c, reason),
		Result:      auditlog.ResultOK,
		BeforeJSON:  toJSON(map[string]string{"assigned_to": oldName, "assigned_user_id": oldUID}),
		AfterJSON:   toJSON(map[string]string{"assigned_to": who, "date": date, "reason": reason}),
	})
	if !foreign || h.userRepo == nil || h.settingsRepo == nil {
		return
	}
	u := h.userRepo.FindAssignable(oldName)
	if u == nil && oldUID != "" {
		u, _ = h.userRepo.GetByID(oldUID)
	}
	if u == nil {
		return
	}
	me := strings.TrimSpace(ctxString(c, "user_name"))
	body := fmt.Sprintf("%s님이 %s 건을 가져갔습니다. 사유: %s", me, label, reason)
	_ = notify.Notify(h.db, h.settingsRepo, notify.FromApp(config.Load()), notify.Message{
		Purpose: notify.PurposeSameDay, OrgID: currentOrg(c),
		ToUserID: u.UserID, ToMobile: u.Mobile, ToEmail: u.Email,
		Title: "담당 건 가져오기: " + label, Body: body, CreatedBy: me,
	})
}
