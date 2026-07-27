package handler

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ASHandler struct {
	repo         *repository.ASRepo
	processRepo  *repository.ASProcessRepo
	customerRepo *repository.CustomerRepo
	assetRepo    *repository.AssetRepo
	contactRepo  *repository.ContactRepo
	codeRepo     *repository.CodeRepo
	userRepo     *repository.UserRepo
	relationRepo *repository.RelationRepo
}

func (h *ASHandler) List(c echo.Context) error {
	status := c.QueryParam("status")
	search := c.QueryParam("search")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	// 기술담당: 기본 내 배정. mine=0 이면 전체 보기
	// assignee / assignee_name 이 있으면 해당 담당자로 필터 (대시보드 담당자별 링크)
	mineParam := c.QueryParam("mine")
	assigneeID := strings.TrimSpace(c.QueryParam("assignee"))
	assigneeName := strings.TrimSpace(c.QueryParam("assignee_name"))
	mine := false
	if assigneeID != "" || assigneeName != "" {
		mine = true
	} else if role == "tech" {
		mine = mineParam != "0"
	} else if mineParam == "1" {
		mine = true
	}

	sort, dir := parseASSort(c)

	var mineUserID string
	var mineKeys []string
	if mine {
		if assigneeID != "" {
			mineUserID = assigneeID
			if u, _ := h.userRepo.GetByID(assigneeID); u != nil {
				mineKeys = []string{u.FullName, u.Username}
			}
		} else if assigneeName != "" {
			mineKeys = []string{assigneeName}
		} else {
			mineUserID = uid
			mineKeys = keys
		}
	}

	items, total, err := h.repo.ListFiltered(status, search, mineUserID, mineKeys, sort, dir, page, 20)
	if err != nil {
		return err
	}

	// 통계 바: 기술담당은 대시보드와 동일(전체 건수 + 내 배정 진행/완료/지연/오늘/주간)
	var stats *model.ASStats
	if role == "tech" {
		allStats, _ := h.repo.DashboardStats("", nil)
		mineStats, _ := h.repo.DashboardStats(uid, keys)
		stats = &model.ASStats{
			TotalReceived: allStats.TotalReceived,
			InProgress:    mineStats.InProgress,
			Completed:     mineStats.Completed,
			Overdue:       mineStats.Overdue,
			TodayReceived: mineStats.TodayReceived,
			WeekReceived:  mineStats.WeekReceived,
			WeekCompleted: mineStats.WeekCompleted,
		}
	} else {
		stats, _ = h.repo.DashboardStats("", nil)
	}

	totalPages := (total + 19) / 20
	mineQ := ""
	if assigneeID == "" && assigneeName == "" {
		if mine {
			mineQ = "&mine=1"
		} else if role == "tech" {
			mineQ = "&mine=0"
		}
	}
	listBase := asListBaseQuery(status, search, sort, dir, mine, role)
	if assigneeID != "" {
		listBase.Set("assignee", assigneeID)
		listBase.Del("mine")
	}
	if assigneeName != "" {
		listBase.Set("assignee_name", assigneeName)
		listBase.Del("mine")
	}
	sortQ := asListQuerySuffix(listBase)

	statusLabel := status
	switch status {
	case "week_received":
		statusLabel = "주간접수"
	case "week_completed":
		statusLabel = "주간완료"
	case "week_in_progress":
		statusLabel = "주간진행중"
	}

	return c.Render(http.StatusOK, "as/list.html", map[string]interface{}{
		"Title": "AS 관리", "Active": "as",
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Status": status, "Search": search, "Stats": stats,
		"Mine": mine, "MineQ": mineQ, "SortQ": sortQ, "ListPath": "/as", "Role": role,
		"Sort": sort, "Dir": dir,
		"SortLinks": asListSortLinks("/as", listBase, sort, dir),
		"PrevURL": asPageURL("/as", listBase, page-1),
		"NextURL": asPageURL("/as", listBase, page+1),
		"DisplayName": ctxString(c, "user_name"),
		"CanReceive":  canReceiveAS(c), "CanProcess": canProcessAS(c),
		"StatusLabel": statusLabel,
		"AssigneeID":  assigneeID, "AssigneeName": assigneeName,
		"IsHoldList":  status == "hold",
	})
}

func parseASSort(c echo.Context) (sort, dir string) {
	sort = strings.TrimSpace(c.QueryParam("sort"))
	dir = strings.TrimSpace(c.QueryParam("dir"))
	if dir != "asc" && dir != "desc" {
		dir = "desc"
	}
	switch sort {
	case "org_name", "days", "as_number", "status", "assigned", "receipt":
	default:
		sort = "receipt"
	}
	return sort, dir
}

func asListBaseQuery(status, search, sort, dir string, mine bool, role string) url.Values {
	v := url.Values{}
	if status != "" {
		v.Set("status", status)
	}
	if search != "" {
		v.Set("search", search)
	}
	if sort == "" {
		sort = "receipt"
	}
	if dir == "" {
		dir = "desc"
	}
	if sort != "receipt" || dir != "desc" {
		v.Set("sort", sort)
		v.Set("dir", dir)
	}
	if role == "tech" {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	}
	return v
}

func asListQuerySuffix(v url.Values) string {
	if s := v.Encode(); s != "" {
		return "&" + s
	}
	return ""
}

// asPageURL 페이지 링크를 template.URL로 반환 (html/template의 href URL 이스케이프 방지)
func asPageURL(path string, base url.Values, page int) template.URL {
	v := cloneURLValues(base)
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	} else {
		v.Del("page")
	}
	if enc := v.Encode(); enc != "" {
		return template.URL(path + "?" + enc)
	}
	return template.URL(path)
}

func asListSortLinks(path string, base url.Values, currentSort, currentDir string) map[string]template.URL {
	fields := []string{"receipt", "as_number", "org_name", "status", "assigned", "days"}
	links := make(map[string]template.URL, len(fields))
	for _, field := range fields {
		v := cloneURLValues(base)
		v.Set("sort", field)
		newDir := "desc"
		if field == "org_name" {
			newDir = "asc"
		}
		if currentSort == field {
			if currentDir == "asc" {
				newDir = "desc"
			} else {
				newDir = "asc"
			}
		}
		v.Set("dir", newDir)
		v.Del("page")
		links[field] = template.URL(path + "?" + v.Encode())
	}
	return links
}

func cloneURLValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vals := range v {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
}

func asStatsListBaseQuery(status, sort, dir string) url.Values {
	v := url.Values{}
	if status != "" && status != "in_progress" {
		v.Set("status", status)
	}
	if sort == "" {
		sort = "receipt"
	}
	if dir == "" {
		dir = "desc"
	}
	if sort != "receipt" || dir != "desc" {
		v.Set("sort", sort)
		v.Set("dir", dir)
	}
	return v
}

func (h *ASHandler) New(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	customers, _ := h.customerRepo.ListAll()
	channels, _ := h.codeRepo.ActiveByGroup("receipt_channel")
	urgencies, _ := h.codeRepo.ActiveByGroup("urgency")
	reqTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	assignees, _ := h.userRepo.ListAssignable()
	reqNames, _ := h.distinctRequesterNames()

	now := time.Now()
	as := &model.ASReceipt{
		Urgency: "normal", Priority: "normal", Status: "received",
		ReceivedBy:      ctxString(c, "user_name"),
		ReceiptDatetime: now,
	}
	if cid := c.QueryParam("customer_id"); cid != "" {
		as.CustomerID = cid
	}
	if aid := c.QueryParam("asset_id"); aid != "" {
		as.AssetID = aid
	}

	var assets []model.Asset
	var contacts []model.Contact
	var history []model.ASHistoryItem
	if as.CustomerID != "" {
		assets, _ = h.assetRepo.ListByCustomer(as.CustomerID)
		contacts, _ = h.contactRepo.ListByCustomer(as.CustomerID)
		history, _ = h.repo.ListPastHistory(as.CustomerID, "", 50)
	}

	return c.Render(http.StatusOK, "as/form.html", map[string]interface{}{
		"Title": "AS 접수", "Active": "as", "IsNew": true,
		"AS": as, "Customers": customers, "Assets": assets, "Contacts": contacts,
		"Channels": channels, "Urgencies": urgencies, "ReqTypes": reqTypes,
		"Assignees": assignees, "RequesterNames": reqNames,
		"ReceiptLocal":     now.Format("2006-01-02T15:04"),
		"CanEditVisitDate": true, // 신규 접수 시 접수 권한자가 설정 가능
		"History":          history,
	})
}

func (h *ASHandler) distinctRequesterNames() ([]string, error) {
	if h.relationRepo == nil {
		return nil, nil
	}
	return h.relationRepo.DistinctCompanyNames()
}

func (h *ASHandler) parseReceiptForm(c echo.Context) *model.ASReceipt {
	requester := c.FormValue("requester_code")
	if requester == "custom" {
		requester = c.FormValue("requester_custom")
	} else if requester == "" {
		requester = c.FormValue("requester")
	}

	reqName := c.FormValue("requester_name_code")
	if reqName == "custom" {
		reqName = c.FormValue("requester_name_custom")
	} else if reqName == "" {
		reqName = c.FormValue("requester_name")
	}

	assignedCode := c.FormValue("assigned_to_code")
	assignedName := ""
	assignedUID := ""
	if assignedCode == "custom" {
		assignedName = strings.TrimSpace(c.FormValue("assigned_to_custom"))
	} else if assignedCode != "" {
		if u, _ := h.userRepo.GetByID(assignedCode); u != nil {
			assignedUID = u.UserID
			assignedName = u.FullName
		} else {
			assignedName = assignedCode
		}
	} else if v := c.FormValue("assigned_to"); v != "" {
		assignedName = v
	}

	as := &model.ASReceipt{
		CustomerID:         c.FormValue("customer_id"),
		AssetID:            c.FormValue("asset_id"),
		ReceiptChannel:     c.FormValue("receipt_channel"),
		Requester:          requester,
		Symptom:            c.FormValue("symptom"),
		Urgency:            c.FormValue("urgency"),
		Priority:           c.FormValue("priority"),
		RequesterType:      c.FormValue("requester_type"),
		RequesterName:      reqName,
		AssignedTo:         assignedName,
		AssignedUserID:     assignedUID,
		ReceivedBy:         strings.TrimSpace(c.FormValue("received_by")),
		VisitScheduledDate: normalizeVisitDate(c.FormValue("visit_scheduled_date")),
		ScheduleConfirmed:  c.FormValue("schedule_confirmed") == "1",
	}
	if as.ReceivedBy == "" {
		as.ReceivedBy = ctxString(c, "user_name")
	}
	if dt := c.FormValue("receipt_datetime"); dt != "" {
		if t, err := time.Parse("2006-01-02T15:04", dt); err == nil {
			as.ReceiptDatetime = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", dt); err == nil {
			as.ReceiptDatetime = t
		}
	}
	if as.Urgency == "" {
		as.Urgency = "normal"
	}
	if as.Priority == "" {
		as.Priority = "normal"
	}
	return as
}

func (h *ASHandler) Create(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	as := h.parseReceiptForm(c)
	if as.ScheduleConfirmed && as.VisitScheduledDate == "" {
		as.ScheduleConfirmed = false
	}
	if err := h.repo.Create(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+as.ASID)
}

func (h *ASHandler) Edit(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}

	customers, _ := h.customerRepo.ListAll()
	channels, _ := h.codeRepo.ActiveByGroup("receipt_channel")
	urgencies, _ := h.codeRepo.ActiveByGroup("urgency")
	reqTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	assignees, _ := h.userRepo.ListAssignable()
	reqNames, _ := h.distinctRequesterNames()

	var assets []model.Asset
	var contacts []model.Contact
	var history []model.ASHistoryItem
	if as.CustomerID != "" {
		assets, _ = h.assetRepo.ListByCustomer(as.CustomerID)
		contacts, _ = h.contactRepo.ListByCustomer(as.CustomerID)
		history, _ = h.repo.ListPastHistory(as.CustomerID, as.ASID, 50)
	}

	return c.Render(http.StatusOK, "as/form.html", map[string]interface{}{
		"Title": "AS 접수 수정", "Active": "as", "IsNew": false,
		"AS": as, "Customers": customers, "Assets": assets, "Contacts": contacts,
		"Channels": channels, "Urgencies": urgencies, "ReqTypes": reqTypes,
		"Assignees": assignees, "RequesterNames": reqNames,
		"ReceiptLocal":     as.ReceiptDatetime.Format("2006-01-02T15:04"),
		"CanEditVisitDate": canEditVisitDate(c, as),
		"History":          history,
		"Err":              c.QueryParam("err"),
	})
}

func (h *ASHandler) UpdateReceipt(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	existing, err := h.repo.GetByID(id)
	if err != nil || existing == nil {
		return echo.ErrNotFound
	}
	as := h.parseReceiptForm(c)
	as.ASID = existing.ASID
	as.ASNumber = existing.ASNumber
	if as.ReceiptDatetime.IsZero() {
		as.ReceiptDatetime = existing.ReceiptDatetime
	}
	// 방문 예정일·일정확정: 관리자·배정담당자만 변경. 그 외는 기존 값 유지
	if !canEditVisitDate(c, existing) {
		as.VisitScheduledDate = existing.VisitScheduledDate
		as.ScheduleConfirmed = existing.ScheduleConfirmed
	} else if as.ScheduleConfirmed && as.VisitScheduledDate == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/edit?err=schedule_date")
	}
	if err := h.repo.UpdateReceipt(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// UpdateVisitDate 상세에서 방문 예정일만 수정 (관리자·배정담당자)
func (h *ASHandler) UpdateVisitDate(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if !canEditVisitDate(c, as) {
		return echo.ErrForbidden
	}
	date := normalizeVisitDate(c.FormValue("visit_scheduled_date"))
	confirmed := c.FormValue("schedule_confirmed") == "1"
	if confirmed && date == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=schedule_date")
	}
	if err := h.repo.UpdateVisitScheduledDate(id, date, confirmed); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

func (h *ASHandler) Show(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}

	processes, _ := h.processRepo.ListByAS(id)
	procTypes, _ := h.codeRepo.ActiveByGroup("process_type")
	causeTypes, _ := h.codeRepo.ActiveByGroup("cause_type")
	resultCodes, _ := h.codeRepo.ActiveByGroup("result_code")
	assignees, _ := h.userRepo.ListAssignable()
	contacts, _ := h.contactRepo.ListByCustomer(as.CustomerID)
	history, _ := h.repo.ListPastHistory(as.CustomerID, as.ASID, 50)

	now := time.Now()
	startLocal := ""
	if as.StartDatetime != nil && !as.StartDatetime.IsZero() {
		startLocal = as.StartDatetime.Format("2006-01-02T15:04")
	}
	completeLocal := ""
	if as.CompleteDatetime != nil && !as.CompleteDatetime.IsZero() {
		completeLocal = as.CompleteDatetime.Format("2006-01-02T15:04")
	}
	cancelLocal := now.Format("2006-01-02")
	if as.CancelDatetime != nil && !as.CancelDatetime.IsZero() {
		cancelLocal = as.CancelDatetime.Format("2006-01-02")
	}

	// 워크플로 콤보: 접수/진행중 → 조치·보류·이관 / 보류 → 조치·이관·접수취소
	showWorkflow := as.Status == "received" || as.Status == "assigned" || as.Status == "in_progress" || as.Status == "hold"
	workflowHoldMode := as.Status == "hold"
	showTransferComplete := as.Status == "transfer"
	confirmerIsCustom := as.CustomerConfirmer != ""
	if confirmerIsCustom {
		for _, ct := range contacts {
			if ct.FullName == as.CustomerConfirmer {
				confirmerIsCustom = false
				break
			}
		}
	}

	return c.Render(http.StatusOK, "as/show.html", map[string]interface{}{
		"Title": as.ASNumber, "Active": "as", "AS": as,
		"Processes": processes,
		"ProcTypes": procTypes, "CauseTypes": causeTypes, "ResultCodes": resultCodes,
		"Assignees":            assignees,
		"Contacts":             contacts,
		"History":              history,
		"CanProcess":           canProcessAS(c),
		"CanReceive":           canReceiveAS(c),
		"CanDelete":            isAdminRole(c),
		"CanEditVisitDate":     canEditVisitDate(c, as),
		"ShowWorkflow":         showWorkflow,
		"WorkflowHoldMode":     workflowHoldMode,
		"ShowTransferComplete": showTransferComplete,
		"ConfirmerIsCustom":    confirmerIsCustom,
		"ProcessNowLocal":      now.Format("2006-01-02T15:04"),
		"StartLocal":           startLocal,
		"CompleteLocal":        completeLocal,
		"CancelLocal":          cancelLocal,
		"TodayLocal":           now.Format("2006-01-02"),
		"WorkerDefault":        ctxString(c, "user_name"),
		"OpenAction":           c.QueryParam("action") == "1",
	})
}

func (h *ASHandler) Update(c echo.Context) error {
	if !canProcessAS(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}

	// 접수담당은 배정만, 기술/관리자는 처리 필드 포함
	assignedCode := c.FormValue("assigned_to_code")
	if assignedCode == "custom" {
		as.AssignedTo = strings.TrimSpace(c.FormValue("assigned_to_custom"))
		as.AssignedUserID = ""
	} else if assignedCode != "" {
		if u, _ := h.userRepo.GetByID(assignedCode); u != nil {
			as.AssignedUserID = u.UserID
			as.AssignedTo = u.FullName
		} else {
			as.AssignedTo = assignedCode
		}
	} else if v := c.FormValue("assigned_to"); v != "" {
		as.AssignedTo = v
		if as.AssignedUserID == "" {
			if users, _ := h.userRepo.ListAssignable(); users != nil {
				for _, u := range users {
					if strings.TrimSpace(u.FullName) == strings.TrimSpace(v) || u.Username == v {
						as.AssignedUserID = u.UserID
						as.AssignedTo = u.FullName
						break
					}
				}
			}
		}
	}
	if canProcessAS(c) {
		formStatus := c.FormValue("status")
		as.Status = formStatus
		as.ProcessType = c.FormValue("process_type")
		as.CauseType = c.FormValue("cause_type")
		as.ActionTaken = c.FormValue("action_taken")
		as.PartsUsed = c.FormValue("parts_used")
		as.ResultCode = c.FormValue("result_code")
		as.RevisitReason = strings.TrimSpace(c.FormValue("revisit_reason"))
		if as.ResultCode != "revisit_needed" {
			as.RevisitReason = ""
		}
		as.FollowupAction = c.FormValue("followup_action")
		confCode := c.FormValue("customer_confirmer_code")
		if confCode == "custom" {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer_custom"))
		} else if confCode != "" {
			as.CustomerConfirmer = confCode
		} else {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer"))
		}
		as.IsRecurrence = c.FormValue("is_recurrence") == "1"
		as.IsReopen = c.FormValue("is_reopen") == "1"
		as.ReplaceReview = c.FormValue("replace_review") == "1"
		if t, ok := parseFormDatetime(c.FormValue("start_datetime")); ok {
			as.StartDatetime = &t
		}
		if t, ok := parseFormDatetime(c.FormValue("complete_datetime")); ok {
			as.CompleteDatetime = &t
		}
		// 완료·보류 등 특수 상태 유지, 그 외는 접수/배정/예정일 기준 자동 파생
		if formStatus == "" || formStatus == "hold" || model.IsASWorkflowStatus(formStatus) {
			if formStatus == "hold" {
				as.Status = "hold"
			} else {
				as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
			}
		}
	} else if model.IsASWorkflowStatus(as.Status) {
		as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
	}

	if err := h.repo.Update(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// Hold 보류 처리 — 후속(조치/이관/접수취소) 선택 후 사유 입력
func (h *ASHandler) Hold(c echo.Context) error {
	if !canProcessAS(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	next := strings.TrimSpace(c.FormValue("hold_next_action"))
	switch next {
	case "action", "transfer", "cancel":
	default:
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=hold_next")
	}
	reason := strings.TrimSpace(c.FormValue("hold_reason"))
	if reason == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=hold_reason")
	}
	if err := h.repo.SetHold(id, reason, next); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// ReleaseHold 보류 삭제(해제) → 필드 기준 워크플로 상태 복귀
func (h *ASHandler) ReleaseHold(c echo.Context) error {
	if !canProcessAS(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if as.Status != "hold" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id)
	}
	if err := h.repo.ReleaseHold(id); err != nil {
		return err
	}
	redir := c.FormValue("redirect")
	if redir == "" {
		redir = c.QueryParam("redirect")
	}
	if redir == "list" {
		return c.Redirect(http.StatusSeeOther, "/as?status=hold")
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// Transfer 이관 처리
func (h *ASHandler) Transfer(c echo.Context) error {
	if !canProcessAS(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if err := h.repo.SetTransfer(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// CompleteTransfer 이관 건 완료 (결과코드 + 완료일)
func (h *ASHandler) CompleteTransfer(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	resultCode := strings.TrimSpace(c.FormValue("result_code"))
	completeDate := strings.TrimSpace(c.FormValue("complete_date"))
	if resultCode == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=result_code")
	}
	if err := h.repo.CompleteTransfer(id, resultCode, completeDate); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// Cancel 접수취소
func (h *ASHandler) Cancel(c echo.Context) error {
	if !canProcessAS(c) && !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	cancelDate := strings.TrimSpace(c.FormValue("cancel_date"))
	if err := h.repo.SetCancelled(id, cancelDate); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

func parseFormDatetime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range []string{"2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (h *ASHandler) AddProcess(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	asID := c.Param("id")
	timeSpent, _ := strconv.Atoi(c.FormValue("time_spent"))
	p := &model.ASProcess{
		ASID:        asID,
		Worker:      c.FormValue("worker"),
		WorkType:    c.FormValue("work_type"),
		WorkContent: c.FormValue("work_content"),
		PartsUsed:   c.FormValue("parts_used"),
		TimeSpent:   timeSpent,
		Notes:       c.FormValue("notes"),
	}
	if t, ok := parseFormDatetime(c.FormValue("process_datetime")); ok {
		p.ProcessDatetime = t
	} else {
		p.ProcessDatetime = time.Now()
	}
	if strings.TrimSpace(p.Worker) == "" {
		p.Worker = ctxString(c, "user_name")
	}
	if err := h.processRepo.Create(p); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+asID)
}

// Delete AS 접수 삭제 (처리 이력 포함) — 관리자만
func (h *ASHandler) Delete(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if err := h.repo.Delete(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as")
}

// DeleteProcess 처리(조치) 이력 삭제 — 관리자만
func (h *ASHandler) DeleteProcess(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	asID := c.Param("id")
	processID := c.Param("process_id")
	as, err := h.repo.GetByID(asID)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if err := h.processRepo.DeleteByASAndID(asID, processID); err != nil {
		if err == sql.ErrNoRows {
			return echo.ErrNotFound
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+asID)
}

// APIHistory 기관별 과거(완료) AS 이력 JSON
func (h *ASHandler) APIHistory(c echo.Context) error {
	customerID := c.Param("customer_id")
	exclude := c.QueryParam("exclude")
	items, err := h.repo.ListPastHistory(customerID, exclude, 50)
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.ASHistoryItem{}
	}
	return c.JSON(http.StatusOK, items)
}

func (h *ASHandler) StatsDashboard(c echo.Context) error {
	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	mineUserID := ""
	var mineKeys []string
	personal := false
	if role == "tech" {
		personal = true
		mineUserID = uid
		mineKeys = keys
	}

	var stats *model.ASStats
	if personal {
		allStats, _ := h.repo.DashboardStats("", nil)
		mineStats, _ := h.repo.DashboardStats(uid, keys)
		stats = &model.ASStats{
			TotalReceived: allStats.TotalReceived,
			InProgress:    mineStats.InProgress,
			Completed:     mineStats.Completed,
			Overdue:       mineStats.Overdue,
			TodayReceived: mineStats.TodayReceived,
			WeekReceived:  mineStats.WeekReceived,
			WeekCompleted: mineStats.WeekCompleted,
		}
	} else {
		stats, _ = h.repo.DashboardStats("", nil)
	}

	byCustomer, _ := h.repo.StatsByCustomer(mineUserID, mineKeys)
	byStatus, _ := h.repo.StatsByStatus(mineUserID, mineKeys)

	listStatus := c.QueryParam("status")
	if listStatus == "" {
		listStatus = "open"
	}
	filterStatus := listStatus
	if listStatus == "all" {
		filterStatus = ""
	}
	sort, dir := parseASSort(c)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	items, listTotal, _ := h.repo.ListFiltered(filterStatus, "", mineUserID, mineKeys, sort, dir, page, 20)
	listTotalPages := (listTotal + 19) / 20
	listBase := asStatsListBaseQuery(listStatus, sort, dir)
	listQ := asListQuerySuffix(listBase)

	title := "AS 현황"
	if personal {
		title = "내 AS 현황"
	}

	return c.Render(http.StatusOK, "as/stats.html", map[string]interface{}{
		"Title": title, "Active": "as_stats",
		"Stats": stats, "ByCustomer": byCustomer, "ByStatus": byStatus,
		"Personal": personal,
		"DisplayName": ctxString(c, "user_name"),
		"Role":        role,
		"Items": items, "Total": listTotal,
		"Page": page, "TotalPages": listTotalPages,
		"ListStatus": listStatus,
		"Sort": sort, "Dir": dir,
		"SortLinks": asListSortLinks("/as/stats", listBase, sort, dir),
		"PrevURL": asPageURL("/as/stats", listBase, page-1),
		"NextURL": asPageURL("/as/stats", listBase, page+1),
		"ListPath": "/as/stats", "ListQ": listQ,
	})
}
