package handler

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-201: AS 조치 화면 「영향 장비」. 조치 저장(POST /as/:id/update)과 같은 폼으로 저장한다.
// 영향 장비는 업무를 만들지 않고 접수·업무 건수에도 더하지 않는다.

const asAffectedDraftCookie = "as_affected_draft"

type affectedInput struct {
	Present   bool
	RawIDs    []string
	LabelsRaw string
	IDs       []string
	Labels    []string
	Skipped   []string
}

// parseAffectedForm 폼의 영향 장비를 검증·정리한다. 오류면 오류 코드(affected_limit·affected_label·affected_customer)를 돌려준다.
// 주 대상 자산과 조치 화면 「설치자산」(asset_id)과 같은 자산은 영향 장비에서 자동 제외한다.
func (h *ASHandler) parseAffectedForm(c echo.Context, as *model.ASReceipt) (affectedInput, string) {
	var in affectedInput
	in.Present = c.FormValue("affected_present") == "1"
	if !in.Present || as == nil {
		return in, ""
	}
	in.RawIDs = formNamedIDList(c, "affected_asset_id")
	in.LabelsRaw = c.FormValue("affected_label")
	exclude := append([]string{as.AssetID}, formAssetIDs(c)...)
	ids, labels, skipped, err := h.repo.CleanAffectedInput(as.CustomerID, in.RawIDs, repository.ParseAffectedLabels(in.LabelsRaw), exclude)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrAffectedLimit):
			return in, "affected_limit"
		case errors.Is(err, repository.ErrAffectedLabelLn):
			return in, "affected_label"
		default:
			return in, "affected_customer"
		}
	}
	in.IDs, in.Labels, in.Skipped = ids, labels, skipped
	return in, ""
}

type affectedDraft struct {
	IDs    []string `json:"ids"`
	Labels string   `json:"labels"`
}

// setAffectedDraft 검증에 실패했을 때 입력값을 잠깐(2분) 보관해 다시 보여 준다. 입력값 유지.
func setAffectedDraft(c echo.Context, in affectedInput) {
	b, _ := json.Marshal(affectedDraft{IDs: in.RawIDs, Labels: in.LabelsRaw})
	c.SetCookie(sessionCookie(asAffectedDraftCookie, url.QueryEscape(string(b)), 120))
}

// takeAffectedDraft 보관한 입력값을 읽고 지운다.
func takeAffectedDraft(c echo.Context) (affectedDraft, bool) {
	ck, err := c.Cookie(asAffectedDraftCookie)
	if err != nil || ck == nil {
		return affectedDraft{}, false
	}
	c.SetCookie(sessionCookie(asAffectedDraftCookie, "", -1))
	raw, err := url.QueryUnescape(ck.Value)
	if err != nil {
		return affectedDraft{}, false
	}
	var d affectedDraft
	if json.Unmarshal([]byte(raw), &d) != nil {
		return affectedDraft{}, false
	}
	return d, true
}

// attachAffectedView 조치 화면·상세에 영향 장비 값을 싣는다.
func (h *ASHandler) attachAffectedView(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if h == nil || as == nil || data == nil {
		return
	}
	items, _ := h.repo.ListAffectedAssets(as.ASID)
	data["AffectedItems"] = items
	data["AffectedLine"] = repository.AffectedLine(items)
	var assets []model.Asset
	var labels []string
	for _, it := range items {
		if it.AssetID != "" && h.assetRepo != nil {
			if a, _ := h.assetRepo.GetByID(it.AssetID); a != nil {
				assets = append(assets, *a)
			}
		} else if it.AssetID == "" && it.Label != "" {
			labels = append(labels, it.Label)
		}
	}
	labelsText := strings.Join(labels, "\n")
	if d, ok := takeAffectedDraft(c); ok { // 검증 실패 뒤: 입력했던 값을 그대로 보여 준다
		assets = nil
		for _, id := range d.IDs {
			if h.assetRepo != nil {
				if a, _ := h.assetRepo.GetByID(id); a != nil {
					assets = append(assets, *a)
				}
			}
		}
		labelsText = d.Labels
		data["AffectedDraft"] = true
	}
	data["AffectedAssets"] = assets
	data["AffectedLabelsText"] = labelsText
	data["AffectedOpen"] = len(assets) > 0 || strings.TrimSpace(labelsText) != "" || data["AffectedDraft"] == true
	if n := c.QueryParam("aff_skip"); n != "" && n != "0" {
		data["AffectedSkipN"] = n
	}
}
