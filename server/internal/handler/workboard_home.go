package handler

import (
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const workboardHomePath = "/workboard/board"

// workboardHomeDate YYYY-MM-DD 만 통과. 그 외는 오늘.
func workboardHomeDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 10 {
		raw = raw[:10]
	}
	if _, err := time.ParseInLocation(dateLayout, raw, time.Local); err != nil {
		return time.Now().In(time.Local).Format(dateLayout)
	}
	return raw
}

// workboardHomeURL 새 업무 관리. date가 있으면 그달 캘린더, 없으면 목록 기본 주소.
func workboardHomeURL(date string) string {
	if strings.TrimSpace(date) == "" {
		return workboardHomePath
	}
	return workboardHomePath + "?view=month&date=" + workboardHomeDate(date)
}

func workboardCalendarURL(workDate, dueDate string) string {
	d := strings.TrimSpace(workDate)
	if d == "" {
		d = strings.TrimSpace(dueDate)
	}
	return workboardHomeURL(workboardHomeDate(d))
}

func workboardNavActive(back string) string {
	if strings.Contains(back, "/workboard/register") {
		return NavWorkRegister
	}
	if strings.Contains(back, "/admin-work/stats") {
		return NavAdminWorkStats
	}
	if strings.Contains(back, "/admin-work") {
		return NavAdminWork
	}
	return NavWorkBoard
}

func workboardBackLabel(back string) string {
	if strings.Contains(back, "/workboard/register") {
		return "← 일일 업무 등록"
	}
	return "← 업무 관리"
}

func requestBackOrHome(c echo.Context) string {
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = strings.TrimSpace(c.FormValue("back"))
	}
	if back == "" {
		return workboardHomeURL("")
	}
	return back
}
