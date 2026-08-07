package handler

import (
	"database/sql"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
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
	Stats          *StatsHandler
	WorkStatus     *WorkStatusHandler
	Analysis       *AnalysisHandler
	Code           *CodeHandler
	Attachment     *AttachmentHandler
	Auth           *AuthHandler
	Maintenance    *MaintenanceHandler
	Project        *ProjectHandler

	customerRepo *repository.CustomerRepo
	asRepo       *repository.ASRepo
	workBoard    *repository.WorkBoardRepo
	assetRepo    *repository.AssetRepo
	attachRepo   *repository.AttachmentRepo
	statsRepo    *repository.StatsRepo
}

func New(db *sql.DB) *Handler {
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

	jwtSecret := []byte("cs-system-jwt-secret-2026")

	return &Handler{
		Customer: &CustomerHandler{
			repo:        customerRepo,
			assetRepo:   assetRepo,
			contactRepo: contactRepo,
			asRepo:      asRepo,
		},
		Space:    &SpaceHandler{repo: spaceRepo, customerRepo: customerRepo},
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
			settingsRepo: repository.NewSettingsRepo(db),
			unlockRepo:   repository.NewASUnlockRepo(db),
			customerRepo: customerRepo, assetRepo: assetRepo,
			contactRepo: contactRepo, codeRepo: codeRepo,
			userRepo: userRepo, relationRepo: relationRepo,
			attachRepo: attachRepo,
		},
		Work:       NewWorkHandler(workBoardRepo),
		Workboard: NewWorkboardHandler(
			repository.NewWBRepo(db), userRepo, customerRepo, contactRepo, codeRepo,
			asRepo, maintRepo,
		),
		Stats:      NewStatsHandler(statsRepo, userRepo, repository.NewWBRepo(db)),
		WorkStatus: NewWorkStatusHandler(repository.NewWorkStatusRepo(db)),
		Analysis:   &AnalysisHandler{db: db},
		Code:       &CodeHandler{repo: codeRepo},
		Attachment: &AttachmentHandler{repo: attachRepo, uploadDir: "data/uploads"},
		Auth:       &AuthHandler{userRepo: userRepo, jwtSecret: jwtSecret},
		Maintenance: &MaintenanceHandler{
			repo: maintRepo, customerRepo: customerRepo, userRepo: userRepo,
		},
		Project: NewProjectHandler(
			repository.NewProjectRepo(db), customerRepo, contactRepo, codeRepo, assetRepo,
		),

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
		role = "viewer"
	}
	userName := ctxString(c, "user_name")
	username := ctxString(c, "username")
	userID := ctxString(c, "user_id")
	mineKeys := []string{userName, username}

	if role == "sales" || role == "viewer" {
		return c.Render(200, "auth/coming_soon.html", map[string]interface{}{
			"Title": "준비 중", "Active": "dashboard",
			"Role": role, "RoleLabel": roleLabelText(role),
		})
	}

	mineUID, mineK := "", []string(nil)
	mine := false
	if role == "tech" {
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
	}

	showAssignee := role == "admin" || role == "receipt"
	data := map[string]interface{}{
		"Title":       "대시보드",
		"Active":      "dashboard",
		"Role":        role,
		"RoleLabel":   roleLabelText(role),
		"DisplayName": userName,
		"LoginID":     username,
		"WorkStats":   stats,
		"WeekKPI":     weekKPI,
		"ExecTarget":  model.StatsExecTargetPct,
		"VisitTarget": model.StatsVisitTargetDays,
		"OpenHref":       workListURL(model.WorkBucketOpen, mine, role),
		"TodayHref":      workListURL(model.WorkBucketToday, mine, role),
		"DelayedHref":    workListURL(model.WorkBucketDelayed, mine, role),
		"CompletedHref":  workListURL(model.WorkBucketCompletedToday, mine, role),
		"PendingHref":    workListURL(model.WorkBucketSchedulePending, mine, role),
		"UnassignedHref": workListURL(model.WorkBucketUnassigned, false, role),
		"TodayList":      todayList,
		"DelayedList":    delayedList,
		"PendingList":    pendingList,
		"UnassignedList": unassignedList,
		"ShowAssignee":   showAssignee,
		"ScopeMine":      mine,
	}

	switch role {
	case "admin":
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work?bucket=today", Label: "오늘 예정", Tone: "sky"},
			{Href: "/work?bucket=unassigned", Label: "미배정", Tone: "amber"},
			{Href: "/maintenance", Label: "정기점검", Tone: "slate"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
		}
	case "receipt":
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/work?bucket=today", Label: "오늘 예정", Tone: "sky"},
			{Href: "/work?bucket=unassigned", Label: "미배정", Tone: "amber"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
			{Href: "/customers", Label: "고객현황", Tone: "green"},
		}
	case "tech":
		data["QuickLinks"] = []dashLink{
			{Href: workListURL(model.WorkBucketToday, true, role), Label: "오늘 예정", Tone: "sky"},
			{Href: workListURL(model.WorkBucketDelayed, true, role), Label: "지연 업무", Tone: "red"},
			{Href: workListURL(model.WorkBucketOpen, true, role), Label: "내 전체 업무", Tone: "yellow"},
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/assets", Label: "설치자산", Tone: "purple"},
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
	m := map[string]string{
		"admin": "관리자", "receipt": "접수담당", "tech": "기술담당",
		"sales": "영업담당", "viewer": "열람사용자",
	}
	if l, ok := m[role]; ok {
		return l
	}
	return role
}
