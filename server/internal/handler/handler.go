package handler

import (
	"database/sql"

	"github.com/labstack/echo/v4"

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
	Stats          *StatsHandler
	WorkStatus     *WorkStatusHandler
	Analysis       *AnalysisHandler
	Code           *CodeHandler
	Attachment     *AttachmentHandler
	Auth           *AuthHandler
	Maintenance    *MaintenanceHandler

	customerRepo *repository.CustomerRepo
	asRepo       *repository.ASRepo
	assetRepo    *repository.AssetRepo
	attachRepo   *repository.AttachmentRepo
}

func New(db *sql.DB) *Handler {
	customerRepo := repository.NewCustomerRepo(db)
	contactRepo := repository.NewContactRepo(db)
	contactHistRepo := repository.NewContactHistoryRepo(db)
	asRepo := repository.NewASRepo(db)
	asProcessRepo := repository.NewASProcessRepo(db)
	assetRepo := repository.NewAssetRepo(db)
	spaceRepo := repository.NewSpaceRepo(db)
	codeRepo := repository.NewCodeRepo(db)
	relationRepo := repository.NewRelationRepo(db)
	swDetailRepo := repository.NewSWDetailRepo(db)
	attachRepo := repository.NewAttachmentRepo(db)
	userRepo := repository.NewUserRepo(db)
	maintRepo := repository.NewMaintenanceRepo(db)
	statsRepo := repository.NewStatsRepo(db)

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
		},
		SWDetail: &SWDetailHandler{repo: swDetailRepo},
		Relation: &RelationHandler{repo: relationRepo, customerRepo: customerRepo, codeRepo: codeRepo},
		AS: &ASHandler{
			repo: asRepo, processRepo: asProcessRepo,
			customerRepo: customerRepo, assetRepo: assetRepo,
			contactRepo: contactRepo, codeRepo: codeRepo,
			userRepo: userRepo, relationRepo: relationRepo,
		},
		Stats:    NewStatsHandler(statsRepo),
		WorkStatus: NewWorkStatusHandler(repository.NewWorkStatusRepo(db)),
		Analysis: &AnalysisHandler{db: db},
		Code:     &CodeHandler{repo: codeRepo},
		Attachment: &AttachmentHandler{repo: attachRepo, uploadDir: "data/uploads"},
		Auth:     &AuthHandler{userRepo: userRepo, jwtSecret: jwtSecret},
		Maintenance: &MaintenanceHandler{
			repo: maintRepo, customerRepo: customerRepo,
		},

		customerRepo: customerRepo,
		asRepo:       asRepo,
		assetRepo:    assetRepo,
		attachRepo:   attachRepo,
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

	var totalCustomers, totalAssets int
	h.customerRepo.CountActive(&totalCustomers)
	h.assetRepo.CountOperating(&totalAssets)

	data := map[string]interface{}{
		"Title":               "대시보드",
		"Active":              "dashboard",
		"Role":                role,
		"RoleLabel":           roleLabelText(role),
		"DisplayName":         userName,
		"LoginID":             username,
		"TotalCustomers":      totalCustomers,
		"TotalAssets":         totalAssets,
		"ShowASStats":         true,
		"ShowCustomers":       false,
		"ShowAssets":          false,
		"ShowSales":           false,
		"ShowAssigned":        false,
		"ShowAssigneeStats":   true,
		"ShowSchedulePending": true,
		"ReadOnly":            false,
		"MineQuery":           "",
		"ProgressHref":        "/as?status=in_progress",
		"OverdueHref":         "/as?status=overdue",
		"TotalHref":           "/as",
		"WeekElapsedDays":     repository.WeekElapsedDays(),
	}

	switch role {
	case "admin":
		stats, err := h.asRepo.DashboardStats("", nil)
		if err != nil {
			return err
		}
		data["Stats"] = stats
		pending, pendingTotal, _ := h.asRepo.ListSchedulePending("", nil, 15)
		data["SchedulePending"] = pending
		data["SchedulePendingTotal"] = pendingTotal
		recentAS, _, _ := h.asRepo.List("", "", 1, 5)
		data["RecentAS"] = recentAS
		data["ListTitle"] = "최근 AS 접수"
		data["ShowCustomers"] = true
		data["ShowAssets"] = true
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/customers/new", Label: "고객 등록", Tone: "green"},
			{Href: "/assets/new", Label: "자산 등록", Tone: "purple"},
			{Href: "/analysis", Label: "교체 분석", Tone: "orange"},
			{Href: "/maintenance", Label: "정기점검", Tone: "slate"},
			{Href: "/as/stats", Label: "AS 현황", Tone: "indigo"},
		}

	case "receipt":
		stats, err := h.asRepo.DashboardStats("", nil)
		if err != nil {
			return err
		}
		data["Stats"] = stats
		pending, pendingTotal, _ := h.asRepo.ListSchedulePending("", nil, 15)
		data["SchedulePending"] = pending
		data["SchedulePendingTotal"] = pendingTotal
		recentAS, _, _ := h.asRepo.List("today", "", 1, 8)
		if len(recentAS) == 0 {
			recentAS, _, _ = h.asRepo.List("", "", 1, 8)
		}
		data["RecentAS"] = recentAS
		data["ListTitle"] = "오늘·최근 AS 접수"
		data["ShowCustomers"] = true
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/as", Label: "AS 목록", Tone: "indigo"},
			{Href: "/as/stats", Label: "AS 현황", Tone: "slate"},
			{Href: "/customers", Label: "고객현황", Tone: "green"},
		}

	case "tech":
		mineStats, err := h.asRepo.DashboardStats(userID, mineKeys)
		if err != nil {
			return err
		}
		data["Stats"] = mineStats
		data["MineQuery"] = "&mine=1"
		data["TotalHref"] = "/as?mine=1"
		data["ProgressHref"] = "/as?status=in_progress&mine=1"
		data["OverdueHref"] = "/as?status=overdue&mine=1"
		pending, pendingTotal, _ := h.asRepo.ListSchedulePending(userID, mineKeys, 15)
		data["SchedulePending"] = pending
		data["SchedulePendingTotal"] = pendingTotal
		assigned, assignedTotal, _ := h.asRepo.ListAssignedOpen(userID, mineKeys, 1, 8)
		data["RecentAS"] = assigned
		data["ListTitle"] = "내 배정 AS (미완료)"
		data["AssignedAS"] = assigned
		data["AssignedTotal"] = assignedTotal
		data["ShowAssigned"] = true
		data["ShowAssets"] = true
		data["QuickLinks"] = []dashLink{
			{Href: "/as/new", Label: "AS 접수", Tone: "blue"},
			{Href: "/as?status=open&mine=1", Label: "내 미처리", Tone: "yellow"},
			{Href: "/as?status=overdue&mine=1", Label: "내 지연", Tone: "red"},
			{Href: "/assets", Label: "설치자산", Tone: "purple"},
			{Href: "/maintenance", Label: "정기점검", Tone: "slate"},
			{Href: "/as/stats", Label: "AS 현황", Tone: "indigo"},
		}

	default:
		return c.Render(200, "auth/coming_soon.html", map[string]interface{}{
			"Title": "준비 중", "Active": "dashboard",
			"Role": role, "RoleLabel": roleLabelText(role),
		})
	}

	assigneeStats, _ := h.asRepo.AssigneeDashboardStats()
	data["AssigneeStats"] = assigneeStats

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
