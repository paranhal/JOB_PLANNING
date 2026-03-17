package handler

import (
	"html/template"
	"io"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// TemplateRenderer Echo용 HTML 템플릿 렌더러
type TemplateRenderer struct{}

func NewRenderer() *TemplateRenderer {
	return &TemplateRenderer{}
}

// Render 템플릿을 렌더링한다.
// HTMX 요청(HX-Request 헤더)이면 "content" 블록만 반환한다.
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(
		"web/templates/layout/base.html",
		filepath.Join("web/templates", name),
	)
	if err != nil {
		return err
	}

	// HTMX 부분 요청이면 content 블록만 반환
	blockName := "base"
	if c.Request().Header.Get("HX-Request") == "true" {
		blockName = "content"
	}

	return tmpl.ExecuteTemplate(w, blockName, data)
}

// funcMap 템플릿에서 사용할 헬퍼 함수
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
		"statusLabel": func(s string) string {
			labels := map[string]string{
				"received":    "접수",
				"in_progress": "진행중",
				"hold":        "보류",
				"completed":   "완료",
				"closed":      "종료",
			}
			if l, ok := labels[s]; ok {
				return l
			}
			return s
		},
		"statusColor": func(s string) string {
			colors := map[string]string{
				"received":    "bg-blue-100 text-blue-800",
				"in_progress": "bg-yellow-100 text-yellow-800",
				"hold":        "bg-gray-100 text-gray-800",
				"completed":   "bg-green-100 text-green-800",
				"closed":      "bg-gray-100 text-gray-500",
			}
			if c, ok := colors[s]; ok {
				return c
			}
			return "bg-gray-100 text-gray-800"
		},
		"urgencyLabel": func(s string) string {
			labels := map[string]string{"high": "상", "normal": "중", "low": "하"}
			if l, ok := labels[s]; ok {
				return l
			}
			return s
		},
		"urgencyColor": func(s string) string {
			colors := map[string]string{
				"high":   "text-red-600 font-bold",
				"normal": "text-yellow-600",
				"low":    "text-gray-500",
			}
			if c, ok := colors[s]; ok {
				return c
			}
			return ""
		},
	}
}
