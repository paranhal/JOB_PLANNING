package handler

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

type Handler struct {
	Customer       *CustomerHandler
	Space          *SpaceHandler
	Contact        *ContactHandler
	ContactHistory *ContactHistoryHandler
	Asset          *AssetHandler
	SWDetail       *SWDetailHandler
	Relation       *RelationHandler
	AS             *ASHandler
	Work           *WorkHandler
	Workboard      *WorkboardHandler
	Meeting        *MeetingHandler
	Stats          *StatsHandler
	WorkStatus     *WorkStatusHandler
	Analysis       *AnalysisHandler
	Code           *CodeHandler
	Attachment     *AttachmentHandler
	Backup         *BackupHandler
	Holiday        *HolidayHandler
	Auth           *AuthHandler
	Maintenance    *MaintenanceHandler
	Project        *ProjectHandler
	Sales          *SalesHandler
	Quotes         *QuotesHandler
	Orders         *OrdersHandler
	Items          *ItemsHandler
	AdminWork      *AdminWorkHandler
	Integration    *IntegrationHandler
	System         *SystemHandler
	notices        *assignNoticeHook

	customerRepo *repository.CustomerRepo
	asRepo       *repository.ASRepo
	workBoard    *repository.WorkBoardRepo
	assetRepo    *repository.AssetRepo
	attachRepo   *repository.AttachmentRepo
	statsRepo    *repository.StatsRepo
}

func New(db *sql.DB) *Handler {
	audit.Init(db)
	customerRepo := repository.NewCustomerRepo(db)
	contactRepo := repository.NewContactRepo(db)
	contactHistRepo := repository.NewContactHistoryRepo(db)
	asRepo := repository.NewASRepo(db)
	asProcessRepo := repository.NewASProcessRepo(db)
	asWorkRepo := repository.NewASWorkRepo(db)
	assetRepo := repository.NewAssetRepo(db)
	spaceRepo := repository.NewSpaceRepo(db)
	codeRepo := repository.NewCodeRepo(db)
	relationRepo := repository.NewRelationRepo(db)
	swDetailRepo := repository.NewSWDetailRepo(db)
	attachRepo := repository.NewAttachmentRepo(db)
	userRepo := repository.NewUserRepo(db)
	maintRepo := repository.NewMaintenanceRepo(db)
	statsRepo := repository.NewStatsRepo(db)
	workBoardRepo := repository.NewWorkBoardRepo(db)

	holidayRepo := repository.NewHolidayRepo(db)
	service.SetHolidayCalendar(service.NewCalendar(holidayRepo))
	jwtSecret := []byte("cs-system-jwt-secret-2026")
	settingsRepo := repository.NewSettingsRepo(db)
	unlockRepo := repository.NewASUnlockRepo(db)
	attachH := &AttachmentHandler{
		repo: attachRepo, asRepo: asRepo, unlockRepo: unlockRepo, uploadDir: "data/uploads",
	}

	wbH := NewWorkboardHandler(
		repository.NewWBRepo(db), userRepo, customerRepo, contactRepo, codeRepo,
		asRepo, maintRepo,
		settingsRepo, repository.NewASUnlockRepo(db),
	)
	wbH.attach = attachH
	wbH.salesRepo = repository.NewSalesRepo(db)
	wbH.workBoard = workBoardRepo
	authH := &AuthHandler{userRepo: userRepo, settingsRepo: settingsRepo, jwtSecret: jwtSecret}
	noticeHook := newAssignNoticeHook(repository.NewAssignNoticeRepo(db), userRepo)
	wbH.notices = noticeHook

	h := &Handler{
		Customer: &CustomerHandler{
			repo:        customerRepo,
			assetRepo:   assetRepo,
			contactRepo: contactRepo,
			asRepo:      asRepo,
		},
		Space: &SpaceHandler{repo: spaceRepo, customerRepo: customerRepo},
		Contact: &ContactHandler{
			repo: contactRepo, customerRepo: customerRepo,
			histRepo: contactHistRepo, codeRepo: codeRepo,
		},
		ContactHistory: &ContactHistoryHandler{
			repo: contactHistRepo, contactRepo: contactRepo, customerRepo: customerRepo,
		},
		Asset: &AssetHandler{
			repo: assetRepo, customerRepo: customerRepo, codeRepo: codeRepo, attachRepo: attachRepo,
			wbRepo: repository.NewWBRepo(db),
		},
		SWDetail: &SWDetailHandler{repo: swDetailRepo},
		Relation: &RelationHandler{repo: relationRepo, customerRepo: customerRepo, codeRepo: codeRepo},
		AS: &ASHandler{
			repo: asRepo, processRepo: asProcessRepo, workRepo: asWorkRepo,
			wbRepo:       repository.NewWBRepo(db),
			settingsRepo: settingsRepo,
			unlockRepo:   unlockRepo,
			customerRepo: customerRepo, assetRepo: assetRepo,
			contactRepo: contactRepo, codeRepo: codeRepo,
			userRepo: userRepo, relationRepo: relationRepo,
			attachRepo: attachRepo,
			attach:     attachH,
			kwRepo:     repository.NewASKeywordRepo(db),
			notices:    noticeHook,
		},
		Work:       NewWorkHandler(workBoardRepo, asRepo, maintRepo, repository.NewWBRepo(db), userRepo, customerRepo),
		Workboard:  wbH,
		Meeting:    NewMeetingHandler(workBoardRepo, statsRepo, userRepo),
		Stats:      NewStatsHandler(statsRepo, userRepo, repository.NewWBRepo(db), workBoardRepo, maintRepo),
		WorkStatus: NewWorkStatusHandler(repository.NewWBRepo(db), userRepo, holidayRepo, repository.NewSalesRepo(db)),
		Analysis:   &AnalysisHandler{db: db},
		Code:       &CodeHandler{repo: codeRepo},
		Attachment: attachH,
		Backup: &BackupHandler{
			cfg:        backup.Config{DataDir: dataDirFromEnv(), DB: db},
			projects:   repository.NewProjectRepo(db),
			settings:   settingsRepo,
			importRepo: repository.NewASImportRepo(db),
			customers:  customerRepo,
		},
		Auth: authH,
		Holiday: &HolidayHandler{
			repo: holidayRepo, leave: repository.NewStaffLeaveRepo(db),
			users: userRepo, auth: authH, api: service.NewHolidayAPIClient(),
		},
		Maintenance: &MaintenanceHandler{
			repo: maintRepo, customerRepo: customerRepo, userRepo: userRepo,
			wbRepo: repository.NewWBRepo(db), settingsRepo: settingsRepo,
			holidayRepo: holidayRepo, notices: noticeHook,
			dataDir: dataDirFromEnv(),
		},
		Project: NewProjectHandler(
			repository.NewProjectRepo(db), repository.NewWBRepo(db),
			customerRepo, contactRepo, codeRepo, assetRepo,
		),
		Sales: NewSalesHandler(
			repository.NewSalesRepo(db), repository.NewProjectRepo(db), repository.NewWBRepo(db),
			customerRepo, contactRepo, userRepo, codeRepo,
		),
		Quotes: NewQuotesHandler(
			repository.NewQuoteRepo(db), repository.NewSalesItemRepo(db), repository.NewSalesRepo(db),
			userRepo, settingsRepo, repository.NewOrderRepo(db),
		),
		Orders: NewOrdersHandler(
			repository.NewOrderRepo(db), repository.NewQuoteRepo(db), repository.NewSalesRepo(db), attachRepo,
		),
		Items:       NewItemsHandler(repository.NewSalesItemRepo(db)),
		AdminWork:   NewAdminWorkHandler(repository.NewWBRepo(db), userRepo, customerRepo),
		Integration: NewIntegrationHandler(customerRepo, contactRepo, codeRepo),
		System:      &SystemHandler{db: db, auth: authH, uploadDir: attachH.uploadDir},

		customerRepo: customerRepo,
		asRepo:       asRepo,
		workBoard:    workBoardRepo,
		assetRepo:    assetRepo,
		attachRepo:   attachRepo,
		statsRepo:    statsRepo,
		notices:      noticeHook,
	}
	h.Work.notices = noticeHook
	h.AdminWork.notices = noticeHook
	bindSchemaDB(db)
	return h
}

func (h *Handler) Dashboard(c echo.Context) error {
	role := ctxString(c, "role")
	if role == "" {
		role = model.RoleObserver
	}
	userName := ctxString(c, "user_name")
	username := ctxString(c, "username")
	userID := ctxString(c, "user_id")
	mineKeys := []string{userName, username}

	role = model.NormalizeRole(role)

	mineUID, mineK := "", []string(nil)
	mine := false
	teamScope := strings.TrimSpace(c.QueryParam("scope")) == "team"
	if model.IsKnownRole(role) && !teamScope {
		mineUID, mineK = userID, mineKeys
		mine = true
	}

	stats, todayList, delayedList, pendingList, unassignedList, err := h.workBoard.DashHome(mineUID, mineK, 10, 8, 8, 8)
	if err != nil {
		return err
	}

	var teamDelayed []model.WorkListItem
	if mine && (role == model.RoleAdmin || role == model.RoleOffice) {
		teamDelayed, _ = h.workBoard.ListBucket(model.WorkBucketDelayed, "", nil, 8)
	}

	now := time.Now()
	var weekReview model.WeekReview
	showWeekReview := false
	if mine && weekReviewShouldShow(now, weekReviewCookieValue(c)) {
		weekReview = loadWeekReview(h.workBoard, mineUID, mineK, now)
		showWeekReview = true
	}
	metricsBase := metricsViewData(h.statsRepo)
	lb := parseLookback(c, metricsBase, now)
	model.ApplyDashboardChartView(&lb)
	var weekKPI model.StatsKPICard
	seriesJSON := []byte("[]")
	if h.statsRepo != nil {
		fromIncl, toIncl := lookbackTimes(lb, now)
		cols := repository.BuildStatsRangeColumns(fromIncl, toIncl)
		f := model.StatsMeetingFilter{Scope: model.StatsScopeTeam}
		// FillPeriodOverview 는 회의 요약·통계 화면용. 대시보드는 KPI·차트만 쓴다.
		weekKPI, _ = h.statsRepo.LoadStatsKPI(model.StatsViewRange, cols, f)
		applyPlanningToKPI(h.workBoard, &weekKPI)
		if series, err := h.statsRepo.LoadStatsChartSeriesWindow(lb.View, fromIncl, toIncl, f); err == nil {
			if b, err := json.Marshal(series); err == nil {
				seriesJSON = b
			}
		}
	}
	var missingComplete repository.MissingCompleteDates
	if h.workBoard != nil {
		missingComplete, _ = h.workBoard.CountMissingCompleteDates()
	}

	showAssignee := !mine && (role == model.RoleAdmin || role == model.RoleOffice)
	canToggleTeam := role == model.RoleAdmin || role == model.RoleOffice || role == model.RoleSales
	data := map[string]interface{}{
		"Title":             "대시보드",
		"Active":            NavDashboard,
		"Role":              role,
		"RoleLabel":         model.RoleLabel(role),
		"DisplayName":       userName,
		"LoginID":           username,
		"WorkStats":         stats,
		"WeekKPI":           weekKPI,
		"Lookback":          lb,
		"LookbackQS":        template.URL(lb.PeriodQueryValues()),
		"LookbackViewID":    "dashLookbackView",
		"LookbackHideView":  true,
		"ChartBucketNote":   model.ChartBucketNote(lb.View),
		"SeriesJSON":        template.JS(string(seriesJSON)),
		"VisitTarget":       model.StatsVisitTargetDays,
		"VisitLeadHint":     model.StatsVisitLeadHint,
		"MetricScopeAS":     model.StatsMetricScopeASOnly,
		"MetricScopeCounts": model.StatsMetricScopeCounts,
		"CompleteTarget":    model.StatsCompleteTargetDays,
		"OpenHref":          "/work/all",
		"TodayHref":         "/work",
		"DelayedHref":       workAllURL("", "delayed"),
		"CompletedHref":     workAllURL("", workStatusComplete),
		"PendingHref":       workAllURL("", workStatusWaiting),
		"UnassignedHref":    workAllURL(workAssigneeNone, ""),
		"UnplannedHref":     planUnplannedURL(mine, role, ""),
		"UpcomingDays":      model.RecurrenceUpcomingDays,
		"MetricsBaseDate":   metricsBase,
		"TodayList":         todayList,
		"DelayedList":       delayedList,
		"PendingList":       pendingList,
		"UnassignedList":    unassignedList,
		"TeamDelayedList":   teamDelayed,
		"ShowAssignee":      showAssignee,
		"ScopeMine":         mine,
		"CanToggleTeam":     canToggleTeam,
		"TeamScopeHref":     "/?scope=team",
		"MineScopeHref":     "/",
		"ShowWeekReview":    showWeekReview,
		"WeekReviewLine":    weekReview.Line(),
		"MissingComplete":   missingComplete,
	}

	switch role {
	case model.RoleAdmin:
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work", Label: "오늘 예정", Tone: "sky"},
			{Href: workAllURL(workAssigneeNone, ""), Label: "미배정", Tone: "amber"},
			{Href: "/maintenance", Label: "정기점검", Tone: "slate"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
		}
	case model.RoleOffice:
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work", Label: "오늘 예정", Tone: "sky"},
			{Href: workAllURL(workAssigneeNone, ""), Label: "미배정", Tone: "amber"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
			{Href: "/customers", Label: "고객현황", Tone: "green"},
		}
	case model.RoleTech:
		data["QuickLinks"] = []dashLink{
			{Href: "/work", Label: "오늘 예정", Tone: "sky"},
			{Href: workAllURL("", "delayed"), Label: "지연 업무", Tone: "red"},
			{Href: "/work/all", Label: "내 전체 업무", Tone: "yellow"},
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/assets", Label: "설치자산", Tone: "purple"},
		}
	case model.RoleSales:
		data["QuickLinks"] = []dashLink{
			{Href: "/sales", Label: "영업 사업", Tone: "blue"},
			{Href: "/sales/activities", Label: "영업 활동", Tone: "sky"},
			{Href: "/customers", Label: "고객현황", Tone: "green"},
		}
	}

	return c.Render(200, "dashboard.html", data)
}

type dashLink struct {
	Href  string
	Label string
	Tone  string
}

func roleLabelText(role string) string {
	return model.RoleLabel(role)
}
