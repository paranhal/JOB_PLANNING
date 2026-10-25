package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	msgASDeleteCSRF     = "보안 확인에 실패했습니다. 페이지를 새로고침한 뒤 다시 시도해 주세요."
	msgASDeletePerm     = "삭제 권한이 없습니다"
	msgASDeleteLinked   = "이관/재접수로 연결된 접수는 삭제할 수 없습니다"
	msgASDeleteGone     = "접수를 찾을 수 없습니다"
	msgASDeleteLock     = "완료·종료된 접수는 잠금을 해제한 뒤에 삭제할 수 있습니다"
	msgASDeleteOtherOrg = "타 조직 업무"
)

func asDeleteReturnURL(c echo.Context) string {
	if u := safeSameOriginPath(c, c.FormValue("back")); u != "" {
		return u
	}
	if c != nil && c.Request() != nil {
		if u := safeSameOriginPath(c, c.Request().Referer()); u != "" {
			return u
		}
	}
	return "/as"
}

// deleteASReceipt 95-38 삭제 핵심. CSRF는 호출 측에서 검사한다.
// deny 가 비어 있지 않으면 사용자에게 그 이유를 보여 주고 접수는 그대로 둔다.
func (h *ASHandler) deleteASReceipt(c echo.Context, id, auditReason string) (number string, deny string, err error) {
	id = strings.TrimSpace(id)
	if h == nil || h.repo == nil || id == "" {
		return "", msgASDeleteGone, nil
	}
	as, err := h.repo.GetByID(currentOrg(c), id)
	if err != nil {
		return "", "", err
	}
	if as == nil {
		if other, _ := h.repo.GetByID(repository.OrgAll, id); other != nil {
			accessLog(c, auditlog.Record{
				Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: id,
				Detail: "타 조직 삭제 거부", Reason: "타조직", Result: auditlog.ResultDeny, BeforeJSON: toJSON(other),
			})
			return "", otherOrgReason("", other.OrgID), nil
		}
		return "", msgASDeleteGone, nil
	}
	if !h.canDeleteThis(c, as) {
		reason := "권한없음"
		msg := msgASDeletePerm
		if isASClosedStatus(as.Status) && isAdminRole(c) && !h.canModifyAS(c, as) {
			reason = "잠금"
			msg = msgASDeleteLock
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: as.ASID,
			SubjectType: "customer", SubjectID: as.CustomerID, SubjectName: as.OrgName,
			Detail: reason, Reason: reason, Result: auditlog.ResultDeny, BeforeJSON: toJSON(as),
		})
		return as.ASNumber, msg, nil
	}
	if linked, _ := h.repo.HasLinkedAS(as.ASID); linked {
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: as.ASID,
			SubjectType: "customer", SubjectID: as.CustomerID, SubjectName: as.OrgName,
			Detail: "연결 건 삭제 거부", Reason: "연결", Result: auditlog.ResultDeny, BeforeJSON: toJSON(as),
		})
		return as.ASNumber, msgASDeleteLinked, nil
	}
	if err := h.repo.Delete(id); err != nil {
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: as.ASID,
			SubjectType: "customer", SubjectID: as.CustomerID, SubjectName: as.OrgName,
			Detail: err.Error(), Reason: auditReason, Result: auditlog.ResultDeny, BeforeJSON: toJSON(as),
		})
		return as.ASNumber, "삭제 중 오류가 발생했습니다", err
	}
	reason := strings.TrimSpace(auditReason)
	if reason == "" {
		reason = accessReason(c, "AS 접수 삭제")
	}
	if asReceiverIsSelf(c, as) && !(isVisionRole(c) || (model.NormalizeRole(currentRole(c)) == model.RoleOrgAdmin && orgAdminInHome(c))) {
		if reason != "업무 리스트 선택 삭제" {
			reason = "접수자 본인 삭제"
		}
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: as.ASID,
		SubjectType: "customer", SubjectID: as.CustomerID, SubjectName: as.OrgName,
		Detail: "AS 접수 삭제 " + as.ASNumber, Reason: reason, Result: auditlog.ResultOK, BeforeJSON: toJSON(as),
	})
	return as.ASNumber, "", nil
}

func (h *ASHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	ret := asDeleteReturnURL(c)
	if !requireCSRF(c) {
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionDelete, TargetTable: "as_receipts", TargetID: id,
			Detail: "CSRF 없음", Reason: "CSRF", Result: auditlog.ResultDeny,
		})
		setFlashErrCookie(c, msgASDeleteCSRF)
		return c.Redirect(http.StatusSeeOther, ret)
	}
	num, deny, err := h.deleteASReceipt(c, id, accessReason(c, "AS 접수 삭제"))
	if err != nil && deny == "" {
		return err
	}
	if deny != "" {
		setFlashErrCookie(c, deny)
		if deny == msgASDeleteLock {
			return h.redirectLocked(c, id, "show")
		}
		return c.Redirect(http.StatusSeeOther, ret)
	}
	if num != "" {
		setFlashOKCookie(c, flashOKDeleted, []string{num})
	}
	return c.Redirect(http.StatusSeeOther, ret)
}
