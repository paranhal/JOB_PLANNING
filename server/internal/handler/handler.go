package handler

import (
	"database/sql"
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
	AdminWork      *AdminWorkHandler
	Integration    *IntegrationHandler

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
	authH := &AuthHandler{userRepo: userRepo, settingsRepo: settingsRepo, jwtSecret: jwtSecret}

	return &Handler{
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
		},
		Work:       NewWorkHandler(workBoardRepo, asRepo, maintRepo, repository.NewWBRepo(db), userRepo),
		Workboard:  wbH,
		Meeting:    NewMeetingHandler(workBoardRepo, statsRepo, userRepo),
		Stats:      NewStatsHandler(statsRepo, userRepo, repository.NewWBRepo(db), workBoardRepo, maintRepo),
		WorkStatus: NewWorkStatusHandler(repository.NewWBRepo(db), userRepo, holidayRepo),
		Analysis:   &AnalysisHandler{db: db},
		Code:       &CodeHandler{repo: codeRepo},
		Attachment: attachH,
		Backup: &BackupHandler{
			cfg:      backup.Config{DataDir: dataDirFromEnv(), DB: db},
			projects: repository.NewProjectRepo(db),
		},
		Auth:    authH,
		Holiday: &HolidayHandler{
			repo: holidayRepo, leave: repository.NewStaffLeaveRepo(db),
			users: userRepo, auth: authH, api: service.NewHolidayAPIClient(),
		},
		Maintenance: &MaintenanceHandler{
			repo: maintRepo, customerRepo: customerRepo, userRepo: userRepo,
			wbRepo: repository.NewWBRepo(db), settingsRepo: settingsRepo,
			holidayRepo: holidayRepo,
			dataDir:     dataDirFromEnv(),
		},
		Project: NewProjectHandler(
			repository.NewProjectRepo(db), repository.NewWBRepo(db),
			customerRepo, contactRepo, codeRepo, assetRepo,
		),
		AdminWork: NewAdminWorkHandler(repository.NewWBRepo(db), userRepo, customerRepo),
		Integration: NewIntegrationHandler(customerRepo, contactRepo, codeRepo),

		customerRepo: customerRepo,
		asRepo:       asRepo,
		workBoard:    workBoardRepo,
		assetRepo:    assetRepo,
		attachRepo:   attachRepo,
		statsRepo:    statsRepo,
	}
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
	if role == model.RoleTech {
		mineUID, mineK = userID, mineKeys
		mine = true
	}

	stats, err := h.workBoard.DashStats(mineUID, mineK)
	if err != nil {
		return err
	}
	todayList, _ := h.workBoard.ListBucket(model.WorkBucketToday, mineUID, mineK, 10)
	delayedList, _ := h.workBoard.ListBucket(model.WorkBucketDelayed, mineUID, mineK, 8)
	pendingList, _ := h.workBoard.ListBucket(model.WorkBucketSchedulePending, mineUID, mineK, 8)
	unassignedList, _ := h.workBoard.ListBucket(model.WorkBucketUnassigned, "", nil, 8)

	// 상단: 주간 통계 KPI(팀전체, 표시 전용) — 일일 업무회의 표 대신
	now := time.Now()
	var weekKPI model.StatsKPICard
	if h.statsRepo != nil {
		weekCols := repository.BuildStatsPeriodColumns(model.StatsViewWeek, now)
		f := model.StatsMeetingFilter{Scope: model.StatsScopeTeam}
		_ = h.statsRepo.FillPeriodOverview(weekCols, f)
		weekKPI, _ = h.statsRepo.LoadStatsKPI(model.StatsViewWeek, weekCols, f)
		applyPlanningToKPI(h.workBoard, &weekKPI)
	}

	showAssignee := role == model.RoleAdmin || role == model.RoleOffice
	data := map[string]interface{}{
		"Title":          "대시보드",
		"Active":         "dashboard",
		"Role":           role,
		"RoleLabel":      model.RoleLabel(role),
		"DisplayName":    userName,
		"LoginID":        username,
		"WorkStats":      stats,
		"WeekKPI":        weekKPI,
		"ExecTarget":     model.StatsExecTargetPct,
		"VisitTarget":    model.StatsVisitTargetDays,
		"CompleteTarget": model.StatsCompleteTargetDays,
		"OpenHref":       workListURL(model.WorkBucketOpen, mine, role),
		"TodayHref":      workListURL(model.WorkBucketToday, mine, role),
		"DelayedHref":    workListURL(model.WorkBucketDelayed, mine, role),
		"CompletedHref":  workListURL(model.WorkBucketCompletedToday, mine, role),
		"PendingHref":    workListURL(model.WorkBucketSchedulePending, mine, role),
		"UnassignedHref": workListURL(model.WorkBucketUnassigned, false, role),
		"UnplannedHref":  planUnplannedURL(mine, role, ""),
		"TodayList":      todayList,
		"DelayedList":    delayedList,
		"PendingList":    pendingList,
		"UnassignedList": unassignedList,
		"ShowAssignee":   showAssignee,
		"ScopeMine":      mine,
	}

	switch role {
	case model.RoleAdmin:
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work?bucket=today", Label: "오늘 예정", Tone: "sky"},
			{Href: "/work?bucket=unassigned", Label: "미배정", Tone: "amber"},
			{Href: "/maintenance", Label: "정기점검", Tone: "slate"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
		}
	case model.RoleOffice:
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work?bucket=today", Label: "오늘 예정", Tone: "sky"},
			{Href: "/work?bucket=unassigned", Label: "미배정", Tone: "amber"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
			{Href: "/customers", Label: "고객현황", Tone: "green"},
		}
	case model.RoleTech:
		data["QuickLinks"] = []dashLink{
			{Href: workListURL(model.WorkBucketToday, true, role), Label: "오늘 예정", Tone: "sky"},
			{Href: workListURL(model.WorkBucketDelayed, true, role), Label: "지연 업무", Tone: "red"},
			{Href: workListURL(model.WorkBucketOpen, true, role), Label: "내 전체 업무", Tone: "yellow"},
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/assets", Label: "설치자산", Tone: "purple"},
		}
	case model.RoleSales:
		data["QuickLinks"] = []dashLink{
			{Href: "/analysis", Label: "분석", Tone: "indigo"},
			{Href: "/stats", Label: "통계", Tone: "sky"},
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
