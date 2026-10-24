package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canLookupCustomers(c echo.Context) bool {
	if currentRole(c) == model.RolePartner {
		return false
	}
	return hasPerm(c, model.PermASView) || hasPerm(c, model.PermMntView) ||
		hasPerm(c, model.PermWorkboard) || hasPerm(c, model.PermMasterView) ||
		hasPerm(c, model.PermSalesView) || hasPerm(c, model.PermAdminView) ||
		hasPerm(c, model.PermMaintenance)
}

func parseLookupQuery(c echo.Context) (repository.CustomerQuery, error) {
	scope := repository.CustomerScope(strings.TrimSpace(c.QueryParam("scope")))
	if err := repository.FormatLookupError(string(scope)); err != nil {
		return repository.CustomerQuery{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	var include []string
	for _, id := range strings.Split(c.QueryParam("include"), ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			include = append(include, id)
		}
	}
	return repository.CustomerQuery{
		Scope:      scope,
		Q:          c.QueryParam("q"),
		IncludeIDs: include,
		ParentID:   strings.TrimSpace(c.QueryParam("parent")),
		Limit:      limit,
		Offset:     offset,
	}, nil
}

type lookupCustomerJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ParentName string `json:"parent_name"`
	Region     string `json:"region"`
	PartyKind  string `json:"party_kind"`
	Inactive   bool   `json:"inactive"`
	Label      string `json:"label"`
}

func (h *CustomerHandler) LookupJSON(c echo.Context) error {
	if !canLookupCustomers(c) {
		return echo.ErrForbidden
	}
	q, err := parseLookupQuery(c)
	if err != nil {
		return err
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	opts, total, err := h.repo.Lookup(q)
	if err != nil {
		return err
	}
	items := make([]lookupCustomerJSON, 0, len(opts))
	for _, o := range opts {
		items = append(items, lookupCustomerJSON{
			ID: o.ID, Name: o.Name, ParentName: o.ParentName, Region: o.Region,
			PartyKind: o.PartyKind, Inactive: o.Inactive, Label: o.Label,
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items, "total": total})
}
