package model

// Org 조직 (§52.2)
type Org struct {
	OrgID     string
	OrgNo     string
	OrgName   string
	ShortName string
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
)
