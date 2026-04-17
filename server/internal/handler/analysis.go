package handler

import (
	"database/sql"
	"net/http"

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

func (h *AnalysisHandler) Dashboard(c echo.Context) error {
	// 노후장비 (5년 이상)
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
		ORDER BY (julianday('now')-julianday(a.install_date)) DESC LIMIT 50`)

	// 장애다발장비 (AS 3건 이상)
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
		ORDER BY cnt DESC LIMIT 50`)

	// 타사설치 자산
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
		ORDER BY c.org_name LIMIT 50`)

	// 통계
	var totalAssets, agingCount, otherCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE operation_status NOT IN ('disposed','retired')`).Scan(&totalAssets)
	h.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE install_date!='' AND operation_status NOT IN ('disposed','retired') AND (julianday('now')-julianday(install_date))/365>=5`).Scan(&agingCount)
	h.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE installer_type IN ('other','manufacturer','partner','unknown') AND operation_status NOT IN ('disposed','retired')`).Scan(&otherCount)

	return c.Render(http.StatusOK, "analysis/dashboard.html", map[string]interface{}{
		"Title": "교체대상 분석 / 영업활용", "Active": "analysis",
		"Aging": aging, "Frequent": frequent, "Other": other,
		"TotalAssets": totalAssets, "AgingCount": agingCount, "OtherCount": otherCount,
	})
}

func (h *AnalysisHandler) queryAnalysis(query string) ([]analysisRow, error) {
	rows, err := h.db.Query(query)
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
