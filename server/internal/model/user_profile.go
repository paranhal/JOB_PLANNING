package model

import "strings"

// NeedsMobile 관리자 외 핸드폰 필수 (§54.2).
func NeedsMobile(role string) bool {
	return !IsAdminGrade(role)
}

// NeedsEmail 조직관리자·비젼관리자는 이메일 필수 (§54.2).
func NeedsEmail(role string) bool {
	return IsAdminGrade(role)
}

// NeedsOrg 비젼관리자만 조직 빈 값 허용 (§54.2).
func NeedsOrg(role string) bool {
	return NormalizeRole(role) != RoleVisionAdmin
}

func (u *User) MissingProfileFields() []string {
	if u == nil {
		return nil
	}
	var miss []string
	if NeedsMobile(u.Role) && strings.TrimSpace(u.Mobile) == "" {
		miss = append(miss, "mobile")
	}
	if NeedsEmail(u.Role) && strings.TrimSpace(u.Email) == "" {
		miss = append(miss, "email")
	}
	if NeedsOrg(u.Role) && strings.TrimSpace(u.OrgID) == "" {
		miss = append(miss, "org")
	}
	return miss
}

func (u *User) NeedsProfileFill() bool {
	if u == nil {
		return false
	}
	if u.ProfileDone && len(u.MissingProfileFields()) == 0 {
		return false
	}
	return len(u.MissingProfileFields()) > 0
}

func (u *User) MarkProfileDoneIfComplete() {
	if u == nil {
		return
	}
	u.ProfileDone = len(u.MissingProfileFields()) == 0
}

// ProfileFieldError 필수 칸 검사. 비어 있지 않으면 "".
func (u *User) ProfileFieldError() string {
	for _, f := range u.MissingProfileFields() {
		switch f {
		case "mobile":
			return "핸드폰 번호를 입력하세요."
		case "email":
			return "이메일을 입력하세요."
		case "org":
			return "조직을 고르세요."
		}
	}
	return ""
}
