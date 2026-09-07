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
		COALESCE(s.name,''),
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
		if err := model.RequireAppDateYear(a.NextActionDate); err != nil {
			return err
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
		if err := wb.UpdateTask(t); err != nil {
			return err
		}
		return r.replaceActivitySupports(wb, t.TaskID, names)
	}

	existing, err := wb.GetTaskBySource(model.WBSourceSalesActivity, a.ActivityID)
	if err != nil {
		return err
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
		if err := wb.UpdateTask(existing); err != nil {
			return err
		}
		return r.replaceActivitySupports(wb, existing.TaskID, names)
	}

	t := &model.WorkTask{
		WorkType:     model.WBWorkAdmin,
		Title:        title,
		Description:  a.Content,
		DueDate:      a.ActivityDate,
		WorkDate:     a.ActivityDate,
		StartTime:    a.StartTime,
		EndTime:      end,
		DurationMin:  a.DurationMin,
		Status:       model.WBTaskWaiting,
		Priority:     model.WBPriorityNormal,
		Assignee:     owner,
		SourceType:   model.WBSourceSalesActivity,
		SourceID:     a.ActivityID,
		CustomerName: customerName,
	}
	if err := wb.CreateTask(t); err != nil {
		return err
	}
	return r.replaceActivitySupports(wb, t.TaskID, names)
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
		&a.SalesName, &a.CustomerName)
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
		UPDATE work_tasks SET source_type=?, source_id=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND TRIM(COALESCE(source_type,''))=''`,
		sourceType, sourceID, taskID)
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
