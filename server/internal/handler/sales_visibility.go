package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// SalesVisibility 영업 조회 범위. D-64 · D-69. 쓰기는 바꾸지 않는다.
type SalesVisibility struct {
	OrgID           string
	ShowOwnerFilter bool
	UserName        string
	UserID          string
	Owner           string // 담당자 필터. 빈 값=조직 전체
}

func SalesVisibilityScope(c echo.Context) SalesVisibility {
	v := SalesVisibility{
		OrgID:    currentOrg(c),
		UserName: ctxString(c, "user_name"),
		UserID:   currentUserID(c),
	}
	role := currentRole(c)
	owner := strings.TrimSpace(c.QueryParam("owner"))
	if owner == "" {
		owner = strings.TrimSpace(c.QueryParam("assignee"))
	}
	switch role {
	case model.RoleVisionAdmin, model.RoleOrgAdmin, model.RoleSales:
		// D-69: 영업도 조직 전체 조회. 담당자 필터는 선택(기본 전체).
		v.ShowOwnerFilter = true
		v.Owner = owner
	default:
		// Q-26: tech·support 등은 조직 범위만. 담당자 필터 UI는 두지 않는다.
	}
	return v
}

func applySalesVisibilityToList(c echo.Context, f *repository.SalesListFilter) {
	if f == nil {
		return
	}
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	if v.Owner != "" {
		f.Owner = v.Owner
	}
}

func visibleActivityFilter(c echo.Context) repository.SalesActivityFilter {
	v := SalesVisibilityScope(c)
	return repository.SalesActivityFilter{OrgID: v.OrgID, PersonName: v.Owner}
}

func visibleQuoteFilter(c echo.Context, f repository.QuoteFilter) repository.QuoteFilter {
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	f.PersonName = v.Owner
	return f
}

func visibleOrderFilter(c echo.Context, f repository.OrderFilter) repository.OrderFilter {
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	f.PersonName = v.Owner
	return f
}
