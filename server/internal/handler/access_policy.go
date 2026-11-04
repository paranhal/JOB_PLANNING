package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// DataScope 한 요청의 조회·쓰기 조직 범위. 95-99. 세션에 캐시하지 않는다.
type DataScope struct {
	All      bool
	OrgIDs   []string
	ReadOnly bool
}

// DataScopeFor 기본 정책 표. currentOrg·canSwitchOrg·isReadOnly 를 내부에서만 쓴다.
func DataScopeFor(c echo.Context) DataScope {
	role := currentRole(c)
	ro := isReadOnly(c)
	home := strings.TrimSpace(ctxString(c, "sim_org_id"))
	if home == "" || home == repository.OrgAll {
		home = loginHomeOrg(c)
	}
	if home == "" {
		home = strings.TrimSpace(ctxString(c, "org_id"))
	}
	if home == repository.OrgAll {
		home = ""
	}

	switch role {
	case model.RoleVisionAdmin, model.RoleCLevel:
		return DataScope{All: true, ReadOnly: ro || role == model.RoleCLevel}
	case model.RoleOrgAdmin:
		ids := []string{}
		if home != "" {
			ids = append(ids, home)
		} else if o := currentOrg(c); o != "" && o != repository.OrgAll {
			ids = append(ids, o)
		}
		ids = append(ids, divisionChildOrgIDs(c, home)...)
		return DataScope{OrgIDs: uniqOrgIDs(ids), ReadOnly: ro}
	default:
		id := home
		if id == "" {
			if o := currentOrg(c); o != "" && o != repository.OrgAll {
				id = o
			}
		}
		ids := []string{}
		if id != "" {
			ids = []string{id}
		}
		return DataScope{OrgIDs: ids, ReadOnly: ro || role == model.RoleObserver}
	}
}

// OrgScopeIDs 95-77 얇은 감싸기. All 이면 nil(전체).
func OrgScopeIDs(c echo.Context) []string {
	s := DataScopeFor(c)
	if s.All {
		return nil
	}
	out := make([]string, len(s.OrgIDs))
	copy(out, s.OrgIDs)
	return out
}

func CanViewOrg(c echo.Context, orgID string) bool {
	orgID = strings.TrimSpace(orgID)
	s := DataScopeFor(c)
	if s.All {
		return true
	}
	if orgID == "" {
		return false
	}
	for _, id := range s.OrgIDs {
		if id == orgID {
			return true
		}
	}
	return false
}

func CanWriteOrg(c echo.Context, orgID string) bool {
	s := DataScopeFor(c)
	if s.ReadOnly {
		return false
	}
	return CanViewOrg(c, orgID)
}

func CanViewRecord(c echo.Context, rec any) bool {
	org := orgIDOf(rec)
	if org == "" || org == repository.OrgAll {
		return DataScopeFor(c).All
	}
	return CanViewOrg(c, org)
}

func denyUnlessCanView(c echo.Context, rec any) error {
	if CanViewRecord(c, rec) {
		return nil
	}
	return echo.ErrNotFound
}

func scopeOrgForRead(c echo.Context) string {
	if DataScopeFor(c).All {
		return repository.OrgAll
	}
	return currentOrg(c)
}

func orgIDOf(rec any) string {
	switch v := rec.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case *model.ASReceipt:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.OrgID)
	case model.ASReceipt:
		return strings.TrimSpace(v.OrgID)
	case *model.WorkTask:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.OrgID)
	case model.WorkTask:
		return strings.TrimSpace(v.OrgID)
	case *model.WorkProject:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.OrgID)
	case model.WorkProject:
		return strings.TrimSpace(v.OrgID)
	case interface{ GetOrgID() string }:
		return strings.TrimSpace(v.GetOrgID())
	default:
		return ""
	}
}

func divisionChildOrgIDs(c echo.Context, parent string) []string {
	parent = strings.TrimSpace(parent)
	if parent == "" || c == nil {
		return nil
	}
	v := c.Get("org_child_ids")
	m, ok := v.(map[string][]string)
	if !ok || m == nil {
		return nil
	}
	return m[parent]
}

func uniqOrgIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func normalizePolicyPath(path string) string {
	p := strings.TrimSpace(path)
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// cLevelWriteBlocked GET 조회는 열고, 쓰기·등록·수정 화면만 막는다. 95-99 §3-2.
func cLevelWriteBlocked(method, path string) bool {
	p := normalizePolicyPath(path)
	if cLevelWriteExempt(p) {
		return false
	}
	m := strings.ToUpper(strings.TrimSpace(method))
	if m != http.MethodGet && m != http.MethodHead {
		return true
	}
	return cLevelWriteScreen(p)
}

func cLevelWriteScreen(path string) bool {
	p := normalizePolicyPath(path)
	if p == "/admin/orgs/switch" {
		return false
	}
	if strings.HasPrefix(p, "/admin") {
		return true
	}
	if strings.HasSuffix(p, "/new") || strings.Contains(p, "/new/") {
		return true
	}
	if strings.HasSuffix(p, "/edit") || strings.Contains(p, "/edit/") {
		return true
	}
	if strings.HasSuffix(p, "/register") || strings.Contains(p, "/register/") {
		return true
	}
	if strings.HasSuffix(p, "/form") {
		return true
	}
	return false
}
