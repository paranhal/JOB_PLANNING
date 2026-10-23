package model

import "strings"

const (
	YearPlanned   = "planned"
	YearActive    = "active"
	YearDone      = "done"
	YearCancelled = "cancelled"

	SitePlanned    = "planned"
	SiteActive     = "active" // 옛값. 075 이후 in_progress 로 읽는다
	SiteInProgress = "in_progress"
	SiteInstalled  = "installed"
	SiteInspected  = "inspected"
	SiteDone       = "done" // 옛값. 075 이후 inspected 로 읽는다
	SiteExcluded   = "excluded"

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
	ContactNote    string
	Status         string
	InstalledAt    string
	InspectedAt    string
	ExcludedReason string
	SortOrder      int
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

type EquipCatImportBatch struct {
	ID        string
	OrgScope  string
	FileName  string
	RowsTotal int
	RowsOK    int
	RowsError int
	Status    string
	CreatedBy string
}

type EquipCatImportPreview struct {
	NewN     int
	UpdN     int
	SameN    int
	ErrN     int
	WarnN    int
	Rows     []EquipCatImportRow
	FileName string
	OrgScope string
}

type EquipCatImportRow struct {
	Sheet      string
	Line       int
	Kind       string // cat|field
	Action     string // new|upd|same|err
	Key        string
	Msg        string
	OrgID      string
	Code       string
	ParentCode string
	Name       string
	FieldKey   string
	Label      string
	FieldType  string
	Options    string
	SortOrder  int
	IsActive   bool
	Required   bool
	Cat        *EquipmentCategory
	Field      *EquipmentField
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
	switch NormalizeSiteStatus(s) {
	case SitePlanned:
		return "예정"
	case SiteInProgress:
		return "진행"
	case SiteInstalled:
		return "설치완료"
	case SiteInspected:
		return "검수완료"
	case SiteExcluded:
		return "제외"
	default:
		return s
	}
}

func NormalizeSiteStatus(s string) string {
	switch strings.TrimSpace(s) {
	case SitePlanned, "예정":
		return SitePlanned
	case SiteActive, SiteInProgress, "운영", "진행":
		return SiteInProgress
	case SiteInstalled, "설치완료", "설치":
		return SiteInstalled
	case SiteDone, SiteInspected, "종료", "검수완료", "검수":
		return SiteInspected
	case SiteExcluded, "제외":
		return SiteExcluded
	default:
		return ""
	}
}

func SiteStatusCountsAsInstalled(s string) bool {
	n := NormalizeSiteStatus(s)
	return n == SiteInstalled || n == SiteInspected
}

// WorkCompleteSiteStatus 일일 업무 「작업」 완료 시 연동하는 사이트 상태.
// 구축·공사·기타 모두 설치완료. 검수완료는 95-3에서 사람이 기록한다.
func WorkCompleteSiteStatus() string {
	return SiteInstalled
}

func SiteAcceptsWorkCompleteStatus(current string) bool {
	switch NormalizeSiteStatus(current) {
	case SitePlanned, SiteInProgress:
		return true
	default:
		return false
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
	return NormalizeSiteStatus(s)
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
