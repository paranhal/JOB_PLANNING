package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/appclock"
	"customer-support/internal/config"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type TemplateRenderer struct {
	mu    sync.Mutex
	cache map[string]*template.Template
}

func chipText(v interface{}, names ...string) string {
	if v == nil {
		return ""
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map:
		for _, n := range names {
			val := rv.MapIndex(reflect.ValueOf(n))
			if val.IsValid() && val.Kind() == reflect.String && strings.TrimSpace(val.String()) != "" {
				return strings.TrimSpace(val.String())
			}
		}
	case reflect.Struct:
		for _, n := range names {
			f := rv.FieldByName(n)
			if f.IsValid() && f.Kind() == reflect.String && strings.TrimSpace(f.String()) != "" {
				return strings.TrimSpace(f.String())
			}
		}
	}
	return ""
}

func chipID(v interface{}) string { return chipText(v, "id", "ID", "AssetID") }

func chipName(v interface{}) string { return chipText(v, "name", "Name", "ProductName") }

func assetChips(assets []model.Asset, id string) []map[string]string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	name := id
	for _, a := range assets {
		if a.AssetID != id {
			continue
		}
		name = strings.TrimSpace(a.ProductName)
		if m := strings.TrimSpace(a.ModelName); m != "" {
			if name != "" {
				name += " · " + m
			} else {
				name = m
			}
		}
		if name == "" {
			name = id
		}
		break
	}
	return []map[string]string{{"id": id, "name": name}}
}

// testSharedRenderer TestMain 이 채우면 NewRenderer 가 인스턴스를 재사용한다(95-98).
var testSharedRenderer *TemplateRenderer

func NewRenderer() *TemplateRenderer {
	if testSharedRenderer != nil {
		return testSharedRenderer
	}
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

// baseTemplateFiles 는 layout/base.html 이 호출하는 공통 조각이다.
// auth/login.html 조기 반환을 포함해 모든 화면이 같은 목록을 쓴다.
func baseTemplateFiles() []string {
	return appendExisting(nil,
		"web/templates/layout/base.html",
		"web/templates/layout/_nav_tree.html",
		"web/templates/_partials/login_briefing.html",
		"web/templates/_partials/schedule_show_toggle.html",
		"web/templates/_partials/page_back.html",
		"web/templates/_partials/dup_username_dialog.html",
		"web/templates/workboard/_carry_modal.html",
	)
}

func templateFiles(name string) []string {
	files := appendExisting(baseTemplateFiles(), filepath.Join("web/templates", name))
	if name == "auth/login.html" {
		return files
	}
	if strings.HasPrefix(name, "exec/") {
		if partials, err := filepath.Glob("web/templates/exec/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "meeting/") {
		if partials, err := filepath.Glob("web/templates/meeting/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "work_group/") {
		if partials, err := filepath.Glob("web/templates/work_group/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "coop/") {
		if partials, err := filepath.Glob("web/templates/coop/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "leave/") {
		if partials, err := filepath.Glob("web/templates/leave/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "as/") {
		if partials, err := filepath.Glob("web/templates/as/_*.html"); err == nil {
			files = append(files, partials...)
		}
		files = appendExisting(files,
			"web/templates/workboard/_segments_form.html",
			"web/templates/workboard/_work_items.html",
		)
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
	if strings.HasPrefix(name, "work_item_template/") {
		if partials, err := filepath.Glob("web/templates/work_item_template/_*.html"); err == nil {
			files = append(files, partials...)
		}
	}
	if strings.HasPrefix(name, "workboard/") {
		if partials, err := filepath.Glob("web/templates/workboard/_*.html"); err == nil {
			files = append(files, partials...)
		}
		if name == "workboard/project_work_action.html" {
			files = appendExisting(files,
				"web/templates/as/_action_parts.html",
				"web/templates/as/_action_attach.html",
				"web/templates/as/_photo_gallery.html",
				"web/templates/as/_photo_zoom.html",
			)
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
	if strings.HasPrefix(name, "maintenance/") {
		if partials, err := filepath.Glob("web/templates/maintenance/_*.html"); err == nil {
			files = append(files, partials...)
		}
		files = appendExisting(files,
			"web/templates/workboard/_segments_form.html",
			"web/templates/workboard/_work_items.html",
		)
	}
	if strings.HasPrefix(name, "project/") {
		if partials, err := filepath.Glob("web/templates/project/_*.html"); err == nil {
			files = append(files, partials...)
		}
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
	files = appendExisting(files, "web/templates/_classify_hint.html", "web/templates/_customer_combo.html",
		"web/templates/_target_picker.html",
		"web/templates/_target_split.html",
		"web/templates/_site_multi_picker.html",
		"web/templates/_org_picker.html",
		"web/templates/_ui_assignee_picker.html",
		"web/templates/_project_year_dates.html",
		"web/templates/_partials/view_group_toggle.html",
		"web/templates/_partials/ui_dropdown.html",
		"web/templates/_partials/project_filter.html",
		"web/templates/_partials/project_picker.html",
		"web/templates/_partials/page_back.html")
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

	cache := "no-cache"
	if dataMap, ok := data.(map[string]interface{}); ok {
		if v, _ := dataMap["CacheControl"].(string); strings.TrimSpace(v) != "" {
			cache = strings.TrimSpace(v)
		}
	}
	c.Response().Header().Set("Cache-Control", cache)

	if data == nil {
		data = map[string]interface{}{}
	}
	// 로그인 상태·빌드 값 주입. 사이드바와 배지가 같은 값을 쓴다. §40.4
	if dataMap, ok := data.(map[string]interface{}); ok {
		injectBuildInfo(dataMap)
		injectFlash(c, dataMap)
		dataMap["CSRFToken"] = ensureCSRFToken(c)
		cfg := config.Load()
		dataMap["NavMail"] = cfg.MailEnabled
		dataMap["NavSMS"] = cfg.SMSEnabled
		dataMap["NavKakao"] = cfg.KakaoEnabled
		dataMap["ShowProjectsSector"] = cfg.ShowProjectsSector()
		// HideNav 는 사이드바 숨김 플래그일 뿐. 키가 있어도 로그인 값은 넣는다.
		// /work-status 가 HideNav=false 를 넣어도 키가 있어 UserRole 이 빠지고 500 이 났다.
		dataMap["UserName"] = ctxString(c, "user_name")
		dataMap["UserRole"] = currentRole(c)
		dataMap["LoginRole"] = loginRole(c)
		dataMap["Username"] = ctxString(c, "username")
		dataMap["UserID"] = ctxString(c, "user_id")
		dataMap["UserPerms"] = currentPerms(c)
		dataMap["IsReadOnly"] = isReadOnly(c)
		dataMap["IsTestUser"] = ctxBool(c, "is_test")
		if isReadOnly(c) {
			dataMap["CanWrite"] = false
			dataMap["CanReceive"] = false
			dataMap["CanProcess"] = false
			dataMap["KanbanDrag"] = false
		}
		if vid := strings.TrimSpace(ctxString(c, "view_as_user_id")); vid != "" {
			dataMap["ViewAsActive"] = true
			dataMap["ViewAsName"] = ctxString(c, "view_as_name")
			dataMap["ViewAsUserID"] = vid
			dataMap["CanWrite"] = false
			dataMap["CanReceive"] = false
			dataMap["CanProcess"] = false
			dataMap["CanDeleteSales"] = false
			dataMap["CanDrop"] = false
			dataMap["KanbanDrag"] = false
		}
		if canUseViewAs(c) {
			if v := c.Get("view_as_users"); v != nil {
				dataMap["ViewAsUsers"] = v
			}
		}
		injectAssignNoticeView(c, dataMap)
		injectLoginBriefingView(c, dataMap)
		injectOrgView(c, dataMap)
		injectNavMenu(c, dataMap)
		injectPageBack(c, dataMap)
		injectPickerNames(dataMap)
		if v := c.Get("auth_unconfirmed"); v != nil {
			if b, ok := v.(bool); ok && b {
				dataMap["AuthUnconfirmed"] = true
			}
		}
	}

	err = tmpl.ExecuteTemplate(w, blockName, data)
	if err != nil {
		log.Printf("템플릿 실행 실패 name=%s block=%s: %v", name, blockName, err)
	}
	return err
}

func injectOrgView(c echo.Context, dataMap map[string]interface{}) {
	var orgs []model.Org
	if v := c.Get("org_switch_list"); v != nil {
		if list, ok := v.([]model.Org); ok {
			orgs = list
			dataMap["OrgSwitchList"] = list
		}
	}
	dataMap["CanViewOrgs"] = canViewOrgs(c)
	dataMap["CanSwitchOrg"] = canSwitchOrg(c)
	dataMap["ViewOnlyBanner"] = DataScopeFor(c).ReadOnly
	id := currentOrg(c)
	dataMap["OrgViewID"] = id
	dataMap["OrgViewAll"] = id == repository.OrgAll
	dataMap["OrgViewLabel"] = orgViewLabel(c, orgs)
	if c.Request() != nil && c.Request().URL != nil {
		dataMap["ReturnPath"] = c.Request().URL.RequestURI()
	}
}

func injectPickerNames(dataMap map[string]interface{}) {
	if dataMap == nil {
		return
	}
	names, _ := dataMap["PickerNames"].(map[string]string)
	if names == nil {
		names = map[string]string{}
	}
	if list, ok := dataMap["Assignees"].([]model.User); ok {
		for id, n := range pickerNamesMap(list) {
			if strings.TrimSpace(names[id]) == "" || strings.HasPrefix(names[id], "USR-") {
				names[id] = n
			}
		}
	}
	dataMap["PickerNames"] = names
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

func ctxBool(c echo.Context, key string) bool {
	v, _ := c.Get(key).(bool)
	return v
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
		"add": func(a, b int) int { return a + b },
		"boardQ": func(base interface{}, key, val string) string {
			s := ""
			switch t := base.(type) {
			case template.URL:
				s = string(t)
			case string:
				s = t
			}
			return boardQueryWith(s, key, val)
		},
		"subtract":                 func(a, b int) int { return a - b },
		"dateLabelMDW":             model.DateLabelMDW,
		"leaveKindLabel":           model.LeaveKindLabel,
		"projectKindLabel":         model.ProjectKindLabel,
		"projectWritable":          repository.ProjectWritableInOrg,
		"yearStatusLabel":          model.YearStatusLabel,
		"siteStatusLabel":          model.SiteStatusLabel,
		"equipStatusLabel":         model.EquipStatusLabel,
		"projectDutyLabel":         model.ProjectDutyLabel,
		"assignmentSelected":       model.AssignmentSelected,
		"hasSuffix":                strings.HasSuffix,
		"urlquery":                 url.QueryEscape,
		"workboardHomeURL":         workboardHomeURL,
		"workboardNewWorkURL":      workboardNewWorkURL,
		"asHref":                   model.ASOpenHref,
		"boardReceipt":             boardReceiptText,
		"boardAction":              boardActionText,
		"boardItemHref":            boardItemHref,
		"boardBarMinutes":          boardBarMinutes,
		"projectMatchPendingBadge": projectMatchPendingBadge,
		"asLegacyHref":             model.ASLegacyHref,
		"asSymptomSnippet":         asSymptomSnippet,
		"weekdayKo":                weekdayKo,
		"visitPeopleIDs":           visitPeopleIDsCSV,
		"visitOwnerID":             visitOwnerUserID,
		"visitGroupIDs":            visitGroupIDsCSV,
		"pickerName":               pickerDisplayName,
		"pickerPersonLabel":        pickerPersonLabel,
		"pickerButtonLabel":        pickerButtonLabel,
		"compactPickerIDs":         compactPickerIDs,
		"chipID":                   chipID,
		"chipName":                 chipName,
		"assetChips":               assetChips,
		"toJSON": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
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
		"safeLen": func(v interface{}) int {
			if v == nil {
				return 0
			}
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
				return rv.Len()
			default:
				return 0
			}
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
				"admin_work": "행정/지원 이관",
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
				"admin_work": "행정/지원 이관",
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
				"admin_work":       "bg-violet-100 text-violet-800",
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
		"supportTypeLabel": model.SupportTypeLabel,
		"workKindBadgeClass": func(kind string) string {
			switch kind {
			case model.WorkKindConfirm:
				return "bg-purple-100 text-purple-800"
			case model.WorkKindSupport:
				return "bg-teal-100 text-teal-800"
			default:
				return "bg-sky-100 text-sky-800"
			}
		},
		"actionResultLabel":   model.ActionResultLabel,
		"formatKoreanTime":    model.FormatKoreanTime,
		"formatLocalDateTime": model.FormatLocalDateTime,
		"formatUTCDateTime":   model.FormatUTCDateTime,
		"transferDetailLabel": model.TransferDetailLabel,
		"workPrefixLabel": func(s string) string {
			return model.WorkPrefixLabel(s)
		},
		"coopKindLabel":   model.CoopKindLabel,
		"coopStatusLabel": model.CoopStatusLabel,
		"workAssignKey":   workAssignKey,
		"workEmbedHref":   workEmbedHref,
		"noticeEmbed": func(n model.AssignNotice) string {
			id := strings.TrimSpace(n.SourceID)
			switch n.SourceType {
			case model.AssignNoticeSourceAS:
				return asEmbedHref(id)
			case model.AssignNoticeSourceMaintenance:
				return "/maintenance/visits/" + id + "?embed=1"
			default:
				return "/workboard/tasks/" + id + "?embed=1"
			}
		},
		"noticeKey": func(n model.AssignNotice) string {
			id := strings.TrimSpace(n.SourceID)
			switch n.SourceType {
			case model.AssignNoticeSourceAS:
				return "as:" + id
			case model.AssignNoticeSourceMaintenance:
				return "mnt:" + id
			default:
				return "task:" + id
			}
		},
		"workPrefixClass": func(s string) string {
			switch s {
			case model.WorkPrefixAS:
				return "bg-blue-100 text-blue-800"
			case model.WorkPrefixConfirm:
				return "bg-purple-100 text-purple-800"
			case model.WorkPrefixSupport:
				return "bg-teal-100 text-teal-800"
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
		"partyKindLabel":    model.PartyKindLabel,
		"processTypePlaces": model.ProcessTypePlaces,
		"causeCat1Label": func(cat1, custom string) string {
			return model.CauseCat1Label(cat1, custom, nil)
		},
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
			return repository.ProductCategoryLabel(s)
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
			return model.ManagementTypeLabel(s)
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
			return model.VisitDeleteProtected(date, appclock.Now())
		},
		"maintCycleLabel": func(s string) string {
			return model.MaintCycleDisplayLabel(s)
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
		"appName": func() string {
			return model.AppName
		},
		"roleLabel": func(s string) string {
			return model.RoleLabel(s)
		},
		"canViewOrgs": func(data interface{}) bool {
			m, ok := data.(map[string]interface{})
			if !ok || m == nil {
				return false
			}
			b, _ := m["CanViewOrgs"].(bool)
			return b
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
		"userPermSource": func(u model.User) string {
			return model.PermissionSourceLabel(u.Role, u.Permissions)
		},
		"permGroups": func() []model.PermGroup {
			return model.PermissionGroups()
		},
		"permAccess": func(role, key string) int {
			return int(model.PermAccess(role, key))
		},
		"adminSectionPerm": func(key string) bool {
			return model.IsAdminSectionPerm(key)
		},
		"clevelAccess": func(key string) int {
			return int(model.ClampCLevelAccess(key, model.PermAccess(model.RoleCLevel, key)))
		},
		"builtinAccess": func(role, key string) int {
			return int(model.BuiltinAccess(role, key))
		},
		"accessLabel": func(v int) string {
			return model.AccessLabel(model.Access(v))
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
		"salesPeriodLabel":    model.SalesPeriodLabel,
		"quoteStatusLabel":    model.QuoteStatusLabel,
		"vatModeLabel":        model.VATModeLabel,
		"roundRuleLabel":      model.RoundRuleLabel,
		"quoteCutLabel":       model.QuoteCutDisplay,
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
