package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-197: 업무에 직접 붙인 활동 분류(work_tasks.activity_kind)를 주간보고 활동 블록으로 모은다.

// virtualActivityRowKey 보고서 행이 따로 없는 활동 분류에 임시로 붙이는 행 키.
func virtualActivityRowKey(kind string) string { return "ACT-" + kind }

// listTaggedActivityTaskLines 활동 분류가 kind 인 업무 줄. done=true 는 전주 완료, false 는 금주 계획·이월.
func (r *StatsRepo) listTaggedActivityTaskLines(kind string, p model.CompanyWeeklyPeriod, orgID string, done bool) ([]keyedLine, error) {
	if !model.IsWeeklyActivityKind(kind) {
		return nil, nil
	}
	orgSQL, orgArgs, err := AppendOrgSQL("t", orgID)
	if err != nil {
		return nil, err
	}
	var q string
	var args []interface{}
	if done {
		dateSQL := `COALESCE(NULLIF(TRIM(t.complete_date),''), NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))`
		q = `SELECT t.task_id, t.title FROM work_tasks t
			WHERE t.activity_kind=? AND t.status='complete'
			  AND TRIM(COALESCE(` + dateSQL + `,'')) != ''
			  AND date(` + dateSQL + `) >= date(?) AND date(` + dateSQL + `) < date(?)` + orgSQL + `
			ORDER BY ` + dateSQL + `, t.title`
		args = append([]interface{}{kind, p.PrevFrom, p.PrevToEx}, orgArgs...)
	} else {
		dateSQL := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))`
		q = `SELECT t.task_id, t.title FROM work_tasks t
			WHERE t.activity_kind=? AND t.status NOT IN ('complete','cancelled')
			  AND (TRIM(COALESCE(` + dateSQL + `,'')) = '' OR date(` + dateSQL + `) < date(?))` + orgSQL + `
			ORDER BY ` + dateSQL + `, t.title`
		args = append([]interface{}{kind, p.ThisToEx}, orgArgs...)
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []keyedLine
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out = append(out, keyedLine{Key: lineKey("task", id), Text: title})
	}
	return out, rows.Err()
}

// appendTaggedLines 기존 줄에 분류 업무 줄을 중복 없이 이어 붙인다.
func appendTaggedLines(base, tagged []keyedLine) []keyedLine {
	seen := map[string]bool{}
	for _, l := range base {
		seen[l.Key] = true
	}
	for _, l := range tagged {
		if seen[l.Key] {
			continue
		}
		seen[l.Key] = true
		base = append(base, l)
	}
	return base
}

// virtualActivityRows 보고서 행이 없는 활동 분류 중, 이 주에 분류된 업무가 있는 것만 가상 행으로 돌려준다.
func (r *StatsRepo) virtualActivityRows(rows []weeklyReportRow, p model.CompanyWeeklyPeriod, orgID string) ([]weeklyReportRow, error) {
	have := map[string]bool{}
	for _, row := range rows {
		if row.Kind == "activity" {
			have[strings.TrimSpace(row.ActivityKind)] = true
		}
	}
	var out []weeklyReportRow
	for i, k := range model.WeeklyActivityKinds {
		if have[k.Key] {
			continue
		}
		prev, err := r.listTaggedActivityTaskLines(k.Key, p, orgID, true)
		if err != nil {
			return nil, err
		}
		plan := prev
		if len(plan) == 0 {
			plan, err = r.listTaggedActivityTaskLines(k.Key, p, orgID, false)
			if err != nil {
				return nil, err
			}
		}
		if len(plan) == 0 {
			continue
		}
		out = append(out, weeklyReportRow{
			Key: virtualActivityRowKey(k.Key), DisplayName: k.Label + "활동", Kind: "activity",
			ActivityKind: k.Key, OrgID: orgID, SortOrder: 10000 + i, Active: true,
		})
	}
	return out, nil
}

// SetTaskActivity 업무의 활동 분류를 바꾼다. kind 가 비어 있으면 분류를 해제한다.
// 사업과 활동은 함께 가질 수 없으므로 분류를 지정하면 사업 연결은 해제한다.
func (r *WBRepo) SetTaskActivity(orgID, taskID, kind string) (beforeLabel, afterLabel string, err error) {
	orgID, err = RequireInsertOrg(orgID)
	if err != nil {
		return "", "", err
	}
	taskID = strings.TrimSpace(taskID)
	kind = strings.TrimSpace(kind)
	if taskID == "" {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	if kind != "" && !model.IsWeeklyActivityKind(kind) {
		return "", "", fmt.Errorf("알 수 없는 활동 분류입니다")
	}
	var beforeKind, beforePID string
	if err := r.db.QueryRow(`SELECT COALESCE(activity_kind,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id=? AND org_id=?`,
		taskID, orgID).Scan(&beforeKind, &beforePID); err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("업무가 없습니다")
		}
		return "", "", err
	}
	beforeLabel = model.WeeklyActivityLabel(beforeKind)
	if beforeLabel == "" {
		beforeLabel = r.WorkProjectName(beforePID)
	}
	afterLabel = model.WeeklyActivityLabel(kind)
	if kind != "" {
		_, err = r.db.Exec(`UPDATE work_tasks SET activity_kind=?, project_id=NULL, updated_at=datetime('now','localtime') WHERE task_id=? AND org_id=?`,
			kind, taskID, orgID)
	} else {
		_, err = r.db.Exec(`UPDATE work_tasks SET activity_kind=NULL, updated_at=datetime('now','localtime') WHERE task_id=? AND org_id=?`,
			taskID, orgID)
	}
	if err != nil {
		return "", "", err
	}
	if kind != "" && beforePID != "" {
		sites, e := r.ListTaskSites(taskID)
		if e == nil {
			var ids []string
			for _, s := range sites {
				if id := strings.TrimSpace(s.SiteID); id != "" {
					ids = append(ids, id)
				}
			}
			_ = r.ReplaceTaskCustomers(taskID, "", ids)
		}
	}
	label := "연결: "
	if beforeLabel == "" {
		label += "없음"
	} else {
		label += beforeLabel
	}
	label += " → "
	if afterLabel == "" {
		label += "없음"
	} else {
		label += afterLabel + "(활동)"
	}
	logUpdate(r.db, "work_tasks", "task_id", taskID, label, beforeKind)
	return beforeLabel, afterLabel, nil
}
