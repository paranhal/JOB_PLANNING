package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *ASHandler) mergeActionPartsData(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if as == nil || data == nil {
		return
	}
	var cust *model.Customer
	if h.customerRepo != nil && strings.TrimSpace(as.CustomerID) != "" {
		cust, _ = h.customerRepo.GetByID(as.CustomerID)
	}
	data["Customer"] = cust
	var hist []model.ASHistoryItem
	if h.repo != nil && strings.TrimSpace(as.CustomerID) != "" {
		hist, _ = h.repo.ListPastHistory(as.CustomerID, as.ASID, 8)
	}
	data["PastHistory"] = hist

	rows := []model.ASPartUsed{}
	if draft, ok := c.Get("as_parts_rows").([]model.ASPartUsed); ok {
		rows = draft
	} else if h.partsRepo != nil {
		got, _ := h.partsRepo.ListByAS(as.ASID)
		if got != nil {
			rows = got
		}
	}
	enabled := len(rows) > 0 || strings.TrimSpace(as.PartsUsed) != ""
	if v, ok := c.Get("as_parts_enabled").(bool); ok {
		enabled = v
	}
	legacy := ""
	if len(rows) == 0 {
		legacy = strings.TrimSpace(as.PartsUsed)
	}
	data["PartRows"] = rows
	data["PartsEnabled"] = enabled
	data["PartsLegacy"] = legacy
	if msg, ok := c.Get("as_parts_msg").(string); ok && msg != "" {
		data["PartsFormErr"] = msg
		if data["ActionErrMsg"] == nil || data["ActionErrMsg"] == "" {
			data["ActionErrMsg"] = msg
			data["ActionErr"] = "parts"
		}
	}
}

func (h *ASHandler) collectASPartsForm(c echo.Context) (bool, []model.ASPartUsed, error) {
	if c.FormValue("parts_form") != "1" {
		return false, nil, nil
	}
	on := c.FormValue("parts_enabled") == "1"
	if !on {
		return false, nil, nil
	}
	_ = c.Request().ParseForm()
	form := c.Request().PostForm
	rows, err := model.ParseASPartsFromForm(form["part_name"], form["part_code"], form["part_qty"], form["part_item_id"])
	return true, rows, err
}

func (h *ASHandler) applyASPartsSave(c echo.Context, asID, processID string) {
	if h.partsRepo == nil || c.FormValue("parts_form") != "1" {
		return
	}
	on, ok := c.Get("as_parts_apply").(bool)
	if !ok {
		return
	}
	if !on {
		_ = h.partsRepo.DeleteByAS(asID)
		return
	}
	rows, _ := c.Get("as_parts_rows").([]model.ASPartUsed)
	if len(rows) == 0 {
		return
	}
	_ = h.partsRepo.ReplaceByAS(asID, processID, rows)
}

func (h *ASHandler) SuggestParts(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	if isReadOnly(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	if utf8.RuneCountInString(q) < 2 {
		return c.JSON(http.StatusOK, []map[string]string{})
	}
	if h.itemRepo == nil {
		return c.JSON(http.StatusOK, []map[string]string{})
	}
	items, err := h.itemRepo.SuggestParts(currentOrg(c), q, 20)
	if err != nil {
		return err
	}
	out := make([]map[string]string, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]string{
			"item_id": it.ItemID,
			"name":    it.Name,
			"code":    model.ItemPartCode(it),
		})
	}
	return c.JSON(http.StatusOK, out)
}
