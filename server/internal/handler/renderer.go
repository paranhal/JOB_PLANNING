package handler

import (
	"html/template"
	"io"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

type TemplateRenderer struct{}

func NewRenderer() *TemplateRenderer { return &TemplateRenderer{} }

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(
		"web/templates/layout/base.html",
		filepath.Join("web/templates", name),
	)
	if err != nil {
		return err
	}

	blockName := "base"
	if c.Request().Header.Get("HX-Request") == "true" {
		blockName = "content"
	}

	// 로그인 상태 주입
	if dataMap, ok := data.(map[string]interface{}); ok {
		if _, exists := dataMap["HideNav"]; !exists {
			dataMap["UserName"] = c.Get("user_name")
			dataMap["UserRole"] = c.Get("role")
		}
	}

	return tmpl.ExecuteTemplate(w, blockName, data)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		"contains": func(s, sub string) bool { return strings.Contains(s, sub) },
		"statusLabel": func(s string) string {
			m := map[string]string{
				"received": "접수", "in_progress": "진행중", "hold": "보류",
				"completed": "완료", "closed": "종료",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"statusColor": func(s string) string {
			m := map[string]string{
				"received":    "bg-blue-100 text-blue-800",
				"in_progress": "bg-yellow-100 text-yellow-800",
				"hold":        "bg-gray-100 text-gray-800",
				"completed":   "bg-green-100 text-green-800",
				"closed":      "bg-gray-100 text-gray-500",
			}
			if c, ok := m[s]; ok {
				return c
			}
			return "bg-gray-100 text-gray-800"
		},
		"urgencyLabel": func(s string) string {
			m := map[string]string{"high": "상", "normal": "중", "low": "하"}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"urgencyColor": func(s string) string {
			m := map[string]string{
				"high": "text-red-600 font-bold", "normal": "text-yellow-600", "low": "text-gray-500",
			}
			if c, ok := m[s]; ok {
				return c
			}
			return ""
		},
		"opStatusLabel": func(s string) string {
			m := map[string]string{
				"operating": "운영중", "maintenance": "점검중", "fault": "장애",
				"retired": "철수", "disposed": "폐기",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"codeLabel": func(val string, codes interface{}) string {
			// 범용 코드→이름 변환
			return val
		},
	}
}
