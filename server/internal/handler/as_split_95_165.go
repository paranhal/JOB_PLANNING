package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *ASHandler) attachASSplitView(as *model.ASReceipt, data map[string]interface{}) {
	if h == nil || as == nil || data == nil {
		return
	}
	data["SplitBaseTitle"] = model.FormatASWorkTitle(as.OrgName, as.ASNumber)
	data["SplitBlocked"] = asSplitBlockedText(as)
	if h.wbRepo == nil {
		return
	}
	if n, err := h.wbRepo.CountASSplitTasks([]string{as.ASID}); err == nil {
		data["SplitTaskCount"] = n[as.ASID]
	}
	linked, _ := h.wbRepo.ListASLinkedAssets(as.ASID)
	if len(linked) == 0 && strings.TrimSpace(as.AssetID) != "" && h.assetRepo != nil {
		if a, _ := h.assetRepo.GetByID(as.AssetID); a != nil {
			linked = []model.Asset{*a}
		}
	}
	data["SplitAssets"] = linked
	if daily, _ := h.wbRepo.GetTaskBySource(model.WBSourceAS, as.ASID); daily != nil {
		var children int
		_ = h.wbRepo.DB().QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE parent_task_id=?`, daily.TaskID).Scan(&children)
		if children > 0 && data["SplitBlocked"] == "" {
			data["SplitBlocked"] = "하위 업무가 있어 여기서 나누지 않습니다."
		}
	}
}

func asSplitBlockedText(as *model.ASReceipt) string {
	if as == nil {
		return ""
	}
	switch as.Status {
	case "completed", "closed", "cancelled":
		return "완료된 접수는 나누지 않습니다."
	}
	return ""
}

func (h *ASHandler) applyPostedASAssets(c echo.Context, as *model.ASReceipt) error {
	if h == nil || h.wbRepo == nil || as == nil || c.FormValue("as_asset_picker") != "1" {
		return nil
	}
	ids := preferReceiptAsset(as.AssetID, formAssetIDs(c))
	if strings.TrimSpace(c.FormValue("split_mode")) == "each" {
		if len(ids) < 2 {
			return nil
		}
		targets := make([]repository.ASSplitTarget, 0, len(ids))
		for _, id := range ids {
			targets = append(targets, repository.ASSplitTarget{
				AssetID: id,
				Name:    c.FormValue("split_label_asset_" + id),
				Title:   c.FormValue("split_title_asset_" + id),
			})
		}
		base := strings.TrimSpace(c.FormValue("split_base_title"))
		if base == "" {
			base = model.FormatASWorkTitle(as.OrgName, as.ASNumber)
		}
		_, err := h.wbRepo.SplitASReceipt(as.ASID, base, targets)
		return err
	}
	has, err := h.wbRepo.HasASAssetSplit(as.ASID)
	if err != nil || has {
		return err
	}
	task, err := h.wbRepo.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil || task == nil {
		return err
	}
	return h.wbRepo.ReplaceTaskAssets(task.TaskID, ids)
}

func preferReceiptAsset(receipt string, ids []string) []string {
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return ids
	}
	found := false
	for _, id := range ids {
		if id == receipt {
			found = true
			break
		}
	}
	if !found {
		return ids
	}
	out := []string{receipt}
	for _, id := range ids {
		if id != receipt {
			out = append(out, id)
		}
	}
	return out
}

func (h *ASHandler) finishASSplitProgress(c echo.Context, as *model.ASReceipt) error {
	if h == nil || h.wbRepo == nil || as == nil {
		return nil
	}
	has, err := h.wbRepo.HasASAssetSplit(as.ASID)
	if err != nil || !has {
		return err
	}
	if strings.TrimSpace(as.ResultCode) == model.ResultDone || as.Status == "completed" {
		if err := h.wbRepo.MarkPrimaryASTaskComplete(as.ASID); err != nil {
			return err
		}
	}
	return h.wbRepo.SyncASReceiptStatusFromSplits(as.ASID)
}

func redirectASSplitErr(c echo.Context, loc string, err error) error {
	if err == nil {
		return nil
	}
	setFlashErrCookie(c, err.Error())
	if strings.Contains(loc, "?") {
		return c.Redirect(http.StatusSeeOther, loc+"&err=split")
	}
	return c.Redirect(http.StatusSeeOther, loc+"?err=split")
}
