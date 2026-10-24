package repository

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	err = r.store().QueryRow(`
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
		if err := r.assertProjectExists(pid); err != nil {
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
	_, err = r.store().Exec(`
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
		if err := r.assertProjectExists(pid); err != nil {
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
	res, err := r.store().Exec(`
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
	res, err := r.store().Exec(`UPDATE weekly_report_rows SET is_active=? WHERE row_key=? AND org_id=?`, v, strings.TrimSpace(key), orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("행이 없거나 다른 조직입니다")
	}
	return nil
}

func (r *StatsRepo) assertProjectExists(projectID string) error {
	var n int
	err := r.store().QueryRow(`SELECT COUNT(*) FROM work_projects WHERE project_id=?`, projectID).Scan(&n)
	if err != nil || n == 0 {
		return fmt.Errorf("사업을 찾을 수 없습니다")
	}
	return nil
}

func (r *StatsRepo) ListWeeklyRowProjects(orgID string) ([]model.WorkProject, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.store().Query(`
		SELECT project_id, name, COALESCE(org_id,''), COALESCE(start_date,''), COALESCE(end_date,'')
		  FROM work_projects
		 WHERE COALESCE(status,'') NOT IN ('cancelled','archived')
		   AND COALESCE(is_test,0)=0
		 ORDER BY CASE WHEN org_id=? THEN 0 ELSE 1 END, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkProject
	for rows.Next() {
		var p model.WorkProject
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.OrgID, &p.StartDate, &p.EndDate); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type WeeklyRowFromProjectResult struct {
	ProjectID string
	Name      string
	Key       string
	Action    string // inserted | activated | exists
	Err       string
}

func nextWeeklyNoAndSort(rows []weeklyReportRow) (no string, sort int) {
	maxN, hasN := 0, false
	maxS := -1
	for _, row := range rows {
		if row.SortOrder > maxS {
			maxS = row.SortOrder
		}
		n, err := strconv.Atoi(strings.TrimSpace(row.NoLabel))
		if err == nil {
			hasN = true
			if n > maxN {
				maxN = n
			}
		}
	}
	if hasN {
		no = strconv.Itoa(maxN + 1)
	}
	return no, maxS + 1
}

func findWeeklyRowByProject(rows []weeklyReportRow, projectID string) *weeklyReportRow {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	for i := range rows {
		if strings.TrimSpace(rows[i].ProjectID) == projectID {
			return &rows[i]
		}
	}
	return nil
}

func (r *StatsRepo) weeklyRowProjectByID(orgID, projectID string) (model.WorkProject, bool, error) {
	list, err := r.ListWeeklyRowProjects(orgID)
	if err != nil {
		return model.WorkProject{}, false, err
	}
	projectID = strings.TrimSpace(projectID)
	for _, p := range list {
		if p.ProjectID == projectID {
			return p, true, nil
		}
	}
	return model.WorkProject{}, false, nil
}

func (r *StatsRepo) AddWeeklyReportRowsFromProjects(orgID string, projectIDs []string, confirmForeign bool) ([]WeeklyRowFromProjectResult, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, raw := range projectIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("사업을 선택하세요")
	}
	for _, id := range ids {
		if _, ok, err := r.weeklyRowProjectByID(orgID, id); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("목록에 없는 사업입니다")
		}
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	inner := &StatsRepo{db: r.db, st: tx}

	var out []WeeklyRowFromProjectResult
	for _, id := range ids {
		res := WeeklyRowFromProjectResult{ProjectID: id}
		p, _, err := inner.weeklyRowProjectByID(orgID, id)
		if err != nil {
			return nil, err
		}
		res.Name = p.Name
		if p.OrgID != "" && p.OrgID != orgID && !confirmForeign {
			res.Err = "타 조직 사업 확인이 필요합니다"
			out = append(out, res)
			continue
		}
		all, err := inner.ListWeeklyReportRowsOrg(orgID, false)
		if err != nil {
			return nil, err
		}
		if existing := findWeeklyRowByProject(all, id); existing != nil {
			res.Key = existing.Key
			if existing.Active {
				res.Action = "exists"
			} else {
				if err := inner.SetWeeklyReportRowActive(orgID, existing.Key, true); err != nil {
					res.Err = err.Error()
					out = append(out, res)
					continue
				}
				res.Action = "activated"
			}
			out = append(out, res)
			continue
		}
		no, sortN := nextWeeklyNoAndSort(all)
		key, err := inner.InsertWeeklyReportRow(WeeklyReportRowInput{
			OrgID: orgID, DisplayName: p.Name, Kind: "project", ProjectID: id,
			NoLabel: no, SortOrder: sortN, Active: true,
		})
		if err != nil {
			res.Err = err.Error()
			out = append(out, res)
			continue
		}
		res.Key, res.Action = key, "inserted"
		out = append(out, res)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return out, nil
}

func (r *StatsRepo) ListUnmappedWeeklyProjects(orgID string, weeks int, now time.Time) ([]model.CompanyWeeklyUnassignedProject, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	if weeks <= 0 {
		weeks = 8
	}
	mon := WeekInfoOf(now).Monday
	order := make([]string, 0)
	by := map[string]model.CompanyWeeklyUnassignedProject{}
	for i := 0; i < weeks; i++ {
		week := mon.AddDate(0, 0, -7*i)
		draft, err := r.BuildCompanyWeeklyDraftOrg(week.AddDate(0, 0, 7), orgID)
		if err != nil {
			return nil, err
		}
		for _, g := range draft.UnassignedNoRow {
			cur, ok := by[g.ProjectID]
			if !ok {
				order = append(order, g.ProjectID)
				by[g.ProjectID] = g
				continue
			}
			cur.Count += g.Count
			cur.Items = append(cur.Items, g.Items...)
			cur.KindSummary = unassignedKindSummary(cur.Items)
			by[g.ProjectID] = cur
		}
	}
	out := make([]model.CompanyWeeklyUnassignedProject, 0, len(order))
	for _, id := range order {
		out = append(out, by[id])
	}
	return out, nil
}

func (r *StatsRepo) HasWeeklyReportRowForProject(orgID, projectID string) (bool, error) {
	rows, err := r.ListWeeklyReportRowsOrg(orgID, false)
	if err != nil {
		return false, err
	}
	return findWeeklyRowByProject(rows, projectID) != nil, nil
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
