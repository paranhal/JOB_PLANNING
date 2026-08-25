package model

import (
	"fmt"
	"strconv"
	"strings"
)

// 영업 사업 단계 코드값. 라벨·확도는 codes 에서 읽는다 (§32.3.1).
const (
	SalesStageLead       = "lead"
	SalesStageContact    = "contact"
	SalesStageProposal   = "proposal"
	SalesStageQuote      = "quote"
	SalesStageRFP        = "rfp"
	SalesStageSubmit     = "submit"
	SalesStageWon        = "won"
	SalesStageContracted = "contracted"
	SalesStageLost       = "lost"
)

const (
	SalesPrecisionMonth   = "month"
	SalesPrecisionQuarter = "quarter"
	SalesPrecisionHalf    = "half"
	SalesPrecisionYear    = "year"
)

const (
	SalesStatusActive   = "active"
	SalesStatusLost     = "lost"
	SalesStatusPromoted = "promoted"
)

const (
	SalesCodeGroupStage      = "sales_stage"
	SalesCodeGroupProb       = "sales_stage_prob"
	SalesCodeGroupLeadSource = "sales_lead_source"
)

// SalesStageDef codes 에서 읽은 단계 정의
type SalesStageDef struct {
	Code        string
	Label       string
	Probability int
	SortOrder   int
}

func (d SalesStageDef) IsPostWin() bool {
	return d.Code == SalesStageWon || d.Code == SalesStageContracted
}

func (d SalesStageDef) IsLost() bool {
	return d.Code == SalesStageLost
}

// SalesProject 수주 전 영업 사업. work_projects 와 표를 나눈다 (§32.1.3).
type SalesProject struct {
	SalesID                 string
	Name                    string
	IsTentativeName         bool
	Stage                   string
	Probability             int
	HasOverride             bool
	OverrideValue           int
	CustomerID              string
	ProspectName            string
	ProspectRegion          string
	CustomerConfirmed       bool
	ExpectedYM              string
	ExpectedPrecision       string
	ExpectedYMConfirmed     bool
	ExpectedAmount          int
	ExpectedAmountConfirmed bool
	SalesOwner              string
	SalesOwnerID            string
	Competitor              string
	LeadSource              string
	LostReason              string
	Status                  string
	Notes                   string
	CreatedAt               string
	UpdatedAt               string

	CustomerName string
	StageLabel   string
}

// SalesStageHistory 단계 변경 이력 (§32.3.3)
type SalesStageHistory struct {
	HistoryID   string
	SalesID     string
	FromStage   string
	ToStage     string
	FromLabel   string
	ToLabel     string
	Reason      string
	ChangedBy   string
	ChangedByID string
	ChangedAt   string
}

func (p *SalesProject) EffectiveProbability() int {
	if p == nil {
		return 0
	}
	if p.HasOverride {
		return p.OverrideValue
	}
	return p.Probability
}

func (p *SalesProject) NameConfirmed() bool {
	return p != nil && !p.IsTentativeName && strings.TrimSpace(p.Name) != ""
}

func (p *SalesProject) CustomerValue() string {
	if p == nil {
		return ""
	}
	if s := strings.TrimSpace(p.CustomerName); s != "" {
		return s
	}
	if s := strings.TrimSpace(p.ProspectName); s != "" {
		return s
	}
	return strings.TrimSpace(p.CustomerID)
}

func (p *SalesProject) ConfirmedCount() (n, total int) {
	total = 4
	if p == nil {
		return 0, total
	}
	if p.NameConfirmed() {
		n++
	}
	if p.CustomerConfirmed {
		n++
	}
	if p.ExpectedYMConfirmed {
		n++
	}
	if p.ExpectedAmountConfirmed {
		n++
	}
	return n, total
}

func (p *SalesProject) ConfirmationLabel() string {
	n, total := p.ConfirmedCount()
	return fmt.Sprintf("정보 확정도 %d/%d", n, total)
}

func (p *SalesProject) UnconfirmedItems() []string {
	if p == nil {
		return []string{"사업명", "고객", "사업 시기", "예상 금액"}
	}
	var items []string
	if !p.NameConfirmed() {
		items = append(items, "사업명")
	}
	if !p.CustomerConfirmed {
		items = append(items, "고객")
	}
	if !p.ExpectedYMConfirmed {
		items = append(items, "사업 시기")
	}
	if !p.ExpectedAmountConfirmed {
		items = append(items, "예상 금액")
	}
	return items
}

func (p *SalesProject) RequireWonConfirmation() error {
	items := p.UnconfirmedItems()
	if len(items) == 0 {
		return nil
	}
	return fmt.Errorf("수주확정으로 넘기려면 %s을(를) 모두 확정해야 합니다", strings.Join(items, "·"))
}

func StageNeedsFullConfirm(stage string) bool {
	switch strings.TrimSpace(stage) {
	case SalesStageWon, SalesStageContracted:
		return true
	default:
		return false
	}
}

func (p *SalesProject) PeriodLabel() string {
	if p == nil {
		return ""
	}
	return FormatSalesPeriod(p.ExpectedYM, p.ExpectedPrecision)
}

func (p *SalesProject) AmountLabel() string {
	if p == nil || p.ExpectedAmount <= 0 {
		return ""
	}
	return formatSalesAmount(p.ExpectedAmount) + "원"
}

func (p *SalesProject) DisplayStage(def *SalesStageDef) string {
	label := strings.TrimSpace(p.Stage)
	if def != nil && def.Label != "" {
		label = def.Label
	}
	if p.StageLabel != "" && (def == nil || def.Label == "") {
		label = p.StageLabel
	}
	if def != nil && (def.IsPostWin() || def.IsLost()) {
		return label
	}
	if p.Stage == SalesStageWon || p.Stage == SalesStageContracted || p.Stage == SalesStageLost {
		return label
	}
	return fmt.Sprintf("%s · %d%%", label, p.EffectiveProbability())
}

func NormalizeSalesPrecision(p string) string {
	switch strings.TrimSpace(p) {
	case SalesPrecisionQuarter, SalesPrecisionHalf, SalesPrecisionYear:
		return strings.TrimSpace(p)
	default:
		return SalesPrecisionMonth
	}
}

func NormalizeSalesYM(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) >= 7 && s[4] == '-' {
		year := s[:4]
		month := s[5:7]
		if _, err := strconv.Atoi(year); err != nil {
			return ""
		}
		m, err := strconv.Atoi(month)
		if err != nil || m < 1 || m > 12 {
			return ""
		}
		return fmt.Sprintf("%s-%02d", year, m)
	}
	return ""
}

func FormatSalesPeriod(ym, precision string) string {
	ym = NormalizeSalesYM(ym)
	if ym == "" {
		return ""
	}
	year, _ := strconv.Atoi(ym[:4])
	month, _ := strconv.Atoi(ym[5:7])
	switch NormalizeSalesPrecision(precision) {
	case SalesPrecisionQuarter:
		q := (month-1)/3 + 1
		return fmt.Sprintf("%d년 %d분기", year, q)
	case SalesPrecisionHalf:
		if month <= 6 {
			return fmt.Sprintf("%d년 상반기", year)
		}
		return fmt.Sprintf("%d년 하반기", year)
	case SalesPrecisionYear:
		return fmt.Sprintf("%d년", year)
	default:
		return ym
	}
}

func formatSalesAmount(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		s = strconv.Itoa(-n)
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if n < 0 {
		return "-" + out
	}
	return out
}

func FindSalesStage(stages []SalesStageDef, code string) *SalesStageDef {
	code = strings.TrimSpace(code)
	for i := range stages {
		if stages[i].Code == code {
			return &stages[i]
		}
	}
	return nil
}

func IsSalesStageBackward(from, to *SalesStageDef) bool {
	if to == nil {
		return false
	}
	if to.IsLost() {
		return true
	}
	if from == nil {
		return false
	}
	return to.SortOrder < from.SortOrder
}

// LoadSalesStages codes 두 그룹을 합쳐 단계 정의를 만든다. 확도 숫자는 sales_stage_prob.code_name 이다.
func LoadSalesStages(stageCodes, probCodes []Code) []SalesStageDef {
	probs := map[string]int{}
	for _, c := range probCodes {
		n, err := strconv.Atoi(strings.TrimSpace(c.CodeName))
		if err != nil {
			continue
		}
		probs[strings.TrimSpace(c.CodeValue)] = n
	}
	var out []SalesStageDef
	for _, c := range stageCodes {
		code := strings.TrimSpace(c.CodeValue)
		if code == "" {
			continue
		}
		out = append(out, SalesStageDef{
			Code:        code,
			Label:       strings.TrimSpace(c.CodeName),
			Probability: probs[code],
			SortOrder:   c.SortOrder,
		})
	}
	if len(out) == 0 {
		return fallbackSalesStages()
	}
	return out
}

func fallbackSalesStages() []SalesStageDef {
	return []SalesStageDef{
		{Code: SalesStageLead, Label: "정보 입수", Probability: 10, SortOrder: 1},
		{Code: SalesStageContact, Label: "담당자 접촉", Probability: 10, SortOrder: 2},
		{Code: SalesStageProposal, Label: "제안 진행", Probability: 15, SortOrder: 3},
		{Code: SalesStageQuote, Label: "견적 요청", Probability: 20, SortOrder: 4},
		{Code: SalesStageRFP, Label: "RFP 제안", Probability: 30, SortOrder: 5},
		{Code: SalesStageSubmit, Label: "제안서 제출", Probability: 50, SortOrder: 6},
		{Code: SalesStageWon, Label: "수주확정", Probability: 90, SortOrder: 7},
		{Code: SalesStageContracted, Label: "계약완료", Probability: 100, SortOrder: 8},
		{Code: SalesStageLost, Label: "수주실패", Probability: 0, SortOrder: 9},
	}
}

func DefaultSalesStageCode() string { return SalesStageLead }
