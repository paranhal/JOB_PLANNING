package repository

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

type WeeklyReportRowInput struct {
	Key          string
	OrgID        string
	Division     string
	Team         string
	NoLabel      string
	DisplayName  string
	ProjectID    string
	Kind         string
	ActivityKind string
	SortOrder    int
	Highlight    bool
	Active       bool
}

func normalizeWeeklyRowKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "activity":
		return "activity"
	case "manual":
		return "manual"
	default:
		return "project"
	}
}

func normalizeActivityKind(kind, rowKind string) string {
	if rowKind != "activity" {
		return ""
	}
	switch strings.TrimSpace(kind) {
	case "asset", "tech", "mnt_as":
		return kind
	default:
		return ""
	}
}

func (r *StatsRepo) GetWeeklyReportRow(orgID, key string) (weeklyReportRow, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return weeklyReportRow{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return weeklyReportRow{}, fmt.Errorf("행이 없습니다")
	}
	var row weeklyReportRow
	var hi, active int
	err = r.db.QueryRow(`
		SELECT row_key, sheet_row, COALESCE(division,''), COALESCE(team,''), COALESCE(no_label,''),
		       COALESCE(display_name,''), COALESCE(project_id,''), COALESCE(row_kind,'project'),
		       COALESCE(highlight,0), COALESCE(is_active,1), COALESCE(org_id,''),
		       COALESCE(activity_kind,''), COALESCE(sort_order,0)
		FROM weekly_report_rows WHERE row_key=? AND org_id=?`, key, orgID).
		Scan(&row.Key, &row.SheetRow, &row.Division, &row.Team, &row.NoLabel,
			&row.DisplayName, &row.ProjectID, &row.Kind, &hi, &active, &row.OrgID, &row.ActivityKind, &row.SortOrder)
	if err != nil {
		return weeklyReportRow{}, err
	}
	row.Highlight = hi != 0
	row.Active = active != 0
	return row, nil
}

func (r *StatsRepo) InsertWeeklyReportRow(in WeeklyReportRowInput) (string, error) {
	orgID, err := RequireInsertOrg(in.OrgID)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return "", fmt.Errorf("표시명을 입력하세요")
	}
	kind := normalizeWeeklyRowKind(in.Kind)
	act := normalizeActivityKind(in.ActivityKind, kind)
	pid := strings.TrimSpace(in.ProjectID)
	if pid != "" {
		if err := r.assertProjectInOrg(pid, orgID); err != nil {
			return "", err
		}
	} else {
		pid = ""
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = "WR" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	}
	hi := 0
	if in.Highlight {
		hi = 1
	}
	active := 1
	if !in.Active {
		active = 0
	}
	sheet := 20 + in.SortOrder
	if sheet < 20 {
		sheet = 20
	}
	_, err = r.db.Exec(`
		INSERT INTO weekly_report_rows
		(row_key, sheet_row, division, team, no_label, display_name, project_id, row_kind, highlight, is_active, org_id, activity_kind, sort_order)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		key, sheet, strings.TrimSpace(in.Division), strings.TrimSpace(in.Team), strings.TrimSpace(in.NoLabel),
		name, nullStr(pid), kind, hi, active, orgID, act, in.SortOrder)
	if err != nil {
		return "", err
	}
	return key, nil
}

func (r *StatsRepo) UpdateWeeklyReportRow(in WeeklyReportRowInput) error {
	orgID, err := RequireInsertOrg(in.OrgID)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return fmt.Errorf("행이 없습니다")
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return fmt.Errorf("표시명을 입력하세요")
	}
	kind := normalizeWeeklyRowKind(in.Kind)
	act := normalizeActivityKind(in.ActivityKind, kind)
	pid := strings.TrimSpace(in.ProjectID)
	if pid != "" {
		if err := r.assertProjectInOrg(pid, orgID); err != nil {
			return err
		}
	}
	hi := 0
	if in.Highlight {
		hi = 1
	}
	active := 1
	if !in.Active {
		active = 0
	}
	sheet := 20 + in.SortOrder
	if sheet < 20 {
		sheet = 20
	}
	res, err := r.db.Exec(`
		UPDATE weekly_report_rows
		   SET sheet_row=?, division=?, team=?, no_label=?, display_name=?, project_id=?,
		       row_kind=?, highlight=?, is_active=?, activity_kind=?, sort_order=?
		 WHERE row_key=? AND org_id=?`,
		sheet, strings.TrimSpace(in.Division), strings.TrimSpace(in.Team), strings.TrimSpace(in.NoLabel),
		name, nullStr(pid), kind, hi, active, act, in.SortOrder, key, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("행이 없거나 다른 조직입니다")
	}
	return nil
}

func (r *StatsRepo) SetWeeklyReportRowActive(orgID, key string, active bool) error {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return err
	}
	v := 0
	if active {
		v = 1
	}
	res, err := r.db.Exec(`UPDATE weekly_report_rows SET is_active=? WHERE row_key=? AND org_id=?`, v, strings.TrimSpace(key), orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("행이 없거나 다른 조직입니다")
	}
	return nil
}

func (r *StatsRepo) assertProjectInOrg(projectID, orgID string) error {
	var got string
	err := r.db.QueryRow(`SELECT COALESCE(org_id,'') FROM work_projects WHERE project_id=?`, projectID).Scan(&got)
	if err != nil {
		return fmt.Errorf("사업을 찾을 수 없습니다")
	}
	if !VisibleInOrg(got, orgID) {
		return fmt.Errorf("다른 조직의 사업은 연결할 수 없습니다")
	}
	return nil
}

func (r *StatsRepo) ListWeeklyRowProjects(orgID string) ([]model.WorkProject, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT project_id, name FROM work_projects
		 WHERE org_id=? AND COALESCE(status,'') NOT IN ('cancelled','archived')
		 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkProject
	for rows.Next() {
		var p model.WorkProject
		if err := rows.Scan(&p.ProjectID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *StatsRepo) OrgShortAndName(orgID string) (short, name string) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return "", "전체 조직"
	}
	_ = r.db.QueryRow(`SELECT COALESCE(NULLIF(TRIM(short_name),''), org_name), org_name FROM orgs WHERE org_id=?`, orgID).Scan(&short, &name)
	if name == "" {
		if orgID == model.OrgIDLibrary {
			return "도서관", model.OrgNameLibrary
		}
		return orgID, orgID
	}
	if short == "" {
		short = name
	}
	return short, name
}
