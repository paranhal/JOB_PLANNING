package handler

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

type LiveAdapter interface {
	WorkType() string
	Enabled() bool
	CanStart(c echo.Context, t *model.WorkTask) error
	IsFieldWork(t *model.WorkTask) bool
	AfterSegmentsChanged(tx *sql.Tx, t *model.WorkTask) error
	EndHref(t *model.WorkTask) string
	CompletionRule(t *model.WorkTask) model.CompletionRule
	IsUrgent(t *model.WorkTask) bool
}

type disabledAdapter struct{ typ string }

func (a disabledAdapter) WorkType() string { return a.typ }
func (a disabledAdapter) Enabled() bool    { return false }
func (a disabledAdapter) CanStart(c echo.Context, t *model.WorkTask) error {
	return echo.NewHTTPError(403, "live_disabled_for_type")
}
func (a disabledAdapter) IsFieldWork(t *model.WorkTask) bool { return false }
func (a disabledAdapter) AfterSegmentsChanged(*sql.Tx, *model.WorkTask) error {
	return nil
}
func (a disabledAdapter) EndHref(*model.WorkTask) string { return "" }
func (a disabledAdapter) CompletionRule(*model.WorkTask) model.CompletionRule {
	return model.CompletionRule{Kind: "source", Label: "원본 화면"}
}
func (a disabledAdapter) IsUrgent(t *model.WorkTask) bool {
	return t != nil && t.Priority == model.WBPriorityUrgent
}

type adminAdapter struct{}

func (adminAdapter) WorkType() string { return model.WBWorkAdmin }
func (adminAdapter) Enabled() bool    { return true }
func (adminAdapter) CanStart(c echo.Context, t *model.WorkTask) error {
	return liveCanStartCommon(c, t)
}
func (a adminAdapter) IsFieldWork(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	return model.NormalizeWorkPlace(t.WorkPlace) == model.WorkPlaceField
}
func (adminAdapter) AfterSegmentsChanged(*sql.Tx, *model.WorkTask) error { return nil }
func (adminAdapter) EndHref(*model.WorkTask) string                     { return "" }
func (adminAdapter) CompletionRule(*model.WorkTask) model.CompletionRule {
	return model.CompletionRule{Kind: "self", Label: "업무 상태"}
}
func (adminAdapter) IsUrgent(t *model.WorkTask) bool {
	return t != nil && t.Priority == model.WBPriorityUrgent
}

type supportAdapter struct{ adminAdapter }

func (supportAdapter) WorkType() string { return model.WBWorkSupport }

type projectWorkAdapter struct{}

func (projectWorkAdapter) WorkType() string { return model.WBWorkProjectWork }
func (projectWorkAdapter) Enabled() bool    { return true }
func (projectWorkAdapter) CanStart(c echo.Context, t *model.WorkTask) error {
	return liveCanStartCommon(c, t)
}
func (projectWorkAdapter) IsFieldWork(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	return model.NormalizeWorkPlace(t.WorkPlace) == model.WorkPlaceField
}
func (projectWorkAdapter) AfterSegmentsChanged(*sql.Tx, *model.WorkTask) error { return nil }
func (projectWorkAdapter) EndHref(t *model.WorkTask) string {
	if t == nil {
		return ""
	}
	return "/workboard/tasks/" + t.TaskID + "/action"
}
func (projectWorkAdapter) CompletionRule(*model.WorkTask) model.CompletionRule {
	return model.CompletionRule{Kind: "action", Label: "작업 조치 결과"}
}
func (projectWorkAdapter) IsUrgent(t *model.WorkTask) bool {
	return t != nil && t.Priority == model.WBPriorityUrgent
}

type asAdapter struct{ disabledAdapter }

func (asAdapter) WorkType() string { return model.WBWorkAS }
func (asAdapter) IsFieldWork(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	return model.NormalizeWorkPlace(t.WorkPlace) == model.WorkPlaceField
}

type maintenanceAdapter struct{ disabledAdapter }

func (maintenanceAdapter) WorkType() string { return model.WBWorkMaintenance }
func (maintenanceAdapter) IsFieldWork(*model.WorkTask) bool {
	return true
}

type salesAdapter struct{ disabledAdapter }

func (salesAdapter) WorkType() string { return model.WBWorkSales }
func (salesAdapter) IsFieldWork(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	return model.NormalizeWorkPlace(t.WorkPlace) == model.WorkPlaceField
}

func adapterFor(workType string) LiveAdapter {
	switch strings.TrimSpace(workType) {
	case model.WBWorkAdmin:
		return adminAdapter{}
	case model.WBWorkSupport:
		return supportAdapter{}
	case model.WBWorkProjectWork:
		return projectWorkAdapter{}
	case model.WBWorkAS:
		return asAdapter{disabledAdapter{typ: model.WBWorkAS}}
	case model.WBWorkMaintenance:
		return maintenanceAdapter{disabledAdapter{typ: model.WBWorkMaintenance}}
	case model.WBWorkSales, model.WBSourceSalesActivity:
		return salesAdapter{disabledAdapter{typ: model.WBWorkSales}}
	default:
		return disabledAdapter{typ: workType}
	}
}

func liveCanStartCommon(c echo.Context, t *model.WorkTask) error {
	if t == nil {
		return echo.ErrNotFound
	}
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel || currentRole(c) == model.RoleObserver {
		return echo.ErrForbidden
	}
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	st := strings.TrimSpace(t.Status)
	if st == model.WBTaskComplete || st == model.WBTaskCancelled {
		return fmt.Errorf("완료된 업무는 시작할 수 없습니다")
	}
	if strings.TrimSpace(t.Assignee) == "" && strings.TrimSpace(t.AssigneeUserID) == "" && strings.TrimSpace(t.WorkGroupID) == "" {
		return fmt.Errorf("담당자가 없는 업무는 시작할 수 없습니다")
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	uid := currentUserID(c)
	name := currentUserDisplayName(c)
	if assigneeIsMine(c, t.Assignee, t.AssigneeUserID) {
		return nil
	}
	if isAdminRole(c) || isOfficeRole(c) {
		return nil
	}
	if memberOfGroup(c, t.WorkGroupID) {
		return nil
	}
	if t.WorkType == model.WBWorkProjectWork && isTechRole(c) && canWriteWorkboard(c) {
		return nil
	}
	_ = uid
	_ = name
	return echo.ErrForbidden
}

func liveTypeStartHint(workType string) string {
	if adapterFor(workType).Enabled() {
		return ""
	}
	return "이 유형은 아직 작업 시작을 지원하지 않습니다 — 작업 후 조치 화면에서 시간을 입력하세요"
}
