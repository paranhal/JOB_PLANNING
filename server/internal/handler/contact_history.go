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

func (h *ContactHistoryHandler) List(c echo.Context) error {
	contactID := c.QueryParam("contact_id")
	customerID := c.QueryParam("customer_id")

	var items []model.ContactHistory
	if contactID != "" {
		items, _ = h.repo.ListByContact(contactID)
		// 담당자만 지정된 경우 소속 기관을 채워 콤보 유지
		if customerID == "" {
			if ct, _ := h.contactRepo.GetByID(contactID); ct != nil {
				customerID = ct.CustomerID
			}
		}
	} else if customerID != "" {
		items, _ = h.repo.ListByCustomer(customerID)
	}

	customers, _ := h.customerRepo.ListAll()
	var contacts []model.Contact
	if customerID != "" {
		contacts, _ = h.contactRepo.ListByCustomer(customerID)
	}

	return c.Render(http.StatusOK, "contact_history/list.html", map[string]interface{}{
		"Title": "담당자 이력", "Active": "contact_history",
		"Items": items, "ContactID": contactID, "CustomerID": customerID,
		"Customers": customers, "Contacts": contacts,
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
	redirect := "/contact-history?contact_id=" + hist.ContactID
	return c.Redirect(http.StatusSeeOther, redirect)
}
