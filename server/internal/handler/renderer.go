package handler

import (
	"html/template"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

type TemplateRenderer struct{}

func NewRenderer() *TemplateRenderer { return &TemplateRenderer{} }

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	files := []string{
		"web/templates/layout/base.html",
		filepath.Join("web/templates", name),
	}
	if strings.HasPrefix(name, "as/") {
		if partials, err := filepath.Glob("web/templates/as/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
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
			dataMap["UserName"] = ctxString(c, "user_name")
			dataMap["UserRole"] = ctxString(c, "role")
			dataMap["Username"] = ctxString(c, "username")
			dataMap["UserID"] = ctxString(c, "user_id")
		}
	}

	return tmpl.ExecuteTemplate(w, blockName, data)
}

func ctxString(c echo.Context, key string) string {
	v := c.Get(key)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// RenderPartial HTMX 부분 응답용 — base.html 없이 템플릿 파일만 직접 렌더링
func RenderPartial(c echo.Context, name string, data interface{}) error {
	tmpl, err := template.New(filepath.Base(name)).Funcs(funcMap()).ParseFiles(
		filepath.Join("web/templates", name),
	)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(c.Response().Writer, data)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"urlquery": url.QueryEscape,
		"hasString": func(list interface{}, s string) bool {
			switch v := list.(type) {
			case []string:
				for _, x := range v {
					if x == s {
						return true
					}
				}
			}
			return false
		},
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
				"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
				"transfer": "이관", "cancelled": "접수취소",
				"completed": "완료", "closed": "종료",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"holdNextLabel": func(s string) string {
			m := map[string]string{
				"action": "조치", "transfer": "이관", "cancel": "접수취소",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"statusColor": func(s string) string {
			m := map[string]string{
				"received":    "bg-blue-100 text-blue-800",
				"assigned":    "bg-indigo-100 text-indigo-800",
				"in_progress": "bg-yellow-100 text-yellow-800",
				"hold":        "bg-orange-100 text-orange-800",
				"transfer":    "bg-purple-100 text-purple-800",
				"cancelled":   "bg-gray-100 text-gray-500",
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
		"productCategoryLabel": func(s string) string {
			m := map[string]string{
				"homepage": "홈페이지", "elibrary": "전자도서관", "mobile": "모바일",
				"rfid": "RFID자동화", "materials": "자료관리", "other": "기타",
			}
			if l, ok := m[s]; ok {
				return l
			}
			if s == "" {
				return "—"
			}
			return s
		},
		"maintContractLabel": func(s string) string {
			m := map[string]string{
				"paid": "유상", "free": "무상", "call": "CALL", "none": "미계약",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"maintCycleLabel": func(s string) string {
			m := map[string]string{
				"monthly": "월", "quarterly": "분기", "semi": "반기",
				"odd_bimonthly": "홀수격월", "even_bimonthly": "짝수격월",
				"yearly": "년1회", "custom": "직접입력",
				// 이전 시드 호환
				"call": "Call", "none": "없음",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"maintBillingCycleLabel": func(s string) string {
			m := map[string]string{
				"monthly": "월", "quarterly": "분기", "semi": "반기",
				"odd_bimonthly": "홀수격월", "even_bimonthly": "짝수격월",
				"custom": "직접입력",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"jobGradeLabel": func(s string) string {
			m := map[string]string{
				"librarian": "사서직", "it": "전산직", "other": "기타", "custom": "직접입력",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"contactRoleLabel": func(s string) string {
			m := map[string]string{
				"primary": "주담당", "secondary": "부담당", "regular": "일반 담당자",
			}
			if l, ok := m[s]; ok {
				return l
			}
			if s == "" {
				return "일반 담당자"
			}
			return s
		},
		"affiliationLabel": func(s string) string {
			m := map[string]string{
				"institution": "소속기관",
				"integrator":  "통합사업자",
				"partner":     "협력업체",
				"other":       "기타",
			}
			if l, ok := m[s]; ok {
				return l
			}
			if s == "" {
				return "소속기관"
			}
			return s
		},
		"roleLabel": func(s string) string {
			m := map[string]string{
				"admin": "관리자", "receipt": "접수담당", "tech": "기술담당",
				"sales": "영업담당", "viewer": "열람사용자",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"initial": func(s string) string {
			for _, r := range s {
				return string(r)
			}
			return "U"
		},
		"changeReasonLabel": func(s string) string {
			m := map[string]string{
				"transfer": "전보", "resign": "퇴직", "role_adjust": "업무조정",
				"전보": "전보", "퇴직": "퇴직", "업무조정": "업무조정",
			}
			if l, ok := m[s]; ok {
				return l
			}
			if s == "" {
				return "현행"
			}
			return s
		},
		"codeLabel": func(val string, codes interface{}) string {
			// 범용 코드→이름 변환
			return val
		},
	}
}
