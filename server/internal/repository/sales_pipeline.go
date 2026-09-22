package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *SalesRepo) Pipeline(now time.Time, extraPeople []string) (model.SalesPipeline, error) {
	return r.PipelineFilter(now, extraPeople, SalesListFilter{})
}

func (r *SalesRepo) PipelineFilter(now time.Time, extraPeople []string, f SalesListFilter) (model.SalesPipeline, error) {
	items, err := r.ListFilter(f)
	if err != nil {
		return model.SalesPipeline{}, err
	}
	winF := f
	winF.IncludeClosed = true
	winItems, err := r.ListFilter(winF)
	if err != nil {
		winItems = items
	}
	skip, err := NewQuoteRepo(r.db).NonDealQuoteSalesIDs()
	if err != nil {
		return model.SalesPipeline{}, err
	}
	if len(skip) > 0 {
		kept := items[:0]
		for i := range items {
			if skip[items[i].SalesID] {
				continue
			}
			kept = append(kept, items[i])
		}
		items = kept
	}
	stages, _ := r.StagesFor(f.DealType)
	hist, err := r.ListAllHistory()
	if err != nil {
		return model.SalesPipeline{}, err
	}
	acts, err := r.ListAllActivities()
	if err != nil {
		return model.SalesPipeline{}, err
	}
	ids := map[string]bool{}
	for i := range items {
		ids[items[i].SalesID] = true
	}
	hist = filterSalesHistoryByID(hist, ids)
	acts = filterSalesActivitiesByID(acts, ids)
	pipe := model.BuildSalesPipeline(items, stages, hist, acts, now, extraPeople)
	pipe.WinContracted, pipe.WinLost = model.SalesWinSample(winItems)
	pipe.WinRateLabel = model.FormatSalesRate(pipe.WinContracted, pipe.WinContracted+pipe.WinLost)
	return pipe, nil
}

func (r *SalesRepo) ListAllHistory() ([]model.SalesStageHistory, error) {
	rows, err := r.db.Query(`
		SELECT history_id, sales_id, COALESCE(from_stage,''), to_stage, COALESCE(reason,''),
			COALESCE(changed_by,''), COALESCE(changed_by_id,''), COALESCE(changed_at,'')
		FROM sales_stage_history
		ORDER BY changed_at, history_id`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	stages := r.allStageDefs()
	var items []model.SalesStageHistory
	for rows.Next() {
		var h model.SalesStageHistory
		if err := rows.Scan(&h.HistoryID, &h.SalesID, &h.FromStage, &h.ToStage, &h.Reason,
			&h.ChangedBy, &h.ChangedByID, &h.ChangedAt); err != nil {
			return nil, err
		}
		h.FromLabel = model.SalesStageDisplayLabel(h.FromStage, stages)
		h.ToLabel = model.SalesStageDisplayLabel(h.ToStage, stages)
		items = append(items, h)
	}
	return items, rows.Err()
}

func (r *SalesRepo) ListAllActivities() ([]model.SalesActivity, error) {
	rows, err := r.db.Query(salesActivitySelect + `
		ORDER BY a.activity_date DESC, COALESCE(NULLIF(a.start_time,''),'00:00') DESC, a.activity_id DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return r.scanActivityRows(rows)
}

func (r *SalesRepo) MoveActivity(id, group, value, byName string) error {
	_ = byName
	a, err := r.GetActivity(id)
	if err != nil {
		return err
	}
	group = strings.TrimSpace(group)
	value = strings.TrimSpace(value)
	switch group {
	case "stage":
		stages, _ := r.Stages()
		if model.FindSalesStage(stages, value) == nil {
			return fmt.Errorf("알 수 없는 단계입니다")
		}
		if a.StageAtTime == value {
			return nil
		}
		_, err = r.db.Exec(`UPDATE sales_activities SET stage_at_time=? WHERE activity_id=?`, value, a.ActivityID)
		return err
	default:
		types, _ := r.ActivityTypes()
		ok := false
		for _, t := range types {
			if t.CodeValue == value {
				ok = true
				a.TypeLabel = t.CodeName
				break
			}
		}
		if !ok {
			return fmt.Errorf("알 수 없는 활동 유형입니다")
		}
		if a.ActivityType == value {
			return nil
		}
		a.ActivityType = value
		if _, err := r.db.Exec(`UPDATE sales_activities SET activity_type=? WHERE activity_id=?`, value, a.ActivityID); err != nil {
			return err
		}
		p, _ := r.Get(a.SalesID)
		return r.syncWorkTask(a, p, nil)
	}
}

func filterSalesHistoryByID(items []model.SalesStageHistory, ids map[string]bool) []model.SalesStageHistory {
	if len(ids) == 0 {
		return nil
	}
	var out []model.SalesStageHistory
	for _, h := range items {
		if ids[h.SalesID] {
			out = append(out, h)
		}
	}
	return out
}

func filterSalesActivitiesByID(items []model.SalesActivity, ids map[string]bool) []model.SalesActivity {
	if len(ids) == 0 {
		return nil
	}
	var out []model.SalesActivity
	for _, a := range items {
		if ids[a.SalesID] {
			out = append(out, a)
		}
	}
	return out
}

type SupplyMetrics struct {
	ConversionSubmitted int
	ConversionWon       int
}

func (r *SalesRepo) LoadSupplyMetrics(_ time.Time, _ bool) (SupplyMetrics, error) {
	items, err := NewQuoteRepo(r.db).List(QuoteFilter{Purpose: model.QuotePurposeDeal})
	if err != nil {
		return SupplyMetrics{}, err
	}
	var m SupplyMetrics
	for _, q := range items {
		st := model.NormalizeQuoteStatus(q.Status)
		if st == model.QuoteStatusDraft {
			continue
		}
		m.ConversionSubmitted++
		if st == model.QuoteStatusWon {
			m.ConversionWon++
		}
	}
	return m, nil
}
