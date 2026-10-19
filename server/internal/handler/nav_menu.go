package handler

import (
	"encoding/json"
	"html/template"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// MenuCtx 사이드바 권한. base.html 에 있던 역할 판정을 그대로 옮긴다.
type MenuCtx struct {
	Role         string
	LoginRole    string
	Perms        []string
	IsReadOnly   bool
	IsTestUser   bool
	ShowProjects bool
	NavMail      bool
	CanViewOrgs  bool
}

func (c MenuCtx) isVision() bool { return c.Role == model.RoleVisionAdmin }
func (c MenuCtx) isOrgAdmin() bool {
	return c.Role == model.RoleOrgAdmin || c.Role == model.RoleAdmin
}
func (c MenuCtx) isAdmin() bool { return c.isVision() || c.isOrgAdmin() }
func (c MenuCtx) isOffice() bool {
	return c.Role == model.RoleSupport || c.Role == model.RoleOffice || c.Role == "receipt"
}
func (c MenuCtx) isTech() bool  { return c.Role == model.RoleTech }
func (c MenuCtx) isSales() bool { return c.Role == model.RoleSales }
func (c MenuCtx) isObserver() bool {
	return c.Role == model.RoleObserver || c.IsReadOnly
}
func (c MenuCtx) isTester() bool {
	return c.Role == model.RoleTester || c.IsTestUser
}
func (c MenuCtx) loginObserver() bool {
	return c.LoginRole == model.RoleObserver || c.IsReadOnly
}
func (c MenuCtx) roleKnown() bool {
	return c.isAdmin() || c.isOffice() || c.isTech() || c.isSales() || c.isObserver() || c.isTester()
}
func (c MenuCtx) has(keys ...string) bool {
	for _, k := range keys {
		if model.HasPermission(c.Perms, k) {
			return true
		}
	}
	return false
}

func (c MenuCtx) visRole() bool { return c.roleKnown() }
func (c MenuCtx) visMnt() bool {
	return c.isAdmin() || c.isTech() || c.isOffice() || c.isSales() || c.isObserver() || c.isTester() ||
		c.has(model.PermMntView, model.PermMaintenance)
}
func (c MenuCtx) visMntSetup() bool {
	return c.isAdmin() || c.isObserver() || c.has(model.PermCodesEdit, model.PermCodesUsers)
}
func (c MenuCtx) visAnalysis() bool {
	return c.isAdmin() || c.isSales() || c.isObserver() || c.isOffice() || c.isTech() ||
		c.has(model.PermSalesView, model.PermAnalysis)
}
func (c MenuCtx) visProjects() bool {
	if !c.ShowProjects {
		return false
	}
	return c.roleKnown() || c.isObserver() || c.has(model.PermAdminCreate, model.PermWorkboard)
}
func (c MenuCtx) visStatsGroup() bool {
	return c.roleKnown() || c.isSales() || c.isObserver() ||
		c.has(model.PermStatsView, model.PermSalesView, model.PermStats, model.PermAnalysis)
}
func (c MenuCtx) visStatsCore() bool {
	return c.roleKnown() || c.isObserver() || c.has(model.PermStatsView, model.PermStats)
}
func (c MenuCtx) visMasterView() bool {
	return c.isAdmin() || c.isObserver() || c.isOffice() || c.isTech() || c.isSales() || c.isTester() ||
		c.has(model.PermMasterView, model.PermMasterEdit, model.PermMasterWrite)
}
func (c MenuCtx) visMasterEdit() bool {
	return c.isAdmin() || c.isObserver() || c.isOffice() || c.isTech() || c.isSales() || c.isTester() ||
		c.has(model.PermMasterEdit, model.PermMasterWrite)
}
func (c MenuCtx) visCodes() bool {
	return c.isAdmin() || c.isObserver() || c.loginObserver() || c.has(model.PermCodesEdit, model.PermCodesUsers)
}
func (c MenuCtx) visAdmin() bool  { return c.isAdmin() }
func (c MenuCtx) visVision() bool { return c.isVision() }
func (c MenuCtx) visOrgs() bool   { return c.CanViewOrgs }
func (c MenuCtx) visMail() bool   { return c.NavMail }

type navVis func(MenuCtx) bool

type navDefItem struct {
	ID       string
	Label    string
	Href     string
	Active   []string
	Show     navVis
	More     bool
	Badge    string // assign | approvals | leaves | account
	Children []navDefItem
}

type navDefGroup struct {
	ID      string
	Label   string
	Show    navVis
	Default bool
	Items   []navDefItem
}

// NavBadges 접힌 부모에도 보이는 숫자.
type NavBadges struct {
	Assign     int
	Approvals  int
	Leaves     int
	AccountReq int
}

type NavItemView struct {
	ID       string
	Label    string
	Href     string
	Active   bool
	Badge    int
	Children []NavItemView
}

type NavGroupView struct {
	ID    string
	Label string
	Count int
	Items []NavItemView
	More  []NavItemView
}

type NavFilterResult struct {
	Groups     []NavGroupView
	CurGroup   string
	CurMore    []string
	CurParents []string
}

func navDefinition() []navDefGroup {
	role := func(c MenuCtx) bool { return c.visRole() }
	return []navDefGroup{
		{
			ID: "work", Label: "업무관리", Show: role, Default: true,
			Items: []navDefItem{
				{ID: "dashboard", Label: "대시보드", Href: "/", Active: []string{NavDashboard}, Show: role},
				{ID: "register", Label: "일일 업무 등록", Href: "/workboard/register", Active: []string{NavWorkRegister}, Show: role},
				{ID: "work_review", Label: "업무 확인", Show: role, Badge: "assign", Children: []navDefItem{
					{ID: "work_today", Label: "오늘 업무", Href: "/work", Active: []string{NavWork, NavWorkPlan}, Show: role, Badge: "assign"},
					{ID: "work_all", Label: "전체 업무", Href: "/work/all", Active: []string{NavWorkAll}, Show: role},
				}},
				{ID: "work_meeting", Label: "업무 회의", Show: role, Children: []navDefItem{
					{ID: "meeting", Label: "일일 업무 회의", Href: "/meeting", Active: []string{NavMeeting}, Show: role},
					{ID: "meeting_weekly", Label: "주간 업무 회의", Href: "/meeting/weekly", Active: []string{NavMeetingWeekly}, Show: role},
				}},
				{ID: "approval", Label: "결재", Show: role, Badge: "approvals_sum", Children: []navDefItem{
					{ID: "approvals_inbox", Label: "결재할 문서", Href: "/approvals/inbox", Active: []string{NavApprovalsInbox, NavApprovalsHub}, Show: role, Badge: "approvals"},
					{ID: "leaves", Label: "휴가 신청", Href: "/leaves", Active: []string{NavLeaves}, Show: role, Badge: "leaves"},
				}},
			},
		},
		{
			ID: "customer", Label: "고객관리", Show: role,
			Items: []navDefItem{
				{ID: "as", Label: "AS 접수·조치", Href: "/as", Active: []string{NavAS}, Show: role},
				{ID: "as_kb", Label: "AS 사례 검색", Href: "/as/knowledge", Active: []string{NavASKnowledge}, Show: role},
				{ID: "mnt", Label: "정기점검", Href: "/maintenance", Active: []string{NavMaintenance}, Show: func(c MenuCtx) bool { return c.visMnt() }},
				{ID: "customers", Label: "고객현황", Href: "/customers", Active: []string{NavCustomers}, Show: role},
				{ID: "assets", Label: "설치자산", Href: "/assets", Active: []string{NavAssets}, Show: role},
				{ID: "contacts", Label: "담당자관리", Href: "/contacts", Active: []string{NavContacts}, Show: func(c MenuCtx) bool { return c.visMasterView() }, More: true},
				{ID: "contact_history", Label: "담당자이력", Href: "/contact-history", Active: []string{NavContactHistory}, Show: func(c MenuCtx) bool { return c.visMasterView() }, More: true},
				{ID: "spaces", Label: "공간관리", Href: "/spaces", Active: []string{NavSpaces}, Show: func(c MenuCtx) bool { return c.visMasterView() }, More: true},
				{ID: "relations", Label: "수행관계", Href: "/relations", Active: []string{NavRelations}, Show: func(c MenuCtx) bool { return c.visMasterEdit() }, More: true},
				{ID: "mnt_sites", Label: "점검 사이트 설정", Href: "/maintenance/sites", Active: []string{NavMaintenanceSites}, Show: func(c MenuCtx) bool { return c.visMntSetup() }, More: true},
				{ID: "region_order", Label: "지역 거리 순서", Href: "/maintenance/sites/distance", Active: []string{NavRegionOrder}, Show: func(c MenuCtx) bool { return c.visMntSetup() }, More: true},
			},
		},
		{
			ID: "sales", Label: "영업관리", Show: role,
			Items: []navDefItem{
				{ID: "sales_dash", Label: "영업 대시보드", Href: "/sales/dashboard", Active: []string{NavSalesDashboard}, Show: role},
				{ID: "sales_pipe", Label: "파이프라인", Href: "/sales?view=pipeline", Active: []string{NavSalesPipeline, NavSales}, Show: role},
				{ID: "sales_act", Label: "영업 활동", Href: "/sales/activities", Active: []string{NavSalesActivities}, Show: role},
				{ID: "docs_hub", Label: "견적·수주·계약", Href: "/quotes", Active: []string{NavQuotes, NavOrders, NavContracts, NavDocsHub}, Show: role},
				{ID: "sales_groups", Label: "사업 대분류", Href: "/sales/groups", Active: []string{NavSalesGroups}, Show: role, More: true},
				{ID: "analysis", Label: "교체대상현황", Href: "/analysis", Active: []string{NavAnalysis}, Show: func(c MenuCtx) bool { return c.visAnalysis() }, More: true},
			},
		},
		{
			ID: "project", Label: "사업관리", Show: func(c MenuCtx) bool { return c.visProjects() },
			Items: []navDefItem{
				{ID: "proj_dash", Label: "사업 대시보드", Href: "/projects/dashboard", Active: []string{NavProjectsDashboard}, Show: func(c MenuCtx) bool { return c.visProjects() }},
				{ID: "projects", Label: "사업·업무관리", Href: "/projects", Active: []string{NavProjects}, Show: func(c MenuCtx) bool { return c.visProjects() }},
				{ID: "proj_docs", Label: "서류 작성·관리", Href: "/projects/docs", Active: []string{NavProjectsDocs}, Show: func(c MenuCtx) bool { return c.visProjects() }},
				{ID: "proj_diary", Label: "업무일지·투입관리", Href: "/projects/diary", Active: []string{NavProjectsDiary}, Show: func(c MenuCtx) bool { return c.visProjects() }},
				{ID: "proj_reg", Label: "사업 등록 관리", Href: "/projects/registry", Active: []string{NavProjectsRegistry}, Show: func(c MenuCtx) bool { return c.visProjects() }},
			},
		},
		{
			ID: "stats", Label: "통계·보고서", Show: func(c MenuCtx) bool { return c.visStatsGroup() },
			Items: []navDefItem{
				{ID: "stats", Label: "통계", Href: "/stats", Active: []string{NavStats}, Show: func(c MenuCtx) bool { return c.visStatsCore() }},
				{ID: "milestones", Label: "마일스톤", Href: "/stats/milestones", Active: []string{NavStatsMilestones}, Show: func(c MenuCtx) bool { return c.visStatsCore() }},
				{ID: "reports", Label: "보고서", Href: "/stats/reports", Active: []string{NavStatsReports, NavStatsDetail}, Show: func(c MenuCtx) bool { return c.visStatsCore() }},
			},
		},
		{
			ID: "admin", Label: "설정·관리", Show: func(c MenuCtx) bool { return c.visAdmin() || c.visRole() },
			Items: []navDefItem{
				{ID: "users", Label: "사용자/권한", Href: "/admin/users", Active: []string{NavUsers}, Show: func(c MenuCtx) bool { return c.visAdmin() }, Badge: "account"},
				{ID: "members", Label: "소속 인원 관리", Href: "/admin/my-org/members", Active: []string{NavMembers}, Show: func(c MenuCtx) bool { return c.visAdmin() }},
				{ID: "holidays", Label: "휴무일 관리", Href: "/admin/holidays", Active: []string{NavHolidays}, Show: func(c MenuCtx) bool { return c.visAdmin() }},
				{ID: "approval_settings", Label: "결재 설정", Href: "/admin/approval-settings", Active: []string{NavApproval}, Show: func(c MenuCtx) bool { return c.visAdmin() }},
				{ID: "items", Label: "품목 마스터", Href: "/items", Active: []string{NavSalesItems}, Show: role},
				{ID: "codes", Label: "코드관리", Href: "/codes", Active: []string{NavCodes}, Show: func(c MenuCtx) bool { return c.visCodes() }},
				{ID: "mail", Label: "메일 보낸 함", Href: "/admin/mail", Active: []string{NavMail}, Show: func(c MenuCtx) bool { return c.visAdmin() && c.visMail() }, More: true},
				{ID: "notify", Label: "알림 설정", Href: "/admin/notify", Active: []string{NavNotify}, Show: func(c MenuCtx) bool { return c.visAdmin() }, More: true},
				{ID: "sales_logs", Label: "영업 로그 관리", Href: "/admin/sales-logs", Active: []string{NavSalesLogs}, Show: func(c MenuCtx) bool { return c.visAdmin() }, More: true},
				{ID: "data", Label: "데이터 관리", Href: "/admin/data", Active: []string{NavData, NavBackup}, Show: func(c MenuCtx) bool { return c.visVision() }, More: true},
				{ID: "orgs", Label: "전체 조직 관리", Href: "/admin/orgs", Active: []string{NavOrgs}, Show: func(c MenuCtx) bool { return c.visOrgs() }, More: true},
				{ID: "permissions", Label: "역할 및 권한", Href: "/admin/permissions", Active: []string{NavPermissions}, Show: func(c MenuCtx) bool { return c.visVision() }, More: true},
				{ID: "system", Label: "시스템 정보", Href: "/admin/system", Active: []string{NavSystem}, Show: func(c MenuCtx) bool { return c.visVision() }, More: true},
				{ID: "test_stats", Label: "테스터 통계", Href: "/admin/test-stats", Active: []string{NavTestStats}, Show: func(c MenuCtx) bool { return c.visVision() }, More: true},
			},
		},
	}
}

func itemMatches(it navDefItem, active string) bool {
	if active == "" {
		return false
	}
	for _, k := range it.Active {
		if k == active {
			return true
		}
	}
	for _, ch := range it.Children {
		if itemMatches(ch, active) {
			return true
		}
	}
	return false
}

func badgeValue(kind string, b NavBadges) int {
	switch kind {
	case "assign":
		return b.Assign
	case "approvals":
		return b.Approvals
	case "leaves":
		return b.Leaves
	case "approvals_sum":
		return b.Approvals + b.Leaves
	case "account":
		return b.AccountReq
	}
	return 0
}

func viewItem(it navDefItem, active string, b NavBadges) NavItemView {
	v := NavItemView{ID: it.ID, Label: it.Label, Href: it.Href, Badge: badgeValue(it.Badge, b)}
	if len(it.Children) == 0 {
		v.Active = itemMatches(it, active)
		return v
	}
	for _, ch := range it.Children {
		v.Children = append(v.Children, viewItem(ch, active, b))
	}
	return v
}

func FilterNav(ctx MenuCtx, active string, badges NavBadges) NavFilterResult {
	var out NavFilterResult
	for _, g := range navDefinition() {
		if g.Show != nil && !g.Show(ctx) {
			continue
		}
		gv := NavGroupView{ID: g.ID, Label: g.Label}
		matched := false
		moreHit := false
		for _, it := range g.Items {
			if it.Show != nil && !it.Show(ctx) {
				continue
			}
			if len(it.Children) > 0 {
				filtered := it
				filtered.Children = nil
				for _, ch := range it.Children {
					if ch.Show != nil && !ch.Show(ctx) {
						continue
					}
					filtered.Children = append(filtered.Children, ch)
				}
				if len(filtered.Children) == 0 {
					continue
				}
				it = filtered
			}
			vi := viewItem(it, active, badges)
			hit := itemMatches(it, active)
			if hit {
				matched = true
				if it.More {
					moreHit = true
				}
				if len(it.Children) > 0 {
					out.CurParents = append(out.CurParents, it.ID)
				}
			}
			if it.More {
				gv.More = append(gv.More, vi)
			} else {
				gv.Items = append(gv.Items, vi)
			}
		}
		gv.Count = len(gv.Items) + len(gv.More)
		if gv.Count == 0 {
			continue
		}
		if matched {
			out.CurGroup = g.ID
			if moreHit {
				out.CurMore = append(out.CurMore, g.ID)
			}
		}
		out.Groups = append(out.Groups, gv)
	}
	return out
}

func navAllHrefs(includeEmpty bool) []string {
	var out []string
	seen := map[string]int{}
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if it.Href != "" || includeEmpty {
				out = append(out, it.Href)
				seen[it.Href]++
			}
			walk(it.Children)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
	return out
}

func navDuplicateHrefs() []string {
	count := map[string]int{}
	for _, h := range navAllHrefs(false) {
		if h == "" {
			continue
		}
		count[h]++
	}
	var dups []string
	for h, n := range count {
		if n > 1 {
			dups = append(dups, h)
		}
	}
	return dups
}

func navSidebarKeys() []string {
	seen := map[string]struct{}{}
	var out []string
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			for _, k := range it.Active {
				if _, ok := seen[k]; !ok {
					seen[k] = struct{}{}
					out = append(out, k)
				}
			}
			walk(it.Children)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
	return out
}

func collectNavHrefs(groups []NavGroupView) []string {
	var out []string
	var walk func([]NavItemView)
	walk = func(items []NavItemView) {
		for _, it := range items {
			if it.Href != "" {
				out = append(out, it.Href)
			}
			walk(it.Children)
		}
	}
	for _, g := range groups {
		walk(g.Items)
		walk(g.More)
	}
	return out
}

func countNavActive(groups []NavGroupView) int {
	n := 0
	var walk func([]NavItemView)
	walk = func(items []NavItemView) {
		for _, it := range items {
			if it.Active {
				n++
			}
			walk(it.Children)
		}
	}
	for _, g := range groups {
		walk(g.Items)
		walk(g.More)
	}
	return n
}

func menuCtxFromData(c echo.Context, data map[string]interface{}) MenuCtx {
	ctx := MenuCtx{
		Role:         currentRole(c),
		LoginRole:    loginRole(c),
		Perms:        currentPerms(c),
		IsReadOnly:   isReadOnly(c),
		IsTestUser:   ctxBool(c, "is_test"),
		NavMail:      boolFrom(data, "NavMail"),
		ShowProjects: boolFrom(data, "ShowProjectsSector"),
		CanViewOrgs:  boolFrom(data, "CanViewOrgs"),
	}
	if ctx.Role == "" {
		ctx.Role, _ = data["UserRole"].(string)
	}
	if ctx.LoginRole == "" {
		ctx.LoginRole, _ = data["LoginRole"].(string)
	}
	if len(ctx.Perms) == 0 {
		switch v := data["UserPerms"].(type) {
		case []string:
			ctx.Perms = v
		}
	}
	return ctx
}

func boolFrom(data map[string]interface{}, k string) bool {
	b, _ := data[k].(bool)
	return b
}

func intFromData(data map[string]interface{}, k string) int {
	switch v := data[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func jsJSON(v interface{}) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(b)
}

var fillApprovalNavBadges func(c echo.Context, data map[string]interface{})

func injectNavMenu(c echo.Context, data map[string]interface{}) {
	if fillApprovalNavBadges != nil {
		fillApprovalNavBadges(c, data)
	}
	active, _ := data["Active"].(string)
	badges := NavBadges{
		Assign:     intFromData(data, "AssignNoticeUnread"),
		AccountReq: intFromData(data, "AccountRequestPending"),
		Approvals:  intFromData(data, "ApprovalPendingCount"),
		Leaves:     intFromData(data, "LeavePendingCount"),
	}
	data["ApprovalPendingCount"] = badges.Approvals
	data["LeavePendingCount"] = badges.Leaves
	res := FilterNav(menuCtxFromData(c, data), active, badges)
	if res.CurMore == nil {
		res.CurMore = []string{}
	}
	if res.CurParents == nil {
		res.CurParents = []string{}
	}
	data["NavGroups"] = res.Groups
	data["NavCurGroup"] = res.CurGroup
	data["NavCurMoreJSON"] = jsJSON(res.CurMore)
	data["NavCurParentsJSON"] = jsJSON(res.CurParents)
}

func parentNavHrefs() []string {
	var empty []string
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if len(it.Children) > 0 && strings.TrimSpace(it.Href) != "" {
				empty = append(empty, it.ID+":"+it.Href)
			}
			if len(it.Children) > 0 && it.Href != "" {
				empty = append(empty, it.Href)
			}
			walk(it.Children)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
	return empty
}
