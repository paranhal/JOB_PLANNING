package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	bulkDeleteMax         = 200
	bulkDeleteAuditReason = "업무 리스트 선택 삭제"
	msgBulkCSRF           = "보안 확인에 실패했습니다. 페이지를 새로고침한 뒤 다시 시도해 주세요."
	msgBulkReadOnly       = "읽기 전용에서는 삭제할 수 없습니다"
	msgBulkTooMany        = "한 번에 200건까지 선택할 수 있습니다"
	msgBulkMnt            = "정기점검 업무는 정기점검 화면에서 관리합니다"
	msgBulkCoop           = "협조 배정 업무는 여기서 삭제할 수 없습니다"
	msgBulkNoPerm         = "삭제 권한이 없습니다"
	msgBulkSubtasks       = "하위 업무가 있어 삭제할 수 없습니다"
	msgBulkComplete       = "완료 업무는 조직관리자·지원만 삭제할 수 있습니다"
	msgBulkOtherOrg       = "타 조직 업무"
	msgBulkGone           = "이미 삭제됨"
	flashBulkCookie       = "jp_flash_bulk"
)

type bulkSkipItem struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Reason string `json:"reason"`
}

type bulkPreviewRow struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Org      string `json:"org"`
	Schedule string `json:"schedule"`
	Assignee string `json:"assignee"`
	Complete bool   `json:"complete"`
}

type bulkDeleteFlash struct {
	Deleted int            `json:"deleted"`
	Skipped []bulkSkipItem `json:"skipped"`
}

func formIDList(c echo.Context) []string {
	if c == nil || c.Request() == nil {
		return nil
	}
	_ = c.Request().ParseForm()
	raw := append([]string{}, c.Request().PostForm["ids"]...)
	raw = append(raw, c.Request().PostForm["ids[]"]...)
	seen := map[string]bool{}
	var out []string
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func bulkReturnURL(c echo.Context) string {
	if u := safeSameOriginPath(c, c.FormValue("back")); u != "" {
		return u
	}
	if c != nil && c.Request() != nil {
		if u := safeSameOriginPath(c, c.Request().Referer()); u != "" {
			return u
		}
	}
	return "/workboard/board?view=list"
}

func setBulkDeleteFlash(c echo.Context, n int, skipped []bulkSkipItem) {
	payload, err := json.Marshal(bulkDeleteFlash{Deleted: n, Skipped: skipped})
	if err != nil {
		return
	}
	c.SetCookie(sessionCookie(flashBulkCookie, url.QueryEscape(string(payload)), 120))
}

func injectBulkDeleteFlash(c echo.Context, data map[string]interface{}) {
	if c == nil || data == nil || c.Request() == nil {
		return
	}
	ck, err := c.Cookie(flashBulkCookie)
	if err != nil || ck == nil || strings.TrimSpace(ck.Value) == "" {
		return
	}
	raw, uerr := url.QueryUnescape(ck.Value)
	if uerr != nil {
		raw = ck.Value
	}
	var pop bulkDeleteFlash
	if json.Unmarshal([]byte(raw), &pop) != nil {
		return
	}
	data["BulkDeletePopup"] = pop
	if !c.Response().Committed {
		c.SetCookie(sessionCookie(flashBulkCookie, "", -1))
	}
}

func logBulkDeleteDeny(c echo.Context, stage, reason string, n int) {
	log.Printf("bulk-delete %s deny reason=%s user=%s org=%s n=%d",
		stage, reason, ctxString(c, "username"), currentOrg(c), n)
}

func (h *WorkboardHandler) BulkDeletePreview(c echo.Context) error {
	ids := formIDList(c)
	if !requireCSRF(c) {
		logBulkDeleteDeny(c, "preview", "csrf", len(ids))
		return c.JSON(http.StatusForbidden, map[string]string{"error": msgBulkCSRF})
	}
	if isReadOnly(c) || viewAsActive(c) {
		logBulkDeleteDeny(c, "preview", "readonly", len(ids))
		return c.JSON(http.StatusForbidden, map[string]string{"error": msgBulkReadOnly})
	}
	if len(ids) > bulkDeleteMax {
		logBulkDeleteDeny(c, "preview", "too_many", len(ids))
		return c.JSON(http.StatusBadRequest, map[string]string{"error": msgBulkTooMany})
	}
	var del []bulkPreviewRow
	var skip []bulkSkipItem
	for _, id := range ids {
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			skip = append(skip, bulkSkipItem{ID: id, Reason: msgBulkGone})
			continue
		}
		reason := h.listDeleteBlockReason(c, t)
		row := bulkPreviewRow{
			ID: t.TaskID, Kind: boardTypeStyleOf(*t).Label, Title: t.Title,
			Org: strings.TrimSpace(t.OrgName), Schedule: strings.TrimSpace(t.WorkDate),
			Assignee: t.Assignee, Complete: t.Status == model.WBTaskComplete,
		}
		if row.Schedule == "" {
			row.Schedule = t.DueDate
		}
		if reason != "" {
			skip = append(skip, bulkSkipItem{ID: id, Title: t.Title, Reason: reason})
			continue
		}
		del = append(del, row)
	}
	return c.JSON(http.StatusOK, map[string]any{"deletable": del, "skipped": skip})
}

func (h *WorkboardHandler) BulkDelete(c echo.Context) error {
	ret := bulkReturnURL(c)
	jsonOut := wantsJSONError(c)
	fail := func(code int, msg string) error {
		if jsonOut {
			return c.JSON(code, map[string]string{"error": msg})
		}
		setFlashErrCookie(c, msg)
		return c.Redirect(http.StatusSeeOther, ret)
	}
	ids := formIDList(c)
	if !requireCSRF(c) {
		logBulkDeleteDeny(c, "delete", "csrf", len(ids))
		return fail(http.StatusForbidden, msgBulkCSRF)
	}
	if isReadOnly(c) || viewAsActive(c) {
		logBulkDeleteDeny(c, "delete", "readonly", len(ids))
		return fail(http.StatusForbidden, msgBulkReadOnly)
	}
	if len(ids) > bulkDeleteMax {
		logBulkDeleteDeny(c, "delete", "too_many", len(ids))
		return fail(http.StatusBadRequest, msgBulkTooMany)
	}
	var deleted []string
	var skipped []bulkSkipItem
	cascadeGone := map[string]bool{}
	for _, id := range ids {
		if cascadeGone[id] {
			deleted = append(deleted, id)
			continue
		}
		t, err := h.repo.GetTask(id)
		if err != nil {
			skipped = append(skipped, bulkSkipItem{ID: id, Reason: "삭제 중 오류가 발생했습니다"})
			continue
		}
		if t == nil {
			skipped = append(skipped, bulkSkipItem{ID: id, Reason: msgBulkGone})
			continue
		}
		if reason := h.listDeleteBlockReason(c, t); reason != "" {
			skipped = append(skipped, bulkSkipItem{ID: id, Title: t.Title, Reason: reason})
			continue
		}
		ok, skipReason, cascade := h.deleteTaskForList(c, t)
		for _, cid := range cascade {
			cascadeGone[cid] = true
		}
		if ok {
			deleted = append(deleted, id)
			continue
		}
		skipped = append(skipped, bulkSkipItem{ID: id, Title: t.Title, Reason: skipReason})
	}
	if len(deleted) > 0 {
		invalidateCompanyWeeklyDownloads()
	}
	if jsonOut {
		return c.JSON(http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped})
	}
	setBulkDeleteFlash(c, len(deleted), skipped)
	return c.Redirect(http.StatusSeeOther, ret)
}

func (h *WorkboardHandler) listDeleteBlockReason(c echo.Context, t *model.WorkTask) string {
	if t == nil {
		return msgBulkGone
	}
	if isReadOnly(c) || viewAsActive(c) {
		return msgBulkReadOnly
	}
	org := currentOrg(c)
	if org != repository.OrgAll && strings.TrimSpace(t.OrgID) != "" && t.OrgID != org {
		return msgBulkOtherOrg
	}
	if h.repo != nil && h.repo.TaskCooperationID(t.TaskID) != "" {
		return msgBulkCoop
	}
	switch strings.TrimSpace(t.SourceType) {
	case model.WBSourceMaintenance:
		return msgBulkMnt
	case model.WBSourceAS:
		if h.asH == nil {
			return msgBulkNoPerm
		}
		as, _ := h.asH.repo.GetByID(currentOrg(c), t.SourceID)
		if as == nil {
			if other, _ := h.asH.repo.GetByID(repository.OrgAll, t.SourceID); other != nil {
				return msgBulkOtherOrg
			}
			return msgBulkGone
		}
		if !h.asH.canDeleteThis(c, as) {
			return msgBulkNoPerm
		}
		if linked, _ := h.asH.repo.HasLinkedAS(as.ASID); linked {
			return msgASDeleteLinked
		}
		return ""
	}
	if t.Status == model.WBTaskComplete && !canDeleteCompletedTask(c) {
		return msgBulkComplete
	}
	if !canEditTask(c, t.Assignee, t.AssigneeUserID, t.WorkGroupID) {
		return msgBulkNoPerm
	}
	if n, err := h.repo.CountSubtasks(t.TaskID); err == nil && n > 0 {
		return msgBulkSubtasks
	}
	if t.RecurrenceRole == model.RecurrenceRoleOccurrence {
		return "실행 작업은 상위 업무에서 일정을 바꾸세요"
	}
	return ""
}

func canDeleteCompletedTask(c echo.Context) bool {
	if isVisionRole(c) {
		return true
	}
	if model.NormalizeRole(currentRole(c)) == model.RoleOrgAdmin && orgAdminInHome(c) {
		return true
	}
	return isOfficeRole(c)
}

func (h *WorkboardHandler) deleteTaskForList(c echo.Context, t *model.WorkTask) (ok bool, reason string, cascade []string) {
	if t == nil {
		return false, msgBulkGone, nil
	}
	if strings.TrimSpace(t.SourceType) == model.WBSourceAS {
		asID := strings.TrimSpace(t.SourceID)
		if h.asH == nil {
			return false, msgBulkNoPerm, nil
		}
		cascade = h.repo.TaskIDsBySource(model.WBSourceAS, asID)
		_, deny, err := h.asH.deleteASReceipt(c, asID, bulkDeleteAuditReason)
		if err != nil && deny == "" {
			return false, "삭제 중 오류가 발생했습니다", nil
		}
		if deny != "" {
			if deny == msgASDeleteGone {
				if err := h.repo.DeleteTask(t.TaskID); err != nil {
					return false, "삭제 중 오류가 발생했습니다", nil
				}
				accessLog(c, auditlog.Record{
					Action: auditlog.ActionDelete, TargetTable: "work_tasks", TargetID: t.TaskID,
					Detail: t.Title, Reason: bulkDeleteAuditReason, Result: auditlog.ResultOK,
				})
				return true, "", []string{t.TaskID}
			}
			return false, deny, nil
		}
		return true, "", cascade
	}
	if t.RecurrenceRole == model.RecurrenceRoleParent {
		if err := h.repo.DeleteParentTask(t.TaskID); err != nil {
			if _, has := model.HasSubtasksN(err); has {
				return false, msgBulkSubtasks, nil
			}
			return false, err.Error(), nil
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionDelete, TargetTable: "work_tasks", TargetID: t.TaskID,
			Detail: t.Title, Reason: bulkDeleteAuditReason, Result: auditlog.ResultOK,
		})
		return true, "", nil
	}
	if err := h.repo.DeleteTask(t.TaskID); err != nil {
		return false, "삭제 중 오류가 발생했습니다", nil
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionDelete, TargetTable: "work_tasks", TargetID: t.TaskID,
		Detail: t.Title, Reason: bulkDeleteAuditReason, Result: auditlog.ResultOK,
	})
	return true, "", nil
}

func (h *WorkboardHandler) annotateListDelete(c echo.Context, items []boardItem) {
	for i := range items {
		reason := h.listDeleteBlockReason(c, &items[i].Task)
		items[i].DeleteDisabledReason = reason
		items[i].CanDelete = reason == ""
	}
}
