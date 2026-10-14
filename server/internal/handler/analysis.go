package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type AnalysisHandler struct{ db *sql.DB }

const assetAgeYears = `(julianday('now')-julianday(install_date))/365`

type ReplacementSummary struct {
	AgingCount   int
	Aging7Count  int
	Aging10Count int
	FreqCount    int
	OtherCount   int
}

func (h *AnalysisHandler) countAgingBetween(minYears, maxYearsExclusive int) int {
	n := 0
	q := `
		SELECT COUNT(*) FROM assets
		WHERE install_date!='' AND operation_status NOT IN ('disposed','retired')
		  AND ` + assetAgeYears + ` >= ?`
	args := []interface{}{minYears}
	if maxYearsExclusive > 0 {
		q += ` AND ` + assetAgeYears + ` < ?`
		args = append(args, maxYearsExclusive)
	}
	h.db.QueryRow(q, args...).Scan(&n)
	return n
}

func (h *AnalysisHandler) ReplacementSummary() ReplacementSummary {
	var s ReplacementSummary
	s.Aging7Count = h.countAgingBetween(7, 10)
	s.Aging10Count = h.countAgingBetween(10, 0)
	s.AgingCount = s.Aging7Count + s.Aging10Count
	h.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT a.asset_id FROM assets a
			WHERE a.operation_status NOT IN ('disposed','retired')
			GROUP BY a.asset_id
			HAVING (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id) >= 3
		)`).Scan(&s.FreqCount)
	h.db.QueryRow(`
		SELECT COUNT(*) FROM assets
		WHERE installer_type IN ('other','manufacturer','partner','unknown')
		  AND operation_status NOT IN ('disposed','retired')`).Scan(&s.OtherCount)
	return s
}

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
	aging7Page := analysisPage(c, "aging7_page")
	aging10Page := analysisPage(c, "aging10_page")
	freqPage := analysisPage(c, "freq_page")
	otherPage := analysisPage(c, "other_page")

	aging7Total := h.countAgingBetween(7, 10)
	aging10Total := h.countAgingBetween(10, 0)

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

	agingSelect := `
		SELECT a.asset_id, c.org_name, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.manufacturer,''), COALESCE(a.installer_type,''),
		       COALESCE(a.install_date,''),
		       CASE WHEN a.install_date!='' THEN CAST((julianday('now')-julianday(a.install_date))/365 AS INTEGER) ELSE 0 END,
		       (SELECT COUNT(*) FROM as_receipts ar WHERE ar.asset_id=a.asset_id),
		       a.operation_status
		FROM assets a JOIN customers c ON c.customer_id=a.customer_id
		WHERE a.install_date!='' AND a.operation_status NOT IN ('disposed','retired')`
	ageA := `(julianday('now')-julianday(a.install_date))/365`
	aging7, _ := h.queryAnalysis(agingSelect+`
		  AND `+ageA+` >= 7 AND `+ageA+` < 10
		ORDER BY (julianday('now')-julianday(a.install_date)) DESC
		LIMIT ? OFFSET ?`, analysisPageSize, (aging7Page-1)*analysisPageSize)
	aging10, _ := h.queryAnalysis(agingSelect+`
		  AND `+ageA+` >= 10
		ORDER BY (julianday('now')-julianday(a.install_date)) DESC
		LIMIT ? OFFSET ?`, analysisPageSize, (aging10Page-1)*analysisPageSize)

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
		"Title": "교체대상 분석 / 영업활용", "Active": NavAnalysis,
		"Aging7": aging7, "Aging10": aging10, "Frequent": frequent, "Other": other,
		"TotalAssets": totalAssets,
		"Aging7Count": aging7Total, "Aging10Count": aging10Total,
		"AgingCount": aging7Total + aging10Total,
		"FreqCount":  freqTotal, "OtherCount": otherTotal,
		"PageSize":     analysisPageSize,
		"Aging7Page":   aging7Page,
		"Aging7Pages":  totalPages(aging7Total, analysisPageSize),
		"Aging10Page":  aging10Page,
		"Aging10Pages": totalPages(aging10Total, analysisPageSize),
		"FreqPage":     freqPage,
		"FreqPages":    totalPages(freqTotal, analysisPageSize),
		"OtherPage":    otherPage,
		"OtherPages":   totalPages(otherTotal, analysisPageSize),
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
