package repository

import (
	"fmt"
	"sort"
	"strings"

	"customer-support/internal/model"
)

// ListMonthBanners 해당 월 캘린더 맨 위 띠 (§39.7). 날짜 칸용 항목이 아니다.
func (r *SalesRepo) ListMonthBanners(year, month int) ([]model.SalesMonthBanner, error) {
	if r == nil || r.db == nil || year < 1 || month < 1 || month > 12 {
		return nil, nil
	}
	target := fmt.Sprintf("%04d-%02d", year, month)
	var out []model.SalesMonthBanner

	projects, err := r.List("", model.SalesStatusActive, "")
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		b, ok := model.SalesMonthBannerFromProject(p)
		if !ok || b.PlaceYM != target {
			continue
		}
		out = append(out, b)
	}

	rows, err := r.db.Query(`
		SELECT a.sales_id, COALESCE(s.name,''), COALESCE(a.next_action,''), COALESCE(a.next_action_date,'')
		FROM sales_activities a
		JOIN sales_projects s ON s.sales_id = a.sales_id
		WHERE COALESCE(s.status,'active') = 'active'
		  AND TRIM(COALESCE(a.next_action_date,'')) != ''`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var salesID, salesName, nextAction, nextDate string
		if err := rows.Scan(&salesID, &salesName, &nextAction, &nextDate); err != nil {
			return nil, err
		}
		b, ok := model.SalesMonthBannerFromNextAction(salesID, salesName, nextAction, nextDate)
		if !ok || b.PlaceYM != target {
			continue
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].SalesID < out[j].SalesID
	})
	return out, nil
}
