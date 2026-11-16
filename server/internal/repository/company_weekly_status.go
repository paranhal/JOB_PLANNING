package repository

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 95-213: 일반 업무의 상태별 「금주 추진내역 / 차주 추진계획」 줄 규칙(줄 생성 한 곳).
//
//	완료            → 금주 제목(M/D) (기존 listCompanyAdminDone, 95-205)
//	진행 중·미완료  → 금주: 실적 주 조치 내용이 있으면 「제목: 조치 요지(진행 중)」, 없고 일정이 실적 주 안이면 「제목(진행 중)」
//	취소            → 금주: 실적 주 조치 내용이 있을 때만 「제목: 조치 요지(취소)」
//	차주            → 업무 계획(차주 하위 일정·세부 조치 항목)이 있으면 계획 줄만, 없으면 이월 줄 1번(취소는 표시 안 함)
//
// 조치 내용 = 일반 업무: work_activities.content(업무 상세 「조치 이력」), 사업 작업: work_tasks.action_taken·action_remark.
// 줄임은 규칙 기반(AI 요약 없음): 공백 정리, 여러 기록은 ·로 이어 한 줄, 최대 weeklyActionMaxRunes 자.
const weeklyActionMaxRunes = 80

// abbrevWeeklyAction 조치 기록들을 한 줄로 줄인다.
func abbrevWeeklyAction(parts []string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.Join(strings.Fields(p), " ")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	s := strings.Join(out, "·")
	if utf8.RuneCountInString(s) <= weeklyActionMaxRunes {
		return s
	}
	rs := []rune(s)
	return strings.TrimRight(string(rs[:weeklyActionMaxRunes]), " ·") + "…"
}

// weeklyStatusText 금주 상태 줄 문구.
func weeklyStatusText(a adminLine) string {
	suffix := "(진행 중)"
	if a.Status == "cancelled" {
		suffix = "(취소)"
	}
	if a.Action != "" {
		return a.Title + ": " + a.Action + suffix
	}
	return a.Title + suffix
}

type weeklyStatusResult struct {
	Prog      []adminLine                   // 금주 「진행 중」·「취소」 줄 후보
	PlanExtra []model.CompanyWeeklyAutoLine // 차주 계획 줄(세부 조치 항목)
	HasPlan   map[string]bool               // 차주 업무 계획이 있는 업무 ID → 이월 줄을 만들지 않는다
}

// listCompanyAdminWeekStatus 완료가 아닌 일반 업무(진행 중·미완료·취소)의 금주 줄·차주 계획을 계산한다.
// 필터는 listCompanyAdminPlan 과 같다(companyWeeklyAdminFilter, SQLRecurrenceWorkUnit, 조직·테스트 제외).
func (r *StatsRepo) listCompanyAdminWeekStatus(projectID, weekFrom, weekToEx, planFrom, planToEx, orgID string) (weeklyStatusResult, error) {
	res := weeklyStatusResult{HasPlan: map[string]bool{}}
	d := companyAdminPlanDateSQL()
	adminSQL, adminArgs := r.filterAdmin(companyWeeklyAdminFilter(orgID, model.BasisVisit))
	rows, err := r.db.Query(`
		SELECT t.task_id, t.title, COALESCE(`+d+`,''), COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,''),
		       COALESCE(t.work_type,''), COALESCE(pt.title,''), COALESCE(t.status,''),
		       COALESCE(t.action_taken,''), COALESCE(t.action_remark,''), COALESCE(t.action_start_at,'')
		FROM work_tasks t
		LEFT JOIN work_tasks pt ON pt.task_id = t.parent_task_id
		WHERE TRIM(COALESCE(t.activity_kind,''))='' AND `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminSQL+`
		  AND t.status != 'complete'
		  AND TRIM(COALESCE(t.project_id,'')) = ?
		ORDER BY `+d+`, t.title`, append(append([]interface{}{}, adminArgs...), projectID)...)
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return res, nil
		}
		return res, err
	}
	type cand struct {
		a            adminLine
		direct       []string // work_tasks 에 직접 적힌 조치(사업 작업)
		directInWeek bool
	}
	var cands []cand
	byID := map[string]int{}
	for rows.Next() {
		var c cand
		var taken, remark, startAt string
		if err := rows.Scan(&c.a.ID, &c.a.Title, &c.a.Date, &c.a.ParentID, &c.a.Role, &c.a.WorkType, &c.a.ParentTitle, &c.a.Status, &taken, &remark, &startAt); err != nil {
			rows.Close()
			return res, err
		}
		c.a.ProjectID = projectID
		if c.a.WorkType == model.WBWorkProjectWork {
			c.a.Title = formatProjectWorkLine(c.a.Title, r.taskSiteNames(c.a.ID), c.a.Date)
			if day := firstISODay(startAt); day != "" && day >= weekFrom && day < weekToEx {
				c.direct = []string{taken, remark}
			}
		}
		byID[c.a.ID] = len(cands)
		cands = append(cands, c)
	}
	rows.Close()
	if len(cands) == 0 {
		return res, nil
	}

	// 실적 주 안에 기록된 조치 이력(일반 업무)
	arows, err := r.db.Query(`
		SELECT a.task_id, COALESCE(a.content,'')
		FROM work_activities a
		JOIN work_tasks t ON t.task_id = a.task_id
		WHERE TRIM(COALESCE(t.project_id,'')) = ?
		  AND date(a.created_at) >= date(?) AND date(a.created_at) < date(?)
		ORDER BY a.created_at, a.activity_id`, projectID, weekFrom, weekToEx)
	if err != nil {
		return res, err
	}
	acts := map[string][]string{}
	for arows.Next() {
		var id, content string
		if err := arows.Scan(&id, &content); err != nil {
			arows.Close()
			return res, err
		}
		if _, ok := byID[id]; ok {
			acts[id] = append(acts[id], content)
		}
	}
	arows.Close()

	// 차주 업무 계획: ① 차주 일정이 있는 하위 일정 ② 차주 일정이 있는 세부 조치 항목(work_actions)
	for _, c := range cands {
		if c.a.ParentID != "" && c.a.Status != "cancelled" && c.a.Date != "" && firstISODay(c.a.Date) >= planFrom && firstISODay(c.a.Date) < planToEx {
			res.HasPlan[c.a.ParentID] = true
		}
	}
	wrows, err := r.db.Query(`
		SELECT w.action_id, w.task_id, COALESCE(w.title,''), COALESCE(NULLIF(TRIM(w.scheduled_date),''), NULLIF(TRIM(w.due_date),''))
		FROM work_actions w
		WHERE w.status NOT IN ('complete','cancelled')
		  AND COALESCE(NULLIF(TRIM(w.scheduled_date),''), NULLIF(TRIM(w.due_date),'')) >= ?
		  AND COALESCE(NULLIF(TRIM(w.scheduled_date),''), NULLIF(TRIM(w.due_date),'')) < ?
		ORDER BY 4, w.sort_order`, planFrom, planToEx)
	if err != nil && !strings.Contains(err.Error(), "no such") {
		return res, err
	}
	if err == nil {
		for wrows.Next() {
			var aid, tid, title, date string
			if err := wrows.Scan(&aid, &tid, &title, &date); err != nil {
				wrows.Close()
				return res, err
			}
			i, ok := byID[tid]
			if !ok || strings.TrimSpace(title) == "" {
				continue
			}
			res.HasPlan[tid] = true
			res.PlanExtra = append(res.PlanExtra, model.CompanyWeeklyAutoLine{
				Key:  lineKey("plan", aid),
				Text: fmt.Sprintf("%s: %s(%s)", cands[i].a.Title, strings.Join(strings.Fields(title), " "), slashMD(date)),
			})
		}
		wrows.Close()
	}

	// 금주 줄
	for _, c := range cands {
		a := c.a
		parts := append(append([]string{}, acts[a.ID]...), c.direct...)
		a.Action = abbrevWeeklyAction(parts)
		day := firstISODay(a.Date)
		inWeek := day != "" && day >= weekFrom && day < weekToEx
		switch {
		case a.Action != "":
			res.Prog = append(res.Prog, a)
		case a.Status != "cancelled" && inWeek:
			res.Prog = append(res.Prog, a)
		}
	}
	return res, nil
}
