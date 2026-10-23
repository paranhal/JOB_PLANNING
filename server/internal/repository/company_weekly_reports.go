package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

const (
	CompanyWeeklyStatusDraft     = "draft"
	CompanyWeeklyStatusConfirmed = "confirmed"

	CompanyWeeklyUpsertSave      = "save"
	CompanyWeeklyUpsertConfirm   = "confirm"
	CompanyWeeklyUpsertUnconfirm = "unconfirm"
)

type CompanyWeeklyReport struct {
	ReportID    string
	OrgID       string
	WeekAnchor  string
	SheetName   string
	Status      string
	ContentJSON string
	ConfirmedBy string
	ConfirmedAt string
	UpdatedBy   string
	UpdatedAt   string
}

type CompanyWeeklySavedRow struct {
	Prev     string `json:"prev"`
	Plan     string `json:"plan"`
	Help     string `json:"help"`
	Decision string `json:"decision"`
}

func ParseCompanyWeeklyContent(raw string) map[string]CompanyWeeklySavedRow {
	out := map[string]CompanyWeeklySavedRow{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func EncodeCompanyWeeklyContent(rows []model.CompanyWeeklyRow) string {
	m := map[string]CompanyWeeklySavedRow{}
	for _, r := range rows {
		k := strings.TrimSpace(r.RowKey)
		if k == "" {
			continue
		}
		m[k] = CompanyWeeklySavedRow{
			Prev: r.PrevText, Plan: r.PlanText, Help: r.HelpText, Decision: r.DecisionText,
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func ApplyCompanyWeeklySaved(rows []model.CompanyWeeklyRow, saved map[string]CompanyWeeklySavedRow) []model.CompanyWeeklyRow {
	if len(saved) == 0 {
		return rows
	}
	out := make([]model.CompanyWeeklyRow, len(rows))
	copy(out, rows)
	for i := range out {
		s, ok := saved[out[i].RowKey]
		if !ok {
			continue
		}
		out[i].PrevText = s.Prev
		out[i].PlanText = s.Plan
		out[i].HelpText = s.Help
		out[i].DecisionText = s.Decision
	}
	return out
}

func (r *StatsRepo) GetCompanyWeeklyReport(orgID, weekAnchor string) (CompanyWeeklyReport, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return CompanyWeeklyReport{}, err
	}
	var row CompanyWeeklyReport
	err = r.db.QueryRow(`
		SELECT report_id, org_id, week_anchor, COALESCE(sheet_name,''), COALESCE(status,'draft'),
		       COALESCE(content_json,''), COALESCE(confirmed_by,''), COALESCE(confirmed_at,''),
		       COALESCE(updated_by,''), COALESCE(updated_at,'')
		FROM company_weekly_reports WHERE org_id=? AND week_anchor=?`, orgID, strings.TrimSpace(weekAnchor)).
		Scan(&row.ReportID, &row.OrgID, &row.WeekAnchor, &row.SheetName, &row.Status,
			&row.ContentJSON, &row.ConfirmedBy, &row.ConfirmedAt, &row.UpdatedBy, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return CompanyWeeklyReport{}, nil
	}
	return row, err
}

func (r *StatsRepo) UpsertCompanyWeeklyReport(orgID, weekAnchor, sheetName, content, by, mode string) error {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return err
	}
	weekAnchor = strings.TrimSpace(weekAnchor)
	if weekAnchor == "" {
		return fmt.Errorf("보고 주차를 선택하세요")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	existing, err := r.GetCompanyWeeklyReport(orgID, weekAnchor)
	if err != nil {
		return err
	}
	status := CompanyWeeklyStatusDraft
	confBy, confAt := "", ""
	switch mode {
	case CompanyWeeklyUpsertConfirm:
		status = CompanyWeeklyStatusConfirmed
		confBy, confAt = by, now
	case CompanyWeeklyUpsertUnconfirm:
		status = CompanyWeeklyStatusDraft
	default:
		if existing.Status == CompanyWeeklyStatusConfirmed && content != existing.ContentJSON {
			status = CompanyWeeklyStatusDraft
		} else if existing.ReportID != "" {
			status = existing.Status
			confBy, confAt = existing.ConfirmedBy, existing.ConfirmedAt
		}
	}
	if existing.ReportID == "" {
		_, err = r.db.Exec(`
			INSERT INTO company_weekly_reports
			(report_id, org_id, week_anchor, sheet_name, status, content_json, confirmed_by, confirmed_at, updated_by, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			uuid.NewString(), orgID, weekAnchor, sheetName, status, content, confBy, confAt, by, now)
		return err
	}
	_, err = r.db.Exec(`
		UPDATE company_weekly_reports
		   SET sheet_name=?, status=?, content_json=?, confirmed_by=?, confirmed_at=?, updated_by=?, updated_at=?
		 WHERE report_id=? AND org_id=?`,
		sheetName, status, content, confBy, confAt, by, now, existing.ReportID, orgID)
	return err
}
