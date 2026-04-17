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
	Analysis       *AnalysisHandler
	Code           *CodeHandler
	Attachment     *AttachmentHandler
	Auth           *AuthHandler
	Maintenance    *MaintenanceHandler

	customerRepo *repository.CustomerRepo
	asRepo       *repository.ASRepo
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

	jwtSecret := []byte("cs-system-jwt-secret-2026")

	return &Handler{
		Customer: &CustomerHandler{repo: customerRepo},
		Space:    &SpaceHandler{repo: spaceRepo, customerRepo: customerRepo},
		Contact:  &ContactHandler{repo: contactRepo, customerRepo: customerRepo, histRepo: contactHistRepo},
		ContactHistory: &ContactHistoryHandler{
			repo: contactHistRepo, contactRepo: contactRepo, customerRepo: customerRepo,
		},
		Asset:    &AssetHandler{repo: assetRepo, customerRepo: customerRepo, codeRepo: codeRepo},
		SWDetail: &SWDetailHandler{repo: swDetailRepo},
		Relation: &RelationHandler{repo: relationRepo, customerRepo: customerRepo, codeRepo: codeRepo},
		AS:       &ASHandler{repo: asRepo, processRepo: asProcessRepo, customerRepo: customerRepo, assetRepo: assetRepo, codeRepo: codeRepo},
		Analysis: &AnalysisHandler{db: db},
		Code:     &CodeHandler{repo: codeRepo},
		Attachment: &AttachmentHandler{repo: attachRepo, uploadDir: "data/uploads"},
		Auth:     &AuthHandler{userRepo: userRepo, jwtSecret: jwtSecret},
		Maintenance: &MaintenanceHandler{
			repo: maintRepo, customerRepo: customerRepo,
		},

		customerRepo: customerRepo,
		asRepo:       asRepo,
	}
}

func (h *Handler) Dashboard(c echo.Context) error {
	stats, err := h.asRepo.Stats()
	if err != nil {
		return err
	}
	recentAS, _, err := h.asRepo.List("", "", 1, 5)
	if err != nil {
		return err
	}

	var totalCustomers, totalAssets int
	h.customerRepo.CountActive(&totalCustomers)

	return c.Render(200, "dashboard.html", map[string]interface{}{
		"Title":          "대시보드",
		"Stats":          stats,
		"RecentAS":       recentAS,
		"Active":         "dashboard",
		"TotalCustomers": totalCustomers,
		"TotalAssets":    totalAssets,
	})
}
