package handler

import "strings"

// oldSidebarLinks 75단계 이전 base.html 사이드바 주소.
// 채널: sidebar / header / tab(흡수 화면, 75-B·D) / drop(중복 주소).
type oldNavLink struct {
	Href    string
	Channel string
	Via     string
}

func oldSidebarCatalog() []oldNavLink {
	return []oldNavLink{
		{Href: "/", Channel: "sidebar"},
		{Href: "/work", Channel: "sidebar"},
		{Href: "/work/all", Channel: "sidebar"},
		{Href: "/workboard/register", Channel: "sidebar"},
		{Href: "/plan/unplanned", Channel: "tab", Via: "/workboard/register"},
		{Href: "/meeting", Channel: "sidebar"},
		{Href: "/admin-work", Channel: "tab", Via: "/work/all"},
		{Href: "/work-status", Channel: "tab", Via: "/stats"},
		{Href: "/leaves", Channel: "sidebar"},
		{Href: "/approvals/inbox", Channel: "sidebar"},
		{Href: "/as", Channel: "sidebar"},
		{Href: "/as/knowledge", Channel: "sidebar"},
		{Href: "/maintenance", Channel: "sidebar"},
		{Href: "/maintenance/sites", Channel: "sidebar"},
		{Href: "/maintenance/sites/distance", Channel: "sidebar"},
		{Href: "/sales/dashboard", Channel: "sidebar"},
		{Href: "/sales", Channel: "tab", Via: "/sales?view=pipeline"},
		{Href: "/sales/groups", Channel: "sidebar"},
		{Href: "/sales/activities", Channel: "sidebar"},
		{Href: "/sales?view=pipeline", Channel: "sidebar"},
		{Href: "/quotes", Channel: "sidebar"},
		{Href: "/orders", Channel: "tab", Via: "/quotes"},
		{Href: "/contracts", Channel: "tab", Via: "/quotes"},
		{Href: "/analysis", Channel: "sidebar"},
		{Href: "/projects/dashboard", Channel: "sidebar"},
		{Href: "/projects", Channel: "sidebar"},
		{Href: "/projects/docs", Channel: "sidebar"},
		{Href: "/projects/diary", Channel: "sidebar"},
		{Href: "/relations", Channel: "sidebar"},
		{Href: "/as/stats", Channel: "tab", Via: "/stats"},
		{Href: "/stats", Channel: "sidebar"},
		{Href: "/stats/milestones", Channel: "sidebar"},
		{Href: "/stats/reports", Channel: "sidebar"},
		{Href: "/customers", Channel: "sidebar"},
		{Href: "/spaces", Channel: "sidebar"},
		{Href: "/contacts", Channel: "sidebar"},
		{Href: "/contact-history", Channel: "sidebar"},
		{Href: "/assets", Channel: "sidebar"},
		{Href: "/items", Channel: "sidebar"},
		{Href: "/codes", Channel: "sidebar"},
		{Href: "/admin/users", Channel: "sidebar"},
		{Href: "/admin/my-org/members", Channel: "sidebar"},
		{Href: "/admin/holidays", Channel: "sidebar"},
		{Href: "/admin/approval-settings", Channel: "sidebar"},
		{Href: "/admin/orgs", Channel: "sidebar"},
		{Href: "/admin/permissions", Channel: "sidebar"},
		{Href: "/admin/data", Channel: "sidebar"},
		{Href: "/admin/test-stats", Channel: "sidebar"},
		{Href: "/admin/system", Channel: "sidebar"},
		{Href: "/admin/mail", Channel: "sidebar"},
		{Href: "/admin/notify", Channel: "sidebar"},
		{Href: "/admin/sales-logs", Channel: "sidebar"},
		{Href: "/account", Channel: "header"},
	}
}

func oldSidebarUnreachable(ctx MenuCtx) []string {
	res := FilterNav(ctx, NavDashboard, NavBadges{})
	have := map[string]struct{}{}
	for _, h := range collectNavHrefs(res.Groups) {
		have[h] = struct{}{}
	}
	var missing []string
	for _, l := range oldSidebarCatalog() {
		switch l.Channel {
		case "sidebar":
			if _, ok := have[l.Href]; !ok {
				// 메일·사업관리는 플래그/설정에 따름
				if l.Href == "/admin/mail" && !ctx.NavMail {
					continue
				}
				if strings.HasPrefix(l.Href, "/projects") && !ctx.ShowProjects {
					continue
				}
				missing = append(missing, l.Href)
			}
		case "header", "tab", "drop":
			// 분류만. 탭 실체는 75-B·D.
		default:
			missing = append(missing, "unclassified:"+l.Href)
		}
	}
	return missing
}
