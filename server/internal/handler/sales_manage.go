package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const errSalesOtherOrgManage = "다른 조직의 사업입니다. 사업 Owner 또는 해당 조직만 변경할 수 있습니다."

func currentUserHomeOrg(c echo.Context) string {
	if h := loginHomeOrg(c); h != "" {
		return h
	}
	if canSwitchOrg(c) {
		return ""
	}
	return model.OrgIDLibrary
}

// canManageSalesProject 수정·단계·수주·계약 전환. 활동 로그는 쓰지 않는다. 95-2G.
func canManageSalesProject(c echo.Context, p *model.SalesProject) bool {
	if c == nil || p == nil || isReadOnly(c) {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		return true
	}
	if !canWriteSalesDeal(c) {
		return false
	}
	home := currentUserHomeOrg(c)
	if home != "" && strings.TrimSpace(p.OrgID) == home {
		return true
	}
	uid := strings.TrimSpace(currentUserID(c))
	if uid != "" && uid == strings.TrimSpace(p.SalesOwnerID) {
		return true
	}
	return false
}

func requireManageSalesProject(c echo.Context, p *model.SalesProject) error {
	if canManageSalesProject(c, p) {
		return nil
	}
	return echo.NewHTTPError(http.StatusForbidden, errSalesOtherOrgManage)
}

func (h *SalesHandler) loadManagedSales(c echo.Context) (*model.SalesProject, error) {
	if h == nil || h.repo == nil {
		return nil, echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil || p == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "영업 사업을 찾을 수 없습니다")
	}
	if err := requireManageSalesProject(c, p); err != nil {
		return nil, err
	}
	return p, nil
}

func applySalesListReadScope(c echo.Context, f *repository.SalesListFilter) {
	if f == nil {
		return
	}
	if model.IsInternalStaffRole(currentRole(c)) {
		f.OrgID = repository.OrgAll
		return
	}
	applySalesVisibilityToList(c, f)
}

func applySalesMyScopeToList(c echo.Context, f *repository.SalesListFilter) {
	if f == nil {
		return
	}
	role := currentRole(c)
	if role == model.RoleVisionAdmin || role == model.RoleCLevel {
		applySalesVisibilityToList(c, f)
		return
	}
	f.MyScope = true
	f.OrgID = currentUserHomeOrg(c)
	f.OwnerUserID = currentUserID(c)
}

func salesContractUnionFilter(c echo.Context) repository.ContractUnionFilter {
	role := currentRole(c)
	if role == model.RoleVisionAdmin || role == model.RoleCLevel {
		return repository.ContractUnionFilter{OrgID: currentOrg(c)}
	}
	return repository.ContractUnionFilter{
		OrgID:       currentUserHomeOrg(c),
		OwnerUserID: currentUserID(c),
		MyScope:     true,
	}
}

func annotateSalesContractManage(c echo.Context, rows []model.ContractRow) {
	home := currentUserHomeOrg(c)
	for i := range rows {
		if rows[i].Source != model.ContractSourceSales {
			continue
		}
		p := &model.SalesProject{OrgID: rows[i].OrgID, SalesOwnerID: rows[i].SalesOwnerID}
		rows[i].CanManage = canManageSalesProject(c, p)
		if strings.TrimSpace(rows[i].OrgLabel) == "" {
			rows[i].OrgLabel = rows[i].OrgID
		}
		oid := strings.TrimSpace(rows[i].OrgID)
		rows[i].ForeignOrg = oid != "" && home != "" && oid != home
	}
}

func salesNextActionEnabled(c echo.Context, p *model.SalesProject) bool {
	return canManageSalesProject(c, p) && !viewAsActive(c)
}

func (h *SalesHandler) filterManageableSalesIDs(c echo.Context, ids []string) []string {
	if h == nil || h.repo == nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		p, err := h.repo.Get(id)
		if err != nil || p == nil {
			continue
		}
		if canManageSalesProject(c, p) {
			out = append(out, id)
		}
	}
	return out
}
