package handler

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// postedFormValue 폼에 키가 있으면 그 값, 없으면 기존 값을 쓴다.
func postedFormValue(c echo.Context, key, existing string) string {
	if c == nil || c.Request() == nil {
		return existing
	}
	r := c.Request()
	_ = r.ParseForm()
	if r.PostForm != nil {
		if _, ok := r.PostForm[key]; ok {
			return strings.TrimSpace(c.FormValue(key))
		}
	}
	if r.Form != nil {
		if _, ok := r.Form[key]; ok {
			return strings.TrimSpace(c.FormValue(key))
		}
	}
	return existing
}
