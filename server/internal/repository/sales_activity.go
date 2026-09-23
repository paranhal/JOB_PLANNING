package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

const salesActivitySelect = `
	SELECT a.activity_id, a.sales_id, COALESCE(a.activity_date,''), COALESCE(a.start_time,''),
		COALESCE(a.duration_min,30), COALESCE(a.activity_type,''), COALESCE(a.title,''),
		COALESCE(a.content,''), COALESCE(a.place,''),
		COALESCE(a.our_members,''), COALESCE(a.counterparts,''),
		COALESCE(a.next_action,''), COALESCE(a.next_action_date,''),
		COALESCE(a.stage_at_time,''), COALESCE(a.created_by,''), COALESCE(a.created_at,''),
		COALESCE(s.name,''), COALESCE(s.sales_no,''),
		COALESCE(NULLIF(TRIM(cu.org_name),''), NULLIF(TRIM(s.prospect_name),''), '')
	FROM sales_activities a
	LEFT JOIN sales_projects s ON s.sales_id = a.sales_id
	LEFT JOIN customers cu ON cu.customer_id = s.customer_id`

func (r *SalesRepo) ActivityTypes() ([]model.Code, error) {
	return r.codeRepo.ActiveByGroup(model.SalesCodeGroupActivityType)
}

func (r *SalesRepo) activityTypeLabel(code string, types []model.Code) string {
	code = strings.TrimSpace(code)
	for _, t := range types {
		if t.CodeValue == code {
			return t.CodeName
		}
	}
	return code
}

func (r *SalesRepo) GetActivity(id string) (*model.SalesActivity, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesActivitySelect+` WHERE a.activity_id=?`, id)
	a, err := scanSalesActivity(row)
	if err != nil {
		return nil, err
	}
	types, _ := r.ActivityTypes()
	r.fillActivityLabels(a, types)
	return a, nil
}

func (r *SalesRepo) ListActivities(salesID string) ([]model.SalesActivity, error) {
	salesID = strings.TrimSpace(salesID)
	if salesID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(salesActivitySelect+`
		WHERE a.sales_id=?
		ORDER BY a.activity_date DESC, COALESCE(NULLIF(a.start_time,''),'00:00') DESC, a.activity_id DESC`, salesID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return r.scanActivityRows(rows)
}

func (r *SalesRepo) ListActivitiesByDate(date string) ([]model.SalesActivity, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		return nil, nil
	}
	rows, err := r.db.Query(salesActivitySelect+`
		WHERE a.activity_date=?
		ORDER BY COALESCE(NULLIF(a.start_time,''),'99:99'), a.title`, date)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return r.scanActivityRows(rows)
}

func (r *SalesRepo) CreateActivity(a *model.SalesActivity, createdBy string, linkTask *model.WorkTask) error {
	if a == nil {
		return fmt.Errorf("활동이 필요합니다")
	}
	p, err := r.Get(a.SalesID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("영업 사업을 찾을 수 없습니다")
		}
		return err
	}
	a.SalesID = p.SalesID
	a.ActivityDate = strings.TrimSpace(a.ActivityDate)
	if a.ActivityDate == "" {
		a.ActivityDate = time.Now().Format("2006-01-02")
	}
	if err := model.RequireAppDateYear(a.ActivityDate); err != nil {
		return err
	}
	if a.NextActionDate != "" {
		if err := model.RequireSalesDateOrYM(a.NextActionDate); err != nil {
			return err
		}
		if ym := model.NormalizeSalesYM(a.NextActionDate); model.IsSalesMonthOnly(a.NextActionDate) {
			a.NextActionDate = ym
		}
	}
	types, _ := r.ActivityTypes()
	a.ActivityType = strings.TrimSpace(a.ActivityType)
	if a.ActivityType == "" {
		return fmt.Errorf("활동 유형을 선택하세요")
	}
	a.TypeLabel = r.activityTypeLabel(a.ActivityType, types)
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" {
		return fmt.Errorf("제목을 입력하세요")
	}
	a.Content = strings.TrimSpace(a.Content)
	a.Place = strings.TrimSpace(a.Place)
	a.OurMembers = model.JoinSalesPeople(model.SplitSalesPeople(a.OurMembers))
	if a.OurMembers == "" {
		a.OurMembers = strings.TrimSpace(createdBy)
	}
	a.Counterparts = model.JoinSalesPeople(model.SplitSalesPeople(a.Counterparts))
	a.NextAction = strings.TrimSpace(a.NextAction)
	a.NextActionDate = strings.TrimSpace(a.NextActionDate)
	a.StageAtTime = p.Stage
	a.CreatedBy = strings.TrimSpace(createdBy)
	a.DurationMin = model.NormalizeDurationMin(a.DurationMin)
	a.StartTime = strings.TrimSpace(a.StartTime)
	if len(a.StartTime) > 5 {
		a.StartTime = a.StartTime[:5]
	}

	owner := ""
	if names := a.OurMemberNames(); len(names) > 0 {
		owner = names[0]
	}
	if a.StartTime == "" {
		start, _, slotErr := NewWBRepo(r.db).NextFreeSlotForAssignee(a.ActivityDate, owner, a.DurationMin, "")
		if slotErr == nil && start != "" {
			a.StartTime = start
		}
	}
	if a.StartTime == "" {
		a.StartTime = "09:00"
	}

	n, err := NextSeq(r.db, "sales_activity")
	if err != nil {
		return err
	}
	a.ActivityID = fmt.Sprintf("SA-%03d", n)
	_, err = r.db.Exec(`
		INSERT INTO sales_activities (
			activity_id, sales_id, activity_date, start_time, duration_min, activity_type,
			title, content, place, our_members, counterparts,
			next_action, next_action_date, stage_at_time, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ActivityID, a.SalesID, a.ActivityDate, a.StartTime, a.DurationMin, a.ActivityType,
		a.Title, a.Content, a.Place, a.OurMembers, a.Counterparts,
		a.NextAction, a.NextActionDate, a.StageAtTime, a.CreatedBy)
	if err != nil {
		return err
	}
	a.SalesName = p.Name
	r.fillActivityLabels(a, types)
	if err := r.syncWorkTask(a, p, linkTask); err != nil {
		return err
	}
	reindexSalesActivitySearch(r.db, a.ActivityID)
	if err := r.EnsureCustomerPartiesFromCounterparts(a.SalesID, a.Counterparts, a.CreatedBy); err != nil {
		return err
	}
	replaceSalesMembers(r.db, a.ActivityID, a.OurMembers)
	return nil
}

func (r *SalesRepo) UpdateActivity(a *model.SalesActivity) error {
	if a == nil || strings.TrimSpace(a.ActivityID) == "" {
		return fmt.Errorf("활동이 필요합니다")
	}
	cur, err := r.GetActivity(a.ActivityID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("활동을 찾을 수 없습니다")
		}
		return err
	}
	p, err := r.Get(cur.SalesID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("영업 사업을 찾을 수 없습니다")
		}
		return err
	}
	a.SalesID = cur.SalesID
	a.CreatedBy = cur.CreatedBy
	a.StageAtTime = cur.StageAtTime
	a.ActivityDate = strings.TrimSpace(a.ActivityDate)
	if a.ActivityDate == "" {
		a.ActivityDate = cur.ActivityDate
	}
	if err := model.RequireAppDateYear(a.ActivityDate); err != nil {
		return err
	}
	a.NextActionDate = strings.TrimSpace(a.NextActionDate)
	if a.NextActionDate != "" {
		if err := model.RequireSalesDateOrYM(a.NextActionDate); err != nil {
			return err
		}
		if ym := model.NormalizeSalesYM(a.NextActionDate); model.IsSalesMonthOnly(a.NextActionDate) {
			a.NextActionDate = ym
		}
	}
	types, _ := r.ActivityTypes()
	a.ActivityType = strings.TrimSpace(a.ActivityType)
	if a.ActivityType == "" {
		a.ActivityType = cur.ActivityType
	}
	if a.ActivityType == "" {
		return fmt.Errorf("활동 유형을 선택하세요")
	}
	a.TypeLabel = r.activityTypeLabel(a.ActivityType, types)
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" {
		return fmt.Errorf("제목을 입력하세요")
	}
	a.Content = strings.TrimSpace(a.Content)
	a.Place = strings.TrimSpace(a.Place)
	a.OurMembers = model.JoinSalesPeople(model.SplitSalesPeople(a.OurMembers))
	if a.OurMembers == "" {
		a.OurMembers = cur.OurMembers
	}
	a.Counterparts = model.JoinSalesPeople(model.SplitSalesPeople(a.Counterparts))
	a.NextAction = strings.TrimSpace(a.NextAction)
	a.DurationMin = model.NormalizeDurationMin(a.DurationMin)
	a.StartTime = strings.TrimSpace(a.StartTime)
	if len(a.StartTime) > 5 {
		a.StartTime = a.StartTime[:5]
	}
	if a.StartTime == "" {
		a.StartTime = strings.TrimSpace(cur.StartTime)
	}
	if a.StartTime == "" {
		a.StartTime = "09:00"
	}
	if _, err := r.db.Exec(`
		UPDATE sales_activities SET
			activity_date=?, start_time=?, duration_min=?, activity_type=?,
			title=?, content=?, place=?, our_members=?, counterparts=?,
			next_action=?, next_action_date=?
		WHERE activity_id=?`,
		a.ActivityDate, a.StartTime, a.DurationMin, a.ActivityType,
		a.Title, a.Content, a.Place, a.OurMembers, a.Counterparts,
		a.NextAction, a.NextActionDate, a.ActivityID); err != nil {
		return err
	}
	a.SalesName = p.Name
	r.fillActivityLabels(a, types)
	if err := r.syncWorkTask(a, p, nil); err != nil {
		return err
	}
	reindexSalesActivitySearch(r.db, a.ActivityID)
	if err := r.EnsureCustomerPartiesFromCounterparts(a.SalesID, a.Counterparts, a.CreatedBy); err != nil {
		return err
	}
	replaceSalesMembers(r.db, a.ActivityID, a.OurMembers)
	return nil
}

func (r *SalesRepo) DeleteActivity(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("활동이 필요합니다")
	}
	a, err := r.GetActivity(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("활동을 찾을 수 없습니다")
		}
		return err
	}
	wb := NewWBRepo(r.db)
	if t, err := wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID); err == nil && t != nil {
		_, _ = r.db.Exec(`DELETE FROM work_task_members WHERE task_id=?`, t.TaskID)
	}
	if err := wb.DeleteTasksBySource(model.WBSourceSalesActivity, a.ActivityID); err != nil {
		return err
	}
	_, _ = r.db.Exec(`DELETE FROM sales_activity_members WHERE activity_id=?`, a.ActivityID)
	if _, err := r.db.Exec(`DELETE FROM sales_activities WHERE activity_id=?`, a.ActivityID); err != nil {
		return err
	}
	if salesActivitySearchHasFTS(r.db) {
		_, _ = r.db.Exec(`DELETE FROM sales_activity_search WHERE activity_id=?`, a.ActivityID)
	}
	return nil
}

func (r *SalesRepo) syncWorkTask(a *model.SalesActivity, p *model.SalesProject, linkTask *model.WorkTask) error {
	wb := NewWBRepo(r.db)
	end := ""
	if sm := model.ParseHHMMMinutes(a.StartTime); sm >= 0 {
		end = model.FormatHHMMMinutes(sm + a.DurationMin)
	}
	owner := ""
	names := a.OurMemberNames()
	if len(names) > 0 {
		owner = names[0]
	}
	customerName := ""
	if p != nil {
		customerName = p.CustomerValue()
	}
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = a.TypeLabel
	}

	if linkTask != nil && strings.TrimSpace(linkTask.TaskID) != "" {
		if strings.TrimSpace(linkTask.SourceType) != "" {
			return fmt.Errorf("이미 원본이 연결된 업무입니다")
		}
		if err := wb.AttachTaskSource(linkTask.TaskID, model.WBSourceSalesActivity, a.ActivityID); err != nil {
			return err
		}
		t, err := wb.GetTask(linkTask.TaskID)
		if err != nil || t == nil {
			return err
		}
		if strings.TrimSpace(t.Title) == "" {
			t.Title = title
		}
		if a.ActivityDate != "" {
			t.WorkDate = a.ActivityDate
			if t.DueDate == "" {
				t.DueDate = a.ActivityDate
			}
		}
		if a.StartTime != "" && t.StartTime == "" {
			t.StartTime = a.StartTime
			t.EndTime = end
			t.DurationMin = a.DurationMin
		}
		if owner != "" && strings.TrimSpace(t.Assignee) == "" {
			t.Assignee = owner
		}
		if customerName != "" && strings.TrimSpace(t.CustomerName) == "" {
			t.CustomerName = customerName
		}
		t.WorkType = model.WBWorkSales
		if err := wb.UpdateTask(t); err != nil {
			return err
		}
		if err := r.replaceActivitySupports(wb, t.TaskID, names); err != nil {
			return err
		}
		return r.syncNextActionTask(wb, a, owner, customerName)
	}

	// 활동 로그는 「한 일」이다. 다만 미래 날짜는 아직 안 한 일이다. §45.3
	today := time.Now().Format("2006-01-02")

	existing, err := wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID, model.WBSourceRoleDone)
	if err != nil {
		return err
	}
	if existing == nil {
		existing, err = wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID)
		if err != nil {
			return err
		}
	}
	if existing != nil {
		existing.Title = title
		existing.Description = a.Content
		existing.WorkDate = a.ActivityDate
		existing.DueDate = a.ActivityDate
		existing.StartTime = a.StartTime
		existing.EndTime = end
		existing.DurationMin = a.DurationMin
		existing.Assignee = owner
		existing.CustomerName = customerName
		existing.WorkType = model.WBWorkSales
		// 날짜를 옮기면 상태도 따라간다. 다만 사람이 손으로 바꾼 것은 덮지 않는다. §45.3
		if existing.Status == model.WBTaskWaiting || existing.Status == model.WBTaskComplete {
			if a.ActivityDate <= today {
				existing.Status = model.WBTaskComplete
				existing.Progress = 100
				existing.CompleteDate = a.ActivityDate
			} else {
				existing.Status = model.WBTaskWaiting
				existing.Progress = 0
				existing.CompleteDate = ""
			}
		}
		if err := wb.UpdateTask(existing); err != nil {
			return err
		}
		if err := r.replaceActivitySupports(wb, existing.TaskID, names); err != nil {
			return err
		}
		return r.syncNextActionTask(wb, a, owner, customerName)
	}

	status, progress, completeDate := model.WBTaskWaiting, 0, ""
	if a.ActivityDate <= today {
		status, progress, completeDate = model.WBTaskComplete, 100, a.ActivityDate
	}

	t := &model.WorkTask{
		WorkType:     model.WBWorkSales,
		Title:        title,
		Description:  a.Content,
		DueDate:      a.ActivityDate,
		WorkDate:     a.ActivityDate,
		StartTime:    a.StartTime,
		EndTime:      end,
		DurationMin:  a.DurationMin,
		Status:       status,
		Progress:     progress,
		CompleteDate: completeDate,
		Priority:     model.WBPriorityNormal,
		Assignee:     owner,
		SourceType:   model.WBSourceSalesActivity,
		SourceID:     a.ActivityID,
		SourceRole:   model.WBSourceRoleDone,
		CustomerName: customerName,
	}
	if err := wb.CreateTask(t); err != nil {
		return err
	}
	if err := r.replaceActivitySupports(wb, t.TaskID, names); err != nil {
		return err
	}
	return r.syncNextActionTask(wb, a, owner, customerName)
}

func (r *SalesRepo) syncNextActionTask(wb *WBRepo, a *model.SalesActivity, owner, customerName string) error {
	if a == nil || wb == nil {
		return nil
	}
	nextTitle := strings.TrimSpace(a.NextAction)
	nextDate := strings.TrimSpace(a.NextActionDate)
	existing, err := wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID, model.WBSourceRoleNext)
	if err != nil {
		return err
	}
	want := nextTitle != "" && nextDate != "" && !model.IsSalesMonthOnly(nextDate) && model.RequireAppDateYear(nextDate) == nil
	if !want {
		if existing != nil && existing.Status != model.WBTaskComplete {
			return wb.DeleteTask(existing.TaskID)
		}
		return nil
	}
	return r.CreateNextActionTask(a.ActivityID, nextDate, owner, "")
}

// CreateNextActionTask 활동의 「다음 할 일」 업무. 미계획함 날짜 지정과 활동 저장이 같은 모양을 쓴다. §45.7·§45.9
func (r *SalesRepo) CreateNextActionTask(activityID, date, assignee, assigneeUID string) error {
	a, err := r.GetActivity(activityID)
	if err != nil {
		return err
	}
	date = strings.TrimSpace(date)
	if date == "" || model.IsSalesMonthOnly(date) {
		return fmt.Errorf("날짜가 필요합니다")
	}
	if err := model.RequireAppDateYear(date); err != nil {
		return err
	}
	owner := strings.TrimSpace(assignee)
	if owner == "" {
		if names := a.OurMemberNames(); len(names) > 0 {
			owner = names[0]
		}
	}
	title := strings.TrimSpace(a.NextAction)
	if title == "" {
		title = strings.TrimSpace(a.Title)
	}
	wb := NewWBRepo(r.db)
	existing, err := wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID, model.WBSourceRoleNext)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Status == model.WBTaskComplete {
			return nil
		}
		existing.Title = title
		existing.DueDate = date
		existing.WorkDate = date
		existing.Assignee = owner
		if strings.TrimSpace(assigneeUID) != "" {
			existing.AssigneeUserID = assigneeUID
		}
		existing.CustomerName = a.CustomerName
		existing.WorkType = model.WBWorkSales
		existing.Status = model.WBTaskWaiting
		existing.Progress = 0
		existing.CompleteDate = ""
		return wb.UpdateTask(existing)
	}
	t := &model.WorkTask{
		WorkType:       model.WBWorkSales,
		Title:          title,
		DueDate:        date,
		WorkDate:       date,
		Status:         model.WBTaskWaiting,
		Progress:       0,
		Priority:       model.WBPriorityNormal,
		Assignee:       owner,
		AssigneeUserID: strings.TrimSpace(assigneeUID),
		SourceType:     model.WBSourceSalesActivity,
		SourceID:       a.ActivityID,
		SourceRole:     model.WBSourceRoleNext,
		CustomerName:   a.CustomerName,
	}
	return wb.CreateTask(t)
}

// ListNextActionYMGaps 다음행동이 월만 지정됐고, 그 달이 되었는데 아직 업무가 없는 활동. §45.9
func (r *SalesRepo) ListNextActionYMGaps(todayYM string) ([]model.SalesNextGap, error) {
	todayYM = strings.TrimSpace(todayYM)
	if len(todayYM) > 7 {
		todayYM = todayYM[:7]
	}
	if todayYM == "" {
		todayYM = time.Now().Format("2006-01")
	}
	rows, err := r.db.Query(`
		SELECT a.activity_id, a.sales_id, COALESCE(s.name,''),
		       COALESCE(a.next_action,''), COALESCE(a.next_action_date,''),
		       COALESCE(a.our_members,''), COALESCE(s.customer_id,''), COALESCE(cu.org_name,'')
		  FROM sales_activities a
		  JOIN sales_projects s ON s.sales_id = a.sales_id
		  LEFT JOIN customers cu ON cu.customer_id = s.customer_id
		 WHERE TRIM(COALESCE(a.next_action,'')) <> ''
		   AND LENGTH(TRIM(COALESCE(a.next_action_date,''))) = 7
		   AND TRIM(a.next_action_date) <= ?
		   AND NOT EXISTS (
		         SELECT 1 FROM work_tasks t
		          WHERE t.source_type = 'sales_activity'
		            AND t.source_id   = a.activity_id
		            AND t.source_role = 'next')`, todayYM)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesNextGap
	for rows.Next() {
		var g model.SalesNextGap
		var members string
		if err := rows.Scan(&g.ActivityID, &g.SalesID, &g.SalesName, &g.NextAction, &g.NextActionYM,
			&members, &g.CustomerID, &g.CustomerName); err != nil {
			return nil, err
		}
		g.NextAction = strings.TrimSpace(g.NextAction)
		g.NextActionYM = strings.TrimSpace(g.NextActionYM)
		if names := model.SplitSalesPeople(members); len(names) > 0 {
			g.OwnerName = names[0]
		}
		g.CustomerName = strings.TrimSpace(g.CustomerName)
		if g.CustomerName == "" {
			g.CustomerName = strings.TrimSpace(g.SalesName)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *SalesRepo) replaceActivitySupports(wb *WBRepo, taskID string, names []string) error {
	if len(names) <= 1 {
		return wb.ReplaceSupportMembers(taskID, nil)
	}
	var supports []model.WorkTaskMember
	for i, n := range names[1:] {
		supports = append(supports, model.WorkTaskMember{
			Assignee:  n,
			Role:      model.WBMemberSupport,
			SortOrder: i + 1,
		})
	}
	return wb.ReplaceSupportMembers(taskID, supports)
}

func (r *SalesRepo) ListFollowupGaps(todayYM string) ([]model.SalesProject, map[string][]string, error) {
	items, err := r.List("", model.SalesStatusActive, "")
	if err != nil {
		return nil, nil, err
	}
	latest, err := r.latestActivityBySales()
	if err != nil {
		return nil, nil, err
	}
	todayYM = strings.TrimSpace(todayYM)
	if len(todayYM) > 7 {
		todayYM = todayYM[:7]
	}
	today := time.Now()
	kinds := map[string][]string{}
	var out []model.SalesProject
	for i := range items {
		p := items[i]
		var ks []string
		if model.SalesStageIsOpen(p.Stage) {
			act, ok := latest[p.SalesID]
			if !ok || !act.HasFollowUp() {
				ks = append(ks, model.UnplannedSalesFollow)
			}
			ym := model.NormalizeSalesYM(p.ExpectedYM)
			if ym != "" && todayYM != "" && ym < todayYM {
				ks = append(ks, model.UnplannedReview)
			}
		}
		if model.SalesContractUnsigned(&p, today) {
			ks = append(ks, model.UnplannedSalesUnsigned)
		}
		if len(ks) == 0 {
			continue
		}
		out = append(out, p)
		kinds[p.SalesID] = ks
	}
	return out, kinds, nil
}

func (r *SalesRepo) LatestActivityDateBySales() (map[string]string, error) {
	latest, err := r.latestActivityBySales()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, a := range latest {
		out[id] = strings.TrimSpace(a.ActivityDate)
	}
	return out, nil
}

func (r *SalesRepo) latestActivityBySales() (map[string]model.SalesActivity, error) {
	rows, err := r.db.Query(salesActivitySelect + `
		ORDER BY a.activity_date DESC, COALESCE(NULLIF(a.start_time,''),'00:00') DESC, a.activity_id DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return map[string]model.SalesActivity{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	all, err := r.scanActivityRows(rows)
	if err != nil {
		return nil, err
	}
	out := map[string]model.SalesActivity{}
	for _, a := range all {
		if _, ok := out[a.SalesID]; ok {
			continue
		}
		out[a.SalesID] = a
	}
	return out, nil
}

func (r *SalesRepo) LatestNextBySales() (map[string]model.SalesActivity, error) {
	rows, err := r.db.Query(salesActivitySelect + `
		WHERE TRIM(COALESCE(a.next_action,'')) != '' OR TRIM(COALESCE(a.next_action_date,'')) != ''
		ORDER BY a.activity_date DESC, a.activity_id DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return map[string]model.SalesActivity{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	all, err := r.scanActivityRows(rows)
	if err != nil {
		return nil, err
	}
	out := map[string]model.SalesActivity{}
	for _, a := range all {
		if _, ok := out[a.SalesID]; ok {
			continue
		}
		out[a.SalesID] = a
	}
	return out, nil
}

func (r *SalesRepo) fillActivityLabels(a *model.SalesActivity, types []model.Code) {
	if a == nil {
		return
	}
	a.TypeLabel = r.activityTypeLabel(a.ActivityType, types)
	if a.StageAtTime != "" {
		stages, _ := r.Stages()
		if d := model.FindSalesStage(stages, a.StageAtTime); d != nil {
			a.StageLabel = d.Label
		} else {
			a.StageLabel = model.SalesStageDisplayLabel(a.StageAtTime, stages)
		}
	}
}

func (r *SalesRepo) scanActivityRows(rows *sql.Rows) ([]model.SalesActivity, error) {
	types, _ := r.ActivityTypes()
	var out []model.SalesActivity
	for rows.Next() {
		a, err := scanSalesActivity(rows)
		if err != nil {
			return nil, err
		}
		r.fillActivityLabels(a, types)
		out = append(out, *a)
	}
	return out, rows.Err()
}

type activityScanner interface {
	Scan(dest ...interface{}) error
}

func scanSalesActivity(sc activityScanner) (*model.SalesActivity, error) {
	var a model.SalesActivity
	err := sc.Scan(
		&a.ActivityID, &a.SalesID, &a.ActivityDate, &a.StartTime, &a.DurationMin, &a.ActivityType,
		&a.Title, &a.Content, &a.Place, &a.OurMembers, &a.Counterparts,
		&a.NextAction, &a.NextActionDate, &a.StageAtTime, &a.CreatedBy, &a.CreatedAt,
		&a.SalesName, &a.SalesNo, &a.CustomerName)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *WBRepo) AttachTaskSource(taskID, sourceType, sourceID string) error {
	taskID = strings.TrimSpace(taskID)
	sourceType = strings.TrimSpace(sourceType)
	sourceID = strings.TrimSpace(sourceID)
	if taskID == "" || sourceType == "" || sourceID == "" {
		return fmt.Errorf("원본 연결 값이 비었습니다")
	}
	_, err := r.db.Exec(`
		UPDATE work_tasks SET source_type=?, source_id=?,
			work_type=CASE WHEN ? = 'sales_activity' THEN 'sales' ELSE work_type END,
			source_role=CASE WHEN ? = 'sales_activity' THEN 'done' ELSE COALESCE(source_role,'') END,
			updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND TRIM(COALESCE(source_type,''))=''`,
		sourceType, sourceID, sourceType, sourceType, taskID)
	return err
}

func (r *WBRepo) SalesIDByActivity(activityID string) string {
	activityID = strings.TrimSpace(activityID)
	if r == nil || r.db == nil || activityID == "" {
		return ""
	}
	var id string
	err := r.db.QueryRow(`SELECT sales_id FROM sales_activities WHERE activity_id=?`, activityID).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}
