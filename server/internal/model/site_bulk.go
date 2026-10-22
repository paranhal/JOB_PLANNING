package model

const SiteBulkMax = 3000

const (
	SiteBulkSkipAlready  = "이미 등록됨"
	SiteBulkSkipOtherOrg = "타 팀 사이트"
	SiteBulkSkipInactive = "비활성"
	SiteBulkSkipPartner  = "협력사"
	SiteBulkSkipIndustry = "업종 불일치"
	SiteBulkSkipNotFound = "없는 고객"
	SiteBulkSkipOrg      = "권한/조직 불일치"
)

type SiteBulkIndustry struct {
	Key   string
	Label string
	Count int
}

type SiteBulkCandidate struct {
	CustomerID    string
	OrgName       string
	ParentName    string
	RegionPreview string
	ShortName     string
	Selectable    bool
	OtherOrgName  string
	WarnSimilar   bool
	WarnEmpty     bool
	WarnNoRegion  bool
}

type SiteBulkSkip struct {
	CustomerID string
	Name       string
	Reason     string
}

type SiteBulkRegisterInput struct {
	OrgID          string
	IndustryKey    string
	Cycle          string
	Rounds         int
	CustomerIDs    []string
	ShortNames     map[string]string
	ProductIDs     []string
	ActorLabel     string
	FallbackRegion string
}

type SiteBulkRegisterResult struct {
	Registered int
	Skipped    []SiteBulkSkip
	BatchID    string
	NoRegion   int
}

type SiteRegionBulkResult struct {
	Changed int
	Skipped []SiteBulkSkip
}

func ValidInspectionCycle(s string) bool {
	switch s {
	case "monthly", "odd_bimonthly", "even_bimonthly", "quarterly", "semi", "yearly":
		return true
	default:
		return false
	}
}

func InspectionCycleLabel(s string) string {
	switch s {
	case "monthly":
		return "월"
	case "odd_bimonthly":
		return "홀수격월"
	case "even_bimonthly":
		return "짝수격월"
	case "quarterly":
		return "분기"
	case "semi":
		return "반기"
	case "yearly":
		return "년1회"
	default:
		return s
	}
}
