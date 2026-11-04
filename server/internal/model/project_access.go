package model

import "strings"

const (
	ProjectAccessNone    = ""
	ProjectAccessOwner   = "owner"
	ProjectAccessRelated = "related"
	ProjectAccessViewer  = "viewer"
)

// ProjectAccess 사업에 대한 조직 역할. orgID 가 "*" 이면 전 조직(비젼관리자) owner.
// 전 직원 조회(95-2E)는 viewer. 전 팀 공개(is_shared, share_to 빈 값)도 viewer 이며 업무 등록은 불가.
func ProjectAccess(p WorkProject, orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ProjectAccessNone
	}
	if orgID == "*" {
		return ProjectAccessOwner
	}
	home := strings.TrimSpace(p.OrgID)
	if home != "" && home == orgID {
		return ProjectAccessOwner
	}
	for _, id := range p.RelatedOrgIDs {
		if strings.TrimSpace(id) == orgID {
			return ProjectAccessRelated
		}
	}
	return ProjectAccessViewer
}

// CanRegisterWorkOnProject 주관·유관만 이 사업으로 업무를 등록할 수 있다(Q-1: 전 팀 공개는 조회만).
func CanRegisterWorkOnProject(p WorkProject, orgID string) bool {
	a := ProjectAccess(p, orgID)
	return a == ProjectAccessOwner || a == ProjectAccessRelated
}

// CanPickOtherProjects 타 부서 사업을 선택·저장할 수 있는 역할. C-Level 은 등록이 막혀 제외.
func CanPickOtherProjects(role string) bool {
	switch NormalizeRole(role) {
	case RoleOrgAdmin, RoleSupport, RoleVisionAdmin:
		return true
	default:
		return false
	}
}

func projectStatusClosed(status string) bool {
	switch strings.TrimSpace(status) {
	case WBProjectArchived, WBProjectComplete, "cancelled":
		return true
	default:
		return false
	}
}

func orgIDIn(id string, ids []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, x := range ids {
		if strings.TrimSpace(x) == id {
			return true
		}
	}
	return false
}

// ProjectInOwnPickScope 주관·유관 또는(조직관리자) 하위 조직 주관·유관.
func ProjectInOwnPickScope(p WorkProject, orgID string, extraOrgIDs []string) bool {
	orgID = strings.TrimSpace(orgID)
	if orgID == "*" {
		return true
	}
	if CanRegisterWorkOnProject(p, orgID) {
		return true
	}
	if orgIDIn(strings.TrimSpace(p.OrgID), extraOrgIDs) {
		return true
	}
	for _, id := range p.RelatedOrgIDs {
		if orgIDIn(id, extraOrgIDs) {
			return true
		}
	}
	return false
}

// CanLinkWorkProject 화면에 보이는 사업만 저장. includeClosed 가 아니면 보관·완료·취소 거절.
func CanLinkWorkProject(p WorkProject, orgID, role string, extraOrgIDs []string, includeClosed bool) bool {
	if !includeClosed && projectStatusClosed(p.Status) {
		return false
	}
	if ProjectInOwnPickScope(p, orgID, extraOrgIDs) {
		return true
	}
	return CanPickOtherProjects(role)
}
