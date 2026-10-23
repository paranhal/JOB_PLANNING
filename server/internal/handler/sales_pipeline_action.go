package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func echoOrKeepHTTP(err error) error {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he
	}
	return echo.NewHTTPError(http.StatusNotFound, err.Error())
}

func salesCanShowNextAction(c echo.Context) bool {
	return canWriteSales(c) && !viewAsActive(c)
}

func (h *SalesHandler) loadSalesForNext(c echo.Context) (*model.SalesProject, string, error) {
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("영업 사업을 찾을 수 없습니다")
		}
		return nil, "", err
	}
	if err := requireManageSalesProject(c, p); err != nil {
		return nil, "", err
	}
	wp := ""
	if h.projectRepo != nil {
		if got, err := h.projectRepo.GetBySalesID(p.SalesID); err == nil && got != nil {
			wp = got.ProjectID
		}
	}
	return p, wp, nil
}

func (h *SalesHandler) WinDialog(c echo.Context) error {
	if !canWriteSales(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	p, wp, err := h.loadSalesForNext(c)
	if err != nil {
		return echoOrKeepHTTP(err)
	}
	act := model.SalesNextActionOf(p, wp, true)
	if act.Kind != model.SalesActionWin {
		return echo.NewHTTPError(http.StatusBadRequest, "수주할 단계가 아닙니다")
	}
	today := time.Now().Format("2006-01-02")
	wonAt := strings.TrimSpace(p.WonAt)
	if wonAt == "" {
		wonAt = today
	}
	return RenderPartial(c, "sales/_win_dialog.html", map[string]interface{}{
		"Project":  p,
		"Action":   act,
		"Today":    today,
		"WonAt":    wonAt,
		"Amount":   p.ExpectedAmount,
		"Return":   strings.TrimSpace(c.QueryParam("return")),
		"ShowLost": act.ShowLost,
	})
}

func (h *SalesHandler) ContractDialog(c echo.Context) error {
	if !canWriteSales(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	p, wp, err := h.loadSalesForNext(c)
	if err != nil {
		return echoOrKeepHTTP(err)
	}
	act := model.SalesNextActionOf(p, wp, true)
	if act.Kind != model.SalesActionConvert {
		return echo.NewHTTPError(http.StatusBadRequest, "계약으로 전환할 단계가 아닙니다")
	}
	amt := p.ContractAmount
	if amt == 0 {
		amt = p.AwardedAmount
	}
	if amt == 0 {
		amt = p.ExpectedAmount
	}
	at := strings.TrimSpace(p.ContractedAt)
	if at == "" {
		at = time.Now().Format("2006-01-02")
	}
	return RenderPartial(c, "sales/_contract_dialog.html", map[string]interface{}{
		"Project": p,
		"Action":  act,
		"Today":   at,
		"Amount":  amt,
		"Return":  strings.TrimSpace(c.QueryParam("return")),
	})
}

func (h *SalesHandler) ConfirmWin(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	p, wp, err := h.loadSalesForNext(c)
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok && he.Code == http.StatusForbidden {
			return he
		}
		return h.replyWinJSON(c, err, id, "id")
	}
	act := model.SalesNextActionOf(p, wp, true)
	result := strings.TrimSpace(c.FormValue("result"))
	if result == "" {
		result = model.SalesBidWon
	}
	if result == model.SalesCloseLost {
		if act.Kind != model.SalesActionWin || !act.ShowLost {
			return h.replyWinJSON(c, fmt.Errorf("입찰 결과 대기 건만 실주할 수 있습니다"), id, "")
		}
		reason := strings.TrimSpace(c.FormValue("reason"))
		if reason == "" {
			return h.replyWinJSON(c, fmt.Errorf("실주 사유가 필요합니다"), id, "reason")
		}
		err = h.repo.SetBidResult(id, model.SalesCloseLost, reason, "", currentUserID(c), ctxString(c, "user_name"))
		if err == nil {
			_ = h.repo.MarkMigratedChecked(id, currentUserID(c), ctxString(c, "user_name"))
		}
		return h.finishWin(c, err, id, "/sales?view=pipeline&ok=lost")
	}

	if act.Kind != model.SalesActionWin {
		return h.replyWinJSON(c, fmt.Errorf("이미 수주된 건입니다"), id, "")
	}
	rawAmt := strings.TrimSpace(strings.ReplaceAll(c.FormValue("amount"), ",", ""))
	if strings.HasPrefix(rawAmt, "-") {
		return h.replyWinJSON(c, fmt.Errorf("수주 금액은 0 이상이어야 합니다"), id, "amount")
	}
	amt := parseSalesAmount(rawAmt)
	wonAt := strings.TrimSpace(c.FormValue("won_at"))
	if wonAt == "" {
		wonAt = time.Now().Format("2006-01-02")
	}
	if _, perr := time.Parse("2006-01-02", wonAt); perr != nil {
		return h.replyWinJSON(c, fmt.Errorf("수주일을 확인하세요"), id, "won_at")
	}

	uid, uname := currentUserID(c), ctxString(c, "user_name")
	stage := strings.TrimSpace(p.Stage)
	switch stage {
	case model.SalesStage4Discover, model.SalesStage4Propose:
		err = h.repo.ChangeStage(id, model.SalesDirectWin, "", uid, uname, false)
	case model.SalesStage4Bid:
		err = h.repo.SetBidResult(id, model.SalesBidWon, "", rawAmt, uid, uname)
	default:
		err = fmt.Errorf("이미 수주된 건입니다")
	}
	if err != nil {
		return h.replyWinJSON(c, err, id, "")
	}
	if err := h.repo.ApplyWonMeta(id, wonAt, amt); err != nil {
		return h.replyWinJSON(c, err, id, "won_at")
	}
	_ = h.repo.MarkMigratedChecked(id, uid, uname)
	return h.finishWin(c, nil, id, "/sales?view=pipeline&ok=won")
}

func (h *SalesHandler) finishWin(c echo.Context, err error, id, okPath string) error {
	if err != nil {
		return h.replyWinJSON(c, err, id, "")
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "next": okPath})
	}
	return c.Redirect(http.StatusSeeOther, okPath)
}

func (h *SalesHandler) replyWinJSON(c echo.Context, err error, id, field string) error {
	if err == nil {
		return nil
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"ok": false, "error": err.Error(), "field": field,
		})
	}
	return h.replyStage(c, err, id, "/sales/"+id)
}
