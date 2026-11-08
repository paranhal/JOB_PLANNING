package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *ASHandler) attachASProjectPicker(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if h == nil || as == nil || data == nil {
		return
	}
	name := ""
	if h.wbRepo != nil {
		if p, err := h.wbRepo.GetProject(as.ProjectID); err == nil && p != nil {
			name = strings.TrimSpace(p.Name)
		}
	}
	year := 0
	if !as.ReceiptDatetime.IsZero() {
		year = as.ReceiptDatetime.Year()
	}
	orgName := strings.TrimSpace(as.OrgName)
	if orgName == "" {
		if cust, ok := data["Customer"].(*model.Customer); ok && cust != nil {
			orgName = strings.TrimSpace(cust.OfficialName)
			if orgName == "" {
				orgName = strings.TrimSpace(cust.OrgName)
			}
		}
	}
	if h.wbRepo != nil {
		attachProjectPicker(c, pickerFromRepo(h.wbRepo), data, projectPickerOptsForAS(
			as.CustomerID, orgName, as.ProjectID, name, year,
		))
	}
	data["ProjectName"] = name
	data["CanLinkProject"] = canLinkASProject(c, as)
}

func canLinkASProject(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	return canLinkASProjectDefault(c, as.AssignedTo, as.AssignedUserID, as.OrgID)
}

func canLinkASProjectDefault(c echo.Context, assignedTo, assignedUID, orgID string) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		org := strings.TrimSpace(nz(c.FormValue("org"), currentOrg(c)))
		return org != "" && org != repository.OrgAll && org == orgID
	}
	if currentRole(c) == model.RoleOrgAdmin && loginHomeOrg(c) == orgID {
		return true
	}
	if currentRole(c) == model.RoleAdmin || currentRole(c) == model.RoleSupport {
		return true
	}
	return assigneeIsMine(c, assignedTo, assignedUID)
}

// UpdateProject 조치 내용과 별개로 사업만 연결한다. 완료·종결이어도 잠금을 요구하지 않는다.
func (h *ASHandler) UpdateProject(c echo.Context) error {
	id := c.Param("id")
	as, err := h.asInScope(c, id)
	if err != nil {
		return err
	}
	if !canLinkASProject(c, as) {
		return echo.ErrForbidden
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if h.wbRepo != nil {
		if err := requireProjectLink(c, h.wbRepo.GetProject, pid, false); err != nil {
			return err
		}
		if err := requireVisitSiteProject(h.wbRepo, as.CustomerID, pid, as.ProjectID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if _, _, err := h.repo.SetASReceiptProject(as.OrgID, as.ASID, pid); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	q := url.Values{}
	q.Set("ok", "project")
	return c.Redirect(http.StatusSeeOther, asOpenHref(as.ASID)+"?"+q.Encode()+"#project")
}
