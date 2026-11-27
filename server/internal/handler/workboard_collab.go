package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-225 §1 공동 작업 · §2 배정 범위 판정.
// 권한 함수(canWriteProjectWorkAction 등)가 저장소 없이 쓰는 자유 함수라서, 공동 작업·배정 조회는 기동할 때 연결하는 함수 변수로 둔다.

var (
	taskCollabLookup  func(taskID string) (scope string, userIDs []string)
	assignScopeLookup func(userID, taskID, targetType, targetID string) bool
	assignTaskLookup  func(userID, taskID string) bool
	taskMetaLookup    func(taskID string) (creatorUserID, packageID string)
)

func bindCollabLookups(r *repository.AssignmentRepo) {
	taskCollabLookup = r.CollabOf
	assignScopeLookup = r.AcceptedScope
	assignTaskLookup = r.HasAcceptedOnTask
	taskMetaLookup = r.TaskMeta
}

// collabAllows 공동 작업 범위에 든 사용자인지. ① 같은 조직 누구나 ② 지정한 사용자만(타 조직 포함).
// 조치·사이트·하부 작업을 다룰 권한만 넓히며, 업무 삭제·공동 작업 해제 같은 것은 포함하지 않는다.
func collabAllows(c echo.Context, t *model.WorkTask) bool {
	if t == nil || taskCollabLookup == nil || isReadOnly(c) || !canWriteWorkboard(c) {
		return false
	}
	scope, users := taskCollabLookup(t.TaskID)
	switch scope {
	case repository.CollabOrg:
		org := strings.TrimSpace(t.OrgID)
		return org != "" && (loginHomeOrg(c) == org)
	case repository.CollabMembers:
		uid := strings.TrimSpace(currentUserID(c))
		for _, id := range users {
			if id == uid {
				return true
			}
		}
	}
	return false
}

// canManageCollab 공동 작업을 정하거나 해제할 수 있는 사람: 등록자(작성자)·조직관리자.
func canManageCollab(c echo.Context, t *model.WorkTask) bool {
	if t == nil || isReadOnly(c) {
		return false
	}
	if isAdminRole(c) || currentRole(c) == model.RoleOrgAdmin {
		return true
	}
	uid := strings.TrimSpace(currentUserID(c))
	if uid == "" || taskMetaLookup == nil {
		return false
	}
	creator, _ := taskMetaLookup(t.TaskID)
	return creator == uid
}

// canCheckSiteByAssignment 수락한 배정 범위의 사이트 체크 권한(타 조직 수락자 포함). 사이트 배정이거나, 이 업무의 하부 업무 배정이면 업무의 모든 사이트를 덮는다.
func canCheckSiteByAssignment(c echo.Context, t *model.WorkTask, customerID string) bool {
	if t == nil || assignScopeLookup == nil || isReadOnly(c) || !canWriteWorkboard(c) {
		return false
	}
	uid := strings.TrimSpace(currentUserID(c))
	if uid == "" {
		return false
	}
	if assignScopeLookup(uid, t.TaskID, repository.AssignTargetSite, strings.TrimSpace(customerID)) {
		return true
	}
	if taskMetaLookup != nil {
		if _, pkg := taskMetaLookup(t.TaskID); pkg != "" && assignScopeLookup(uid, t.TaskID, repository.AssignTargetSub, pkg) {
			return true
		}
	}
	return false
}
