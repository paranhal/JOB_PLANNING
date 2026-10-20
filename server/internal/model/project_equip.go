package model

import "strings"

const (
	YearPlanned   = "planned"
	YearActive    = "active"
	YearDone      = "done"
	YearCancelled = "cancelled"

	SitePlanned  = "planned"
	SiteActive   = "active"
	SiteDone     = "done"
	SiteExcluded = "excluded"

	EquipPlanned    = "planned"
	EquipDelivered  = "delivered"
	EquipInstalled  = "installed"
	EquipAccepted   = "accepted"
	EquipReplaced   = "replaced"
	EquipRemoved    = "removed"

	EquipFieldText   = "text"
	EquipFieldNumber = "number"
	EquipFieldDate   = "date"
	EquipFieldSelect = "select"
)

type ContractYear struct {
	ID         string
	ProjectID  string
	YearNo     int
	Label      string
	StartDate  string
	EndDate    string
	Amount     int
	Status     string
	Note       string
	OrgID      string
	CreatedBy  string
	IsCurrent  bool
}

type ProjectSite struct {
	ID          string
	ProjectID   string
	CustomerID  string
	SiteName    string
	Region      string
	Address     string
	ContactNote string
	Status      string
	SortOrder   int
	IsActive    bool
	Note        string
	OrgID       string
	EquipN      int
}

type EquipmentCategory struct {
	ID        string
	OrgID     string
	ParentID  string
	Code      string
	Name      string
	SortOrder int
	IsActive  bool
	InUse     bool
	Fields    []EquipmentField
}

type EquipmentField struct {
	ID         string
	CategoryID string
	FieldKey   string
	Label      string
	FieldType  string
	Options    string
	Required   bool
	SortOrder  int
	IsActive   bool
	InUse      bool
}

type ProjectEquipment struct {
	ID              string
	ProjectID       string
	SiteID          string
	SiteName        string
	YearID          string
	YearNo          int
	CategoryID      string
	CategoryCode    string
	ProductName     string
	Model           string
	Manufacturer    string
	Qty             int
	SerialNo        string
	InstallLocation string
	InstallDate     string
	Status          string
	AssetID         string
	Note            string
	OrgID           string
	IsActive        bool
	Values          map[string]string
	SerialWarn      bool
}

type EquipImportBatch struct {
	ID         string
	ProjectID  string
	FileName   string
	RowsTotal  int
	RowsOK     int
	RowsError  int
	Status     string
	CreatedBy  string
}

type EquipImportPreview struct {
	SitesNew    int
	SitesUpd    int
	EquipNew    int
	EquipUpd    int
	ErrN        int
	Rows        []EquipImportRow
	FileName    string
}

type EquipImportRow struct {
	Sheet  string
	Line   int
	Kind   string // site|equip
	Action string // new|upd|err
	Key    string
	Msg    string
	Site   *ProjectSite
	Equip  *ProjectEquipment
}

func YearStatusLabel(s string) string {
	switch strings.TrimSpace(s) {
	case YearPlanned:
		return "예정"
	case YearActive:
		return "진행"
	case YearDone:
		return "완료"
	case YearCancelled:
		return "취소"
	default:
		return s
	}
}

func SiteStatusLabel(s string) string {
	switch strings.TrimSpace(s) {
	case SitePlanned:
		return "예정"
	case SiteActive:
		return "운영"
	case SiteDone:
		return "종료"
	case SiteExcluded:
		return "제외"
	default:
		return s
	}
}

func EquipStatusLabel(s string) string {
	switch strings.TrimSpace(s) {
	case EquipPlanned:
		return "예정"
	case EquipDelivered:
		return "납품"
	case EquipInstalled:
		return "설치"
	case EquipAccepted:
		return "검수"
	case EquipReplaced:
		return "교체"
	case EquipRemoved:
		return "철수"
	default:
		return s
	}
}

func ParseSiteStatus(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case SitePlanned, SiteActive, SiteDone, SiteExcluded, "예정", "운영", "종료", "제외":
		if s == "예정" {
			return SitePlanned
		}
		if s == "운영" {
			return SiteActive
		}
		if s == "종료" {
			return SiteDone
		}
		if s == "제외" {
			return SiteExcluded
		}
		return s
	default:
		return ""
	}
}

func ParseEquipStatus(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case EquipPlanned, EquipDelivered, EquipInstalled, EquipAccepted, EquipReplaced, EquipRemoved:
		return s
	case "예정":
		return EquipPlanned
	case "납품":
		return EquipDelivered
	case "설치":
		return EquipInstalled
	case "검수":
		return EquipAccepted
	case "교체":
		return EquipReplaced
	case "철수":
		return EquipRemoved
	default:
		return ""
	}
}
