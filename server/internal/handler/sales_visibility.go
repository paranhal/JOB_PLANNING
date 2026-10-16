package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// SalesVisibility 영업 조회 범위. D-64 · 73-J. 쓰기는 바꾸지 않는다.
type SalesVisibility struct {
	OrgID        string
	MineOnly     bool
	ShowOwnerFilter bool
	UserName     string
	UserID       string
	Owner        string // 조직관리자·비전 담당자 필터. 빈 값=전체
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
	case model.RoleVisionAdmin:
		v.ShowOwnerFilter = true
		v.Owner = owner
	case model.RoleOrgAdmin:
		v.ShowOwnerFilter = true
		v.Owner = owner
	case model.RoleSales:
		v.MineOnly = true
	default:
		// Q-26: tech·support 등은 조직 범위만. 본인 필터는 넣지 않는다.
	}
	return v
}

func applySalesVisibilityToList(c echo.Context, f *repository.SalesListFilter) {
	if f == nil {
		return
	}
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	if v.MineOnly {
		f.PersonName = v.UserName
		f.PersonUserID = v.UserID
		f.Owner = ""
		return
	}
	if v.Owner != "" {
		f.Owner = v.Owner
	}
}

func visibleActivityFilter(c echo.Context) repository.SalesActivityFilter {
	v := SalesVisibilityScope(c)
	f := repository.SalesActivityFilter{OrgID: v.OrgID}
	if v.MineOnly {
		f.PersonName = v.UserName
		f.PersonUserID = v.UserID
		return f
	}
	f.PersonName = v.Owner
	return f
}

func visibleQuoteFilter(c echo.Context, f repository.QuoteFilter) repository.QuoteFilter {
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	if v.MineOnly {
		f.PersonName = v.UserName
		f.PersonUserID = v.UserID
	} else {
		f.PersonName = v.Owner
	}
	return f
}

func visibleOrderFilter(c echo.Context, f repository.OrderFilter) repository.OrderFilter {
	v := SalesVisibilityScope(c)
	f.OrgID = v.OrgID
	if v.MineOnly {
		f.PersonName = v.UserName
		f.PersonUserID = v.UserID
	} else {
		f.PersonName = v.Owner
	}
	return f
}
