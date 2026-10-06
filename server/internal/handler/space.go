package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SpaceHandler struct {
	repo         *repository.SpaceRepo
	customerRepo *repository.CustomerRepo
}

func spacesRedirect(customerID, buildingID, floorID string) string {
	u := "/spaces/edit?customer_id=" + url.QueryEscape(customerID)
	if buildingID != "" {
		u += "&building_id=" + url.QueryEscape(buildingID)
	}
	if floorID != "" {
		u += "&floor_id=" + url.QueryEscape(floorID)
	}
	return u
}

func nextRoomSheetLabel(rooms []model.CustomerRoom) string {
	next := len(rooms)
	for _, r := range rooms {
		name := strings.TrimSpace(r.RoomName)
		name = strings.TrimSuffix(name, "호")
		if n, err := strconv.Atoi(name); err == nil && n+1 > next {
			next = n + 1
		}
	}
	return fmt.Sprintf("%02d호", next)
}

// List 47-A 리다이렉트는 남긴다. 리스트는 §50.2.
func (h *SpaceHandler) List(c echo.Context) error {
	if c.QueryParam("building_id") != "" || c.QueryParam("floor_id") != "" {
		return c.Redirect(http.StatusFound, "/spaces/edit?"+c.QueryString())
	}
	if c.QueryParam("view") == "stats" {
		return h.Stats(c)
	}

	orgID := currentOrg(c)
	q := strings.TrimSpace(c.QueryParam("q"))
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	sort := strings.TrimSpace(c.QueryParam("sort"))
	dir := strings.TrimSpace(c.QueryParam("dir"))
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	switch sort {
	case "org", "building", "floor", "room":
	default:
		sort = ""
	}
	includeInactive := c.QueryParam("include_inactive") == "1"
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 50
	offset := (page - 1) * pageSize

	rows, total, counts, err := h.repo.ListSpaceRows(orgID, customerID, q, sort, dir, includeInactive, pageSize, offset)
	if err != nil {
		return err
	}
	totalPages := 1
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}
	customers, _, _ := h.customerRepo.List(orgID, "", "", "", "", "", 1, 2000, false, model.PartyKindCustomer)

	nextDir := func(col string) string {
		if sort == col && dir == "asc" {
			return "desc"
		}
		return "asc"
	}
	qs := spaceListQuery(q, customerID, sort, dir, includeInactive)
	listHref := "/spaces"
	if s := qs.Encode(); s != "" {
		listHref += "?" + s
	}
	sq := cloneURLValues(qs)
	sq.Set("view", "stats")
	statsHref := "/spaces?" + sq.Encode()
	excelHref := "/spaces/export.xlsx"
	if s := qs.Encode(); s != "" {
		excelHref += "?" + s
	}

	return c.Render(http.StatusOK, "space/list.html", map[string]interface{}{
		"Title": "공간 관리", "Active": NavSpaces,
		"Rows": rows, "Total": total, "Counts": counts,
		"Page": page, "TotalPages": totalPages,
		"Q": q, "CustomerID": customerID,
		"Customers": customers,
		"Sort":      sort, "Dir": dir,
		"IncludeInactive": includeInactive,
		"SortDirOrg":      nextDir("org"),
		"SortDirBld":      nextDir("building"),
		"SortDirFloor":    nextDir("floor"),
		"SortDirRoom":     nextDir("room"),
		"CanWrite":        canWriteMaster(c),
		"ListHref":        listHref,
		"StatsHref":       statsHref,
		"ExcelHref":       excelHref,
		"View":            "",
	})
}

func spaceListQuery(q, customerID, sort, dir string, includeInactive bool) url.Values {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if customerID != "" {
		v.Set("customer_id", customerID)
	}
	if sort != "" {
		v.Set("sort", sort)
	}
	if dir != "" && dir != "asc" {
		v.Set("dir", dir)
	}
	if includeInactive {
		v.Set("include_inactive", "1")
	}
	return v
}

// Stats 47-C 에서 채운다. 지금은 리스트로 되돌린다.
func (h *SpaceHandler) Stats(c echo.Context) error {
	u := *c.Request().URL
	qv := u.Query()
	qv.Del("view")
	u.RawQuery = qv.Encode()
	return c.Redirect(http.StatusFound, u.RequestURI())
}

func (h *SpaceHandler) Edit(c echo.Context) error {
	orgID := currentOrg(c)
	customerID := c.QueryParam("customer_id")
	buildingID := c.QueryParam("building_id")
	floorID := c.QueryParam("floor_id")
	customers, _, _ := h.customerRepo.List(orgID, "", "", "", "", "", 1, 2000, false, model.PartyKindCustomer)

	var buildings []model.CustomerBuilding
	var floors []model.CustomerFloor
	var selectedBuilding *model.CustomerBuilding
	var selectedFloor *model.CustomerFloor

	if customerID != "" {
		buildings, _ = h.repo.ListBuildings(orgID, customerID)
		if buildingID != "" {
			for i := range buildings {
				if buildings[i].BuildingID == buildingID {
					b, _ := h.repo.GetBuilding(orgID, buildingID)
					if b != nil {
						selectedBuilding = b
					}
					break
				}
			}
		}
		if selectedBuilding == nil && len(buildings) > 0 {
			b, _ := h.repo.GetBuilding(orgID, buildings[0].BuildingID)
			if b != nil {
				selectedBuilding = b
				buildingID = b.BuildingID
			}
		}
	}

	if selectedBuilding != nil {
		floors = selectedBuilding.Floors
		if floorID != "" {
			for i := range floors {
				if floors[i].FloorID == floorID {
					selectedFloor = &floors[i]
					break
				}
			}
		}
		if selectedFloor == nil && len(floors) > 0 {
			selectedFloor = &floors[0]
			floorID = selectedFloor.FloorID
		}
	}

	return c.Render(http.StatusOK, "space/edit.html", map[string]interface{}{
		"Title": "공간 관리", "Active": NavSpaces,
		"Customers": customers, "CustomerID": customerID,
		"Buildings": buildings, "BuildingID": buildingID,
		"Selected": selectedBuilding,
		"Floors":   floors, "FloorID": floorID, "SelectedFloor": selectedFloor,
	})
}

// ── 건물 CRUD ──

func (h *SpaceHandler) CreateBuilding(c echo.Context) error {
	customerID := c.FormValue("customer_id")
	name := strings.TrimSpace(c.FormValue("building_name"))
	if name == "" {
		return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, "", ""))
	}
	b := &model.CustomerBuilding{
		CustomerID:   customerID,
		BuildingName: name,
		BuildingType: "",
		Address:      "",
		IsActive:     true,
	}
	if err := h.repo.CreateBuilding(b); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(b.CustomerID, b.BuildingID, ""))
}

func (h *SpaceHandler) UpdateBuilding(c echo.Context) error {
	customerID := c.FormValue("customer_id")
	buildingID := c.Param("id")
	floorID := c.FormValue("floor_id")
	name := strings.TrimSpace(c.FormValue("building_name"))
	if name == "" {
		return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
	}
	b := &model.CustomerBuilding{
		BuildingID:   buildingID,
		BuildingName: name,
		BuildingType: "",
		Address:      "",
		IsActive:     true,
	}
	if err := h.repo.UpdateBuilding(b); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
}

func (h *SpaceHandler) DeleteBuilding(c echo.Context) error {
	customerID := c.QueryParam("customer_id")
	h.repo.DeleteBuilding(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, "", ""))
}

// ── 층 CRUD ──

func (h *SpaceHandler) CreateFloor(c echo.Context) error {
	order, _ := strconv.Atoi(c.FormValue("sort_order"))
	buildingID := c.FormValue("building_id")
	customerID := c.FormValue("customer_id")
	f := &model.CustomerFloor{
		BuildingID: buildingID,
		FloorName:  strings.TrimSpace(c.FormValue("floor_name")),
		SortOrder:  order,
	}
	if f.FloorName == "" {
		return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, ""))
	}
	if err := h.repo.CreateFloor(f); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, f.FloorID))
}

func (h *SpaceHandler) UpdateFloor(c echo.Context) error {
	customerID := c.FormValue("customer_id")
	buildingID := c.FormValue("building_id")
	floorID := c.Param("id")
	name := strings.TrimSpace(c.FormValue("floor_name"))
	if name == "" {
		return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
	}
	order, _ := strconv.Atoi(c.FormValue("sort_order"))
	f := &model.CustomerFloor{
		FloorID:   floorID,
		FloorName: name,
		SortOrder: order,
	}
	if err := h.repo.UpdateFloor(f); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
}

func (h *SpaceHandler) DeleteFloor(c echo.Context) error {
	h.repo.DeleteFloor(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, spacesRedirect(
		c.QueryParam("customer_id"), c.QueryParam("building_id"), ""))
}

// ── 실 CRUD ──

func (h *SpaceHandler) CreateRoom(c echo.Context) error {
	orgID := currentOrg(c)
	floorID := c.FormValue("floor_id")
	customerID := c.FormValue("customer_id")
	buildingID := c.FormValue("building_id")
	name := strings.TrimSpace(c.FormValue("room_name"))
	if name == "" {
		rooms, _ := h.repo.ListRooms(orgID, floorID)
		name = nextRoomSheetLabel(rooms)
	}
	rm := &model.CustomerRoom{
		FloorID:  floorID,
		RoomName: name,
	}
	h.repo.CreateRoom(rm)
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
}

// BatchUpdateRooms 호실 시트 일괄 수정
func (h *SpaceHandler) BatchUpdateRooms(c echo.Context) error {
	customerID := c.FormValue("customer_id")
	buildingID := c.FormValue("building_id")
	floorID := c.FormValue("floor_id")
	_ = c.Request().ParseForm()
	ids := c.Request().Form["room_id"]
	names := c.Request().Form["room_name"]
	n := len(ids)
	if len(names) < n {
		n = len(names)
	}
	for i := 0; i < n; i++ {
		name := strings.TrimSpace(names[i])
		if ids[i] == "" || name == "" {
			continue
		}
		_ = h.repo.UpdateRoomName(ids[i], name)
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
}

func (h *SpaceHandler) UpdateRoom(c echo.Context) error {
	customerID := c.FormValue("customer_id")
	buildingID := c.FormValue("building_id")
	floorID := c.FormValue("floor_id")
	name := strings.TrimSpace(c.FormValue("room_name"))
	if name == "" {
		return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
	}
	rm := &model.CustomerRoom{
		RoomID:   c.Param("id"),
		RoomName: name,
	}
	if err := h.repo.UpdateRoom(rm); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, spacesRedirect(customerID, buildingID, floorID))
}

func (h *SpaceHandler) DeleteRoom(c echo.Context) error {
	h.repo.DeleteRoom(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, spacesRedirect(
		c.QueryParam("customer_id"), c.QueryParam("building_id"), c.QueryParam("floor_id")))
}

// ── HTMX API: 건물→층→실 cascade ──

func (h *SpaceHandler) APIFloors(c echo.Context) error {
	buildingID := c.Param("building_id")
	floors, _ := h.repo.AllFloorsForBuilding(currentOrg(c), buildingID)
	return c.JSON(http.StatusOK, floors)
}

func (h *SpaceHandler) APIRooms(c echo.Context) error {
	floorID := c.Param("floor_id")
	rooms, _ := h.repo.AllRoomsForFloor(currentOrg(c), floorID)
	return c.JSON(http.StatusOK, rooms)
}

func (h *SpaceHandler) APIBuildings(c echo.Context) error {
	customerID := c.Param("customer_id")
	buildings, _ := h.repo.AllBuildingsForCustomer(currentOrg(c), customerID)
	return c.JSON(http.StatusOK, buildings)
}
