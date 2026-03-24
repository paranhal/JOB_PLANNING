package handler

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

// ── Space 핸들러 ─────────────────────────────────────────────────

type SpaceHandler struct{ db *sql.DB }

func (h *SpaceHandler) List(c echo.Context) error {
	return c.Render(http.StatusOK, "space/list.html", map[string]interface{}{
		"Title": "공간 관리", "Active": "spaces",
	})
}

// ── Asset 핸들러 ─────────────────────────────────────────────────

type AssetHandler struct {
	db           *sql.DB
	customerRepo *repository.CustomerRepo
}

func (h *AssetHandler) List(c echo.Context) error {
	return c.Render(http.StatusOK, "asset/list.html", map[string]interface{}{
		"Title": "설치자산 관리", "Active": "assets",
	})
}

func (h *AssetHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	return c.Render(http.StatusOK, "asset/form.html", map[string]interface{}{
		"Title": "자산 등록", "Active": "assets",
		"Customers": customers, "IsNew": true,
	})
}

func (h *AssetHandler) Create(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/assets")
}

func (h *AssetHandler) Show(c echo.Context) error {
	return c.Render(http.StatusOK, "asset/show.html", map[string]interface{}{
		"Title": "자산 상세", "Active": "assets",
	})
}

// ── Analysis 핸들러 ──────────────────────────────────────────────

type AnalysisHandler struct{ db *sql.DB }

func (h *AnalysisHandler) Dashboard(c echo.Context) error {
	return c.Render(http.StatusOK, "analysis/dashboard.html", map[string]interface{}{
		"Title": "분석 / 영업활용", "Active": "analysis",
	})
}

// ── Code 핸들러 ──────────────────────────────────────────────────

type CodeHandler struct{ db *sql.DB }

func (h *CodeHandler) List(c echo.Context) error {
	rows, err := h.db.Query(
		`SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes ORDER BY code_group, sort_order`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type codeItem struct {
		CodeID    string
		CodeGroup string
		CodeValue string
		CodeName  string
		SortOrder int
		IsActive  bool
	}
	var codes []codeItem
	for rows.Next() {
		var item codeItem
		var isActive int
		if err := rows.Scan(&item.CodeID, &item.CodeGroup, &item.CodeValue,
			&item.CodeName, &item.SortOrder, &isActive); err != nil {
			return err
		}
		item.IsActive = isActive == 1
		codes = append(codes, item)
	}

	return c.Render(http.StatusOK, "code/list.html", map[string]interface{}{
		"Title": "코드 관리", "Active": "codes", "Codes": codes,
	})
}
