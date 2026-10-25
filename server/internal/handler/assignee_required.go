package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const (
	errAssigneeRequired     = "assignee_required"
	msgAssigneeRequired     = "담당자를 선택하세요"
	focusAssigneeQuery      = "err=assignee_required&focus=assignee"
	assigneeFocusTargetName = "assignee"
)

func assigneeFilled(name, uid, groupID string) bool {
	return strings.TrimSpace(name) != "" || strings.TrimSpace(uid) != "" || strings.TrimSpace(groupID) != ""
}

func rejectAssigneeRequired(c echo.Context, path string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Retarget", "[data-focus-target=assignee]")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return c.HTML(http.StatusUnprocessableEntity,
			`<p class="text-red-600 text-sm" data-assignee-error>`+msgAssigneeRequired+`</p>`)
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	if strings.Contains(path, "err=assignee_required") {
		return c.Redirect(http.StatusSeeOther, path)
	}
	return c.Redirect(http.StatusSeeOther, path+sep+focusAssigneeQuery)
}

func applySupportAssigneeDefault(c echo.Context, as *model.ASReceipt) {
	if as == nil || currentRole(c) != model.RoleSupport {
		return
	}
	if assigneeFilled(as.AssignedTo, as.AssignedUserID, as.WorkGroupID) {
		return
	}
	as.AssignedUserID = strings.TrimSpace(currentUserID(c))
	as.AssignedTo = strings.TrimSpace(ctxString(c, "user_name"))
}

func asWorkerDefault(c echo.Context, as *model.ASReceipt) string {
	if as != nil && strings.TrimSpace(as.WorkGroupID) != "" {
		self := strings.TrimSpace(ctxString(c, "user_name"))
		if memberOfGroup(c, as.WorkGroupID) && self != "" {
			return self
		}
	}
	if as != nil {
		if n := strings.TrimSpace(as.AssignedTo); n != "" {
			return n
		}
	}
	if currentRole(c) == model.RoleSupport {
		return strings.TrimSpace(ctxString(c, "user_name"))
	}
	return ""
}

func stampOnBehalf(c echo.Context, ownerUID, ownerName string) (actedUID, actedName string, onBehalf bool) {
	actedUID = strings.TrimSpace(ctxString(c, "user_id"))
	actedName = strings.TrimSpace(ctxString(c, "user_name"))
	if actedName == "" {
		actedName = strings.TrimSpace(ctxString(c, "username"))
	}
	ownerUID, ownerName = strings.TrimSpace(ownerUID), strings.TrimSpace(ownerName)
	if ownerUID == "" && ownerName == "" {
		return actedUID, actedName, false
	}
	onBehalf = !isSelfAssign(actedUID, actedName, ctxString(c, "username"), ownerUID, ownerName, "")
	return actedUID, actedName, onBehalf
}
