package handler

import (
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

type TemplateRenderer struct {
	mu    sync.Mutex
	cache map[string]*template.Template
}

func NewRenderer() *TemplateRenderer {
	return &TemplateRenderer{cache: map[string]*template.Template{}}
}

func appendExisting(files []string, paths ...string) []string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			files = append(files, p)
		}
	}
	return files
}

func templateFiles(name string) []string {
	files := []string{
		"web/templates/layout/base.html",
		filepath.Join("web/templates", name),
	}
	if name == "auth/login.html" {
		return files
	}
	if strings.HasPrefix(name, "meeting/") {
		if partials, err := filepath.Glob("web/templates/meeting/_*.html"); err == nil {
			files = append(files, partials...)
		}
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
	if strings.HasPrefix(name, "work/") {
		if partials, err := filepath.Glob("web/templates/work/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "workboard/") {
		if partials, err := filepath.Glob("web/templates/workboard/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "sales/") {
		if partials, err := filepath.Glob("web/templates/sales/_*.html"); err == nil {
			files = append(files, partials...)
		}
		if partials, err := filepath.Glob("web/templates/items/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "items/") || strings.HasPrefix(name, "quotes/") {
		if partials, err := filepath.Glob("web/templates/items/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "admin_work/") {
		if partials, err := filepath.Glob("web/templates/admin_work/_*.html"); err == nil {
			files = append(files, partials...)
		}
		files = appendExisting(files, "web/templates/workboard/_recurrence_fields.html")
	}
	if strings.HasPrefix(name, "stats/") {
		if partials, err := filepath.Glob("web/templates/stats/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "work_status/") {
		if partials, err := filepath.Glob("web/templates/work_status/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if name == "dashboard.html" {
		if partials, err := filepath.Glob("web/templates/stats/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if kpartials, err := filepath.Glob("web/templates/kanban/_*.html"); err == nil {
		files = append(files, kpartials...)
	}
	if spartials, err := filepath.Glob("web/templates/sort/_*.html"); err == nil {
		files = append(files, spartials...)
	}
	return files
}

func (t *TemplateRenderer) lookup(name string) (*template.Template, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tmpl, ok := t.cache[name]; ok {
		return tmpl, nil
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(templateFiles(name)...)
	if err != nil {
		return nil, err
	}
	if t.cache == nil {
		t.cache = map[string]*template.Template{}
	}
	t.cache[name] = tmpl
	return tmpl, nil
}

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tmpl, err := t.lookup(name)
	if err != nil {
		return err
	}

	blockName := "base"
	if c.Request().Header.Get("HX-Request") == "true" {
		blockName = "content"
	}

	c.Response().Header().Set("Cache-Control", "no-cache")

	if data == nil {
		data = map[string]interface{}{}
	}
	// 로그인 상태·빌드 값 주입. 사이드바와 배지가 같은 값을 쓴다. §40.4
	if dataMap, ok := data.(map[string]interface{}); ok {
		injectBuildInfo(dataMap)
		if _, exists := dataMap["HideNav"]; !exists {
			dataMap["UserName"] = ctxString(c, "user_name")
			dataMap["UserRole"] = model.NormalizeRole(ctxString(c, "role"))
			dataMap["Username"] = ctxString(c, "username")
			dataMap["UserID"] = ctxString(c, "user_id")
			dataMap["UserPerms"] = currentPerms(c)
			dataMap["SalesNav"] = IsSalesNav(c.Request().URL.Path, c.Request().URL.RawQuery)
			injectAssignNoticeView(c, dataMap)
			if v := c.Get("auth_unconfirmed"); v != nil {
				if b, ok := v.(bool); ok && b {
					dataMap["AuthUnconfirmed"] = true
				}
			}
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
		"add":            func(a, b int) int { return a + b },
		"subtract":       func(a, b int) int { return a - b },
		"dateLabelMDW":   model.DateLabelMDW,
		"leaveKindLabel": model.LeaveKindLabel,
		"hasSuffix":      strings.HasSuffix,
		"urlquery":       url.QueryEscape,
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
		"percent": func(part, max interface{}) float64 {
			to := func(v interface{}) float64 {
				switch n := v.(type) {
				case int:
					return float64(n)
				case int64:
					return float64(n)
				case float64:
					return n
				default:
					return 0
				}
			}
			m := to(max)
			if m <= 0 {
				return 0
			}
			p := to(part) * 100 / m
			if p > 100 {
				p = 100
			}
			if p < 0 {
				return 0
			}
			return p
		},
		"salesStageStripe":   model.SalesStageStripeClass,
		"salesActBadgeClass": model.SalesActivityTypeBadgeClass,
		"salesActBadgeLabel": model.SalesActivityBadgeLabel,
		"sub":                func(a, b int) int { return a - b },
		"mul":                func(a, b int) int { return a * b },
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
		"contains":      func(s, sub string) bool { return strings.Contains(s, sub) },
		"upper":         strings.ToUpper,
		"displayPerson": model.DisplayPerson,
		"ymd": func(t time.Time) string {
			d, _ := model.KnowledgeWhen(t)
			return d
		},
		"ymdt": func(t time.Time) string {
			_, f := model.KnowledgeWhen(t)
			return f
		},

		"statusLabel": func(s string) string {
			m := map[string]string{
				"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "대기",
				"transfer": "이관", "cancelled": "접수취소",
				"partial_complete": "부분완료",
				"completed":        "완료", "closed": "종료",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		// statusBadge 상태 뱃지. 부분완료는 「부분완료」로 명확히 표시한다.
		"statusBadge": func(s string) template.HTML {
			label := map[string]string{
				"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "대기",
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
		"actionResultLabel":   model.ActionResultLabel,
		"transferDetailLabel": model.TransferDetailLabel,
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
			case model.WorkPrefixSales:
				return "bg-orange-100 text-orange-800"
			default:
				return "bg-gray-100 text-gray-700"
			}
		},
		"visitLabel":           visitLabel,
		"mntVisitBadge":        model.VisitStatusBadgeOf,
		"mntViewLabel":         mntViewLabel,
		"mntProductClass":      mntProductClass,
		"mntProductStyle":      mntProductStyle,
		"mntProductBadgeClass": mntProductBadgeClass,
		"mntProductBadgeStyle": mntProductBadgeStyle,
		"assigneeColorStyle":   model.AssigneeColorStyle,
		"workCardColorStyle":   model.WorkCardColorStyle,
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
		"urgencyReasonText": func(code, note string) string {
			return model.UrgencyReasonLabel(code, note, nil)
		},
		"partyKindLabel":      model.PartyKindLabel,
		"processTypePlaces":   model.ProcessTypePlaces,
		"urgencyReasonFamily": model.UrgencyReasonFamilyFromGroup,
		"assetUrgencyFamily": func(a model.Asset) string {
			return model.AssetUrgencyFamily(a.ProductCategory, a.ProductName, a.ProductType)
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
		"installerTypeLabel": func(s string) string {
			m := map[string]string{
				"self": "자사", "other": "타사", "manufacturer": "제조사",
				"partner": "협력사", "unknown": "미상",
			}
			if l, ok := m[s]; ok {
				return l
			}
			return s
		},
		"managementTypeLabel": func(s string) string {
			m := map[string]string{
				"direct": "직접유지보수", "fault": "장애대응", "periodic": "정기점검",
				"on_demand": "요청시지원", "reference": "참고관리", "third_party": "타사장비",
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
		"visitProtected": func(date string) bool {
			return model.VisitDeleteProtected(date, time.Now())
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
			return model.RoleLabel(s)
		},
		"hasUserPerm": func(perms interface{}, key string) bool {
			switch v := perms.(type) {
			case []string:
				return model.HasPermission(v, key)
			case string:
				return model.HasPermission(model.ParsePermissions(v), key)
			default:
				return false
			}
		},
		"userPermList": func(u model.User) []string {
			return u.PermList()
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
		"wbTaskStatusLabel":     model.WBTaskStatusLabel,
		"occurrenceStatusLabel": model.OccurrenceStatusLabel,
		"recurrenceRuleLabel":   model.RecurrenceRuleLabel,
		"wbAdminStatusLabel":    model.WBAdminStatusLabel,
		"wbKanbanBadge":         model.WBKanbanBadge,
		"wbKanbanBadgeClass":    model.WBKanbanBadgeClass,
		"wbActionStatusLabel":   model.WBActionStatusLabel,
		"wbActivityTypeLabel":   model.WBActivityTypeLabel,
		"wbWaitPartyKindLabel":  model.WBWaitPartyKindLabel,
		"wbPriorityLabel":       model.WBPriorityLabel,
		"wbWorkTypeLabel":       model.WBWorkTypeLabel,
		"wbWorkTypeClass":       model.WBWorkTypeClass,
		"workPlaceLabel":        model.WorkPlaceLabel,
		"attDisplayName":        func(a model.Attachment) string { return a.DisplayName() },
		"formatBytes":           model.FormatByteSize,
		"wbCategoryLabel":       model.WBCategoryLabel,
		"wbCategoryClass":       model.WBCategoryClass,
		"wbProjectStatusLabel":  model.WBProjectStatusLabel,
		"productKeyLabel":       model.ProductKeyLabel,
		"productKeysLabel":      model.ProductKeysLabel,
		"scopeWorkKindLabel":    model.ScopeWorkKindLabel,
		"scopeWorkKindsLabel":   model.ScopeWorkKindsLabel,
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
		"wbBoardStatusLabel": func(t model.WorkTask) string {
			if model.IsAdminGTDTask(t) {
				return model.WBAdminStatusLabel(t.Status)
			}
			return model.WBTaskStatusLabel(t.Status)
		},
		"kanbanCol": func(root interface{}, title, border, key string) map[string]interface{} {
			m, _ := root.(map[string]interface{})
			items := []model.WorkTask{}
			switch key {
			case model.WBTaskWaiting, model.WBTaskInProgress, model.WBTaskComplete:
				if by, ok := m["ByStatus"].(map[string][]model.WorkTask); ok {
					items = by[key]
				}
			default:
				if by, ok := m["ByWorkType"].(map[string][]model.WorkTask); ok {
					if list, ok := by[key]; ok {
						items = list
					}
				}
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
		"printf":              fmt.Sprintf,
		"won":                 formatSalesWon,
		"quoteStatusLabel":    model.QuoteStatusLabel,
		"vatModeLabel":        model.VATModeLabel,
		"roundRuleLabel":      model.RoundRuleLabel,
		"quotePurposeLabel":   model.QuotePurposeLabel,
		"quotePurposeClass":   model.QuotePurposeBadgeClass,
		"quoteFormLabel":      model.QuoteFormLabel,
		"quoteExpiryLabel":    quoteExpiryLabelNow,
		"quoteExpiredRow":     quoteExpiredRowClass,
		"orderStatusLabel":    model.OrderStatusLabel,
		"orderScheduleLabel":  model.OrderScheduleLabel,
		"purchaseStatusLabel": model.PurchaseStatusLabel,
	}
}

func fmtInt(n int) string {
	if n < 0 {
		n = -n
	}
	return strconv.Itoa(n)
}

func quoteExpiryLabelNow(q model.SalesQuote) string {
	return q.ExpiryLabel(time.Now().Format("2006-01-02"))
}

func quoteExpiredRowClass(q model.SalesQuote) string {
	today := time.Now().Format("2006-01-02")
	if q.IsExpiredSent(today) || q.IsExpiringSoon(today) {
		return "bg-orange-50"
	}
	return ""
}
