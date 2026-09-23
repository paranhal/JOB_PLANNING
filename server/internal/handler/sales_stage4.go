package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *SalesHandler) SetBidResult(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	result := strings.TrimSpace(c.FormValue("result"))
	if result == "" {
		result = strings.TrimSpace(c.FormValue("bid_status"))
	}
	reason := strings.TrimSpace(c.FormValue("reason"))
	amount := strings.TrimSpace(c.FormValue("amount"))
	err := h.repo.SetBidResult(id, result, reason, amount, currentUserID(c), ctxString(c, "user_name"))
	if err == nil {
		_ = h.repo.MarkMigratedChecked(id, currentUserID(c), ctxString(c, "user_name"))
	}
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) StartNegotiation(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	err := h.repo.StartNegotiation(id, currentUserID(c), ctxString(c, "user_name"))
	if err == nil {
		_ = h.repo.MarkMigratedChecked(id, currentUserID(c), ctxString(c, "user_name"))
	}
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) CloseContracted(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	at := strings.TrimSpace(c.FormValue("contracted_at"))
	amt := parseSalesAmount(c.FormValue("contract_amount"))
	err := h.repo.CloseContracted(id, at, amt, currentUserID(c), ctxString(c, "user_name"))
	if err != nil {
		return h.replyStage(c, err, id, "/sales/"+id)
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"/promote")
}

func (h *SalesHandler) CloseNegotiationFailed(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	reason := strings.TrimSpace(c.FormValue("reason"))
	err := h.repo.CloseNegotiationFailed(id, reason, currentUserID(c), ctxString(c, "user_name"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) SetWinProb(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	n, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("win_prob")))
	err := h.repo.SetWinProb(id, n, currentUserID(c), ctxString(c, "user_name"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) SetRFPReceived(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	err := h.repo.SetRFPReceived(id, strings.TrimSpace(c.FormValue("rfp_received_at")), currentUserID(c), ctxString(c, "user_name"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) MarkMigratedChecked(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	err := h.repo.MarkMigratedChecked(id, currentUserID(c), ctxString(c, "user_name"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) DropForm(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	p, err := h.repo.Get(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"ok": false, "error": "없음"})
	}
	if !canDropSales(c, p) {
		return echo.ErrForbidden
	}
	prev, _ := h.repo.DropPreview(id)
	codes, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupDropReason)
	until := model.DefaultDormantUntil(time.Now(), h.repo.DormantDefaultMonth())
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "sales_id": p.SalesID, "name": p.Name, "stage": p.Stage,
		"prob": model.SalesProbability(p), "open_tasks": prev.OpenTasks, "quotes": prev.Quotes,
		"reasons": codes, "dormant_until": until,
	})
}

func (h *SalesHandler) Drop(c echo.Context) error {
	id := c.Param("id")
	p, err := h.repo.Get(id)
	if err != nil {
		return h.replyStage(c, err, id, "/sales")
	}
	if !canDropSales(c, p) {
		return echo.ErrForbidden
	}
	code := strings.TrimSpace(c.FormValue("reason_code"))
	if code == "postponed" && (c.FormValue("as_dormant") == "1" || c.FormValue("sleep") == "1") {
		until := strings.TrimSpace(c.FormValue("until"))
		reason := strings.TrimSpace(c.FormValue("reason"))
		if reason == "" {
			reason = "고객 사업 무기한 연기"
		}
		err = h.repo.Sleep(id, until, reason, currentUserID(c), ctxString(c, "user_name"), false)
		return h.replyStage(c, err, id, "/sales/"+id+"?ok=dormant")
	}
	err = h.repo.Drop(id, code, c.FormValue("reason"), currentUserID(c), ctxString(c, "user_name"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func canDropSales(c echo.Context, p *model.SalesProject) bool {
	if p == nil {
		return false
	}
	if isAdminRole(c) {
		return true
	}
	uid := strings.TrimSpace(currentUserID(c))
	return uid != "" && uid == strings.TrimSpace(p.SalesOwnerID)
}

func (h *SalesHandler) ChangeStage(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	to := strings.TrimSpace(c.FormValue("stage"))
	reason := strings.TrimSpace(c.FormValue("reason"))
	if to == model.SalesStage4Closed {
		return h.replyStage(c, fmt.Errorf("상세에서 종료하세요"), id, "/sales/"+id)
	}
	keep := c.FormValue("keep_override") == "1"
	err := h.repo.ChangeStage(id, to, reason, currentUserID(c), ctxString(c, "user_name"), keep)
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=stage")
}

func salesListFilterFromRequest(c echo.Context) repository.SalesListFilter {
	f := parseSalesListFilter(c)
	f.IncludeClosed = c.QueryParam("closed") == "1"
	f.CloseReason = strings.TrimSpace(c.QueryParam("close"))
	f.ContractTarget = strings.TrimSpace(c.QueryParam("contract_target"))
	return parseSalesListFilterQ(c, f)
}
