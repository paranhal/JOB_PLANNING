package handler

import (
	"database/sql"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

// Handler 모든 핸들러를 묶는 컨테이너
type Handler struct {
	Customer *CustomerHandler
	Space    *SpaceHandler
	Contact  *ContactHandler
	Asset    *AssetHandler
	AS       *ASHandler
	Analysis *AnalysisHandler
	Code     *CodeHandler

	customerRepo *repository.CustomerRepo
	asRepo       *repository.ASRepo
}

func New(db *sql.DB) *Handler {
	customerRepo := repository.NewCustomerRepo(db)
	asRepo := repository.NewASRepo(db)

	return &Handler{
		Customer: &CustomerHandler{repo: customerRepo},
		Space:    &SpaceHandler{db: db},
		Contact:  &ContactHandler{db: db},
		Asset:    &AssetHandler{db: db, customerRepo: customerRepo},
		AS:       &ASHandler{repo: asRepo, customerRepo: customerRepo},
		Analysis: &AnalysisHandler{db: db},
		Code:     &CodeHandler{db: db},

		customerRepo: customerRepo,
		asRepo:       asRepo,
	}
}

// Dashboard 메인 대시보드
func (h *Handler) Dashboard(c echo.Context) error {
	stats, err := h.asRepo.Stats()
	if err != nil {
		return err
	}

	recentAS, _, err := h.asRepo.List("received", "", 1, 5)
	if err != nil {
		return err
	}

	return c.Render(200, "dashboard.html", map[string]interface{}{
		"Title":    "대시보드",
		"Stats":    stats,
		"RecentAS": recentAS,
		"Active":   "dashboard",
	})
}
