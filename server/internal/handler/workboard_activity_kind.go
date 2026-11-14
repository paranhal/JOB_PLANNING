package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-198: 업무 등록·수정 화면의 「활동 분류」 입력. 사업에 속하지 않는 업무를 주간보고서 활동 블록에 모은다.
// 사업과 활동은 함께 가질 수 없다: 사업이 있으면 사업이 우선이고 활동 분류는 비운다.

// activityKindPosted 폼이 활동 분류 칸을 보냈는지와 그 값.
func activityKindPosted(c echo.Context) (kind string, posted bool) {
	_ = c.Request().ParseForm()
	vals, ok := c.Request().Form["activity_kind"]
	if !ok || len(vals) == 0 {
		return "", false
	}
	return strings.TrimSpace(vals[0]), true
}

// validateActivityKindForm 알 수 없는 분류 값을 저장 전에 거절한다.
func validateActivityKindForm(c echo.Context) error {
	kind, posted := activityKindPosted(c)
	if posted && kind != "" && !model.IsWeeklyActivityKind(kind) {
		return echo.NewHTTPError(http.StatusBadRequest, "알 수 없는 활동 분류입니다")
	}
	return nil
}

// applyActivityKindForm 저장된 업무에 활동 분류를 반영한다. 일반(admin) 업무만 대상이다.
func applyActivityKindForm(c echo.Context, repo *repository.WBRepo, tasks ...*model.WorkTask) {
	kind, posted := activityKindPosted(c)
	if !posted {
		// 칸을 보내지 않는 화면에서 사업을 연결했으면 남아 있는 활동 분류를 비운다.
		for _, t := range tasks {
			if t != nil && strings.TrimSpace(t.ProjectID) != "" && repo.TaskActivityKind(t.TaskID) != "" {
				_, _, _ = repo.SetTaskActivity(t.OrgID, t.TaskID, "")
			}
		}
		return
	}
	for _, t := range tasks {
		if t == nil || strings.TrimSpace(t.TaskID) == "" {
			continue
		}
		want := kind
		if t.WorkType != model.WBWorkAdmin || strings.TrimSpace(t.ProjectID) != "" {
			want = "" // 사업이 있거나 일반 업무가 아니면 활동 분류는 두지 않는다
		}
		if want == "" && repo.TaskActivityKind(t.TaskID) == "" {
			continue
		}
		if _, _, err := repo.SetTaskActivity(t.OrgID, t.TaskID, want); err != nil {
			continue
		}
	}
}
