package repository

import (
	"database/sql"
	"log"
	"strings"
)

const weeklyReportRowsMetaKey = "__meta:weekly_report_rows_v1"

const weeklyReportRowsSchema = `
CREATE TABLE IF NOT EXISTS weekly_report_rows (
  row_key      TEXT PRIMARY KEY,
  sheet_row    INTEGER NOT NULL,
  division     TEXT NOT NULL DEFAULT '',
  team         TEXT NOT NULL DEFAULT '',
  no_label     TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  project_id   TEXT,
  row_kind     TEXT NOT NULL DEFAULT 'project',
  highlight    INTEGER NOT NULL DEFAULT 0,
  is_active    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_project ON weekly_report_rows(project_id);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_sheet ON weekly_report_rows(sheet_row);
`

// applyWeeklyReportRows 마이그레이션 006. 테이블 + §16.6.5 초기 6행.
func applyWeeklyReportRows(db *sql.DB) {
	if _, err := db.Exec(weeklyReportRowsSchema); err != nil {
		log.Printf("006 weekly_report_rows schema: %v", err)
		return
	}
	seedWeeklyReportRows(db)
}

func seedWeeklyReportRows(db *sql.DB) {
	if metaDone(db, weeklyReportRowsMetaKey) {
		return
	}
	type seed struct {
		key, division, team, no, name, project, kind string
		sheet, highlight                             int
	}
	rows := []seed{
		{"401", "경영전략기획", "도서관사업부", "401",
			"2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리", "WPSEED01", "project", 20, 0},
		{"402", "", "도서관사업부", "402",
			"2026년 제천시립도서관 홈페이지 유지보수 용역", "WPSEED04", "project", 21, 0},
		{"403", "", "도서관사업부", "403",
			"세종시 도서관 ICT 통합정보시스템 유지관리(2026년)", "WPSEED02", "project", 22, 0},
		{"404", "", "도서관사업부", "404",
			"2026 충남교육청 통합전자도서관 AI기반 구독형 독서플랫폼(전자책/오디오북 및 멀티미디어) 서비스 용역",
			"", "project", 23, 0},
		{"X01", "", "도서관사업부", "",
			"충남세종지역 앤로보틱스 RFID자동화 장비 유지보수 사업", "WPSEED03", "project", 24, 0},
		{"X02", "", "도서관사업부", "",
			"##자산관리활동", "", "manual", 25, 1},
	}
	for _, s := range rows {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO weekly_report_rows
			(row_key, sheet_row, division, team, no_label, display_name, project_id, row_kind, highlight, is_active)
			VALUES (?,?,?,?,?,?,?,?,?,1)`,
			s.key, s.sheet, s.division, s.team, s.no, s.name, nullStr(s.project), s.kind, s.highlight); err != nil {
			log.Printf("006 weekly_report_rows seed %s: %v", s.key, err)
			return
		}
	}
	markMetaDone(db, weeklyReportRowsMetaKey)
}

func (r *StatsRepo) listWeeklyReportRows() ([]weeklyReportRow, error) {
	rows, err := r.db.Query(`
		SELECT row_key, sheet_row, COALESCE(division,''), COALESCE(team,''), COALESCE(no_label,''),
		       COALESCE(display_name,''), COALESCE(project_id,''), COALESCE(row_kind,'project'),
		       COALESCE(highlight,0), COALESCE(is_active,1)
		FROM weekly_report_rows
		WHERE COALESCE(is_active,1)=1
		ORDER BY sheet_row, row_key`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []weeklyReportRow
	for rows.Next() {
		var row weeklyReportRow
		var hi, active int
		if err := rows.Scan(&row.Key, &row.SheetRow, &row.Division, &row.Team, &row.NoLabel,
			&row.DisplayName, &row.ProjectID, &row.Kind, &hi, &active); err != nil {
			return nil, err
		}
		row.Highlight = hi != 0
		row.Active = active != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

type weeklyReportRow struct {
	Key, Division, Team, NoLabel, DisplayName, ProjectID, Kind string
	SheetRow                                                   int
	Highlight, Active                                          bool
}
