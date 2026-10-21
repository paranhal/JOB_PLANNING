package model

const (
	SchoolImportMaxBytes = 10 << 20
	SchoolImportMaxRows  = 3000
)

type SchoolImportPreview struct {
	FileName     string
	NewN         int
	SkipN        int
	ErrN         int
	NeedParent   bool
	CreateParent bool
	Rows         []SchoolImportRow
}

type SchoolImportRow struct {
	Line       int
	Action     string // new|skip|err
	Msg        string
	ParentName string
	ParentID   string
	Name       string
	OrgCode    string
	Region     string
	Address    string
	Phone      string
	Notes      string
	OrgID      string
}
