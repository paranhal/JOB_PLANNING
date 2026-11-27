package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-226: 진행률(자동 3모드+수동)·일자별 부분 완료·하위 업무 부하량·연기(대기 업무) 화면 동작.

func (h *WorkboardHandler) progressTask(c echo.Context) (*model.WorkTask, error) {
	t, err := h.repo.GetTask(strings.TrimSpace(c.Param("id")))
	if err != nil || t == nil {
		return nil, echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return nil, err
	}
	if !h.canProgressEdit(c, t) {
		return nil, echo.ErrForbidden
	}
	return t, nil
}

// canProgressEdit 담당·참여자·관리자급·공동 작업 범위의 사용자.
func (h *WorkboardHandler) canProgressEdit(c echo.Context, t *model.WorkTask) bool {
	if t == nil || isReadOnly(c) || !canWriteWorkboard(c) {
		return false
	}
	members, _ := h.repo.ListMembers(t.TaskID)
	return canWriteProjectWorkAction(c, t, members) || canEditTask(c, t.Assignee, t.AssigneeUserID, t.WorkGroupID)
}

func (h *WorkboardHandler) progressDone(c echo.Context, t *model.WorkTask, q string) error {
	back := safeWorkboardRedirect(c.FormValue("redirect"), "/workboard/tasks/"+url.PathEscape(t.TaskID))
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+q)
}

func (h *WorkboardHandler) progressFail(c echo.Context, t *model.WorkTask, err error) error {
	return h.progressDone(c, t, "err="+url.QueryEscape(err.Error()))
}

// ProgressManual POST /workboard/tasks/:id/progress — 수동 진행률(비우면 해제).
func (h *WorkboardHandler) ProgressManual(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(c.FormValue("pct"))
	var pct *float64
	if raw != "" {
		v, perr := repository.ParseLoad(raw) // 0~100 숫자
		if perr != nil || v == nil {
			return h.progressFail(c, t, repository.ErrManualPct)
		}
		pct = v
	}
	if err := h.repo.SetManualProgress(t.TaskID, pct, currentUserID(c)); err != nil {
		return h.progressFail(c, t, err)
	}
	return h.progressDone(c, t, "ok=progress")
}

// ProgressDay POST /workboard/tasks/:id/day-progress — 한 날의 부하량·완료 비율.
func (h *WorkboardHandler) ProgressDay(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	load, lerr := repository.ParseLoad(c.FormValue("load_pct"))
	if lerr != nil {
		return h.progressFail(c, t, lerr)
	}
	done, derr := repository.ParseLoad(c.FormValue("done_pct"))
	if derr != nil || done == nil {
		return h.progressFail(c, t, repository.ErrDonePct)
	}
	completed, err := h.repo.SaveDayProgress(t.TaskID, strings.TrimSpace(c.FormValue("day_date")), load, *done, c.FormValue("note"), currentUserID(c))
	if err != nil {
		return h.progressFail(c, t, err)
	}
	if completed {
		return h.progressDone(c, t, "ok=day_complete")
	}
	return h.progressDone(c, t, "ok=progress")
}

// ProgressCompleteMulti POST /workboard/tasks/:id/complete-multi — 여러 날 업무 완료 선택(오늘까지만 / 전체 완료). 95-223 #3.
func (h *WorkboardHandler) ProgressCompleteMulti(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	date := strings.TrimSpace(c.FormValue("date"))
	if date == "" {
		date = time.Now().Format(dateLayout)
	}
	if _, err := h.repo.CompleteMultiDay(t.TaskID, strings.TrimSpace(c.FormValue("mode")), date, currentUserID(c)); err != nil {
		return h.progressFail(c, t, err)
	}
	return h.progressDone(c, t, "ok=progress")
}

// ProgressLoads POST /workboard/tasks/:id/loads — 하위 업무 부하량(load_<하위업무ID>=%). 합 100 초과 등은 거부한다.
func (h *WorkboardHandler) ProgressLoads(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	loads := map[string]*float64{}
	for k, vals := range c.Request().PostForm {
		if !strings.HasPrefix(k, "load_") || len(vals) == 0 {
			continue
		}
		v, perr := repository.ParseLoad(vals[0])
		if perr != nil {
			return h.progressFail(c, t, perr)
		}
		loads[strings.TrimPrefix(k, "load_")] = v
	}
	if err := h.repo.SaveSubtaskLoads(t.TaskID, loads, currentUserID(c)); err != nil {
		return h.progressFail(c, t, err)
	}
	return h.progressDone(c, t, "ok=progress")
}

// TaskDefer POST /workboard/tasks/:id/defer — 연기: 날짜를 비우고 대기로 보낸다(사유 입력 없음, 지연 통계에 넣지 않음).
func (h *WorkboardHandler) TaskDefer(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	if err := h.repo.DeferTask(t.TaskID, currentUserID(c)); err != nil {
		return h.progressFail(c, t, err)
	}
	return h.progressDone(c, t, "ok=defer")
}

// TaskUndefer POST /workboard/tasks/:id/undefer — 대기 업무를 오늘(또는 지정한 날) 업무로 등록한다.
func (h *WorkboardHandler) TaskUndefer(c echo.Context) error {
	t, err := h.progressTask(c)
	if err != nil {
		return err
	}
	date := strings.TrimSpace(c.FormValue("date"))
	if date == "" {
		date = time.Now().Format(dateLayout)
	}
	if err := h.repo.UndeferTask(t.TaskID, date, strings.TrimSpace(c.FormValue("start_time")), strings.TrimSpace(c.FormValue("end_time")), currentUserID(c)); err != nil {
		return h.progressFail(c, t, err)
	}
	return h.progressDone(c, t, "ok=undefer")
}

// DeferredGroup 대기 영역의 한 그룹(사업 또는 업무 구분).
type DeferredGroup struct {
	Heading string
	Items   []repository.DeferredItem
}

// deferredGroupsFor 업무 화면의 「대기」 영역. 미완료로 남은 건은 먼저 대기로 되돌리고, 사업(구분)별로 묶는다.
// 등록자·담당·참여·공동 작업·수락한 배정 범위에만 보이고(관리자급은 조직 전체), 0건이면 카드를 숨긴다.
func (h *WorkboardHandler) deferredGroupsFor(c echo.Context) []DeferredGroup {
	if h == nil || h.repo == nil {
		return nil
	}
	h.repo.ReturnUnfinishedDeferred(time.Now().Format(dateLayout))
	wide := isAdminRole(c) || currentRole(c) == model.RoleOrgAdmin
	list := h.repo.ListDeferred(currentOrg(c), strings.TrimSpace(currentUserID(c)), wide)
	var groups []DeferredGroup
	idx := map[string]int{}
	for _, it := range list {
		key := it.Heading()
		i, ok := idx[key]
		if !ok {
			i = len(groups)
			idx[key] = i
			groups = append(groups, DeferredGroup{Heading: key})
		}
		groups[i].Items = append(groups[i].Items, it)
	}
	return groups
}

// fillProgressPanel 조치·상세 화면의 진행률 패널 데이터.
func (h *WorkboardHandler) fillProgressPanel(c echo.Context, data map[string]interface{}, t *model.WorkTask) {
	if data == nil || t == nil || h == nil || h.repo == nil {
		return
	}
	info := h.repo.ComputeProgress(t.TaskID)
	data["ProgressInfo"] = info
	data["ProgressDays"] = h.repo.ListDays(t.TaskID)
	data["ProgressKids"] = h.repo.SubtaskLoadView(t.TaskID)
	data["ProgressCanEdit"] = h.canProgressEdit(c, t)
	data["ProgressToday"] = time.Now().Format(dateLayout)
	closed := t.Status == model.WBTaskComplete || t.Status == model.WBTaskCancelled
	data["ProgressOpen"] = !closed
	var deferred string
	_ = h.repo.DB().QueryRow(`SELECT COALESCE(deferred_at,'') FROM work_tasks WHERE task_id=?`, t.TaskID).Scan(&deferred)
	data["TaskDeferred"] = deferred != ""
}
