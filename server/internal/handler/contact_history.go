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

func historyToListItem(h model.ContactHistory) model.ContactHistoryListItem {
	role := h.ContactRole
	if role == "" {
		role = "regular"
	}
	return model.ContactHistoryListItem{
		ContactID:    h.ContactID,
		Name:         h.ContactName,
		StartDate:    h.StartDate,
		EndDate:      h.EndDate,
		ContactRole:  role,
		ChangeReason: h.ChangeReason,
		Phone:        h.Phone,
		Email:        h.Email,
		IsCurrent:    false,
	}
}

func contactToCurrentListItem(ct model.Contact) model.ContactHistoryListItem {
	role := ct.ContactRole
	if role == "" {
		if ct.IsPrimary {
			role = "primary"
		} else {
			role = "regular"
		}
	}
	return model.ContactHistoryListItem{
		ContactID:    ct.ContactID,
		Name:         ct.FullName,
		StartDate:    ct.StartDate,
		EndDate:      ct.EndDate,
		ContactRole:  role,
		ChangeReason: "",
		Phone:        ct.Phone,
		Email:        ct.Email,
		IsCurrent:    true,
	}
}

func filterMeaningful(hist []model.ContactHistory) []model.ContactHistory {
	var out []model.ContactHistory
	for _, h := range hist {
		if repository.IsMeaningfulChangeReason(h.ChangeReason) {
			out = append(out, h)
		}
	}
	return out
}

// List 기관별 담당자 변경 이력 (미변경 시 현행 담당자만 표시)
func (h *ContactHistoryHandler) List(c echo.Context) error {
	contactID := c.QueryParam("contact_id")
	customerID := c.QueryParam("customer_id")

	customers, _ := h.customerRepo.ListAll()

	var contacts []model.Contact
	var items []model.ContactHistoryListItem
	showingCurrent := false

	if contactID != "" {
		if ct, _ := h.contactRepo.GetByID(contactID); ct != nil {
			if customerID == "" {
				customerID = ct.CustomerID
			}
		}
	}

	if customerID != "" {
		contacts, _ = h.contactRepo.ListByCustomer(customerID)

		var hist []model.ContactHistory
		if contactID != "" {
			hist, _ = h.repo.ListByContact(contactID)
		} else {
			hist, _ = h.repo.ListByCustomer(customerID)
		}
		hist = filterMeaningful(hist)

		if len(hist) > 0 {
			for _, row := range hist {
				items = append(items, historyToListItem(row))
			}
		} else {
			// 아직 담당자가 바뀌지 않은 경우: 현행 담당자만 표시
			showingCurrent = true
			if contactID != "" {
				if ct, _ := h.contactRepo.GetByID(contactID); ct != nil {
					items = append(items, contactToCurrentListItem(*ct))
				}
			} else {
				for _, ct := range contacts {
					// 미변경 시에는 재직 중 담당자를 우선 표시
					if ct.Status == "active" || ct.Status == "" {
						items = append(items, contactToCurrentListItem(ct))
					}
				}
				// 재직자가 없으면 등록된 전원(보통 1명) 표시
				if len(items) == 0 {
					for _, ct := range contacts {
						items = append(items, contactToCurrentListItem(ct))
					}
				}
			}
		}
	}

	return c.Render(http.StatusOK, "contact_history/list.html", map[string]interface{}{
		"Title": "담당자 이력", "Active": "contact_history",
		"Items": items, "ContactID": contactID, "CustomerID": customerID,
		"Customers": customers, "Contacts": contacts,
		"ItemCount": len(items), "ShowingCurrent": showingCurrent,
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
		ContactRole:  c.FormValue("contact_role"),
		ChangeReason: c.FormValue("change_reason"),
	}
	h.repo.Create(hist)
	redirect := "/contact-history?customer_id=" + hist.CustomerID
	if hist.ContactID != "" {
		redirect += "&contact_id=" + hist.ContactID
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}
