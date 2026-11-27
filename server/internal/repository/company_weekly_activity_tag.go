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
	for i, k := range model.WeeklyActivityKindList() {
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
	if kind != "" && kind != beforeKind && !model.IsWeeklyActivitySelectable(kind) {
		return "", "", fmt.Errorf("사용하지 않는 업무 구분입니다")
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
	logUpdateWithReason(r.db, "work_tasks", "task_id", taskID, label, beforeKind, "업무 구분(category_code) 변경: "+beforeKind+" → "+kind)
	return beforeLabel, afterLabel, nil
}

// TaskActivityKind 업무의 활동 분류 키(없으면 빈 문자열).
func (r *WBRepo) TaskActivityKind(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return ""
	}
	var k string
	_ = r.db.QueryRow(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&k)
	return strings.TrimSpace(k)
}

// listActivityProgressLines 95-212: 활동 행의 「진행 중」 — 일정(작업일)이 실적 주 안에 있으나 끝나지 않은 업무.
// 분류 태그 업무 + (기술영업) 영업 업무 + (자산) 설치·회수 업무. 같은 업무는 한 줄(Key prog:ID).
func (r *StatsRepo) listActivityProgressLines(kind string, p model.CompanyWeeklyPeriod, orgID, exclProjects string) ([]keyedLine, error) {
	orgSQL, orgArgs, err := AppendOrgSQL("t", orgID)
	if err != nil {
		return nil, err
	}
	dateSQL := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))`
	base := `SELECT t.task_id, COALESCE(NULLIF(TRIM(p.name),''), ''), t.title
		FROM work_tasks t LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(` + dateSQL + `,'')) != ''
		  AND date(` + dateSQL + `) >= date(?) AND date(` + dateSQL + `) < date(?)`
	type q struct {
		cond string
		args []interface{}
		biz  bool
	}
	var qs []q
	if model.IsWeeklyActivityKind(kind) {
		qs = append(qs, q{cond: ` AND t.activity_kind=?`, args: []interface{}{kind}})
	}
	switch kind {
	case "tech":
		qs = append(qs, q{cond: ` AND (t.work_type='sales' OR COALESCE(t.source_type,'')='sales_activity')`, biz: true})
	case "asset":
		qs = append(qs, q{cond: ` AND TRIM(COALESCE(t.project_id,'')) NOT IN (` + exclProjects + `) AND (t.title LIKE '%설치%' OR t.title LIKE '%회수%' OR t.title LIKE '%이동%'
			OR EXISTS (SELECT 1 FROM work_task_assets wta WHERE wta.task_id=t.task_id))`})
	}
	seen := map[string]bool{}
	var out []keyedLine
	for _, x := range qs {
		args := append([]interface{}{p.PrevFrom, p.PrevToEx}, x.args...)
		rows, err := r.db.Query(base+x.cond+orgSQL+` ORDER BY `+dateSQL+`, t.title`, append(args, orgArgs...)...)
		if err != nil {
			if strings.Contains(err.Error(), "no such column") {
				continue
			}
			return nil, err
		}
		for rows.Next() {
			var id, pname, title string
			if err := rows.Scan(&id, &pname, &title); err != nil {
				rows.Close()
				return nil, err
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			text := strings.TrimSpace(title)
			if x.biz {
				text = prefixBiz(pname, title)
			}
			out = append(out, keyedLine{Key: lineKey("prog", id), Text: text + "(진행 중)"})
		}
		rows.Close()
	}
	return out, nil
}

// TaskActivityKinds 업무 ID 목록의 업무 구분 키를 한 번에 읽는다(없으면 맵에 없음).
func (r *WBRepo) TaskActivityKinds(ids []string) map[string]string {
	out := map[string]string{}
	if r == nil || r.db == nil || len(ids) == 0 {
		return out
	}
	for start := 0; start < len(ids); start += 500 {
		end := start + 500
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		args := make([]interface{}, 0, len(part))
		for _, id := range part {
			args = append(args, id)
		}
		rows, err := r.db.Query(`SELECT task_id, COALESCE(activity_kind,'') FROM work_tasks WHERE task_id IN (`+placeholders(len(part))+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id, k string
			if rows.Scan(&id, &k) == nil && strings.TrimSpace(k) != "" {
				out[id] = strings.TrimSpace(k)
			}
		}
		rows.Close()
	}
	return out
}
