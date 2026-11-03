package handler

import (
	"net/url"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

const pageBackHome = "/"

const (
	pageBackKindBack    = "back"
	pageBackKindParent  = "parent"
	pageBackKindHistory = "history"
	pageBackKindHome    = "home"
)

// pageParentExact 사이드바·목록 화면 → 대시보드(또는 지정 부모).
var pageParentExact = map[string]string{
	"/":                           "",
	"/workboard/board":            pageBackHome,
	"/workboard":                  "/workboard/board",
	"/workboard/register":         "/workboard/board",
	"/workboard/kanban":           "/workboard/board",
	"/workboard/tasks":            "/workboard/board",
	"/work":                       pageBackHome,
	"/work/all":                   pageBackHome,
	"/work-groups":                pageBackHome,
	"/work-status":                "/stats",
	"/plan/unplanned":             "/workboard/register",
	"/meeting":                    pageBackHome,
	"/meeting/board":              pageBackHome,
	"/meeting/weekly":             pageBackHome,
	"/meeting/weekly/me":          pageBackHome,
	"/approvals/inbox":            pageBackHome,
	"/leaves":                     pageBackHome,
	"/admin-work":                 pageBackHome,
	"/as":                         pageBackHome,
	"/as/knowledge":               "/as",
	"/maintenance":                pageBackHome,
	"/maintenance/sites":          "/maintenance",
	"/maintenance/products":       "/maintenance",
	"/maintenance/sites/distance": "/maintenance",
	"/customers":                  pageBackHome,
	"/assets":                     pageBackHome,
	"/contacts":                   pageBackHome,
	"/contact-history":            pageBackHome,
	"/spaces":                     pageBackHome,
	"/relations":                  pageBackHome,
	"/sales":                      pageBackHome,
	"/sales/dashboard":            pageBackHome,
	"/sales/activities":           pageBackHome,
	"/sales/groups":               pageBackHome,
	"/quotes":                     pageBackHome,
	"/orders":                     "/quotes",
	"/contracts":                  "/quotes",
	"/labor-rates":                "/quotes",
	"/analysis":                   pageBackHome,
	"/projects":                   pageBackHome,
	"/projects/dashboard":         pageBackHome,
	"/projects/docs":              pageBackHome,
	"/projects/diary":             pageBackHome,
	"/projects/registry":          pageBackHome,
	"/projects/import":            "/projects",
	"/cooperations/inbox":         "/projects",
	"/cooperations/sent":          "/projects",
	"/stats":                      pageBackHome,
	"/stats/milestones":           "/stats",
	"/stats/reports":              "/stats",
	"/stats/company-weekly":       pageBackHome,
	"/items":                      pageBackHome,
	"/codes":                      pageBackHome,
	"/product-groups":             pageBackHome,
	"/work-item-templates":        pageBackHome,
	"/account":                    pageBackHome,
	"/admin/users":                pageBackHome,
	"/admin/my-org/members":       pageBackHome,
	"/admin/holidays":             pageBackHome,
	"/admin/approval-settings":    pageBackHome,
	"/admin/mail":                 pageBackHome,
	"/admin/notify":               pageBackHome,
	"/admin/sales-logs":           pageBackHome,
	"/admin/data":                 pageBackHome,
	"/admin/orgs":                 "/admin/users",
	"/admin/permissions":          "/admin/users",
	"/admin/system":               pageBackHome,
	"/admin/test-stats":           pageBackHome,
}

// pageParentPrefix 긴 것부터 매칭. 상세·조치 화면.
var pageParentPrefix = []struct {
	prefix string
	parent string
}{
	{"/workboard/tasks/", "/workboard/board"},
	{"/maintenance/visits/", "/workboard/board"},
	{"/admin-work/", "/admin-work"},
	{"/work-groups/", "/work-groups"},
	{"/as/knowledge/", "/as/knowledge"},
	{"/as/cause-categories", "/as"},
	{"/product-groups/", "/product-groups"},
	{"/work-item-templates/", "/work-item-templates"},
	{"/assets/", "/assets"},
	{"/items/", "/items"},
	{"/customers/", "/customers"},
	{"/as/work/", "/as"},
	{"/contacts/", "/contacts"},
	{"/spaces/", "/spaces"},
	{"/sales/groups/", "/sales/groups"},
	{"/sales/activities/", "/sales/activities"},
	{"/quotes/", "/quotes"},
	{"/orders/", "/orders"},
	{"/contracts/", "/contracts"},
	{"/labor-rates/", "/labor-rates"},
	{"/projects/equipment-categories", "/projects"},
	{"/projects/", "/projects"},
	{"/stats/company-weekly/", "/stats/company-weekly"},
	{"/stats/", "/stats"},
	{"/meeting/", "/meeting"},
	{"/leaves/", "/leaves"},
	{"/approvals/", "/approvals/inbox"},
	{"/admin/users/", "/admin/users"},
	{"/maintenance/", "/maintenance"},
	{"/sales/", "/sales"},
	{"/as/", "/as"},
}

func normalizePagePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if q := strings.IndexByte(path, '?'); q >= 0 {
		path = path[:q]
	}
	if i := strings.IndexByte(path, '#'); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	if !strings.HasPrefix(path, "/") {
		return "/"
	}
	return path
}

// ParentPage 라우트별 부모 화면. 대시보드는 빈 문자열.
func ParentPage(path string) string {
	path = normalizePagePath(path)
	if p, ok := pageParentExact[path]; ok {
		return p
	}
	if strings.HasPrefix(path, "/as/") && strings.Contains(path, "/report") {
		id := strings.TrimPrefix(path, "/as/")
		if i := strings.IndexByte(id, '/'); i > 0 {
			id = id[:i]
		}
		if id != "" && id != "work" && id != "knowledge" {
			return "/as/" + id + "/action"
		}
	}
	if strings.HasPrefix(path, "/assets/") && strings.HasSuffix(path, "/sw") {
		return strings.TrimSuffix(path, "/sw")
	}
	if strings.Contains(path, "/action") &&
		(strings.HasPrefix(path, "/as/") || strings.HasPrefix(path, "/maintenance/visits/") || strings.HasPrefix(path, "/workboard/tasks/")) &&
		!strings.HasPrefix(path, "/as/work/") {
		return "/workboard/board"
	}
	for _, r := range pageParentPrefix {
		if path == strings.TrimSuffix(r.prefix, "/") || strings.HasPrefix(path, r.prefix) {
			return r.parent
		}
	}
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	if path != "/" {
		return pageBackHome
	}
	return ""
}

// SafePageBack 같은 오리진 상대 경로만. 불가하면 fallback(빈 값 허용).
func SafePageBack(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if strings.ContainsAny(raw, "\r\n\t") || strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || !strings.HasPrefix(u.Path, "/") {
		return fallback
	}
	if strings.Contains(u.Path, "..") {
		return fallback
	}
	p := normalizePagePath(u.Path)
	switch p {
	case "/login", "/logout", "/signup":
		return fallback
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}

func safeBackFromReferer(referer, currentPath string) string {
	referer = strings.TrimSpace(referer)
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	cand := u.Path
	if u.RawQuery != "" {
		cand = u.Path + "?" + u.RawQuery
	}
	got := SafePageBack(cand, "")
	if got == "" {
		return ""
	}
	if normalizePagePath(u.Path) == normalizePagePath(currentPath) {
		return ""
	}
	return got
}

// ResolvePageBack §3: 안전한 back → 부모 맵 → history.back(href는 대시보드) → 대시보드.
func ResolvePageBack(currentPath, backQuery, referer string) (href, kind string) {
	cur := normalizePagePath(currentPath)
	if b := strings.TrimSpace(backQuery); b != "" {
		if got := SafePageBack(b, ""); got != "" {
			return got, pageBackKindBack
		}
	}
	if ref := safeBackFromReferer(referer, cur); ref != "" {
		return ref, pageBackKindBack
	}
	if p := ParentPage(cur); p != "" {
		return p, pageBackKindParent
	}
	if cur == "/" || cur == "" {
		return pageBackHome, pageBackKindHistory
	}
	return pageBackHome, pageBackKindHome
}

func hidePageBack(c echo.Context, data map[string]interface{}) bool {
	if data != nil {
		if v, ok := data["HideNav"].(bool); ok && v {
			return true
		}
	}
	if c == nil || c.Request() == nil || c.Request().URL == nil {
		return false
	}
	switch normalizePagePath(c.Request().URL.Path) {
	case "/login", "/logout", "/signup":
		return true
	}
	return false
}

func injectPageBack(c echo.Context, data map[string]interface{}) {
	if data == nil {
		return
	}
	if hidePageBack(c, data) {
		data["HidePageBack"] = true
		return
	}
	path, backQ, ref := "", "", ""
	if c != nil && c.Request() != nil {
		if c.Request().URL != nil {
			path = c.Request().URL.Path
		}
		backQ = strings.TrimSpace(c.QueryParam("back"))
		if backQ == "" {
			backQ = strings.TrimSpace(c.FormValue("back"))
		}
		ref = c.Request().Referer()
	}
	href, kind := ResolvePageBack(path, backQ, ref)
	data["HidePageBack"] = false
	data["PageBackHref"] = href
	data["PageBackKind"] = kind
}

func pageParentTable() [][2]string {
	var paths []string
	for p := range pageParentExact {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var rows [][2]string
	for _, p := range paths {
		rows = append(rows, [2]string{p, pageParentExact[p]})
	}
	for _, r := range pageParentPrefix {
		rows = append(rows, [2]string{r.prefix + "*", r.parent})
	}
	return rows
}
