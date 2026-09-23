package repository

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *SalesRepo) DormantDefaultMonth() int {
	var v string
	_ = r.db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, model.SettingSalesDormantMonth).Scan(&v)
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	if n < 1 || n > 12 {
		return 6
	}
	return n
}

func (r *SalesRepo) Sleep(id, until, reason, byID, byName string, noPlan bool) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Stage) == model.SalesStage4Closed ||
		p.CloseReason == model.SalesCloseContracted ||
		p.CloseReason == model.SalesCloseLost ||
		p.CloseReason == model.SalesCloseDropped {
		return fmt.Errorf("종료된 사업은 휴면할 수 없습니다")
	}
	until = model.NormalizeDormantYM(until)
	if until == "" {
		return fmt.Errorf("다시 볼 연월이 필요합니다")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("휴면 사유가 필요합니다")
	}
	before := rowJSON(r.db, "sales_projects", "sales_id", p.SalesID)
	now := time.Now().Format("2006-01-02 15:04:05")
	histN, err := NextSeq(r.db, "sales_stage_history")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := `UPDATE sales_projects SET status=?, dormant_until=?, dormant_reason=?, dormant_at=?, dormant_by=?`
	args := []interface{}{model.SalesStatusDormant, until, reason, now, strings.TrimSpace(byName)}
	if noPlan {
		q += `, budget_status=?`
		args = append(args, model.SalesBudgetNoPlan)
	}
	q += ` WHERE sales_id=?`
	args = append(args, p.SalesID)
	if _, err := tx.Exec(q, args...); err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, p.Stage, p.Stage, reason, "dormant", byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdateWithReason(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, before, reason)
	return nil
}

func (r *SalesRepo) Wake(id, byID, byName string, budgetYear int, budgetStatus string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if p.Status != model.SalesStatusDormant {
		return fmt.Errorf("휴면 사업이 아닙니다")
	}
	before := rowJSON(r.db, "sales_projects", "sales_id", p.SalesID)
	if budgetYear <= 0 {
		if p.BudgetYear > 0 {
			budgetYear = p.BudgetYear + 1
		}
	}
	st := model.NormalizeSalesBudgetStatus(budgetStatus)
	if st == "" {
		st = model.SalesBudgetUnknown
	}
	histN, err := NextSeq(r.db, "sales_stage_history")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE sales_projects SET status=?, dormant_until='', dormant_reason='', dormant_at='', dormant_by='',
			budget_year=?, budget_status=? WHERE sales_id=?`,
		model.SalesStatusActive, budgetYear, st, p.SalesID); err != nil {
		return err
	}
	if err := insertSalesHistory(tx, fmt.Sprintf("SH-%03d", histN), p.SalesID, p.Stage, p.Stage, "휴면 해제", "wake", byID, byName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdateWithReason(r.db, "sales_projects", "sales_id", p.SalesID, p.Name, before, "휴면 해제")
	return nil
}

func (r *SalesRepo) NextActionBeforeDormant(id, until string) bool {
	until = model.NormalizeDormantYM(until)
	if until == "" {
		return false
	}
	nextBy, _ := r.LatestNextBySales()
	nx, ok := nextBy[id]
	if !ok {
		return false
	}
	d := strings.TrimSpace(nx.NextActionDate)
	if len(d) >= 7 {
		return d[:7] < until
	}
	return false
}

func (r *SalesRepo) BulkSetBizType(ids []string, biz string) (int, error) {
	biz = model.NormalizeSalesBizType(biz)
	if biz == "" {
		return 0, fmt.Errorf("사업 유형을 고르세요")
	}
	return r.bulkSet(ids, `UPDATE sales_projects SET biz_type=?, updated_at=CURRENT_TIMESTAMP WHERE sales_id=?`, biz)
}

func (r *SalesRepo) BulkSetBudgetStatus(ids []string, st string) (int, error) {
	st = model.NormalizeSalesBudgetStatus(st)
	if st == "" {
		return 0, fmt.Errorf("예산 상태를 고르세요")
	}
	return r.bulkSet(ids, `UPDATE sales_projects SET budget_status=?, updated_at=CURRENT_TIMESTAMP WHERE sales_id=?`, st)
}

func (r *SalesRepo) bulkSet(ids []string, q, val string) (int, error) {
	n := 0
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		p, err := r.Get(id)
		if err != nil {
			continue
		}
		before := rowJSON(r.db, "sales_projects", "sales_id", id)
		if _, err := r.db.Exec(q, val, id); err != nil {
			return n, err
		}
		logUpdate(r.db, "sales_projects", "sales_id", id, p.Name, before)
		n++
	}
	return n, nil
}

func (r *SalesRepo) ListDormantDue(ym string) ([]model.SalesProject, error) {
	ym = model.NormalizeDormantYM(ym)
	if ym == "" {
		ym = time.Now().Format("2006-01")
	}
	q := salesSelect + ` WHERE s.status=? AND COALESCE(s.dormant_until,'') != '' AND s.dormant_until<=? ORDER BY s.dormant_until, s.name`
	rows, err := r.db.Query(q, model.SalesStatusDormant, ym)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSalesRows(rows)
}

func (r *SalesRepo) EnsureDormantReviewTasks(now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now()
	}
	ym := now.Format("2006-01")
	items, err := r.ListFilter(SalesListFilter{IncludeDormant: true, Status: model.SalesStatusDormant})
	if err != nil {
		return 0, err
	}
	wb := NewWBRepo(r.db)
	n := 0
	for i := range items {
		p := items[i]
		until := model.NormalizeDormantYM(p.DormantUntil)
		if until == "" || until[:7] > ym {
			continue
		}
		if strings.TrimSpace(p.SalesOwner) == "" && strings.TrimSpace(p.SalesOwnerID) == "" {
			continue
		}
		day := until + "-01"
		src := p.SalesID + ":" + until
		exist, err := wb.GetTaskBySource(model.WBSourceSales, src)
		if err != nil {
			return n, err
		}
		if exist != nil {
			continue
		}
		title := "휴면 해제 검토 · " + strings.TrimSpace(p.DisplayNo()+" "+p.Name)
		t := &model.WorkTask{
			WorkType:       model.WBWorkSales,
			Title:          strings.TrimSpace(title),
			Description:    strings.TrimSpace(p.DormantReason),
			DueDate:        day,
			WorkDate:       day,
			Status:         model.WBTaskWaiting,
			Priority:       model.WBPriorityNormal,
			Assignee:       p.SalesOwner,
			AssigneeUserID: p.SalesOwnerID,
			SourceType:     model.WBSourceSales,
			SourceID:       src,
			CustomerName:   p.CustomerValue(),
		}
		if err := wb.CreateTask(t); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
