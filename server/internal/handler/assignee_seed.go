package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// AssigneeSeed 95-102 담당자 초기값. 조직관리자는 비우고 필수, 개인은 본인.
type AssigneeSeed struct {
	Name     string
	OwnerID  string
	IDs      []string
	Required bool
}

func defaultAssigneeSeed(c echo.Context) AssigneeSeed {
	role := currentRole(c)
	req := requiresAssigneeChoice(c)
	if role == model.RoleOrgAdmin || role == model.RoleAdmin {
		return AssigneeSeed{Required: true}
	}
	id := strings.TrimSpace(currentUserID(c))
	name := strings.TrimSpace(currentUserDisplayName(c))
	ids := []string{}
	if id != "" {
		ids = []string{id}
	}
	return AssigneeSeed{Name: name, OwnerID: id, IDs: ids, Required: req}
}
