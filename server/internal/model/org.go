package model

import "strings"

// Org 조직 (§52.2)
type Org struct {
	OrgID     string
	OrgNo     string
	OrgName   string
	ShortName string
	Kind      string
	IsActive  bool
	SortOrder int
	Note      string
	CreatedAt string
	CreatedBy string
}

const (
	OrgIDLibrary   = "O01"
	OrgNoLibrary   = "O01"
	OrgNameLibrary = "도서관사업팀"

	OrgKindBusiness = "business"
	OrgKindSupport  = "support"
)

func NormalizeOrgKind(kind string) string {
	if strings.TrimSpace(kind) == OrgKindSupport {
		return OrgKindSupport
	}
	return OrgKindBusiness
}

func (o Org) NavLabel() string {
	if s := strings.TrimSpace(o.ShortName); s != "" {
		return s
	}
	return strings.TrimSpace(o.OrgName)
}
