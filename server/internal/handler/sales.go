package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SalesHandler struct {
	repo         *repository.SalesRepo
	projectRepo  *repository.ProjectRepo
	wbRepo       *repository.WBRepo
	customerRepo *repository.CustomerRepo
	contactRepo  *repository.ContactRepo
	userRepo     *repository.UserRepo
	codeRepo     *repository.CodeRepo
}

func NewSalesHandler(
	repo *repository.SalesRepo,
	projectRepo *repository.ProjectRepo,
	wbRepo *repository.WBRepo,
	customerRepo *repository.CustomerRepo,
	contactRepo *repository.ContactRepo,
	userRepo *repository.UserRepo,
	codeRepo *repository.CodeRepo,
) *SalesHandler {
	return &SalesHandler{
		repo: repo, projectRepo: projectRepo, wbRepo: wbRepo, customerRepo: customerRepo,
		contactRepo: contactRepo, userRepo: userRepo, codeRepo: codeRepo,
	}
}

func (h *SalesHandler) List(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := parseSalesListFilter(c)
	view := salesListView(c.QueryParam("display"), c.QueryParam("view"))
	items, err := h.repo.ListFilter(f)
	if err != nil {
		return err
	}
	stages, _ := h.repo.StagesFor(f.DealType)
	rows := make([]map[string]interface{}, 0, len(items))
	for i := range items {
		def := model.FindSalesStage(stages, items[i].Stage)
		rows = append(rows, h.viewProject(&items[i], def))
	}
	fromTask := strings.TrimSpace(c.QueryParam("from_task"))
	dealType, buildHref, supplyHref, newHref, resetHref, filterQ := salesDealBoardLinks("/sales", view, f, fromTask)
	lastAct, _ := h.repo.LatestActivityDateBySales()
	kanbanCols := salesProjectKanban(items, stages, fromTask, lastAct, false)
	kanbanTotal := 0
	for _, col := range kanbanCols {
		kanbanTotal += col.Count
	}
	users, _ := h.userRepo.ListAssignable()
	return c.Render(http.StatusOK, "sales/list.html", map[string]interface{}{
		"Title": "영업 사업", "Active": NavSales,
		"Projects": rows, "Stages": stages,
		"KanbanColumns": kanbanCols,
		"KanbanTotal":   kanbanTotal,
		"KanbanDrag":    canWriteSales(c),
		"KanbanDrop":    "sales",
		"KanbanHint":    "열 = 단계. 카드를 끌어 단계를 옮깁니다. 뒤로 가거나 실주로 보낼 때는 사유가 필요합니다.",
		"Timeline":      model.BuildSalesTimelineAxis(items, stages, time.Now()),
		"View":          view,
		"FilterQ":       filterQ,
		"Filter":        f,
		"DealType":      dealType,
		"DealBuildHref": buildHref, "DealSupplyHref": supplyHref,
		"NewHref":       newHref, "FilterReset": resetHref,
		"Search":        f.Search, "Status": f.Status, "Stage": f.Stage,
		"Owner": f.Owner, "Period": f.Period, "Customer": f.Customer, "AmountConfirmed": f.AmountConfirmed,
		"PeriodOptions": salesPeriodOptions(time.Now()),
		"Users":         users,
		"FilterAction":  "/sales",
		"CanWrite":      canWriteSales(c),
		"FlashOK":       c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"FormError":     querySalesErr(c.QueryParam("err")),
		"FromTask":      fromTask,
		"ShowPipelineMetrics": false,
	})
}

func (h *SalesHandler) New(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p := &model.SalesProject{
		IsTentativeName: true,
		DealType:        model.NormalizeSalesDealType(c.QueryParam("deal")),
		Status:          model.SalesStatusActive,
	}
	p.Stage = model.DefaultSalesStageCodeFor(p.DealType)
	return h.renderForm(c, p, false, "")
}

func (h *SalesHandler) Create(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p := h.parseForm(c)
	if err := h.repo.Create(p); err != nil {
		return h.renderForm(c, p, false, err.Error())
	}
	loc := "/sales/" + p.SalesID + "?ok=created"
	if from := strings.TrimSpace(c.FormValue("from_task")); from != "" {
		loc = "/sales/" + p.SalesID + "?tab=timeline&from_task=" + url.QueryEscape(from) + "&ok=created"
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func (h *SalesHandler) Show(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
		}
		return err
	}
	stages, _ := h.repo.StagesFor(p.DealType)
	def := model.FindSalesStage(stages, p.Stage)
	hist, _ := h.repo.ListHistory(p.SalesID)
	acts, _ := h.repo.ListActivities(p.SalesID)
	changes, _ := h.repo.ListChanges(p.SalesID)
	parties, _ := h.repo.ListParties(p.SalesID, true)
	types, _ := h.repo.ActivityTypes()
	users, _ := h.userRepo.ListAssignable()
	partyTypes, _ := h.repo.PartyTypes()
	partyRoles, _ := h.repo.PartyRoles()
	var contacts []model.Contact
	if p.CustomerID != "" && h.contactRepo != nil {
		contacts, _ = h.contactRepo.ListByCustomer(p.CustomerID)
	}
	timeline := model.MergeSalesTimeline(acts, hist, changes)
	tab := strings.TrimSpace(c.QueryParam("tab"))
	if tab != "parties" {
		tab = "timeline"
	}
	var activeParties, inactiveParties []model.SalesParty
	for _, pt := range parties {
		if pt.IsActive {
			activeParties = append(activeParties, pt)
		} else {
			inactiveParties = append(inactiveParties, pt)
		}
	}
	var workProjectID string
	if h.projectRepo != nil {
		if wp, err := h.projectRepo.GetBySalesID(p.SalesID); err == nil && wp != nil {
			workProjectID = wp.ProjectID
		}
	}
	fromTaskID := strings.TrimSpace(c.QueryParam("from_task"))
	var fromTask *model.WorkTask
	if fromTaskID != "" && h.wbRepo != nil {
		fromTask, _ = h.wbRepo.GetTask(fromTaskID)
	}
	return c.Render(http.StatusOK, "sales/show.html", map[string]interface{}{
		"Title": p.Name, "Active": NavSales,
		"Project":         h.viewProject(p, def),
		"Raw":             p,
		"WorkProjectID":   workProjectID,
		"CanPromote":      model.CanPromoteSales(p) && workProjectID == "",
		"ShowProgress":    !p.IsSupply() && p.Stage == model.SalesStageProposal,
		"Progress":        model.SalesProposalProgressFrom(acts),
		"Stages":          stages,
		"History":         hist,
		"Activities":      acts,
		"Changes":         changes,
		"Timeline":        timeline,
		"ActivityTypes":   types,
		"Users":           users,
		"PartyTypes":      partyTypes,
		"PartyRoles":      partyRoles,
		"Parties":         parties,
		"ActiveParties":   activeParties,
		"InactiveParties": inactiveParties,
		"Contacts":        contacts,
		"Tab":             tab,
		"ActView":         salesActView(c.QueryParam("actview")),
		"ActGroup":        salesActGroup(c.QueryParam("group")),
		"ActCols":         salesActivityKanban(acts, c.QueryParam("group"), types, stages),
		"KanbanColumns":   salesActivityKanbanColumns(acts, c.QueryParam("group"), types, stages),
		"KanbanDrag":      canWriteSales(c),
		"KanbanDrop":      "act",
		"KanbanHint":      "열 = 활동 유형. 카드를 끌어 옮깁니다.",
		"FormAction":      "/sales/" + p.SalesID + "/activities",
		"FixedSalesID":    p.SalesID,
		"DefaultOwner":    ctxString(c, "user_name"),
		"PartyHints":      model.CustomerPartyHintNames(parties),
		"Today":           time.Now().Format("2006-01-02"),
		"CanWrite":        canWriteSales(c), "CanDelete": canDeleteSales(c),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"FormError": querySalesErr(c.QueryParam("err")),
		"FromTask":  fromTask,
		"ListBack": func() string {
			if p.IsSupply() {
				return "/sales?deal=supply"
			}
			return "/sales"
		}(),
	})
}

func (h *SalesHandler) Edit(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
	}
	return h.renderForm(c, p, true, "")
}

func (h *SalesHandler) Update(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	cur, err := h.repo.Get(id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
	}
	p := h.parseForm(c)
	p.SalesID = cur.SalesID
	p.Stage = cur.Stage
	p.Status = cur.Status
	p.LegacyStage = cur.LegacyStage
	p.WonAt = cur.WonAt
	if p.ContractedAt == "" {
		p.ContractedAt = cur.ContractedAt
	}
	if p.LostReason == "" {
		p.LostReason = cur.LostReason
	}
	if err := h.repo.Update(p, ctxString(c, "user_name")); err != nil {
		p.SalesID = cur.SalesID
		return h.renderForm(c, p, true, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?ok=updated")
}

func (h *SalesHandler) ChangeStage(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	to := strings.TrimSpace(c.FormValue("stage"))
	reason := strings.TrimSpace(c.FormValue("reason"))
	keep := c.FormValue("keep_override") == "1"
	err := h.repo.ChangeStage(id, to, reason, currentUserID(c), ctxString(c, "user_name"), keep)
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) Delete(c echo.Context) error {
	if !canDeleteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		if err == sql.ErrNoRows {
			return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/sales?ok=deleted")
}

func (h *SalesHandler) Activities(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	pageView := salesActivitiesPageView(c)
	if pageView == "log" {
		data, err := h.renderActivitiesLogData(c)
		if err != nil {
			return err
		}
		if strings.TrimSpace(c.QueryParam("partial")) == "1" {
			return h.renderSalesActMain(c, data)
		}
		return c.Render(http.StatusOK, "sales/activities.html", data)
	}
	date := strings.TrimSpace(c.QueryParam("date"))
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if err := model.RequireAppDateYear(date); err != nil {
		date = time.Now().Format("2006-01-02")
	}
	assigneeFilter := strings.TrimSpace(c.QueryParam("assignee"))
	view := pageView
	group := salesActGroup(c.QueryParam("group"))
	base, err := time.Parse("2006-01-02", date)
	if err != nil {
		base = time.Now()
		date = base.Format("2006-01-02")
	}

	placed, err := h.wbRepo.ListTasksByDate(date)
	if err != nil {
		return err
	}
	var salesTasks []model.WorkTask
	for _, t := range placed {
		if t.SourceType == model.WBSourceSalesActivity {
			salesTasks = append(salesTasks, t)
		}
	}
	memberMap := map[string][]model.WorkTaskMember{}
	if ids := taskIDsOf(salesTasks); len(ids) > 0 {
		if m, err := h.wbRepo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}

	users, _ := h.userRepo.ListAssignable()
	var salesNames []string
	var order []string
	seenName := map[string]bool{}
	for _, u := range users {
		n := strings.TrimSpace(u.FullName)
		if n == "" || seenName[n] {
			continue
		}
		seenName[n] = true
		order = append(order, n)
		if u.Role == model.RoleSales {
			salesNames = append(salesNames, n)
		}
	}
	sort.Strings(order)
	sort.Strings(salesNames)

	toCard := func(t model.WorkTask) model.WBCard {
		card := taskCardFromWork(t)
		if t.SourceType == model.WBSourceSalesActivity {
			if sid := h.wbRepo.SalesIDByActivity(t.SourceID); sid != "" {
				card.SourceHref = "/sales/" + sid
				card.ActionHref = "/sales/" + sid
			}
		}
		return card
	}
	dateCol := RegisterColumn{Date: date, From: date, To: date, Label: date}
	dayCols := buildRegisterAssigneeColumnsAlways(dateCol, salesTasks, toCard, order, assigneeFilter, memberMap, salesNames)

	slotTimes := registerSlotTimes()
	gridH, slotTops := registerGridStyles(len(slotTimes))
	prev := base.AddDate(0, 0, -1).Format("2006-01-02")
	next := base.AddDate(0, 0, 1).Format("2006-01-02")
	filterQ := ""
	if assigneeFilter != "" {
		filterQ = "&assignee=" + url.QueryEscape(assigneeFilter)
	}

	var colorNames []string
	colorNames = append(colorNames, salesNames...)
	for _, t := range salesTasks {
		colorNames = append(colorNames, t.Assignee)
	}
	model.SetAssigneeColorOrder(colorNames)

	actTypes, _ := h.repo.ActivityTypes()
	stages, _ := h.repo.Stages()
	actCols := []salesActCol{}
	var kanbanCols []model.KanbanColumn
	if view == "kanban" {
		var acts []model.SalesActivity
		if c.QueryParam("scope") == "day" {
			acts, _ = h.repo.ListActivitiesByDate(date)
		} else {
			acts, _ = h.repo.ListAllActivities()
		}
		actCols = salesActivityKanban(acts, group, actTypes, stages)
		kanbanCols = salesActivityKanbanColumns(acts, group, actTypes, stages)
	}
	projects, _ := h.repo.List("", model.SalesStatusActive, "")

	return c.Render(http.StatusOK, "sales/activities.html", map[string]interface{}{
		"Title":           "영업 활동",
		"Active":          NavSalesActivities,
		"Date":            date,
		"PrevDate":        prev,
		"NextDate":        next,
		"DayColumns":      dayCols,
		"SlotTimes":       slotTimes,
		"SlotRem":         registerSlotRem,
		"GridHeightStyle": gridH,
		"SlotTopStyles":   slotTops,
		"AssigneeColMin":  registerAssigneeColMinPx,
		"Assignees":       users,
		"AssigneeFilter":  assigneeFilter,
		"FilterQ":         filterQ,
		"CanWrite":        canWriteSales(c),
		"ShowHeaderAdd":   canWriteSales(c) && view != "day",
		"LogQuery":        "",
		"TypeQuery":       "",
		"View":            view,
		"ActGroup":        group,
		"ActCols":         actCols,
		"KanbanColumns":   kanbanCols,
		"KanbanDrag":      canWriteSales(c),
		"KanbanDrop":      "act",
		"KanbanHint":      "열 = 활동 유형. 카드를 끌어 옮깁니다.",
		"ActivityTypes":   actTypes,
		"Today":           time.Now().Format("2006-01-02"),
		"FormAction":      "/sales/activities",
		"SalesOptions":    projects,
		"DefaultOwner":    ctxString(c, "user_name"),
		"Users":           users,
		"PartyHints":      []string{},
		"Scope":           c.QueryParam("scope"),
	})
}

func (h *SalesHandler) Pipeline(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := parseSalesListFilter(c)
	items, err := h.repo.ListFilter(f)
	if err != nil {
		return err
	}
	var extra []string
	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
		for _, u := range users {
			if u.Role == model.RoleSales && strings.TrimSpace(u.FullName) != "" {
				extra = append(extra, u.FullName)
			}
		}
	}
	pipe, err := h.repo.PipelineFilter(time.Now(), extra, f)
	if err != nil {
		return err
	}
	stages, _ := h.repo.StagesFor(f.DealType)
	lastAct, _ := h.repo.LatestActivityDateBySales()
	kanbanCols := salesProjectKanban(items, stages, "", lastAct, true)
	kanbanTotal := 0
	for _, col := range kanbanCols {
		kanbanTotal += col.Count
	}
	dealType, buildHref, supplyHref, _, resetHref, filterQ := salesDealBoardLinks("/sales/pipeline", "", f, "")
	return c.Render(http.StatusOK, "sales/pipeline.html", map[string]interface{}{
		"Title": "영업 파이프라인", "Active": NavSalesPipeline,
		"Pipe": pipe, "CanWrite": canWriteSales(c),
		"Projects": items, "Stages": stages,
		"KanbanColumns": kanbanCols,
		"KanbanTotal":   kanbanTotal,
		"KanbanDrag":    canWriteSales(c),
		"KanbanDrop":    "sales",
		"KanbanHint":    "열 = 단계. 실주 열은 기본으로 접혀 있습니다.",
		"FilterQ":       filterQ,
		"Filter":        f,
		"DealType":      dealType,
		"DealBuildHref": buildHref, "DealSupplyHref": supplyHref,
		"FilterReset":         resetHref,
		"Search":        f.Search, "Status": f.Status, "Stage": f.Stage,
		"Owner": f.Owner, "Period": f.Period, "Customer": f.Customer, "AmountConfirmed": f.AmountConfirmed,
		"PeriodOptions":       salesPeriodOptions(time.Now()),
		"Users":               users,
		"FilterAction":        "/sales/pipeline",
		"View":                "kanban",
		"ShowPipelineMetrics": true,
		"FlashOK":             c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"FormError":           querySalesErr(c.QueryParam("err")),
	})
}

func (h *SalesHandler) renderForm(c echo.Context, p *model.SalesProject, isEdit bool, formErr string) error {
	stages, _ := h.repo.StagesFor(p.DealType)
	customers, _ := h.customerRepo.ListAll()
	users, _ := h.userRepo.ListAssignable()
	sources, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupLeadSource)
	title := "영업 사업 등록"
	if isEdit {
		title = "영업 사업 수정"
	}
	def := model.FindSalesStage(stages, p.Stage)
	if def != nil && !p.HasOverride && !p.IsSupply() {
		p.Probability = def.Probability
	}
	fromTask := strings.TrimSpace(c.QueryParam("from_task"))
	if fromTask == "" {
		fromTask = strings.TrimSpace(c.FormValue("from_task"))
	}
	listBack := "/sales"
	if p.IsSupply() {
		listBack = "/sales?deal=supply"
	}
	return c.Render(http.StatusOK, "sales/form.html", map[string]interface{}{
		"Title": title, "Active": NavSales,
		"Project": p, "IsEdit": isEdit, "FormError": formErr,
		"Stages": stages, "Customers": customers, "Users": users, "LeadSources": sources,
		"DisplayStage": p.DisplayStage(def),
		"FromTask":     fromTask,
		"ListBack":     listBack,
	})
}

func (h *SalesHandler) parseForm(c echo.Context) *model.SalesProject {
	p := &model.SalesProject{
		Name:                    strings.TrimSpace(c.FormValue("name")),
		IsTentativeName:         c.FormValue("is_tentative_name") == "1",
		DealType:                model.NormalizeSalesDealType(c.FormValue("deal_type")),
		Stage:                   strings.TrimSpace(c.FormValue("stage")),
		CustomerID:              strings.TrimSpace(c.FormValue("customer_id")),
		ProspectName:            strings.TrimSpace(c.FormValue("prospect_name")),
		ProspectRegion:          strings.TrimSpace(c.FormValue("prospect_region")),
		CustomerConfirmed:       c.FormValue("customer_confirmed") == "1",
		ExpectedYM:              strings.TrimSpace(c.FormValue("expected_ym")),
		ExpectedPrecision:       strings.TrimSpace(c.FormValue("expected_precision")),
		ExpectedYMConfirmed:     c.FormValue("expected_ym_confirmed") == "1",
		ExpectedAmount:          parseSalesAmount(c.FormValue("expected_amount")),
		ExpectedAmountConfirmed: c.FormValue("expected_amount_confirmed") == "1",
		SalesOwnerID:            strings.TrimSpace(c.FormValue("sales_owner_id")),
		SalesOwner:              strings.TrimSpace(c.FormValue("sales_owner")),
		Competitor:              strings.TrimSpace(c.FormValue("competitor")),
		LeadSource:              strings.TrimSpace(c.FormValue("lead_source")),
		LostReason:              strings.TrimSpace(c.FormValue("lost_reason")),
		Notes:                   strings.TrimSpace(c.FormValue("notes")),
		ContractedAt:            strings.TrimSpace(c.FormValue("contracted_at")),
		PONo:                    strings.TrimSpace(c.FormValue("po_no")),
		DeliveredAt:             strings.TrimSpace(c.FormValue("delivered_at")),
		Status:                  model.SalesStatusActive,
	}
	if p.SalesOwnerID != "" && p.SalesOwner == "" && h.userRepo != nil {
		if u, err := h.userRepo.GetByID(p.SalesOwnerID); err == nil && u != nil {
			p.SalesOwner = u.FullName
		}
	}
	stages, _ := h.repo.StagesFor(p.DealType)
	if p.Stage == "" {
		p.Stage = model.DefaultSalesStageCodeFor(p.DealType)
	}
	def := model.FindSalesStage(stages, p.Stage)
	stageProb := 0
	if def != nil {
		stageProb = def.Probability
	}
	if p.IsSupply() {
		p.Probability = 0
		p.HasOverride = false
		p.OverrideValue = 0
		return p
	}
	rawProb := strings.TrimSpace(c.FormValue("probability"))
	if rawProb != "" {
		if n, err := strconv.Atoi(rawProb); err == nil {
			if n != stageProb {
				p.HasOverride = true
				p.OverrideValue = n
			}
		}
	}
	p.Probability = stageProb
	return p
}

func (h *SalesHandler) viewProject(p *model.SalesProject, def *model.SalesStageDef) map[string]interface{} {
	n, total := p.ConfirmedCount()
	customer := p.CustomerValue()
	period := p.PeriodLabel()
	amount := p.AmountLabel()
	return map[string]interface{}{
		"SalesID":                 p.SalesID,
		"Name":                    p.Name,
		"IsTentativeName":         p.IsTentativeName,
		"NameConfirmed":           p.NameConfirmed(),
		"DealType":                model.NormalizeSalesDealType(p.DealType),
		"IsSupply":                p.IsSupply(),
		"Stage":                   p.Stage,
		"StageLabel":              stageLabel(def, p.Stage),
		"DisplayStage":            p.DisplayStage(def),
		"HasOverride":             p.HasOverride,
		"EffectiveProb":           p.EffectiveProbability(),
		"Customer":                customer,
		"CustomerConfirmed":       p.CustomerConfirmed,
		"Period":                  period,
		"ExpectedYMConfirmed":     p.ExpectedYMConfirmed,
		"Amount":                  amount,
		"ExpectedAmountConfirmed": p.ExpectedAmountConfirmed,
		"Owner":                   p.SalesOwner,
		"Status":                  p.Status,
		"LostReason":              p.LostReason,
		"Notes":                   p.Notes,
		"WonAt":                   p.WonAt,
		"ContractedAt":            p.ContractedAt,
		"PONo":                    p.PONo,
		"DeliveredAt":             p.DeliveredAt,
		"Competitor":              p.Competitor,
		"LeadSource":              p.LeadSource,
		"ExpectedYM":              p.ExpectedYM,
		"ExpectedAmount":          p.ExpectedAmount,
		"ConfirmedCount":          n,
		"ConfirmedTotal":          total,
		"ConfirmationLabel":       p.ConfirmationLabel(),
		"Unconfirmed":             p.UnconfirmedItems(),
	}
}

func (h *SalesHandler) CreateActivity(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		id = strings.TrimSpace(c.FormValue("sales_id"))
	}
	a := h.parseActivityForm(c)
	a.SalesID = id
	replyErr := func(err error, link *model.WorkTask) error {
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "code": salesErrCode(err), "error": err.Error()})
		}
		if id == "" {
			return c.Redirect(http.StatusSeeOther, "/sales/activities?err="+salesErrCode(err))
		}
		q := "/sales/" + id + "?err=" + salesErrCode(err)
		if link != nil {
			q = "/sales/" + id + "?tab=timeline&from_task=" + url.QueryEscape(link.TaskID) + "&err=" + salesErrCode(err)
		}
		return c.Redirect(http.StatusSeeOther, q)
	}
	var link *model.WorkTask
	if tid := strings.TrimSpace(c.FormValue("from_task")); tid != "" && h.wbRepo != nil {
		t, err := h.wbRepo.GetTask(tid)
		if err == nil && t != nil {
			if strings.TrimSpace(t.SourceType) != "" {
				return replyErr(fmt.Errorf("이미 원본이 연결된 업무입니다"), t)
			}
			link = t
		}
	}
	if err := h.repo.CreateActivity(a, ctxString(c, "user_name"), link); err != nil {
		return replyErr(err, link)
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "activity_id": a.ActivityID})
	}
	if strings.TrimSpace(c.Param("id")) == "" {
		return c.Redirect(http.StatusSeeOther, "/sales/activities?ok=activity")
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?ok=activity")
}

func (h *SalesHandler) PartyHints(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	names, err := h.repo.ListCustomerPartyHintNames(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"names": []string{}})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"names": names})
}

func (h *SalesHandler) MoveActivity(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	aid := strings.TrimSpace(c.Param("aid"))
	group := salesActGroup(c.FormValue("group"))
	value := strings.TrimSpace(c.FormValue("value"))
	err := h.repo.MoveActivity(aid, group, value, ctxString(c, "user_name"))
	if wantsJSON(c) {
		if err != nil {
			code := salesErrCode(err)
			msg := querySalesErr(code)
			if msg == "" {
				msg = err.Error()
			}
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "code": code, "error": msg})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	back := salesReturnPath(c, "/sales/activities?view=kanban&group="+group)
	if err != nil {
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, back+sep+"err="+salesErrCode(err))
	}
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	if !strings.Contains(back, "ok=") {
		back += sep + "ok=moved"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func (h *SalesHandler) CreateParty(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	p := h.parsePartyForm(c)
	p.SalesID = id
	h.fillPartyFromUser(p)
	if err := h.repo.CreateParty(p, ctxString(c, "user_name")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&err="+salesErrCode(err))
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&ok=party")
}

func (h *SalesHandler) ReplaceParty(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	pid := strings.TrimSpace(c.Param("pid"))
	old, err := h.repo.GetParty(pid)
	if err != nil || old == nil || old.SalesID != id {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&err=party")
	}
	neu := h.parsePartyForm(c)
	h.fillPartyFromUser(neu)
	reason := strings.TrimSpace(c.FormValue("replaced_reason"))
	if err := h.repo.ReplaceParty(pid, neu, reason, ctxString(c, "user_name")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&err="+salesErrCode(err))
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&ok=replaced")
}

func (h *SalesHandler) LinkParty(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	pid := strings.TrimSpace(c.Param("pid"))
	old, err := h.repo.GetParty(pid)
	if err != nil || old == nil || old.SalesID != id {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&err=party")
	}
	if err := h.repo.LinkPartyContact(pid, c.FormValue("contact_id"), ctxString(c, "user_name")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&err="+salesErrCode(err))
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?tab=parties&ok=link")
}

func (h *SalesHandler) parsePartyForm(c echo.Context) *model.SalesParty {
	return &model.SalesParty{
		PartyType:  strings.TrimSpace(c.FormValue("party_type")),
		OrgName:    strings.TrimSpace(c.FormValue("org_name")),
		PersonName: strings.TrimSpace(c.FormValue("person_name")),
		Title:      strings.TrimSpace(c.FormValue("title")),
		Phone:      strings.TrimSpace(c.FormValue("phone")),
		Email:      strings.TrimSpace(c.FormValue("email")),
		PartyRole:  strings.TrimSpace(c.FormValue("party_role")),
		IsPrimary:  c.FormValue("is_primary") == "1",
		UserID:     strings.TrimSpace(c.FormValue("user_id")),
		ContactID:  strings.TrimSpace(c.FormValue("contact_id")),
		Note:       strings.TrimSpace(c.FormValue("note")),
	}
}

func (h *SalesHandler) fillPartyFromUser(p *model.SalesParty) {
	if p == nil || p.UserID == "" || h.userRepo == nil {
		return
	}
	u, err := h.userRepo.GetByID(p.UserID)
	if err != nil || u == nil {
		return
	}
	if strings.TrimSpace(p.PersonName) == "" {
		p.PersonName = u.FullName
	}
}

func (h *SalesHandler) FromTask(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	taskID := strings.TrimSpace(c.FormValue("task_id"))
	if taskID == "" {
		taskID = strings.TrimSpace(c.Param("id"))
	}
	back := "/workboard/tasks/" + taskID
	if h.wbRepo == nil {
		return c.Redirect(http.StatusSeeOther, back+"?err=sales")
	}
	t, err := h.wbRepo.GetTask(taskID)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if strings.TrimSpace(t.SourceType) != "" {
		return c.Redirect(http.StatusSeeOther, back+"?err=sales_linked")
	}
	a := h.parseActivityForm(c)
	a.SalesID = strings.TrimSpace(c.FormValue("sales_id"))
	if a.ActivityDate == "" {
		a.ActivityDate = t.WorkDate
	}
	if a.StartTime == "" {
		a.StartTime = t.StartTime
	}
	if a.DurationMin <= 0 {
		a.DurationMin = t.DurationMin
	}
	if a.Title == "" {
		a.Title = t.Title
	}
	if a.Content == "" {
		a.Content = t.Description
	}
	if a.OurMembers == "" {
		a.OurMembers = t.Assignee
	}
	if err := h.repo.CreateActivity(a, ctxString(c, "user_name"), t); err != nil {
		return c.Redirect(http.StatusSeeOther, back+"?err="+salesErrCode(err))
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+a.SalesID+"?ok=activity")
}

func (h *SalesHandler) parseActivityForm(c echo.Context) *model.SalesActivity {
	dur, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("duration_min")))
	if dur <= 0 {
		dur = 30
	}
	return &model.SalesActivity{
		ActivityDate:   strings.TrimSpace(c.FormValue("activity_date")),
		StartTime:      strings.TrimSpace(c.FormValue("start_time")),
		DurationMin:    dur,
		ActivityType:   strings.TrimSpace(c.FormValue("activity_type")),
		Title:          strings.TrimSpace(c.FormValue("title")),
		Content:        strings.TrimSpace(c.FormValue("content")),
		Place:          strings.TrimSpace(c.FormValue("place")),
		OurMembers:     strings.TrimSpace(c.FormValue("our_members")),
		Counterparts:   strings.TrimSpace(c.FormValue("counterparts")),
		NextAction:     strings.TrimSpace(c.FormValue("next_action")),
		NextActionDate: strings.TrimSpace(c.FormValue("next_action_date")),
	}
}

func stageLabel(def *model.SalesStageDef, code string) string {
	if def != nil && def.Label != "" {
		return def.Label
	}
	return code
}

func parseSalesAmount(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	s = strings.TrimSuffix(s, "원")
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}

func salesErrCode(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "실패 사유") || strings.Contains(msg, "실주할") {
		return "lost"
	}
	if strings.Contains(msg, "되돌릴") {
		return "reason"
	}
	if strings.Contains(msg, "교체 사유") {
		return "replace"
	}
	if strings.Contains(msg, "수주") {
		return "won"
	}
	if strings.Contains(msg, "원본") || strings.Contains(msg, "연결") {
		return "sales_linked"
	}
	if strings.Contains(msg, "관계자") || strings.Contains(msg, "기관명") || strings.Contains(msg, "담당자") {
		return "party"
	}
	if strings.Contains(msg, "활동") || strings.Contains(msg, "날짜") || strings.Contains(msg, "활동 유형") {
		return "activity"
	}
	return "stage"
}

func querySalesErr(code string) string {
	switch code {
	case "reason":
		return "단계를 되돌릴 때는 사유가 필요합니다."
	case "lost":
		return "실주할 때는 실패 사유가 필요합니다."
	case "won":
		return "수주로 넘기려면 사업명·고객·시기·금액을 모두 확정해야 합니다."
	case "activity":
		return "활동을 저장할 수 없습니다. 날짜를 확인하세요."
	case "sales_linked":
		return "이미 원본이 연결된 업무입니다."
	case "replace":
		return "관계자를 바꿀 때는 사유가 필요합니다. 옛 사람은 지우지 않습니다."
	case "party":
		return "관계자를 저장할 수 없습니다. 이름 또는 기관명을 확인하세요."
	case "stage":
		return "단계를 바꿀 수 없습니다."
	case "notfound":
		return "영업 사업을 찾을 수 없습니다."
	case "not_contracted":
		return "수주 단계에서만 사업관리로 등록할 수 있습니다."
	case "already_promoted":
		return "이미 사업관리로 등록된 영업 사업입니다."
	default:
		return ""
	}
}
