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
