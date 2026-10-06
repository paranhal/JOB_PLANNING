package repository

import (
	"fmt"
	"strings"
	"time"
)

// CountCustomerVisitsInMonth 그 기관의 해당 연월 정기점검 방문 수. §68.7.1
func (r *MaintenanceRepo) CountCustomerVisitsInMonth(customerID, yearMonth string) (int, string, error) {
	customerID = strings.TrimSpace(customerID)
	yearMonth = strings.TrimSpace(yearMonth)
	if customerID == "" || len(yearMonth) < 7 {
		return 0, "", nil
	}
	t, err := time.Parse("2006-01", yearMonth[:7])
	if err != nil {
		return 0, "", nil
	}
	from := t.Format("2006-01-02")
	toEx := t.AddDate(0, 1, 0).Format("2006-01-02")
	var n int
	err = r.db.QueryRow(`
		SELECT COUNT(*) FROM maintenance_visits
		WHERE customer_id=? AND visit_date >= ? AND visit_date < ?`, customerID, from, toEx).Scan(&n)
	if err != nil {
		return 0, "", err
	}
	label := fmt.Sprintf("%d월", int(t.Month()))
	return n, label, nil
}
