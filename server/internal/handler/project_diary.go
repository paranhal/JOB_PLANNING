package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/config"
)

func (h *ProjectHandler) Diary(c echo.Context) error {
	if !config.Load().ShowProjectsSector() {
		return echo.ErrNotFound
	}
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	workDate := time.Now()
	if raw := strings.TrimSpace(c.QueryParam("date")); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			workDate = t
		}
	}
	day := workDate.Format("2006-01-02")
	example := config.Load().ShowProjectExampleData()
	seed := diarySeed{}
	if example {
		seed = sampleProjectDiary()
	}
	return c.Render(http.StatusOK, "project/diary.html", map[string]interface{}{
		"Title":   "업무일지·투입관리",
		"Active":  NavProjectsDiary,
		"Date":    day,
		"Seed":    seed,
		"Example": example,
	})
}

func (h *ProjectHandler) DiaryDraft(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "hook": "draft", "persisted": false,
		"message": "일일 업무 등록 통계 화면입니다. 임시저장은 쓰지 않습니다.",
	})
}

func (h *ProjectHandler) DiaryReview(c echo.Context) error {
	if !canWriteProjects(c) {
		return echo.ErrForbidden
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "hook": "review", "persisted": false,
		"message": "일일 업무 등록 통계 화면입니다. 검토 요청은 쓰지 않습니다.",
	})
}
