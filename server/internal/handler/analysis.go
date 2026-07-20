package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type AnalysisHandler struct{ db *sql.DB }

type analysisRow struct {
	AssetID       string
	CustomerName  string
	ProductName   string
	ProductType   string
	Manufacturer  string
	InstallerType string
	InstallDate   string
	InstallYears  int
	ASCount       int
	OpStatus      string
}

const analysisPageSize = 20

func analysisPage(c echo.Context, key string) int {
	page, _ := strconv.Atoi(c.QueryParam(key))
	if page < 1 {
		page = 1
	}
	return page
}

func totalPages(total, pageSize int) int {
	if total <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func (h *AnalysisHandler) Dashboard(c echo.Context) error {
	agingPage := analysisPage(c, "aging_page")
	freqPage := analysisPage(c, "freq_page")
	otherPage := analysisPage(c, "other_page")

	agingTotal := 0
	h.db.QueryRow(`
		SELECT COUNT(*) FROM assets
		WHERE install_date!='' AND operation_status NOT IN ('disposed','retired')
		  AND (julianday('now')-julianday(install_date))/365 >= 5`).Scan(&agingTotal)

	freqTotal := 0
	h.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT a.asset_id FROM assets a
			WHERE a.operation_status NOT IN ('disposed','retired')
			GROUP BY a.asset_id
			HAVING (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id) >= 3
		)`).Scan(&freqTotal)

	otherTotal := 0
	h.db.QueryRow(`
		SELECT COUNT(*) FROM assets
		WHERE installer_type IN ('other','manufacturer','partner','unknown')
		  AND operation_status NOT IN ('disposed','retired')`).Scan(&otherTotal)

	aging, _ := h.queryAnalysis(`
		SELECT a.asset_id, c.org_name, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.manufacturer,''), COALESCE(a.installer_type,''),
		       COALESCE(a.install_date,''),
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id),
		       a.operation_status
		FROM assets a JOIN customers c ON c.customer_id=a.customer_id
		WHERE a.install_date!='' AND a.operation_status NOT IN ('disposed','retired')
		  AND (julianday('now')-julianday(a.install_date))/365 >= 5
		ORDER BY (julianday('now')-julianday(a.install_date)) DESC
		LIMIT ? OFFSET ?`, analysisPageSize, (agingPage-1)*analysisPageSize)

	frequent, _ := h.queryAnalysis(`
		SELECT a.asset_id, c.org_name, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.manufacturer,''), COALESCE(a.installer_type,''),
		       COALESCE(a.install_date,''),
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id) AS cnt,
		       a.operation_status
		FROM assets a JOIN customers c ON c.customer_id=a.customer_id
		WHERE a.operation_status NOT IN ('disposed','retired')
		GROUP BY a.asset_id
		HAVING cnt >= 3
		ORDER BY cnt DESC
		LIMIT ? OFFSET ?`, analysisPageSize, (freqPage-1)*analysisPageSize)

	other, _ := h.queryAnalysis(`
		SELECT a.asset_id, c.org_name, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.manufacturer,''), COALESCE(a.installer_type,''),
		       COALESCE(a.install_date,''),
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id),
		       a.operation_status
		FROM assets a JOIN customers c ON c.customer_id=a.customer_id
		WHERE a.installer_type IN ('other','manufacturer','partner','unknown')
		  AND a.operation_status NOT IN ('disposed','retired')
		ORDER BY c.org_name, a.product_name
		LIMIT ? OFFSET ?`, analysisPageSize, (otherPage-1)*analysisPageSize)

	var totalAssets int
	h.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE operation_status NOT IN ('disposed','retired')`).Scan(&totalAssets)

	return c.Render(http.StatusOK, "analysis/dashboard.html", map[string]interface{}{
		"Title": "교체대상 분석 / 영업활용", "Active": "analysis",
		"Aging": aging, "Frequent": frequent, "Other": other,
		"TotalAssets": totalAssets,
		"AgingCount":  agingTotal, "FreqCount": freqTotal, "OtherCount": otherTotal,
		"PageSize":      analysisPageSize,
		"AgingPage":     agingPage,
		"AgingPages":    totalPages(agingTotal, analysisPageSize),
		"FreqPage":      freqPage,
		"FreqPages":     totalPages(freqTotal, analysisPageSize),
		"OtherPage":     otherPage,
		"OtherPages":    totalPages(otherTotal, analysisPageSize),
	})
}

func (h *AnalysisHandler) queryAnalysis(query string, args ...interface{}) ([]analysisRow, error) {
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []analysisRow
	for rows.Next() {
		var r analysisRow
		if err := rows.Scan(&r.AssetID, &r.CustomerName, &r.ProductName, &r.ProductType,
			&r.Manufacturer, &r.InstallerType, &r.InstallDate,
			&r.InstallYears, &r.ASCount, &r.OpStatus); err != nil {
			continue
		}
		items = append(items, r)
	}
	return items, nil
}
