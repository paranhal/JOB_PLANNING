package handler

import (
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
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
	if strings.HasPrefix(name, "asset/") {
		if partials, err := filepath.Glob("web/templates/asset/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "workboard/") {
		if partials, err := filepath.Glob("web/templates/workboard/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "stats/") {
		if partials, err := filepath.Glob("web/templates/stats/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if name == "dashboard.html" {
		files = append(files, "web/templates/stats/_period_table.html")
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
		"add":       func(a, b int) int { return a + b },
		"subtract":  func(a, b int) int { return a - b },
		"hasSuffix": strings.HasSuffix,
		"urlquery":  url.QueryEscape,
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
		"dict": func(pairs ...interface{}) map[string]interface{} {
			m := make(map[string]interface{}, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				key, _ := pairs[i].(string)
				m[key] = pairs[i+1]
			}
			return m
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
		"upper":    strings.ToUpper,

		"statusLabel": func(s string) string {
			m := map[string]string{
				"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
				"transfer": "이관", "cancelled": "접수취소",
				"partial_complete": "부분완료",
				"completed": "완료", "closed": "종료",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		// statusBadge 상태 뱃지. 부분완료는 「부분완료」로 명확히 표시한다.
		"statusBadge": func(s string) template.HTML {
			label := map[string]string{
				"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
				"transfer": "이관", "cancelled": "접수취소",
				"partial_complete": "부분완료", "completed": "완료", "closed": "종료",
			}[s]
			if label == "" {
				label = s
			}
			color := map[string]string{
				"received":         "bg-blue-100 text-blue-800",
				"assigned":         "bg-indigo-100 text-indigo-800",
				"in_progress":      "bg-yellow-100 text-yellow-800",
				"hold":             "bg-orange-100 text-orange-800",
				"transfer":         "bg-purple-100 text-purple-800",
				"cancelled":        "bg-gray-100 text-gray-500",
				"partial_complete": "bg-teal-100 text-teal-800",
				"completed":        "bg-green-100 text-green-800",
				"closed":           "bg-gray-100 text-gray-500",
			}[s]
			if color == "" {
				color = "bg-gray-100 text-gray-800"
			}
			title := ""
			if s == model.StatusPartialComplete {
				title = ` title="통계는 완료 집계 · 하위업무는 접수/미완료 · 운영은 진행중"`
			}
			return template.HTML(fmt.Sprintf(
				`<span class="px-2 py-0.5 rounded-full text-xs font-medium %s"%s>%s</span>`,
				color, title, template.HTMLEscapeString(label),
			))
		},
		"workKindLabel": func(s string) string {
			return model.WorkKindLabel(s)
		},
		"workPrefixLabel": func(s string) string {
			return model.WorkPrefixLabel(s)
		},
		"workPrefixClass": func(s string) string {
			switch s {
			case model.WorkPrefixAS:
				return "bg-blue-100 text-blue-800"
			case model.WorkPrefixConfirm:
				return "bg-purple-100 text-purple-800"
			case model.WorkPrefixMaintenance:
				return "bg-slate-100 text-slate-800"
			case model.WorkPrefixGeneral:
				return "bg-emerald-100 text-emerald-800"
			default:
				return "bg-gray-100 text-gray-700"
			}
		},
		"visitLabel":           visitLabel,
		"mntViewLabel":         mntViewLabel,
		"mntProductClass":      mntProductClass,
		"mntProductStyle":      mntProductStyle,
		"mntProductBadgeClass": mntProductBadgeClass,
		"mntProductBadgeStyle": mntProductBadgeStyle,
		"list": func(items ...string) []string {
			return items
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
				"received":         "bg-blue-100 text-blue-800",
				"assigned":         "bg-indigo-100 text-indigo-800",
				"in_progress":      "bg-yellow-100 text-yellow-800",
				"hold":             "bg-orange-100 text-orange-800",
				"transfer":         "bg-purple-100 text-purple-800",
				"cancelled":        "bg-gray-100 text-gray-500",
				"partial_complete": "bg-teal-100 text-teal-800",
				"completed":        "bg-green-100 text-green-800",
				"closed":           "bg-gray-100 text-gray-500",
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
			return val
		},
		"wbTaskStatusLabel": model.WBTaskStatusLabel,
		"wbPriorityLabel":   model.WBPriorityLabel,
		"wbWorkTypeLabel":   model.WBWorkTypeLabel,
		"wbWorkTypeClass":   model.WBWorkTypeClass,
		"wbCategoryLabel":   model.WBCategoryLabel,
		"wbCategoryClass":   model.WBCategoryClass,
		"wbProjectStatusLabel": model.WBProjectStatusLabel,
		"productKeyLabel":      model.ProductKeyLabel,
		"productKeysLabel":     model.ProductKeysLabel,
		"scopeWorkKindLabel":   model.ScopeWorkKindLabel,
		"scopeWorkKindsLabel":  model.ScopeWorkKindsLabel,
		"wbPriorityClass": func(p string) string {
			switch p {
			case model.WBPriorityUrgent:
				return "bg-red-100 text-red-700"
			case model.WBPriorityHigh:
				return "bg-orange-100 text-orange-700"
			case model.WBPriorityNormal:
				return "bg-sky-100 text-sky-700"
			default:
				return "bg-gray-100 text-gray-600"
			}
		},
		"wbDdayLabel": func(d int, status string) string {
			if status == model.WBTaskComplete {
				if d < 0 {
					return "+" + fmtInt(-d) + "일"
				}
				return "완료"
			}
			if d == 0 {
				return "D-day"
			}
			if d > 0 {
				return "D-" + fmtInt(d)
			}
			return "D+" + fmtInt(-d)
		},
		"kanbanCol": func(root interface{}, title, border, key string) map[string]interface{} {
			m, _ := root.(map[string]interface{})
			items := []model.WorkTask{}
			// 완료 열은 상태 기준, 그 외는 업무 유형 기준
			if key == model.WBTaskComplete {
				if by, ok := m["ByStatus"].(map[string][]model.WorkTask); ok {
					items = by[key]
				}
			} else if by, ok := m["ByWorkType"].(map[string][]model.WorkTask); ok {
				if list, ok := by[key]; ok {
					items = list
				}
			} else if by, ok := m["ByStatus"].(map[string][]model.WorkTask); ok {
				items = by[key]
			}
			return map[string]interface{}{
				"Title": title, "Border": border, "Items": items,
			}
		},
		"wbPalette": func(title, border, text, headBg string, cards []model.WBCard, canWrite bool) map[string]interface{} {
			return map[string]interface{}{
				"Title": title, "Border": border, "Text": text, "HeadBg": headBg,
				"Cards": cards, "CanWrite": canWrite,
			}
		},
		"printf": fmt.Sprintf,
	}
}

func fmtInt(n int) string {
	if n < 0 {
		n = -n
	}
	return strconv.Itoa(n)
}
