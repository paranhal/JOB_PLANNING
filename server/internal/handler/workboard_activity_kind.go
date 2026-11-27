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

// validateActivityKindForm 알 수 없거나 사용하지 않는 업무 구분, 사업·구분 연결 규칙 위반을 저장 전에 거절한다. (95-224)
// 새 입력 화면은 「연결 대상」(link_target = project | category | later)을 보낸다. 이 값이 있으면 정확히 하나만 허용한다.
// link_target 이 없는 요청(옛 화면·연동)은 종전대로 두되, 구분을 붙일 때는 아래 값 검사를 똑같이 적용한다.
func validateActivityKindForm(c echo.Context) error {
	kind, posted := activityKindPosted(c)
	if posted && kind != "" {
		if !model.IsWeeklyActivityKind(kind) {
			return echo.NewHTTPError(http.StatusBadRequest, "알 수 없는 업무 구분입니다")
		}
		// 비활성 구분은 새로 고를 수 없다. 수정 화면에서 이미 붙어 있던 값을 그대로 보내는 경우만 허용한다.
		if !model.IsWeeklyActivitySelectable(kind) && strings.TrimSpace(c.Param("id")) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "사용하지 않는 업무 구분입니다")
		}
	}
	lt := strings.TrimSpace(c.FormValue("link_target"))
	if lt == "" {
		return nil
	}
	if wt := strings.TrimSpace(c.FormValue("work_type")); wt != "" && wt != model.WBWorkAdmin && wt != model.WBWorkSupport {
		return nil // 사업 연결 규칙은 일반·지원 업무에만 적용한다. 접수·점검·영업·작업은 원본/사업 구조를 따른다.
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	hasKind := posted && kind != ""
	switch lt {
	case "project":
		if hasKind {
			return echo.NewHTTPError(http.StatusBadRequest, "사업과 업무 구분은 함께 지정할 수 없습니다")
		}
		if pid == "" && c.FormValue("project_match_pending") != "1" {
			return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하세요")
		}
	case "category":
		if pid != "" {
			return echo.NewHTTPError(http.StatusBadRequest, "사업과 업무 구분은 함께 지정할 수 없습니다")
		}
		if !hasKind {
			return echo.NewHTTPError(http.StatusBadRequest, "업무 구분을 선택하세요")
		}
	case "later":
		if pid != "" || hasKind {
			return echo.NewHTTPError(http.StatusBadRequest, "「나중에 지정」은 사업·업무 구분을 비워 둔 채 저장합니다")
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "연결 대상이 올바르지 않습니다")
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
