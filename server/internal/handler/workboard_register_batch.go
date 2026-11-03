package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type WorkBatchItem struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Reason string `json:"reason,omitempty"`
	TaskID string `json:"task_id,omitempty"`
}

type WorkBatchResult struct {
	Kind     string          `json:"kind"`
	Mode     string          `json:"mode"`
	Created  []WorkBatchItem `json:"created"`
	Skipped  []WorkBatchItem `json:"skipped"`
	Failed   []WorkBatchItem `json:"failed"`
	Message  string          `json:"message"`
}

func uniqueNonEmptyIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func formNamedIDList(c echo.Context, keys ...string) []string {
	if c == nil || c.Request() == nil {
		return nil
	}
	_ = c.Request().ParseForm()
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		for _, v := range c.Request().PostForm[k] {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func parseMultiSiteMode(c echo.Context, n int) string {
	if n < 2 {
		return "separate"
	}
	m := strings.TrimSpace(c.FormValue("multi_site_mode"))
	if m == "joint" {
		return "joint"
	}
	return "separate"
}

func (r *WorkBatchResult) summarize() {
	if r == nil {
		return
	}
	r.Message = fmt.Sprintf("업무 %d건 등록", len(r.Created))
	if len(r.Skipped) > 0 {
		r.Message += fmt.Sprintf(" / %d건 건너뜀", len(r.Skipped))
		var bits []string
		for _, s := range r.Skipped {
			if s.Label != "" && s.Reason != "" {
				bits = append(bits, s.Label+"("+s.Reason+")")
			}
		}
		if len(bits) > 0 {
			r.Message += ": " + strings.Join(bits, ", ")
		}
	}
	if len(r.Failed) > 0 {
		r.Message += fmt.Sprintf(" / %d건 실패", len(r.Failed))
	}
}

func (h *WorkboardHandler) RegisterWorkBatch(c echo.Context, kind string, ids []string, mode string, sel Selection, workDate, dueDate, start, end string, dur int) (*WorkBatchResult, error) {
	out := &WorkBatchResult{Kind: kind, Mode: mode}
	if len(ids) == 0 {
		out.Failed = append(out.Failed, WorkBatchItem{Reason: "대상이 없습니다"})
		out.summarize()
		return out, nil
	}
	if mode != "joint" {
		mode = "separate"
		out.Mode = mode
	}
	org := currentOrg(c)
	if mode == "joint" {
		first := ids[0]
		task, skip, err := h.batchOneSource(c, kind, first, sel, workDate, dueDate, start, end, dur, org)
		if skip != nil {
			out.Skipped = append(out.Skipped, *skip)
		} else if err != nil {
			if errors.Is(err, errOverlapResponse) {
				return out, err
			}
			out.Failed = append(out.Failed, WorkBatchItem{ID: first, Reason: err.Error()})
		} else if task != nil {
			out.Created = append(out.Created, WorkBatchItem{ID: first, TaskID: task.TaskID, Label: task.Title})
			var extraSites []string
			if strings.TrimSpace(task.CustomerID) != "" {
				extraSites = append(extraSites, task.CustomerID)
			}
			for _, id := range ids[1:] {
				if kind == model.WBWorkAS {
					as, e := h.asRepo.GetByID(repository.OrgAll, id)
					if e != nil || as == nil {
						out.Skipped = append(out.Skipped, WorkBatchItem{ID: id, Reason: "접수를 찾을 수 없습니다"})
						continue
					}
					if as.OrgID != "" && org != "" && org != repository.OrgAll && as.OrgID != org {
						out.Skipped = append(out.Skipped, WorkBatchItem{ID: id, Label: as.ASNumber, Reason: "타 조직"})
						continue
					}
					if cid := strings.TrimSpace(as.CustomerID); cid != "" {
						extraSites = append(extraSites, cid)
					}
				} else {
					extraSites = append(extraSites, id)
				}
			}
			_ = h.repo.ReplaceTaskSites(task.TaskID, extraSites)
		}
		out.summarize()
		return out, nil
	}
	for _, id := range ids {
		task, skip, err := h.batchOneSource(c, kind, id, sel, workDate, dueDate, start, end, dur, org)
		if skip != nil {
			out.Skipped = append(out.Skipped, *skip)
			continue
		}
		if err != nil {
			if errors.Is(err, errOverlapResponse) {
				return out, err
			}
			out.Failed = append(out.Failed, WorkBatchItem{ID: id, Reason: err.Error()})
			continue
		}
		if task != nil {
			out.Created = append(out.Created, WorkBatchItem{ID: id, TaskID: task.TaskID, Label: task.Title})
		}
	}
	out.summarize()
	return out, nil
}

func (h *WorkboardHandler) batchOneSource(c echo.Context, kind, sourceID string, sel Selection, workDate, dueDate, start, end string, dur int, org string) (*model.WorkTask, *WorkBatchItem, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, &WorkBatchItem{Reason: "대상 없음"}, nil
	}
	if existing, _ := h.repo.GetTaskBySource(kind, sourceID); existing != nil {
		label := existing.Title
		return nil, &WorkBatchItem{ID: sourceID, Label: label, Reason: "이미 등록됨", TaskID: existing.TaskID}, nil
	}
	if strings.TrimSpace(start) != "" && strings.TrimSpace(workDate) != "" {
		if err := h.rejectIfOverlap(c, workDate, start, end, "", occupancyNames(sel.PrimaryName, nil)); err != nil {
			return nil, nil, err
		}
	}
	t, err := h.buildSourceTask(kind, sourceID, workDate, start, end, dur, sel.PrimaryName, "")
	if err != nil {
		return nil, nil, err
	}
	if dueDate != "" {
		t.DueDate = dueDate
	}
	applySelectionToTask(t, sel)
	t.OrgID = org
	if strings.TrimSpace(t.OrgID) == "" || t.OrgID == repository.OrgAll {
		ins, ierr := resolveInsertOrg(c)
		if ierr != nil {
			return nil, nil, ierr
		}
		t.OrgID = ins
	}
	if err := h.repo.CreateTask(t); err != nil {
		return nil, nil, err
	}
	if err := h.applyPendingInterrupt(c, t.TaskID); err != nil {
		return nil, nil, err
	}
	_ = h.saveSelectionOnTask(t.TaskID, sel)
	h.syncVisitDateFromTask(kind, sourceID, workDate)
	return t, nil, nil
}

func (h *WorkboardHandler) respondWorkBatch(c echo.Context, res *WorkBatchResult) error {
	if res == nil {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=task"))
	}
	accept := c.Request().Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		code := http.StatusOK
		if len(res.Created) == 0 && len(res.Failed) > 0 {
			code = http.StatusBadRequest
		}
		return c.JSON(code, res)
	}
	q := "ok=batch"
	if len(res.Created) == 0 && len(res.Skipped) > 0 {
		q = "ok=skip"
	}
	if len(res.Failed) > 0 && len(res.Created) == 0 {
		q = "err=task"
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, q+"&n="+fmt.Sprintf("%d", len(res.Created))))
}
