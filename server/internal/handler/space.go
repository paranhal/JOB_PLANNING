package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SpaceHandler struct {
	repo         *repository.SpaceRepo
	customerRepo *repository.CustomerRepo
}

func (h *SpaceHandler) List(c echo.Context) error {
	customerID := c.QueryParam("customer_id")
	customers, _ := h.customerRepo.ListAll()

	var buildings []model.CustomerBuilding
	if customerID != "" {
		buildings, _ = h.repo.ListBuildings(customerID)
		for i := range buildings {
			b, _ := h.repo.GetBuilding(buildings[i].BuildingID)
			if b != nil {
				buildings[i].Floors = b.Floors
			}
		}
	}

	return c.Render(http.StatusOK, "space/list.html", map[string]interface{}{
		"Title": "공간 관리", "Active": "spaces",
		"Customers": customers, "CustomerID": customerID,
		"Buildings": buildings,
	})
}

// ── 건물 CRUD ──

func (h *SpaceHandler) CreateBuilding(c echo.Context) error {
	b := &model.CustomerBuilding{
		CustomerID:   c.FormValue("customer_id"),
		BuildingName: c.FormValue("building_name"),
		BuildingType: c.FormValue("building_type"),
		Address:      c.FormValue("address"),
		IsActive:     true,
	}
	if err := h.repo.CreateBuilding(b); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+b.CustomerID)
}

func (h *SpaceHandler) UpdateBuilding(c echo.Context) error {
	b := &model.CustomerBuilding{
		BuildingID:   c.Param("id"),
		BuildingName: c.FormValue("building_name"),
		BuildingType: c.FormValue("building_type"),
		Address:      c.FormValue("address"),
		IsActive:     c.FormValue("is_active") != "0",
	}
	customerID := c.FormValue("customer_id")
	if err := h.repo.UpdateBuilding(b); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+customerID)
}

func (h *SpaceHandler) DeleteBuilding(c echo.Context) error {
	customerID := c.QueryParam("customer_id")
	h.repo.DeleteBuilding(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+customerID)
}

// ── 층 CRUD ──

func (h *SpaceHandler) CreateFloor(c echo.Context) error {
	order, _ := strconv.Atoi(c.FormValue("sort_order"))
	f := &model.CustomerFloor{
		BuildingID: c.FormValue("building_id"),
		FloorName:  c.FormValue("floor_name"),
		SortOrder:  order,
	}
	h.repo.CreateFloor(f)
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+c.FormValue("customer_id"))
}

func (h *SpaceHandler) DeleteFloor(c echo.Context) error {
	h.repo.DeleteFloor(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+c.QueryParam("customer_id"))
}

// ── 실 CRUD ──

func (h *SpaceHandler) CreateRoom(c echo.Context) error {
	rm := &model.CustomerRoom{
		FloorID:    c.FormValue("floor_id"),
		RoomName:   c.FormValue("room_name"),
		RoomNumber: c.FormValue("room_number"),
		Purpose:    c.FormValue("purpose"),
	}
	h.repo.CreateRoom(rm)
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+c.FormValue("customer_id"))
}

func (h *SpaceHandler) DeleteRoom(c echo.Context) error {
	h.repo.DeleteRoom(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/spaces?customer_id="+c.QueryParam("customer_id"))
}

// ── HTMX API: 건물→층→실 cascade ──

func (h *SpaceHandler) APIFloors(c echo.Context) error {
	buildingID := c.Param("building_id")
	floors, _ := h.repo.AllFloorsForBuilding(buildingID)
	return c.JSON(http.StatusOK, floors)
}

func (h *SpaceHandler) APIRooms(c echo.Context) error {
	floorID := c.Param("floor_id")
	rooms, _ := h.repo.AllRoomsForFloor(floorID)
	return c.JSON(http.StatusOK, rooms)
}

func (h *SpaceHandler) APIBuildings(c echo.Context) error {
	customerID := c.Param("customer_id")
	buildings, _ := h.repo.AllBuildingsForCustomer(customerID)
	return c.JSON(http.StatusOK, buildings)
}
