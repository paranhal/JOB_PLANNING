package model

import "html/template"

// ASActionMissing 조치 이력이 없을 때 검색·비슷한 사례에 쓰는 문구. §12.11.5
const ASActionMissing = "조치 기록 없음"

// ASSearchFilter 접수·조치 본문 검색. §4.5 기준일은 넣지 않는다. §12.11.4
type ASSearchFilter struct {
	Query       string
	DateFrom    string
	DateTo      string
	CustomerID  string
	Product     string
	CauseType   string
	ProcessType string
	Assigned    string
	Sort        string // relevance | newest
	KeywordID     string // 키워드 모아보기. §12.11.6
	CauseCat      string // §34.3.3 cause_cat*
	RequireAction bool   // 조치 기록 있는 건만. §41.1.2
	Page          int
	PageSize      int
}

// ASSearchHit 검색 한 줄. 증상·조치를 나란히 보여 주기 위한 원문.
type ASSearchHit struct {
	ASID         string
	ASNumber     string
	OrgName      string
	ReceiptDate  string
	Symptom      string
	Action       string
	CauseName    string
	HasAction     bool
	SymptomHTML   template.HTML
	ActionHTML    template.HTML
	AssignedTo    string
	CompleteDate  string
	CustomerID    string
	MatchSymptom  bool
	MatchAction   bool
	SymptomLong   bool
	LeadDays      int
	LeadLabel     string
}

// ASKnowledgeSite 사이트 탭 한 줄. 건수는 ar.customer_id 기준. §41.1.1 · §41.3.2
type ASKnowledgeSite struct {
	CustomerID string
	OrgName    string
	Count      int
}

// ASKnowledgeSummary 사이트 탭 요약.
type ASKnowledgeSummary struct {
	OrgName       string
	Total         int
	WithAction    int
	LatestReceipt string
	AvgLeadLabel  string
}

// ASKeywordFreq 사이트에서 많이 나온 사전 낱말. as_keywords 재사용.
type ASKeywordFreq struct {
	Keyword string
	Count   int
}

// ASSimilarFilter 접수·조치 화면의 비슷한 사례. §12.11.5
type ASSimilarFilter struct {
	Query      string
	CustomerID string
	AssetID    string
	ExcludeID  string
	Limit      int
}

// ASSimilarCase 비슷한 과거 건 카드.
type ASSimilarCase struct {
	ASID           string `json:"as_id"`
	ASNumber       string `json:"as_number"`
	OrgName        string `json:"org_name"`
	ReceiptDate    string `json:"receipt_date"`
	ProcessDate    string `json:"process_date"`
	SymptomSummary string `json:"symptom_summary"`
	ActionSummary  string `json:"action_summary"`
	HasAction      bool   `json:"has_action"`
	Status         string `json:"status"`
	SameAsset      bool   `json:"same_asset"`
	SameCustomer   bool   `json:"same_customer"`
	SameProduct    bool   `json:"same_product"`
	CanReopen      bool   `json:"can_reopen"`
	WeightLabel    string `json:"weight_label"`
	CompleteDate   string `json:"complete_date,omitempty"`
	LeadDays       int    `json:"lead_days,omitempty"`
	LeadLabel      string `json:"lead_label,omitempty"`
}
