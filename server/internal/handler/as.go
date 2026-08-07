package handler

import (
	"database/sql"
	"fmt"
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
	workRepo     *repository.ASWorkRepo
	wbRepo       *repository.WBRepo
	settingsRepo *repository.SettingsRepo
	unlockRepo   *repository.ASUnlockRepo
	customerRepo *repository.CustomerRepo
	assetRepo    *repository.AssetRepo
	contactRepo  *repository.ContactRepo
	codeRepo     *repository.CodeRepo
	userRepo     *repository.UserRepo
	relationRepo *repository.RelationRepo
	attachRepo   *repository.AttachmentRepo
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
	if len(items) > 0 && h.workRepo != nil {
		ids := make([]string, len(items))
		for i := range items {
			ids[i] = items[i].ASID
		}
		if byAS, err := h.workRepo.ListByASIDs(ids); err == nil {
			for i := range items {
				items[i].WorkChildren = byAS[items[i].ASID]
			}
		}
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
	case "visit_past":
		statusLabel = "예정일 경과"
	case "visit_done_open":
		statusLabel = "다음 일정 미정"
	case "visit_today":
		statusLabel = "오늘 방문"
	case "visit_upcoming":
		statusLabel = "예정(미도래)"
	case "transfer_overdue":
		statusLabel = "이관 지연(확인일 경과)"
	case "overdue":
		statusLabel = "접수 지연"
	}

	return c.Render(http.StatusOK, "as/list.html", map[string]interface{}{
		"Title": "AS 관리", "Active": "as",
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Status": status, "Search": search,
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

// ASKanbanColumn 칸반 한 컬럼 (상태 기준)
type ASKanbanColumn struct {
	Key    string
	Title  string
	Border string
	Items  []model.ASListItem
	Total  int
	MoreQ  string
}

// Kanban AS 접수를 상태별 칸반으로 표시
func (h *ASHandler) Kanban(c echo.Context) error {
	role := currentRole(c)
	search := strings.TrimSpace(c.QueryParam("search"))

	mineParam := c.QueryParam("mine")
	mine := false
	if role == "tech" {
		mine = mineParam != "0"
	} else if mineParam == "1" {
		mine = true
	}

	var mineUserID string
	var mineKeys []string
	if mine {
		mineUserID = currentUserID(c)
		mineKeys = assigneeKeys(c)
	}

	defs := []ASKanbanColumn{
		{Key: "received", Title: "접수", Border: "border-blue-200"},
		{Key: "assigned", Title: "담당자 배정", Border: "border-indigo-200"},
		{Key: "in_progress", Title: "진행중", Border: "border-amber-200"},
		{Key: "hold", Title: "보류", Border: "border-orange-200"},
		{Key: "completed", Title: "완료", Border: "border-green-200"},
	}

	const perColumn = 30
	columns := make([]ASKanbanColumn, 0, len(defs))
	grand := 0
	for _, col := range defs {
		items, total, err := h.repo.ListFiltered(col.Key, search, mineUserID, mineKeys, "", "", 1, perColumn)
		if err != nil {
			return err
		}
		col.Items = items
		col.Total = total
		q := url.Values{}
		q.Set("status", col.Key)
		if search != "" {
			q.Set("search", search)
		}
		if role == "tech" || mine {
			if mine {
				q.Set("mine", "1")
			} else {
				q.Set("mine", "0")
			}
		}
		col.MoreQ = "/as?" + q.Encode()
		columns = append(columns, col)
		grand += total
	}

	return c.Render(http.StatusOK, "as/kanban.html", map[string]interface{}{
		"Title": "AS 관리", "Active": "as",
		"Columns": columns, "Total": grand,
		"Search": search, "Mine": mine, "Role": role,
		"PerColumn":  perColumn,
		"CanReceive": canReceiveAS(c), "CanProcess": canProcessAS(c),
	})
}

func parseASSort(c echo.Context) (sort, dir string) {
	sort = strings.TrimSpace(c.QueryParam("sort"))
	dir = strings.TrimSpace(c.QueryParam("dir"))
	if dir != "asc" && dir != "desc" {
		dir = "desc"
	}
	switch sort {
	case "org_name", "days", "as_number", "status", "assigned", "receipt", "visit":
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
	fields := []string{"receipt", "as_number", "org_name", "status", "assigned", "days", "visit"}
	links := make(map[string]template.URL, len(fields))
	for _, field := range fields {
		v := cloneURLValues(base)
		v.Set("sort", field)
		newDir := "desc"
		if field == "org_name" || field == "visit" {
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
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "show")
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
	if isASClosedStatus(existing.Status) && !h.canModifyAS(c, existing) {
		return h.redirectLocked(c, id, "show")
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

	var customer *model.Customer
	if as.CustomerID != "" {
		customer, _ = h.customerRepo.GetByID(as.CustomerID)
	}
	var asset *model.Asset
	var assetHistory []model.ASHistoryItem
	if as.AssetID != "" {
		asset, _ = h.assetRepo.GetByID(as.AssetID)
		assetHistory, _ = h.repo.ListHistoryByAsset(as.AssetID, as.ASID, 50)
	}
	reopens, _ := h.repo.ListReopens(as.ASID)
	var parent *model.ASReceipt
	if as.ParentASID != "" {
		parent, _ = h.repo.GetByID(as.ParentASID)
	}
	assignees, _ := h.userRepo.ListAssignable()

	var dailyTask *model.WorkTask
	if h.wbRepo != nil {
		dailyTask, _ = h.wbRepo.GetTaskBySource(model.WBSourceAS, as.ASID)
	}

	closed := isASClosedStatus(as.Status)
	unlocked, unlockExp, _ := h.unlockActive(c, as.ASID)
	canMod := h.canModifyAS(c, as)
	now := time.Now()
	unlockExpLocal := ""
	if unlocked && !unlockExp.IsZero() {
		unlockExpLocal = unlockExp.Format("15:04")
	}
	embed := c.QueryParam("embed") == "1"
	data := map[string]interface{}{
		"Title": as.ASNumber, "Active": "as", "AS": as,
		"Customer": customer, "Asset": asset,
		"AssetHistory":     assetHistory,
		"ParentAS":         parent,
		"Reopens":          reopens,
		"Assignees":        assignees,
		"DailyTask":        dailyTask,
		"CanReopen":        closed && canReceiveAS(c) && !embed,
		"CanReceive":       canReceiveAS(c),
		"IsClosed":         closed,
		"EditUnlocked":     unlocked,
		"UnlockExpires":    unlockExpLocal,
		"CanUnlockEdit":    closed && isAdminRole(c) && !unlocked && !embed,
		"CanEditReceipt":   !embed && ((!closed && canReceiveAS(c)) || (closed && canMod)),
		"CanDelete":        !embed && isAdminRole(c) && (!closed || canMod),
		"CanWriteDaily":    !embed && canWriteWorkboard(c) && !closed,
		"TodayLocal":       now.Format("2006-01-02"),
		"ActionErr":        c.QueryParam("err"),
		"ActionOK":         c.QueryParam("ok"),
		"OpenDaily":        !embed && c.QueryParam("daily") == "1",
		"Embed":            embed,
	}
	if embed {
		data["HideNav"] = true
		data["UserName"] = ctxString(c, "user_name")
		data["UserRole"] = ctxString(c, "role")
		data["Username"] = ctxString(c, "username")
		data["UserID"] = ctxString(c, "user_id")
	}
	return c.Render(http.StatusOK, "as/show.html", data)
}

// Action 조치 전용 화면 — 일일 업무에서 진입(완료·종료는 조회 전용)
func (h *ASHandler) Action(c echo.Context) error {
	// 조회: 접수·기술·관리자·일일업무 작성 가능 역할. 저장·워크플로는 POST·권한에서 차단.
	if !canReceiveAS(c) && !canWriteWorkboard(c) && !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}

	processes, _ := h.processRepo.ListByAS(id)
	workItems, _ := h.workRepo.ListByAS(id)
	var attachments []model.Attachment
	if h.attachRepo != nil {
		attachments, _ = h.attachRepo.ListByRef("as", id)
	}
	procTypes, _ := h.codeRepo.ActiveByGroup("process_type")
	causeTypes, _ := h.codeRepo.ActiveByGroup("cause_type")
	resultCodes, _ := h.codeRepo.ActiveByGroup("result_code")
	assignees, _ := h.userRepo.ListAssignable()
	contacts, _ := h.contactRepo.ListByCustomer(as.CustomerID)

	var dailyTask *model.WorkTask
	if h.wbRepo != nil {
		dailyTask, _ = h.wbRepo.GetTaskBySource(model.WBSourceAS, as.ASID)
	}

	var parent *model.ASReceipt
	if as.ParentASID != "" {
		parent, _ = h.repo.GetByID(as.ParentASID)
	}

	closed := isASClosedStatus(as.Status)
	unlocked, unlockExp, _ := h.unlockActive(c, as.ASID)
	canMod := h.canModifyAS(c, as)
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

	canProcess := canProcessAS(c) && (canMod || !closed)
	showWorkflow := canProcess && !closed && (as.Status == "received" || as.Status == "assigned" || as.Status == "in_progress" || as.Status == "hold" || as.Status == model.StatusPartialComplete)
	workflowHoldMode := as.Status == "hold"
	showTransferComplete := as.Status == "transfer" && canProcess
	confirmerIsCustom := as.CustomerConfirmer != ""
	if confirmerIsCustom {
		for _, ct := range contacts {
			if ct.FullName == as.CustomerConfirmer {
				confirmerIsCustom = false
				break
			}
		}
	}
	unlockExpLocal := ""
	if unlocked && !unlockExp.IsZero() {
		unlockExpLocal = unlockExp.Format("15:04")
	}
	titleSuffix := " · 조치"
	if closed {
		titleSuffix = " · 조치 완료"
	}

	return c.Render(http.StatusOK, "as/action.html", map[string]interface{}{
		"Title": as.ASNumber + titleSuffix, "Active": "as", "AS": as,
		"ParentAS":  parent,
		"Processes": processes, "WorkItems": workItems, "Attachments": attachments,
		"ProcTypes": procTypes, "CauseTypes": causeTypes, "ResultCodes": resultCodes,
		"Assignees":            assignees,
		"Contacts":             contacts,
		"DailyTask":            dailyTask,
		"IsClosed":             closed,
		"EditUnlocked":         unlocked,
		"UnlockExpires":        unlockExpLocal,
		"CanUnlockEdit":        closed && isAdminRole(c) && !unlocked,
		"CanProcess":           canProcess,
		"ReadOnly":             closed && !canMod,
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
		"ActionErr":            c.QueryParam("err"),
		"ActionOK":             c.QueryParam("ok"),
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
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "action")
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
	var applyOut *model.ActionApplyResult
	if canProcessAS(c) {
		formStatus := c.FormValue("status")
		as.Status = formStatus
		as.ProcessType = c.FormValue("process_type")
		as.CauseType = c.FormValue("cause_type")
		as.ActionTaken = c.FormValue("action_taken")
		as.PartsUsed = c.FormValue("parts_used")
		as.ResultCode = c.FormValue("result_code")
		as.RevisitReason = strings.TrimSpace(c.FormValue("revisit_reason"))
		as.FollowupAction = c.FormValue("followup_action")
		confCode := c.FormValue("customer_confirmer_code")
		if confCode == "custom" {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer_custom"))
		} else if confCode != "" {
			as.CustomerConfirmer = confCode
		} else {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer"))
		}
		if t, ok := parseFormDatetime(c.FormValue("start_datetime")); ok {
			as.StartDatetime = &t
		}
		if t, ok := parseFormDatetime(c.FormValue("complete_datetime")); ok {
			as.CompleteDatetime = &t
		}

		nextVisit := normalizeVisitDate(c.FormValue("visit_scheduled_date"))
		confirmDate := normalizeVisitDate(c.FormValue("confirm_scheduled_date"))
		revisitDate := normalizeVisitDate(c.FormValue("revisit_scheduled_date"))
		scheduleConfirmed := c.FormValue("schedule_confirmed") == "1"
		transferDetail := strings.TrimSpace(c.FormValue("transfer_detail"))
		confirmTarget := strings.TrimSpace(c.FormValue("confirm_target"))
		confirmContact := strings.TrimSpace(c.FormValue("confirm_contact"))
		createRevisit := c.FormValue("create_revisit_after") == "1"
		revisitConfirmed := c.FormValue("revisit_schedule_confirmed") == "1"

		if as.ResultCode != "" {
			in := model.ActionApplyInput{
				ScheduleConfirmed:         scheduleConfirmed,
				TransferDetail:            transferDetail,
				ConfirmTarget:             confirmTarget,
				ConfirmContact:            confirmContact,
				CreateRevisitAfterConfirm: createRevisit,
				RevisitDate:               revisitDate,
				RevisitScheduleConfirmed:  revisitConfirmed,
			}
			switch as.ResultCode {
			case model.ResultRevisit, model.ResultTemporary, model.ResultPartial:
				in.NextDate = firstNonEmpty(revisitDate, nextVisit)
			case model.ResultTransfer, model.ResultEscalation:
				in.NextDate = firstNonEmpty(confirmDate, nextVisit)
				as.TransferDetail = transferDetail
				as.ConfirmTarget = confirmTarget
				as.ConfirmContact = confirmContact
			default:
				in.NextDate = nextVisit
			}
			out, err := model.ApplyActionResult(as, in, time.Now())
			if err != nil {
				switch err {
				case model.ErrRevisitReasonRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=revisit_reason")
				case model.ErrRevisitDateRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=revisit_date")
				case model.ErrTemporaryDateRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=temporary_date")
				case model.ErrTransferDetailRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=transfer_detail")
				case model.ErrConfirmDateRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=confirm_date")
				case model.ErrConfirmTargetRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=confirm_target")
				case model.ErrConfirmContactRequired:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=confirm_contact")
				default:
					return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=action")
				}
			}
			applyOut = out
		} else {
			as.RevisitReason = ""
			if nextVisit != "" {
				as.VisitScheduledDate = nextVisit
			}
			if formStatus == "" || formStatus == "hold" || model.IsASWorkflowStatus(formStatus) {
				if formStatus == "hold" {
					as.Status = "hold"
				} else {
					as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
				}
			}
		}
	} else if model.IsASWorkflowStatus(as.Status) {
		as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
	}

	if err := h.repo.Update(as); err != nil {
		return err
	}
	if canProcessAS(c) {
		_ = h.appendActionProcess(c, as)
		if applyOut != nil {
			for _, d := range applyOut.WorkItems {
				_ = h.workRepo.Create(&model.ASWorkItem{
					ASID:              as.ASID,
					WorkKind:          d.WorkKind,
					ScheduledDate:     d.ScheduledDate,
					ScheduleConfirmed: d.ScheduleConfirmed,
					ConfirmTarget:     d.ConfirmTarget,
					ConfirmContact:    d.ConfirmContact,
					Notes:             d.Notes,
					Status:            "open",
				})
			}
		}
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func actionResultLabel(code string) string {
	switch strings.TrimSpace(code) {
	case model.ResultDone:
		return "완료"
	case model.ResultPartial:
		return "부분완료"
	case model.ResultTemporary:
		return "임시조치"
	case model.ResultRevisit:
		return "재방문필요"
	case model.ResultTransfer:
		return "타사이관"
	case model.ResultEscalation:
		return "제조사에스컬레이션"
	default:
		if code == "" {
			return ""
		}
		return code
	}
}

// appendActionProcess 조치 폼 저장 내용을 처리 이력으로 남긴다.
func (h *ASHandler) appendActionProcess(c echo.Context, as *model.ASReceipt) error {
	action := strings.TrimSpace(as.ActionTaken)
	result := strings.TrimSpace(as.ResultCode)
	if action == "" && result == "" {
		return nil
	}
	content := action
	if content == "" {
		content = "조치 저장"
	}
	notesParts := []string{}
	if lbl := actionResultLabel(result); lbl != "" {
		notesParts = append(notesParts, "결과:"+lbl)
	}
	if as.TransferDetail != "" && result == model.ResultTransfer {
		if as.TransferDetail == model.TransferDetailWaiting {
			notesParts = append(notesParts, "세부:조치결과대기")
		} else if as.TransferDetail == model.TransferDetailCompleted {
			notesParts = append(notesParts, "세부:완료")
		}
	}
	if as.VisitScheduledDate != "" && (result == model.ResultRevisit || result == model.ResultTransfer || result == model.ResultPartial) {
		label := "방문예정일"
		if result == model.ResultTransfer {
			label = "확인예정일"
		}
		notesParts = append(notesParts, label+":"+as.VisitScheduledDate)
	}
	if as.ConfirmTarget != "" {
		notesParts = append(notesParts, "확인대상:"+as.ConfirmTarget)
	}
	if as.RevisitReason != "" {
		notesParts = append(notesParts, "재방문사유:"+as.RevisitReason)
	}
	worker := strings.TrimSpace(as.AssignedTo)
	if worker == "" {
		worker = ctxString(c, "user_name")
	}
	p := &model.ASProcess{
		ASID:        as.ASID,
		Worker:      worker,
		WorkType:    as.ProcessType,
		WorkContent: content,
		PartsUsed:   as.PartsUsed,
		Notes:       strings.Join(notesParts, " · "),
	}
	if as.StartDatetime != nil && !as.StartDatetime.IsZero() {
		p.ProcessDatetime = *as.StartDatetime
	} else {
		p.ProcessDatetime = time.Now()
	}
	return h.processRepo.Create(p)
}

// Hold 보류 처리 — 후속(조치/이관/접수취소) 선택 후 사유 입력
func (h *ASHandler) Hold(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "action")
	}
	next := strings.TrimSpace(c.FormValue("hold_next_action"))
	switch next {
	case "action", "transfer", "cancel":
	default:
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=hold_next")
	}
	reason := strings.TrimSpace(c.FormValue("hold_reason"))
	if reason == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=hold_reason")
	}
	if err := h.repo.SetHold(id, reason, next); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
}

// ReleaseHold 보류 삭제(해제) → 필드 기준 워크플로 상태 복귀
func (h *ASHandler) ReleaseHold(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if as.Status != "hold" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
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
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
}

// Transfer 이관 처리
func (h *ASHandler) Transfer(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "action")
	}
	if err := h.repo.SetTransfer(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
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
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "action")
	}
	resultCode := strings.TrimSpace(c.FormValue("result_code"))
	completeDate := strings.TrimSpace(c.FormValue("complete_date"))
	if resultCode == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=result_code")
	}
	if err := h.repo.CompleteTransfer(id, resultCode, completeDate); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
}

// Cancel 접수취소
func (h *ASHandler) Cancel(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "action")
	}
	cancelDate := strings.TrimSpace(c.FormValue("cancel_date"))
	if err := h.repo.SetCancelled(id, cancelDate); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action")
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

// Reopen 완료된 접수를 같은 증상으로 다시 접수한다.
// 완료 건을 되돌리지 않고 새 접수번호를 발급해 원 접수와 이어 둔다.
func (h *ASHandler) Reopen(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	src, err := h.repo.GetByID(id)
	if err != nil || src == nil {
		return echo.ErrNotFound
	}
	if !model.CanReopenAS(src.Status) {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err="+url.QueryEscape("완료·종료된 건만 재접수할 수 있습니다"))
	}
	reason := strings.TrimSpace(c.FormValue("reopen_reason"))
	if reason == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err="+url.QueryEscape("재접수 사유를 입력하세요"))
	}

	as := model.NewReopenReceipt(src, reason, time.Now())
	as.ReceivedBy = ctxString(c, "user_name")
	if ch := strings.TrimSpace(c.FormValue("receipt_channel")); ch != "" {
		as.ReceiptChannel = ch
	}
	if sym := strings.TrimSpace(c.FormValue("symptom")); sym != "" {
		as.Symptom = sym
	}
	if u := strings.TrimSpace(c.FormValue("urgency")); u != "" {
		as.Urgency = u
	}
	if code := strings.TrimSpace(c.FormValue("assigned_to_code")); code != "" {
		if u, _ := h.userRepo.GetByID(code); u != nil {
			as.AssignedUserID = u.UserID
			as.AssignedTo = u.FullName
		}
	}
	as.VisitScheduledDate = normalizeVisitDate(c.FormValue("visit_scheduled_date"))
	as.ScheduleConfirmed = as.VisitScheduledDate != "" && c.FormValue("schedule_confirmed") == "1"

	if err := h.repo.Create(as); err != nil {
		return err
	}
	// 원 접수에도 재접수 사실을 이력으로 남긴다.
	h.appendReopenProcess(src, as, reason, c)
	return c.Redirect(http.StatusSeeOther, "/as/"+as.ASID)
}

func (h *ASHandler) appendReopenProcess(src, as *model.ASReceipt, reason string, c echo.Context) {
	worker := ctxString(c, "user_name")
	_ = h.processRepo.Create(&model.ASProcess{
		ASID:            src.ASID,
		ProcessDatetime: time.Now(),
		Worker:          worker,
		Notes:           fmt.Sprintf("동일 증상 재접수 — %s (사유: %s)", as.ASNumber, reason),
	})
}

// Delete AS 접수 삭제 (처리 이력 포함) — 관리자만 (완료·종료는 잠금 해제 필요)
func (h *ASHandler) Delete(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, id, "show")
	}
	if err := h.repo.Delete(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as")
}

// DeleteProcess 처리(조치) 이력 삭제 — 관리자만 (완료·종료는 잠금 해제 필요)
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
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return h.redirectLocked(c, asID, "action")
	}
	if err := h.processRepo.DeleteByASAndID(asID, processID); err != nil {
		if err == sql.ErrNoRows {
			return echo.ErrNotFound
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+asID+"/action")
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

// APIAssetHistory 자산별 AS 이력 JSON
func (h *ASHandler) APIAssetHistory(c echo.Context) error {
	assetID := c.Param("asset_id")
	exclude := c.QueryParam("exclude")
	items, err := h.repo.ListHistoryByAsset(assetID, exclude, 50)
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
			TotalReceived:   allStats.TotalReceived,
			InProgress:      mineStats.InProgress,
			Completed:       mineStats.Completed,
			Overdue:         mineStats.Overdue,
			TodayReceived:   mineStats.TodayReceived,
			WeekReceived:    mineStats.WeekReceived,
			WeekCompleted:   mineStats.WeekCompleted,
			VisitPast:       mineStats.VisitPast,
			VisitDoneOpen:   mineStats.VisitDoneOpen,
			VisitToday:      mineStats.VisitToday,
			VisitUpcoming:   mineStats.VisitUpcoming,
			TransferOverdue: mineStats.TransferOverdue,
		}
	} else {
		stats, _ = h.repo.DashboardStats("", nil)
	}

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
		"Stats":    stats,
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
