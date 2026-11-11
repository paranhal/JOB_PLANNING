package handler

import (
	"strings"

	"customer-support/internal/model"
)

// prepareCalendarTasks 같은 날 접수 본체와 하위업무가 겹치면 본체 카드를 뺀다.
// 키는 as:<접수> 와 as_work:<work_id> 로 갈라 두어 95-158 중복 제거에 섞이지 않는다.
func (h *WorkboardHandler) prepareCalendarTasks(tasks []model.WorkTask) ([]model.WorkTask, map[string]bool) {
	var ids []string
	for _, t := range tasks {
		if t.SourceType == model.WBSourceASWork && strings.TrimSpace(t.SourceID) != "" {
			ids = append(ids, t.SourceID)
		}
	}
	asOf, unconf := map[string]string{}, map[string]bool{}
	if h != nil && h.repo != nil && len(ids) > 0 {
		asOf, unconf = h.repo.ASWorkLinks(ids)
	}
	return dropASParentsShadowed(tasks, asOf), unconf
}

func dropASParentsShadowed(tasks []model.WorkTask, asOf map[string]string) []model.WorkTask {
	covered := map[string]bool{}
	for _, t := range tasks {
		if t.SourceType != model.WBSourceASWork {
			continue
		}
		asID := strings.TrimSpace(asOf[t.SourceID])
		day := strings.TrimSpace(t.WorkDate)
		if asID == "" || day == "" {
			continue
		}
		covered[asID+"|"+day] = true
	}
	if len(covered) == 0 {
		return tasks
	}
	out := make([]model.WorkTask, 0, len(tasks))
	for _, t := range tasks {
		day := strings.TrimSpace(t.WorkDate)
		if t.SourceType == model.WBSourceAS && day != "" && covered[t.SourceID+"|"+day] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func markASWorkTentative(tasks []model.WorkTask, unconf map[string]bool, toCard func(model.WorkTask) model.WBCard) func(model.WorkTask) model.WBCard {
	return func(t model.WorkTask) model.WBCard {
		c := toCard(t)
		if t.SourceType == model.WBSourceASWork && unconf[t.SourceID] && strings.TrimSpace(t.WorkDate) != "" {
			c.Tentative = true
		}
		return c
	}
}

func undatedASWorkCards(tasks []model.WorkTask) []model.WBCard {
	out := make([]model.WBCard, 0, len(tasks))
	for _, t := range tasks {
		dur := t.DurationMin
		if dur <= 0 {
			dur = 30
		}
		href := ""
		if id := strings.TrimSpace(t.SourceID); id != "" {
			href = "/as/work/" + id + "/action"
		}
		out = append(out, model.WBCard{
			Kind:         "task",
			RefID:        t.TaskID,
			Category:     model.WBSourceASWork,
			Title:        t.Title,
			SubTitle:     t.Description,
			SourceNumber: t.SourceID,
			SourceHref:   href,
			ActionHref:   href,
			Assignee:     t.Assignee,
			DurationMin:  dur,
			TaskID:       t.TaskID,
		})
	}
	return out
}
