package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func (h *WorkboardHandler) MntMonthHint(c echo.Context) error {
	if h.mntRepo == nil {
		return c.JSON(http.StatusOK, map[string]string{"hint": ""})
	}
	cid := strings.TrimSpace(c.QueryParam("customer_id"))
	month := strings.TrimSpace(c.QueryParam("month"))
	n, label, err := h.mntRepo.CountCustomerVisitsInMonth(cid, month)
	if err != nil || n == 0 {
		return c.JSON(http.StatusOK, map[string]string{"hint": ""})
	}
	hint := fmt.Sprintf("이 기관은 %s 정기점검 일정이 이미 있습니다. 그 일정에서 처리하세요.", label)
	return c.JSON(http.StatusOK, map[string]string{"hint": hint})
}
