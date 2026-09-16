package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
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
	attach       *AttachmentHandler
	kwRepo       *repository.ASKeywordRepo

	reportTemplateBytes []byte
	reportTemplatePath  string
	reportDocxBytes     []byte
	reportDocxPath      string
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

	if model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view")) == "kanban" {
		return h.renderASKanban(c, status, search, mineUserID, mineKeys, mine, role, sort, dir, assigneeID, assigneeName)
	}

	items, total, err := h.repo.ListFiltered(status, search, mineUserID, mineKeys, sort, dir, page, 20)
	if err != nil {
		return err
	}
	if len(items) > 0 {
		ids := make([]string, len(items))
		for i := range items {
			ids[i] = items[i].ASID
		}
		if h.workRepo != nil {
			if byAS, err := h.workRepo.ListByASIDs(ids); err == nil {
				for i := range items {
					items[i].WorkChildren = byAS[items[i].ASID]
				}
			}
		}
		if h.attachRepo != nil {
			if counts, err := h.attachRepo.CountByRefIDs(model.RefTypeASReceipt, ids); err == nil {
				for i := range items {
					items[i].PhotoCount = counts[items[i].ASID]
				}
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

	kanbanLink := cloneURLValues(listBase)
	kanbanLink.Set("display", "kanban")

	return c.Render(http.StatusOK, "as/list.html", map[string]interface{}{
		"Title": "AS 관리", "Active": NavAS,
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Status": status, "Search": search,
		"Mine": mine, "MineQ": mineQ, "SortQ": sortQ, "ListPath": "/as", "Role": role,
		"Sort": sort, "Dir": dir,
		"SortLinks":   asListSortLinks("/as", listBase, sort, dir),
		"SortSelect":  sortSelectOptions(asListSortCols(), asSortHrefStrings(asListSortLinks("/as", listBase, sort, dir)), sort, dir),
		"PrevURL":     asPageURL("/as", listBase, page-1),
		"NextURL":     asPageURL("/as", listBase, page+1),
		"DisplayName": ctxString(c, "user_name"),
		"CanReceive":  canReceiveAS(c), "CanProcess": canProcessAS(c),
		"StatusLabel": statusLabel,
		"AssigneeID":  assigneeID, "AssigneeName": assigneeName,
		"IsHoldList": status == "hold",
		"Display":    "list",
		"KanbanHref": "/as?" + kanbanLink.Encode(),
		"ListHref":   "/as?" + listBase.Encode(),
	})
}

func (h *ASHandler) renderASKanban(c echo.Context, status, search, mineUserID string, mineKeys []string, mine bool, role, sort, dir, assigneeID, assigneeName string) error {
	items, listTotal, err := h.repo.ListFiltered(status, search, mineUserID, mineKeys, sort, dir, 1, 2000)
	if err != nil {
		return err
	}
	board := model.FillASListKanban(model.ASListKanbanColumnDefs(), items)
	listBase := asListBaseQuery(status, search, sort, dir, mine, role)
	if assigneeID != "" {
		listBase.Set("assignee", assigneeID)
		listBase.Del("mine")
	}
	if assigneeName != "" {
		listBase.Set("assignee_name", assigneeName)
		listBase.Del("mine")
	}
	kanbanQ := cloneURLValues(listBase)
	kanbanQ.Set("display", "kanban")
	listQ := cloneURLValues(listBase)
	listQ.Del("display")
	listQ.Del("view")
	return c.Render(http.StatusOK, "as/kanban.html", map[string]interface{}{
		"Title": "AS 관리", "Active": NavAS,
		"KanbanColumns": board.Columns,
		"KanbanTotal":   board.Total,
		"KanbanDrag":    false,
		"KanbanDrop":    "",
		"KanbanHint":    "대기·진행중·검토·완료. 보류·이관은 진행중 열 뱃지. 한 카드는 한 열에만 있습니다.",
		"Total":         listTotal,
		"Search":        search, "Mine": mine, "Role": role,
		"Status":     status,
		"CanReceive": canReceiveAS(c), "CanProcess": canProcessAS(c),
		"ListHref":   "/as?" + listQ.Encode(),
		"KanbanHref": "/as?" + kanbanQ.Encode(),
		"Display":    "kanban",
	})
}

// Kanban /as/kanban — /as?display=kanban 과 같다.
func (h *ASHandler) Kanban(c echo.Context) error {
	q := c.QueryParams()
	q.Set("display", "kanban")
	return c.Redirect(http.StatusSeeOther, "/as?"+q.Encode())
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

func asSortHrefStrings(links map[string]template.URL) map[string]string {
	out := make(map[string]string, len(links))
	for k, v := range links {
		out[k] = string(v)
	}
	return out
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
	customers, _ := h.customerRepo.ListForReceipt()
	channels, _ := h.codeRepo.ActiveByGroup("receipt_channel")
	urgReasons := h.urgencyReasonCodes()
	assignees, _ := h.userRepo.ListAssignable()

	now := time.Now()
	as := &model.ASReceipt{
		Urgency: "normal", Priority: "normal", Status: "received",
		UrgencyReason:   model.UrgencyReasonNone,
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
		"Title": "AS 접수", "Active": NavAS, "IsNew": true,
		"AS": as, "Customers": customers, "Assets": assets, "Contacts": contacts,
		"Channels": channels, "UrgencyReasons": urgReasons,
		"Assignees":        assignees,
		"CustomersJSON":    receiptCustomersJSON(customers),
		"ReceiptLocal":     now.Format("2006-01-02T15:04"),
		"CanEditVisitDate": true, // 신규 접수 시 접수 권한자가 설정 가능
		"History":          history,
		"CanReceive":       true,
		"KeywordChecks":    h.keywordChecks("", model.KWFieldSymptom, as.Symptom),
		"KeywordField":     model.KWFieldSymptom,
		"Err":              c.QueryParam("err"),
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
		ConfirmContact:     strings.TrimSpace(c.FormValue("confirm_contact")),
		UrgencyReason:      strings.TrimSpace(c.FormValue("urgency_reason")),
		UrgencyReasonNote:  strings.TrimSpace(c.FormValue("urgency_reason_note")),
	}
	if as.ReceivedBy == "" {
		as.ReceivedBy = ctxString(c, "user_name")
	}
	if dt := c.FormValue("receipt_datetime"); dt != "" {
		if t, ok := parseFormDatetime(dt); ok {
			as.ReceiptDatetime = t
		}
	}
	applyReceiptDerivedFields(as)
	return as
}

func applyReceiptDerivedFields(as *model.ASReceipt) {
	if as.Priority == "" {
		as.Priority = "normal"
	}
	if strings.TrimSpace(as.UrgencyReason) != "" {
		as.Urgency = model.UrgencyFromReason(as.UrgencyReason)
	} else if as.Urgency == "" {
		as.UrgencyReason = model.UrgencyReasonNone
		as.Urgency = "normal"
	}
	as.ScheduleConfirmed = model.SyncScheduleConfirmed(as.VisitScheduledDate)
}

func (h *ASHandler) urgencyReasonCodes() []model.Code {
	if h.codeRepo == nil {
		return nil
	}
	var all []model.Code
	for _, g := range []string{"urgency_reason_common", "urgency_reason_rfid", "urgency_reason_klas", "urgency_reason_web"} {
		cs, _ := h.codeRepo.ActiveByGroup(g)
		all = append(all, cs...)
	}
	return all
}

func receiptCustomersJSON(list []model.Customer) template.JS {
	type row struct {
		ID        string `json:"id"`
		OrgName   string `json:"org_name"`
		ShortName string `json:"short_name"`
		Region    string `json:"region"`
	}
	out := make([]row, 0, len(list))
	for _, c := range list {
		out = append(out, row{
			ID: c.CustomerID, OrgName: c.OrgName, ShortName: c.ShortName, Region: c.Region,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(b)
}

func formVals(c echo.Context, key string) []string {
	req := c.Request()
	ct := req.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		_ = req.ParseMultipartForm(32 << 20)
		if req.MultipartForm != nil {
			return req.MultipartForm.Value[key]
		}
	}
	_ = req.ParseForm()
	return req.PostForm[key]
}

func formAt(ss []string, i int) string {
	if i < 0 || i >= len(ss) {
		return ""
	}
	return strings.TrimSpace(ss[i])
}

func (h *ASHandler) resolveAssigneeCode(code, custom string) (name, userID string) {
	code = strings.TrimSpace(code)
	custom = strings.TrimSpace(custom)
	if code == "custom" {
		return custom, ""
	}
	if code == "" {
		return "", ""
	}
	if h.userRepo != nil {
		if u, _ := h.userRepo.GetByID(code); u != nil {
			return u.FullName, u.UserID
		}
	}
	return code, ""
}

// parseEquipmentRows 신규 접수 화면의 장비별 행. 필드가 없으면 nil (기존 단일 폼). §34.2.1
func (h *ASHandler) parseEquipmentRows(c echo.Context, base *model.ASReceipt) []*model.ASReceipt {
	assets := formVals(c, "eq_asset_id")
	symptoms := formVals(c, "eq_symptom")
	urgencies := formVals(c, "eq_urgency")
	reasons := formVals(c, "eq_urgency_reason")
	reasonNotes := formVals(c, "eq_urgency_note")
	visits := formVals(c, "eq_visit_date")
	codes := formVals(c, "eq_assigned_code")
	customs := formVals(c, "eq_assigned_custom")
	n := len(assets)
	if len(symptoms) > n {
		n = len(symptoms)
	}
	if n == 0 {
		return nil
	}
	var out []*model.ASReceipt
	for i := 0; i < n; i++ {
		asset := formAt(assets, i)
		symptom := formAt(symptoms, i)
		if asset == "" && symptom == "" {
			continue
		}
		row := *base
		row.AssetID = asset
		row.Symptom = symptom
		if len(reasons) > 0 {
			reason := formAt(reasons, i)
			if reason == "" {
				reason = model.UrgencyReasonNone
			}
			row.UrgencyReason = reason
			row.UrgencyReasonNote = formAt(reasonNotes, i)
			row.Urgency = model.UrgencyFromReason(reason)
		} else if u := formAt(urgencies, i); u != "" {
			row.Urgency = u
		}
		rawVisit := formAt(visits, i)
		row.VisitScheduledDate = normalizeVisitDate(rawVisit)
		applyReceiptDerivedFields(&row)
		if name, uid := h.resolveAssigneeCode(formAt(codes, i), formAt(customs, i)); name != "" || uid != "" {
			row.AssignedTo, row.AssignedUserID = name, uid
		}
		out = append(out, &row)
	}
	return out
}

func equipmentVisitDateYearErr(c echo.Context) bool {
	for _, raw := range formVals(c, "eq_visit_date") {
		raw = strings.TrimSpace(raw)
		if raw != "" && normalizeVisitDate(raw) == "" {
			return true
		}
	}
	return false
}

func (h *ASHandler) Create(c echo.Context) error {
	if !canReceiveAS(c) {
		return echo.ErrForbidden
	}
	base := h.parseReceiptForm(c)
	rows := h.parseEquipmentRows(c, base)
	if len(rows) == 0 {
		rows = []*model.ASReceipt{base}
	}
	if raw := strings.TrimSpace(c.FormValue("visit_scheduled_date")); raw != "" && base.VisitScheduledDate == "" {
		return c.Redirect(http.StatusSeeOther, "/as/new?err=date_year")
	}
	if equipmentVisitDateYearErr(c) {
		return c.Redirect(http.StatusSeeOther, "/as/new?err=date_year")
	}
	if len(rows) >= 2 {
		gid := repository.NewReceiptGroupID()
		for _, r := range rows {
			r.ReceiptGroupID = gid
		}
	}
	var first *model.ASReceipt
	for i, as := range rows {
		applyReceiptDerivedFields(as)
		if err := h.repo.Create(as); err != nil {
			log.Printf("AS 접수 등록 실패: %v", err)
			return c.Redirect(http.StatusSeeOther, "/as/new?err="+receiptCreateErrQuery(err))
		}
		h.syncASPlannedDailyTask(as.ASID)
		if i == 0 {
			h.saveKeywordChecks(c, as.ASID, model.KWFieldSymptom)
			first = as
			if h.attach != nil {
				if files, err := uploadFileHeaders(c); err == nil && len(files) > 0 {
					if err := h.attach.saveReceiptPhotos(as.ASID, strings.TrimSpace(c.FormValue("photo_memo")), files); err != nil {
						msg := err.Error()
						if httpErr, ok := err.(*echo.HTTPError); ok {
							if s, ok := httpErr.Message.(string); ok {
								msg = s
							}
						}
						return c.Redirect(http.StatusSeeOther, "/as/"+as.ASID+"?err="+url.QueryEscape(msg))
					}
				}
			}
		}
	}
	if first == nil {
		return c.Redirect(http.StatusSeeOther, "/as/new?err="+url.QueryEscape("증상을 입력하세요"))
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+first.ASID)
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

	customers, _ := h.customerRepo.ListForReceiptIncluding(as.CustomerID)
	channels, _ := h.codeRepo.ActiveByGroup("receipt_channel")
	urgReasons := h.urgencyReasonCodes()
	assignees, _ := h.userRepo.ListAssignable()

	var assets []model.Asset
	var contacts []model.Contact
	var history []model.ASHistoryItem
	if as.CustomerID != "" {
		assets, _ = h.assetRepo.ListByCustomer(as.CustomerID)
		contacts, _ = h.contactRepo.ListByCustomer(as.CustomerID)
		history, _ = h.repo.ListPastHistory(as.CustomerID, as.ASID, 50)
	}

	data := map[string]interface{}{
		"Title": "AS 접수 수정", "Active": NavAS, "IsNew": false,
		"AS": as, "Customers": customers, "Assets": assets, "Contacts": contacts,
		"Channels": channels, "UrgencyReasons": urgReasons,
		"Assignees":        assignees,
		"CustomersJSON":    receiptCustomersJSON(customers),
		"ReceiptLocal":     as.ReceiptDatetime.Format("2006-01-02T15:04"),
		"CanEditVisitDate": canEditVisitDate(c, as),
		"History":          history,
		"Err":              c.QueryParam("err"),
		"KeywordChecks":    h.keywordChecks(as.ASID, model.KWFieldSymptom, as.Symptom),
		"KeywordField":     model.KWFieldSymptom,
	}
	h.mergeReceiptPhotoData(c, as, data)
	data["PhotoRedirect"] = "/as/" + as.ASID + "/edit"
	data["ReceiptGallery"] = receiptGalleryFromPage(as, data)
	return c.Render(http.StatusOK, "as/form.html", data)
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
	if raw := strings.TrimSpace(c.FormValue("visit_scheduled_date")); raw != "" && as.VisitScheduledDate == "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/edit?err=date_year")
	}
	// 방문 예정일: 관리자·배정담당자만 변경. 그 외는 기존 값 유지. 확정은 서버가 날짜로 채운다. §34.2.3
	if !canEditVisitDate(c, existing) {
		as.VisitScheduledDate = existing.VisitScheduledDate
	}
	applyReceiptDerivedFields(as)
	as.PreserveTransferFollowupClone(existing)
	if err := h.repo.UpdateReceipt(as); err != nil {
		return err
	}
	h.syncASPlannedDailyTask(id)
	h.saveKeywordChecks(c, id, model.KWFieldSymptom)
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
	date, err := model.ParseAppDate(c.FormValue("visit_scheduled_date"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=date_year")
	}
	confirmed := model.SyncScheduleConfirmed(date)
	if err := h.repo.UpdateVisitScheduledDate(id, date, confirmed); err != nil {
		return err
	}
	h.syncASPlannedDailyTask(id)
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// syncASPlannedDailyTask 담당자가 있으면 일일업무(work_tasks)를 만들고/맞춘다. §42.2
// 예정일이 없어도 올린다(날짜는 비움). 담당자 없으면 일일업무에서 내린다.
func (h *ASHandler) syncASPlannedDailyTask(asID string) {
	if h.wbRepo == nil || strings.TrimSpace(asID) == "" {
		return
	}
	as, err := h.repo.GetByID(asID)
	if err != nil || as == nil {
		return
	}
	if err := h.wbRepo.SyncASDailyTask(as); err != nil {
		log.Printf("AS 일일업무 동기화 실패 %s: %v", asID, err)
	}
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
	if as.AssetID != "" {
		asset, _ = h.assetRepo.GetByID(as.AssetID)
	}
	reopens, _ := h.repo.ListReopens(as.ASID)
	followups, _ := h.repo.ListTransferFollowups(as.ASID)
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
		"Title": as.ASNumber, "Active": NavAS, "AS": as,
		"Customer": customer, "Asset": asset,
		"ParentAS":          parent,
		"Reopens":           reopens,
		"TransferFollowups": followups,
		"Assignees":         assignees,
		"DailyTask":         dailyTask,
		"CanReopen":         closed && canReceiveAS(c) && !embed,
		"CanReceive":        canReceiveAS(c),
		"CanProcess":        canProcessAS(c) && (!closed || canMod),
		"IsClosed":          closed,
		"EditUnlocked":      unlocked,
		"UnlockExpires":     unlockExpLocal,
		"CanUnlockEdit":     closed && isAdminRole(c) && !unlocked && !embed,
		"CanEditReceipt":    !embed && ((!closed && canReceiveAS(c)) || (closed && canMod)),
		"CanDelete":         !embed && isAdminRole(c) && (!closed || canMod),
		"CanWriteDaily":     !embed && canWriteWorkboard(c) && !closed,
		"TodayLocal":        now.Format("2006-01-02"),
		"ActionErr":         actionErrMessage(c.QueryParam("err")),
		"ActionOK":          c.QueryParam("ok"),
		"OpenDaily":         !embed && c.QueryParam("daily") == "1",
		"Embed":             embed,
	}
	if as.ReceiptGroupID != "" {
		mates, _ := h.repo.ListByReceiptGroup(as.ReceiptGroupID)
		data["GroupMates"] = mates
	}
	if h.kwRepo != nil {
		if links, err := h.kwRepo.LinksByAS(as.ASID); err == nil && len(links) > 0 {
			data["KeywordLinks"] = links
		}
	}
	if embed {
		data["HideNav"] = true
		data["UserName"] = ctxString(c, "user_name")
		data["UserRole"] = ctxString(c, "role")
		data["Username"] = ctxString(c, "username")
		data["UserID"] = ctxString(c, "user_id")
	}
	h.mergeReceiptPhotoData(c, as, data)
	h.mergeASReportData(c, as, data)
	return c.Render(http.StatusOK, "as/show.html", data)
}

func (h *ASHandler) mergeReceiptPhotoData(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if as == nil {
		return
	}
	var photos []model.Attachment
	if h.attachRepo != nil {
		photos, _ = h.attachRepo.ListByRef(model.RefTypeASReceipt, as.ASID)
	}
	assetCount := 0
	if as.AssetID != "" && h.attachRepo != nil {
		assetCount, _ = h.attachRepo.CountByRef(model.RefTypeAsset, as.AssetID)
	}
	closed := isASClosedStatus(as.Status)
	canMod := h.canModifyAS(c, as)
	canEdit := canManageReceiptPhoto(c, as) && (!closed || canMod)
	if embed, _ := data["Embed"].(bool); embed {
		canEdit = false
	}
	data["ReceiptPhotos"] = photos
	data["ReceiptPhotoCount"] = len(photos)
	data["CanEditPhotos"] = canEdit
	data["CanPromote"] = (canWriteMaster(c) || canReceiveAS(c)) && as.AssetID != ""
	data["AssetImageCount"] = assetCount
	data["PhotoRedirect"] = "/as/" + as.ASID
	data["PhotoSectionTitle"] = "증상 사진"
	data["HideIfEmpty"] = false
	data["ReceiptGallery"] = receiptGalleryFromPage(as, data)
}

// Action 조치 전용 화면 — 쓰기 권한 없으면 조회 전용(버튼·저장 숨김)
func (h *ASHandler) Action(c echo.Context) error {
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
	procTypes = filterSelectableProcessTypes(procTypes)
	causeTypes, _ := h.codeRepo.ActiveByGroup("cause_type")
	resultCodes, _ := h.codeRepo.ActiveByGroup("result_code")
	causeCats, _ := h.repo.ListCauseCategories()
	var causeCatL1 []model.CauseCategory
	for _, cat := range causeCats {
		if cat.Level == 1 {
			causeCatL1 = append(causeCatL1, cat)
		}
	}
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
	followups, _ := h.repo.ListTransferFollowups(as.ASID)

	closed := isASClosedStatus(as.Status)
	unlocked, unlockExp, _ := h.unlockActive(c, as.ASID)
	canMod := h.canModifyAS(c, as)
	now := time.Now()
	startLocal := ""
	if as.StartDatetime != nil && !as.StartDatetime.IsZero() {
		startLocal = as.StartDatetime.Format("2006-01-02 15:04")
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
	// 쓰기 불가(옵저버 등)면 항상 조회 전용 — 저장·완료 버튼 비표시
	readOnly := !canProcess
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
	if readOnly && !closed {
		titleSuffix = " · 조치 (조회)"
	}

	data := map[string]interface{}{
		"Title": as.ASNumber + titleSuffix, "Active": NavAS, "AS": as,
		"ParentAS":          parent,
		"TransferFollowups": followups,
		"Processes":         processes, "WorkItems": workItems, "Attachments": attachments,
		"ProcTypes": procTypes, "CauseTypes": causeTypes, "ResultCodes": resultCodes,
		"CauseCatL1":           causeCatL1,
		"CauseCategoriesJSON":  causeCategoriesJSON(causeCats),
		"ActionResults":        model.ActionResultOptions(),
		"Assignees":            assignees,
		"Contacts":             contacts,
		"DailyTask":            dailyTask,
		"IsClosed":             closed,
		"EditUnlocked":         unlocked,
		"UnlockExpires":        unlockExpLocal,
		"CanUnlockEdit":        closed && isAdminRole(c) && !unlocked,
		"CanProcess":           canProcess,
		"ReadOnly":             readOnly,
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
		"ActionErrMsg":         actionErrMessage(c.QueryParam("err")),
		"AttachErrMsg":         attachErrMessage(c.QueryParam("err")),
		"ActionWarn":           c.QueryParam("warn"),
		"ActionWarnMsg":        actionWarnMessage(c.QueryParam("warn")),
		"ActionOK":             c.QueryParam("ok"),
		"ConclusionDraft":      asConclusionDraft(as, processes),
		"CanReceive":           canReceiveAS(c),
		"KeywordChecks":        h.keywordChecks(as.ASID, model.KWFieldAction, as.ActionTaken+" "+as.Symptom),
		"KeywordField":         model.KWFieldAction,
	}
	h.mergeReceiptPhotoData(c, as, data)
	data["CanEditPhotos"] = false
	data["CanPromote"] = false
	data["PhotoRedirect"] = "/as/" + as.ASID + "/action"
	data["PhotoSectionTitle"] = "접수 시 증상 사진"
	data["PhotoPanelClass"] = "bg-sky-50 border-2 border-sky-200"
	data["HideIfEmpty"] = true
	data["ReceiptGallery"] = receiptGalleryFromPage(as, data)
	h.mergeActionPhotoData(as, data, canProcess && !readOnly)
	h.mergeASReportData(c, as, data)
	data["OfferReport"] = c.QueryParam("report") == "1" && canIssueASReportStatus(as.Status)
	return c.Render(http.StatusOK, "as/action.html", data)
}

func (h *ASHandler) mergeActionPhotoData(as *model.ASReceipt, data map[string]interface{}, canEdit bool) {
	if as == nil {
		return
	}
	var photos []model.Attachment
	if h.attachRepo != nil {
		photos, _ = h.attachRepo.ListByRef(model.RefTypeASActionPhoto, as.ASID)
	}
	data["ActionPhotos"] = photos
	data["ActionGallery"] = PhotoGalleryVM{
		AS:                as,
		Photos:            photos,
		CanEditPhotos:     canEdit,
		CanPromote:        false,
		PhotoRedirect:     "/as/" + as.ASID + "/action",
		PhotoSectionTitle: "조치 사진",
		PhotoPanelClass:   "bg-white border border-slate-200",
		HideIfEmpty:       false,
		PhotoMax:          model.MaxActionPhotos,
		PhotoRefType:      model.RefTypeASActionPhoto,
		PhotoAccept:       "image/*",
		PhotoHint:         "조치 과정·결과 사진 (jpg · png · webp · heic). 최대 3장.",
		PhotoEmpty:        "등록된 조치 사진이 없습니다.",
		PhotoAddLabel:     "올리기",
		PhotoGalleryID:    "action-photo",
	}
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
	formResult := ""
	prepNotes := ""
	var followupChild *model.ASReceipt
	if canProcessAS(c) {
		as.ProcessType = strings.TrimSpace(c.FormValue("process_type"))
		as.ProcessTypeReason = strings.TrimSpace(c.FormValue("process_type_reason"))
		as.WorkPlace = model.NormalizeWorkPlace(c.FormValue("work_place"))
		as.ActionTaken = strings.TrimSpace(c.FormValue("action_taken"))
		na := c.FormValue("action_na") == "1"
		naReason := strings.TrimSpace(c.FormValue("action_na_reason"))
		formResult = strings.TrimSpace(c.FormValue("result_code"))
		as.ResultCode = formResult
		savingAction := na || as.ActionTaken != "" || formResult != ""
		if savingAction {
			resolved, code := model.ResolveActionContent(as.ActionTaken, na, naReason)
			if code != "" {
				return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err="+code)
			}
			as.ActionTaken = resolved
			if model.ActionContentTooShort(as.ActionTaken) && c.FormValue("action_short_ok") != "1" {
				return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err="+model.ActionErrShort)
			}
		}
		as.CauseDetail = strings.TrimSpace(c.FormValue("cause_detail"))
		as.CauseCat1 = strings.TrimSpace(c.FormValue("cause_cat1"))
		as.CauseCat2 = strings.TrimSpace(c.FormValue("cause_cat2"))
		as.CauseCat3 = strings.TrimSpace(c.FormValue("cause_cat3"))
		causeCats, _ := h.repo.ListCauseCategories()
		as.CauseType = model.CauseTypeFromCat2(causeCats, as.CauseCat2)
		as.RevisitReason = strings.TrimSpace(c.FormValue("revisit_reason"))
		prepNotes = strings.TrimSpace(c.FormValue("revisit_prep"))
		confCode := c.FormValue("customer_confirmer_code")
		if confCode == "custom" {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer_custom"))
		} else if confCode != "" {
			as.CustomerConfirmer = confCode
		} else {
			as.CustomerConfirmer = strings.TrimSpace(c.FormValue("customer_confirmer"))
		}
		// 착수시각은 폼에서 받지 않는다. 첫 조치의 process_datetime으로 서버가 채운다(§4.2 · §12.5-4).
		if t, ok := parseFormDatetime(c.FormValue("complete_datetime")); ok {
			as.CompleteDatetime = &t
		}
		if as.ProcessType != model.ProcessTypeUndetermined {
			as.ProcessTypeReason = ""
		}
		if as.ProcessType == model.ProcessTypeVisit {
			as.VisitDate = normalizeVisitDate(c.FormValue("visit_date"))
		} else {
			as.VisitDate = ""
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
		transferNeedFollowup := c.FormValue("transfer_need_followup") == "1"
		followupNote := strings.TrimSpace(c.FormValue("followup_note"))

		if raw := strings.TrimSpace(c.FormValue("visit_scheduled_date")); raw != "" && nextVisit == "" {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=date_year")
		}
		if raw := strings.TrimSpace(c.FormValue("complete_datetime")); raw != "" && as.CompleteDatetime == nil {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=date_year")
		}
		if raw := strings.TrimSpace(c.FormValue("visit_date")); as.ProcessType == model.ProcessTypeVisit && raw != "" && as.VisitDate == "" {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=date_year")
		}

		if formResult != "" && !model.IsSelectableActionResult(formResult) {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=result_not_allowed")
		}
		if code := actionSaveMissingErr(as, formResult); code != "" {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err="+code)
		}
		if code := actionCauseCatMissingErr(as, causeCats); code != "" {
			return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err="+code)
		}

		if formResult != "" {
			if formResult == model.ResultTransfer {
				if transferNeedFollowup {
					if followupNote == "" {
						return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err=followup_note")
					}
					transferDetail = model.TransferDetailFollowup
				} else {
					transferDetail = model.TransferDetailCompleted
				}
			}
			in := model.ActionApplyInput{
				ScheduleConfirmed:         scheduleConfirmed,
				TransferDetail:            transferDetail,
				ConfirmTarget:             confirmTarget,
				ConfirmContact:            confirmContact,
				CreateRevisitAfterConfirm: createRevisit,
				RevisitDate:               revisitDate,
				RevisitScheduleConfirmed:  revisitConfirmed,
			}
			switch formResult {
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
			if formResult == model.ResultTransfer && transferDetail == model.TransferDetailFollowup {
				child := model.NewTransferFollowupReceipt(as, followupNote, time.Now())
				child.ReceivedBy = ctxString(c, "user_name")
				if err := h.repo.Create(child); err != nil {
					return err
				}
				followupChild = child
			}
		} else {
			as.RevisitReason = ""
			if nextVisit != "" {
				as.VisitScheduledDate = nextVisit
			}
			if model.IsASWorkflowStatus(as.Status) {
				as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
			}
		}
	} else if model.IsASWorkflowStatus(as.Status) {
		as.Status = model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)
	}

	timeSpent := 0
	if canProcessAS(c) {
		timeSpent = h.actionDurationMin(as.ASID)
	}
	if err := h.repo.Update(as); err != nil {
		return err
	}
	h.syncASPlannedDailyTask(as.ASID)
	h.saveKeywordChecks(c, as.ASID, model.KWFieldAction)
	if canProcessAS(c) {
		_ = h.appendActionProcess(c, as, formResult, prepNotes, timeSpent)
		if applyOut != nil {
			// 하부업무 담당자(폼) — 미입력 시 Create가 원 접수 담당자를 초기값으로만 복사
			workTo, workUID := resolveAssigneeFormFields(c, h.userRepo,
				"work_assigned_to_code", "work_assigned_to_custom", "work_assigned_to")
			for _, d := range applyOut.WorkItems {
				aTo, aUID := d.AssignedTo, d.AssignedUserID
				if strings.TrimSpace(aTo) == "" && strings.TrimSpace(aUID) == "" {
					aTo, aUID = workTo, workUID
				}
				_ = h.workRepo.Create(&model.ASWorkItem{
					ASID:              as.ASID,
					WorkKind:          d.WorkKind,
					ScheduledDate:     d.ScheduledDate,
					ScheduleConfirmed: d.ScheduleConfirmed,
					ConfirmTarget:     d.ConfirmTarget,
					ConfirmContact:    d.ConfirmContact,
					AssignedTo:        aTo,
					AssignedUserID:    aUID,
					Notes:             d.Notes,
					Status:            "open",
				})
			}
		}
	}
	loc := "/as/" + id + "/action"
	q := url.Values{}
	if followupChild != nil {
		loc = "/as/" + followupChild.ASID
		q.Set("ok", "이관 후속 접수를 만들었습니다")
	} else {
		if canIssueASReportStatus(as.Status) {
			q.Set("report", "1")
		}
		if model.ShowsASCauseReport(formResult) && strings.TrimSpace(as.CauseDetail) == "" {
			q.Set("warn", "cause_report")
		}
	}
	if len(q) > 0 {
		loc += "?" + q.Encode()
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

// actionSaveMissingErr 부록 A.2: 조치 저장 시 조치내용·근무구분·원인분류·처리유형 필수.
// 결과코드가 있으면 조치내용 공백은 action_required (완료만 빠져 있던 구멍).
func actionSaveMissingErr(as *model.ASReceipt, formResult string) string {
	if as == nil {
		return ""
	}
	action := strings.TrimSpace(as.ActionTaken)
	saving := action != "" || strings.TrimSpace(formResult) != ""
	if strings.TrimSpace(formResult) != "" && action == "" {
		return "action_required"
	}
	if !saving {
		return ""
	}
	switch as.WorkPlace {
	case model.WorkPlaceOffice, model.WorkPlaceField:
	default:
		return "work_place"
	}
	if strings.TrimSpace(as.ProcessType) == "" {
		return "process_type"
	}
	if as.ProcessType == model.ProcessTypeUndetermined {
		if strings.TrimSpace(as.ProcessTypeReason) == "" {
			return "process_type_reason"
		}
	} else if !model.ProcessTypeAllowed(as.WorkPlace, as.ProcessType) {
		return "process_type_mismatch"
	}
	if as.ProcessType == model.ProcessTypeVisit && strings.TrimSpace(as.VisitDate) == "" {
		return "visit_date"
	}
	return ""
}

func actionCauseCatMissingErr(as *model.ASReceipt, cats []model.CauseCategory) string {
	if as == nil {
		return ""
	}
	action := strings.TrimSpace(as.ActionTaken)
	saving := action != "" || strings.TrimSpace(as.ResultCode) != ""
	if !saving {
		return ""
	}
	if strings.TrimSpace(as.CauseCat1) == "" {
		return "cause_cat1"
	}
	if strings.TrimSpace(as.CauseCat2) == "" {
		return "cause_cat2"
	}
	if model.CauseCat2HasChildren(cats, as.CauseCat2) && strings.TrimSpace(as.CauseCat3) == "" {
		return "cause_cat3"
	}
	return ""
}

func attachErrMessage(raw string) string {
	s := strings.TrimSpace(raw)
	switch s {
	case "attach_forbidden":
		return "권한이 없습니다"
	case "attach_file":
		return "파일이 필요합니다"
	case "attach_photo_max":
		return "사진은 최대 3장입니다"
	case "attach_closed":
		return "완료·종료 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요"
	case "attach_not_image":
		return "조치 사진은 이미지만 올릴 수 있습니다"
	case "권한이 없습니다", "파일이 필요합니다":
		return s
	}
	if strings.Contains(s, "최대") && strings.Contains(s, "장") {
		return "사진은 최대 3장입니다"
	}
	if strings.Contains(s, "이미지만") {
		return "조치 사진은 이미지만 올릴 수 있습니다"
	}
	if strings.Contains(s, "읽기 전용") {
		return s
	}
	if strings.Contains(s, "파일은 20MB") || strings.Contains(s, "이미지") {
		return s
	}
	if strings.Contains(s, "까지 올릴 수 있습니다") {
		return s
	}
	if strings.Contains(s, "동영상은 한 건에") {
		return s
	}
	return ""
}

func actionErrMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "action_required":
		return "조치내용을 입력하세요."
	case "action_na_reason":
		return "해당 없음이면 사유를 입력하세요."
	case "action_short":
		return "조금 더 적어 주시겠습니까?"
	case "work_place":
		return "근무구분(내근/외근)을 선택하세요."
	case "cause_type":
		return "원인분류를 선택하세요."
	case "cause_cat1":
		return "1차 분류를 선택하세요."
	case "cause_cat2":
		return "2차 분류를 선택하세요."
	case "cause_cat3":
		return "3차 분류를 선택하세요."
	case "process_type":
		return "처리유형을 선택하세요."
	case "process_type_reason":
		return "처리유형이 미정이면 사유를 입력하세요."
	case "process_type_mismatch":
		return "근무구분에 맞는 처리유형을 선택하세요."
	case "visit_date":
		return "현장방문이면 방문일을 입력하세요."
	case "result_not_allowed":
		return "완료 · 추가조치 필요 · 이관만 선택할 수 있습니다."
	case "time_spent":
		return "소요시간은 일일 업무 등록에서 입력합니다."
	case "revisit_date":
		return "재방문·추가조치 시 방문예정일자를 입력하세요."
	case "revisit_reason":
		return "재방문 사유를 입력하세요."
	case "temporary_date":
		return "임시조치 시 방문예정일자를 입력하세요."
	case "transfer_detail":
		return "이관 시 「추가 조치 필요」 여부를 확인하세요."
	case "followup_note":
		return "이관 후 추가 조치가 있으면 「추가 접수 내용」을 입력하세요."
	case "confirm_date":
		return "우리 팀 추가 작업 시 확인예정일자를 입력하세요."
	case "confirm_target":
		return "확인대상자를 입력하세요."
	case "confirm_contact":
		return "연락처를 입력하세요."
	case "hold_reason":
		return "대기 사유를 입력하세요."
	case "hold_next":
		return "대기 후속(조치/이관/접수취소)을 선택하세요."
	case "result_code":
		return "처리결과코드를 선택하세요."
	case "action":
		return "조치 내용을 확인 후 다시 저장하세요."
	case "date_year":
		return "날짜 연도는 2000~2100 사이여야 합니다."
	case "schedule_date":
		return "일정 확정 시 예정업무일을 먼저 입력하세요."
	default:
		return code
	}
}

func filterSelectableProcessTypes(codes []model.Code) []model.Code {
	var out []model.Code
	for _, c := range codes {
		switch c.CodeValue {
		case model.ProcessTypeRemote, model.ProcessTypeVisit, model.ProcessTypeInquiry, model.ProcessTypeUndetermined:
			out = append(out, c)
		}
	}
	return out
}

func causeCategoriesJSON(list []model.CauseCategory) template.JS {
	if list == nil {
		list = []model.CauseCategory{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return template.JS(b)
}

func actionWarnMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "cause_report":
		return "장애원인이 비어 있습니다. 지금은 저장했고, 보고서 발급 때 다시 필요합니다."
	default:
		return ""
	}
}

func asConclusionDraft(as *model.ASReceipt, processes []model.ASProcess) string {
	return model.ASConclusionDraftFrom(as, processes)
}

func (h *ASHandler) actionDurationMin(asID string) int {
	if h.wbRepo != nil {
		if t, _ := h.wbRepo.GetTaskBySource(model.WBSourceAS, asID); t != nil {
			return model.NormalizeDurationMin(t.DurationMin)
		}
	}
	return model.NormalizeDurationMin(0)
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
	return model.ActionResultLabel(code)
}

// appendActionProcess 조치 폼 저장 내용을 처리 이력으로 남긴다. 회차별 결과·사유는 컬럼에 보존한다.
func (h *ASHandler) appendActionProcess(c echo.Context, as *model.ASReceipt, result, prepNotes string, timeSpent int) error {
	action := strings.TrimSpace(as.ActionTaken)
	result = strings.TrimSpace(result)
	if action == "" && result == "" {
		return nil
	}
	content := action
	if content == "" {
		content = model.ActionResultLabel(result)
		if content == "" {
			content = "조치 저장"
		}
	}
	waitReason := strings.TrimSpace(as.HoldReason)
	if waitReason == "" {
		waitReason = strings.TrimSpace(as.RevisitReason)
	}
	nextDate := strings.TrimSpace(as.VisitScheduledDate)
	notesParts := []string{}
	if lbl := model.ActionResultLabel(result); lbl != "" {
		notesParts = append(notesParts, "결과:"+lbl)
	}
	if result == model.ResultTransfer {
		if d := model.TransferDetailLabel(as.TransferDetail); d != "" {
			notesParts = append(notesParts, "이관:"+d)
		}
	}
	if nextDate != "" && (result == model.ResultRevisit || result == model.ResultTransfer || result == model.ResultPartial || result == model.ResultHold) {
		label := "다음예정일"
		if result == model.ResultTransfer && as.TransferDetail == model.TransferDetailWaiting {
			label = "확인예정일"
		}
		notesParts = append(notesParts, label+":"+nextDate)
	}
	if as.ConfirmTarget != "" {
		notesParts = append(notesParts, "확인대상:"+as.ConfirmTarget)
	}
	if waitReason != "" {
		notesParts = append(notesParts, "사유:"+waitReason)
	}
	if prepNotes != "" {
		notesParts = append(notesParts, "준비:"+prepNotes)
	}
	worker := strings.TrimSpace(as.AssignedTo)
	if worker == "" {
		worker = ctxString(c, "user_name")
	}
	p := &model.ASProcess{
		ASID:            as.ASID,
		Worker:          worker,
		WorkType:        as.ProcessType,
		WorkContent:     content,
		PartsUsed:       as.PartsUsed,
		Notes:           strings.Join(notesParts, " · "),
		ResultCode:      result,
		TransferDetail:  as.TransferDetail,
		NextActionDate:  nextDate,
		WaitReason:      waitReason,
		PrepNotes:       prepNotes,
		TimeSpent:       timeSpent,
		ProcessDatetime: time.Now(),
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
	as.Status = "hold"
	as.HoldReason = reason
	as.HoldNextAction = next
	_ = h.appendActionProcess(c, as, model.ResultHold, "", h.actionDurationMin(id))
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

func receiptCreateErrQuery(err error) string {
	if errors.Is(err, model.ErrAppDateYear) {
		return "date_year"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") {
		return url.QueryEscape("접수번호가 중복되었습니다. 다시 등록해 주세요.")
	}
	if strings.Contains(msg, "foreign key") {
		return url.QueryEscape("선택한 기관 또는 자산이 유효하지 않습니다.")
	}
	return url.QueryEscape("접수 저장에 실패했습니다.")
}

func parseFormDatetime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range []string{"2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			if !model.AppDateYearOK(t) {
				return time.Time{}, false
			}
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
	if timeSpent <= 0 {
		timeSpent = h.actionDurationMin(asID)
	}
	p := &model.ASProcess{
		ASID:        asID,
		Worker:      c.FormValue("worker"),
		WorkType:    c.FormValue("work_type"),
		WorkContent: strings.TrimSpace(c.FormValue("work_content")),
		PartsUsed:   c.FormValue("parts_used"),
		TimeSpent:   timeSpent,
		Notes:       c.FormValue("notes"),
	}
	na := c.FormValue("action_na") == "1"
	resolved, code := model.ResolveActionContent(p.WorkContent, na, strings.TrimSpace(c.FormValue("action_na_reason")))
	if code != "" {
		return c.Redirect(http.StatusSeeOther, "/as/"+asID+"?err="+code)
	}
	if model.ActionContentTooShort(resolved) && c.FormValue("action_short_ok") != "1" {
		return c.Redirect(http.StatusSeeOther, "/as/"+asID+"?err="+model.ActionErrShort)
	}
	p.WorkContent = resolved
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
	as.ScheduleConfirmed = model.SyncScheduleConfirmed(as.VisitScheduledDate)

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
	id := c.Param("id")
	if !isAdminRole(c) {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "as_receipts",
			TargetID:    id,
			Detail:      "권한없음",
			Reason:      "권한없음",
			Result:      auditlog.ResultDeny,
		})
		return echo.ErrForbidden
	}
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "as_receipts",
			TargetID:    as.ASID,
			SubjectType: "customer",
			SubjectID:   as.CustomerID,
			SubjectName: as.OrgName,
			Detail:      "완료·종료 건 삭제 거부",
			Reason:      "잠금",
			Result:      auditlog.ResultDeny,
			BeforeJSON:  toJSON(as),
		})
		return h.redirectLocked(c, id, "show")
	}
	if err := h.repo.Delete(id); err != nil {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "as_receipts",
			TargetID:    as.ASID,
			SubjectType: "customer",
			SubjectID:   as.CustomerID,
			SubjectName: as.OrgName,
			Detail:      err.Error(),
			Reason:      accessReason(c, "AS 접수 삭제"),
			Result:      auditlog.ResultDeny,
			BeforeJSON:  toJSON(as),
		})
		return err
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		TargetTable: "as_receipts",
		TargetID:    as.ASID,
		SubjectType: "customer",
		SubjectID:   as.CustomerID,
		SubjectName: as.OrgName,
		Detail:      "AS 접수 삭제 " + as.ASNumber,
		Reason:      accessReason(c, "AS 접수 삭제"),
		Result:      auditlog.ResultOK,
		BeforeJSON:  toJSON(as),
	})
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
		"Title": title, "Active": NavASStats,
		"Stats":       stats,
		"Personal":    personal,
		"DisplayName": ctxString(c, "user_name"),
		"Role":        role,
		"Items":       items, "Total": listTotal,
		"Page": page, "TotalPages": listTotalPages,
		"ListStatus": listStatus,
		"Sort":       sort, "Dir": dir,
		"SortLinks":  asListSortLinks("/as/stats", listBase, sort, dir),
		"SortSelect": sortSelectOptions(asListSortCols(), asSortHrefStrings(asListSortLinks("/as/stats", listBase, sort, dir)), sort, dir),
		"PrevURL":    asPageURL("/as/stats", listBase, page-1),
		"NextURL":    asPageURL("/as/stats", listBase, page+1),
		"ListPath":   "/as/stats", "ListQ": listQ,
	})
}
