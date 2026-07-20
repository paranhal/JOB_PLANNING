package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ContactHistoryHandler struct {
	repo         *repository.ContactHistoryRepo
	contactRepo  *repository.ContactRepo
	customerRepo *repository.CustomerRepo
}

// List 기관별 등록 담당자 정보 목록 (수정 스냅샷이 아닌 담당자 본인 정보)
func (h *ContactHistoryHandler) List(c echo.Context) error {
	contactID := c.QueryParam("contact_id")
	customerID := c.QueryParam("customer_id")

	customers, _ := h.customerRepo.ListAll()

	var contacts []model.Contact
	var items []model.Contact

	if contactID != "" {
		if ct, _ := h.contactRepo.GetByID(contactID); ct != nil {
			if customerID == "" {
				customerID = ct.CustomerID
			}
			items = []model.Contact{*ct}
		}
	}

	if customerID != "" {
		contacts, _ = h.contactRepo.ListByCustomer(customerID)
		if contactID == "" {
			items = contacts
		}
	}

	var orgName string
	for _, cu := range customers {
		if cu.CustomerID == customerID {
			orgName = cu.OrgName
			break
		}
	}
	for i := range items {
		if items[i].OrgName == "" {
			items[i].OrgName = orgName
		}
	}

	return c.Render(http.StatusOK, "contact_history/list.html", map[string]interface{}{
		"Title": "담당자 이력", "Active": "contact_history",
		"Items": items, "ContactID": contactID, "CustomerID": customerID,
		"Customers": customers, "Contacts": contacts,
		"ItemCount": len(items),
	})
}

func (h *ContactHistoryHandler) Create(c echo.Context) error {
	hist := &model.ContactHistory{
		ContactID:    c.FormValue("contact_id"),
		CustomerID:   c.FormValue("customer_id"),
		StartDate:    c.FormValue("start_date"),
		EndDate:      c.FormValue("end_date"),
		Department:   c.FormValue("department"),
		JobRole:      c.FormValue("job_role"),
		Title:        c.FormValue("title"),
		Phone:        c.FormValue("phone"),
		Email:        c.FormValue("email"),
		Status:       c.FormValue("status"),
		ChangeReason: c.FormValue("change_reason"),
	}
	h.repo.Create(hist)
	redirect := "/contact-history?customer_id=" + hist.CustomerID
	if hist.ContactID != "" {
		redirect += "&contact_id=" + hist.ContactID
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}
