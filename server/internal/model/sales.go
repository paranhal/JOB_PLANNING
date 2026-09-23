package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 영업 사업 단계 코드값. 라벨·확도는 codes 에서 읽는다 (§32.3.1).
const (
	// 44 이전. legacy_stage·이력 표시 전용
	SalesStageLead        = "lead"
	SalesStageContact     = "contact"
	SalesStageProposal    = "proposal"
	SalesStageNegotiation = "negotiation"
	SalesStageQuote       = "quote"      // 폐 8단계. 이력·legacy_stage 전용
	SalesStageRFP         = "rfp"        // 폐 8단계. 이력·legacy_stage 전용
	SalesStageSubmit      = "submit"     // 폐 8단계. 이력·legacy_stage 전용
	SalesStageWon         = "won"        // 44 이전. legacy_stage·이력 표시 전용
	SalesStageContracted  = "contracted" // 폐 8단계. 이력·legacy_stage 전용
	SalesStageLost        = "lost"       // 44 이전. legacy_stage·이력 표시 전용
)

const (
	SalesActTypeResearch = "research"
	SalesActTypeCall     = "call"
	SalesActTypeVisit    = "visit"
	SalesActTypeOnline   = "online"
	SalesActTypeMail     = "mail"
	SalesActTypeInternal = "internal"
	SalesActTypeMaterial = "material"
	SalesActTypeQuote    = "quote"
	SalesActTypeProposal = "proposal"
	SalesActTypeRFP      = "rfp"
	SalesActTypeBid      = "bid"
	SalesActTypeOther    = "other"
	SalesActTypeRequirement = "requirement"
	SalesActTypeRFPReceived = "rfp_received"
)

// SalesActivityChipCodes 목록 칩 6개. 순서 고정 (§32.8.2).
func SalesActivityChipCodes() []string {
	return []string{
		SalesActTypeVisit, SalesActTypeCall, SalesActTypeMail,
		SalesActTypeOnline, SalesActTypeInternal, SalesActTypeResearch,
	}
}

func SalesActivityIsChipType(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesActTypeVisit, SalesActTypeCall, SalesActTypeMail,
		SalesActTypeOnline, SalesActTypeInternal, SalesActTypeResearch:
		return true
	default:
		return false
	}
}

// SalesContractUnsignedDays 수주 후 contracted_at 이 이보다 오래 비면 미계획함 (§32.10).
const SalesContractUnsignedDays = 30

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
	SalesStatusDormant  = "dormant"
)

const (
	SalesBizBuild        = "build"
	SalesBizMaintenance  = "maintenance"
	SalesBizGoods        = "goods"
	SalesBizDevelop      = "develop"
	SalesBizConstruction = "construction"
	SalesBizEtc          = "etc"

	SalesBudgetUnknown      = "unknown"
	SalesBudgetNotReflected = "not_reflected"
	SalesBudgetRequesting   = "requesting"
	SalesBudgetConfirmed    = "confirmed"
	SalesBudgetNoPlan       = "no_plan"

	SalesCodeGroupBizType      = "sales_biz_type"
	SalesCodeGroupBudgetStatus = "sales_budget_status"
	SettingSalesDormantMonth   = "sales_dormant_default_month"
)

const (
	SalesCodeGroupStage        = "sales_stage"
	SalesCodeGroupProb         = "sales_stage_prob"
	SalesCodeGroupLeadSource   = "sales_lead_source"
	SalesCodeGroupActivityType = "sales_activity_type"
	SalesCodeGroupPartyType    = "sales_party_type"
	SalesCodeGroupPartyRole    = "sales_party_role"
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
	return d.Code == SalesStageLost || d.Code == SalesStageDropped
}

// SalesProject 수주 전 영업 사업. work_projects 와 표를 나눈다 (§32.1.3).
type SalesProject struct {
	SalesID                 string
	Name                    string
	IsTentativeName         bool
	DealType                string
	Stage                   string
	Probability             int
	PONo                    string
	DeliveredAt             string
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
	LegacyStage             string
	WonAt                   string
	ContractedAt            string
	CreatedAt               string
	UpdatedAt               string

	SalesNo          string
	BidStatus        string
	CloseReason      string
	RFPReceivedAt    string
	WinProb          *int
	ProbabilityFinal *int
	AwardedAmount    int
	ContractAmount   int
	ContractTarget   string
	ProcurementRoute string
	ContractMethod   string
	BidEvalMethod    string
	MallContractType string
	DropReasonCode   string
	DropReason       string
	DroppedAt        string
	DroppedBy        string
	DroppedFromStage string
	PrevSalesID      string

	BizType       string
	BudgetYear    int
	BudgetStatus  string
	DormantUntil  string
	DormantReason string
	DormantAt     string
	DormantBy     string

	BidYM              string
	RevenueYM          string
	RevenueFrom        string
	RevenueTo          string
	BillingCycle       string
	AmountVATIncluded  bool
	ForecastGroups     []string

	CustomerName string
	StageLabel   string

	PipeAmount int
	PipeSource string
	PipeQuoteN int
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
	return SalesProbability(p)
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
	return fmt.Errorf("수주로 넘기려면 %s을(를) 모두 확정해야 합니다", strings.Join(items, "·"))
}

// CanPromoteSales 는 sales_stage4.go 에 있다.

// WorkProjectFromSales 승격 폼에 사업명·고객·발주처·금액을 옮긴다.
// 계약기간·계약방식·청구방식·유상무상·범위 규칙은 비워 두고 §22.1에서 처음 입력한다.
func WorkProjectFromSales(s *SalesProject) *WorkProject {
	if s == nil {
		return &WorkProject{IsPaid: true, Status: WBProjectActive, Color: "#3B82F6"}
	}
	cust := strings.TrimSpace(s.CustomerID)
	p := &WorkProject{
		Name:            strings.TrimSpace(s.Name),
		CustomerID:      cust,
		OrderingPartyID: cust,
		IsPaid:          true,
		Status:          WBProjectActive,
		Color:           "#3B82F6",
		SalesProjectID:  s.SalesID,
		ContractAmount:  s.ContractAmount,
		StartDate:       strings.TrimSpace(s.ContractedAt),
	}
	if p.ContractAmount == 0 {
		p.ContractAmount = s.ExpectedAmount
	}
	if ym := NormalizeSalesYM(s.ExpectedYM); len(ym) >= 4 {
		if y, err := strconv.Atoi(ym[:4]); err == nil {
			p.PlanYear = y
		}
	}
	notes := strings.TrimSpace(s.Notes)
	if amt := s.AmountLabel(); amt != "" {
		line := "영업 예상 금액: " + amt
		if notes == "" {
			notes = line
		} else {
			notes = notes + "\n" + line
		}
	}
	p.Notes = notes
	return p
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

func (p *SalesProject) PipelineAmountLabel() string {
	if p == nil || p.PipeAmount <= 0 {
		return ""
	}
	return formatSalesAmount(p.PipeAmount) + "원"
}

func (p *SalesProject) AmountUnconfirmed() bool {
	return p != nil && p.PipeSource == "expected" && !p.ExpectedAmountConfirmed
}

// DisplayNo 화면에 보이는 사업번호. 없으면 내부 키 sales_id. §47.10.3
func (p *SalesProject) DisplayNo() string {
	if p == nil {
		return ""
	}
	if s := strings.TrimSpace(p.SalesNo); s != "" {
		return s
	}
	return strings.TrimSpace(p.SalesID)
}

func (p *SalesProject) DisplayStage(def *SalesStageDef) string {
	label := strings.TrimSpace(p.Stage)
	if def != nil && def.Label != "" {
		label = def.Label
	}
	if p.StageLabel != "" && (def == nil || def.Label == "") {
		label = p.StageLabel
	}
	stage := CurrentSalesStage(p.Stage)
	if IsSalesStage4(p.Stage) {
		if p.Stage == SalesStage4Closed {
			extra := ""
			switch p.CloseReason {
			case SalesCloseContracted:
				extra = "계약"
				if strings.TrimSpace(p.ContractedAt) != "" {
					extra += " " + p.ContractedAt
				}
				if p.ContractAmount > 0 {
					extra += " · " + formatSalesAmount(p.ContractAmount) + "원"
				}
			case SalesCloseLost:
				extra = "실주"
				if r := strings.TrimSpace(p.LostReason); r != "" {
					extra += " · " + r
				}
			case SalesCloseDropped:
				extra = "포기"
				if r := strings.TrimSpace(p.DropReason); r != "" {
					extra += " · " + r
				} else if r := strings.TrimSpace(p.LostReason); r != "" {
					extra += " · " + r
				}
			}
			if extra != "" {
				return extra
			}
			return label
		}
		pct := SalesProbability(p)
		if p.Stage == SalesStage4Bid {
			st := knownSalesStageLabel(p.BidStatus)
			if st == "" {
				st = p.BidStatus
			}
			if st != "" {
				if pct > 0 && pct < 100 {
					return label + " · " + st + " " + strconv.Itoa(pct) + "%"
				}
				return label + " · " + st
			}
		}
		if pct >= 100 {
			return label
		}
		return fmt.Sprintf("%s · %d%%", label, pct)
	}
	if stage == SalesStageLost || stage == SalesStageDropped || (def != nil && def.IsLost()) {
		if r := strings.TrimSpace(p.LostReason); r != "" {
			return label + " · " + r
		}
		return label
	}
	if p.IsSupply() {
		if amt := p.AmountLabel(); amt != "" {
			return label + " · " + amt
		}
		return label
	}
	if def != nil && def.IsPostWin() {
		return label
	}
	if stage == SalesStageWon || stage == SalesStageContracted {
		return label
	}
	prob := p.Probability
	if p.HasOverride {
		prob = p.OverrideValue
	} else if def != nil && def.Probability > 0 && p.Probability == 0 {
		prob = def.Probability
	}
	return fmt.Sprintf("%s · %d%%", label, prob)
}

// CurrentSalesStage 폐 8단계 코드를 현행 6단계로 옮긴다 (§32.3.5). 이력 원문은 바꾸지 않는다.
func CurrentSalesStage(code string) string {
	switch strings.TrimSpace(code) {
	case SalesStageQuote, SalesStageRFP, SalesStageSubmit:
		return SalesStageProposal
	case SalesStageContracted:
		return SalesStageWon
	default:
		return strings.TrimSpace(code)
	}
}

func SalesLegacyStageLabel(code string) string {
	// 폐 8단계 이력 원문만 둔다. 현행 6단계 이름은 codes 를 본다. §32.3.5·§46.2
	switch strings.TrimSpace(code) {
	case SalesStageQuote:
		return "견적 요청"
	case SalesStageRFP:
		return "RFP 제안"
	case SalesStageSubmit:
		return "제안서 제출"
	case SalesStageContracted:
		return "계약완료"
	default:
		return ""
	}
}

func SalesStageDisplayLabel(code string, stages []SalesStageDef) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if d := FindSalesStage(stages, code); d != nil && d.Label != "" {
		return d.Label
	}
	if s := knownSalesStageLabel(code); s != "" {
		return s
	}
	if s := SalesLegacyStageLabel(code); s != "" {
		return s
	}
	return code
}

// SalesProposalProgress §32.3.2 견적 단계 진척. 활동 유형으로만 판정한다.
type SalesProposalProgress struct {
	Quote    bool
	RFP      bool
	Proposal bool
}

func (p SalesProposalProgress) Mark(ok bool) string {
	if ok {
		return "✔"
	}
	return "○"
}

func SalesProposalProgressFrom(acts []SalesActivity) SalesProposalProgress {
	var p SalesProposalProgress
	for _, a := range acts {
		t := strings.TrimSpace(a.ActivityType)
		title := a.Title
		switch t {
		case SalesActTypeQuote:
			p.Quote = true
		case SalesActTypeRFP, SalesActTypeBid:
			p.RFP = true
		case SalesActTypeProposal:
			p.Proposal = true
		}
		if strings.Contains(title, "RFP") {
			p.RFP = true
		}
	}
	return p
}

func SalesMigratedActivity(legacy string) (actType, title string, ok bool) {
	switch strings.TrimSpace(legacy) {
	case SalesStageQuote:
		return SalesActTypeQuote, "[이관] 견적 요청", true
	case SalesStageRFP:
		return SalesActTypeRFP, "[이관] 제안서제출(RFP)", true
	case SalesStageSubmit:
		return SalesActTypeProposal, "[이관] 제안서 제출", true
	default:
		return "", "", false
	}
}

// SalesContractUnsigned 는 sales_stage4.go 에 있다.

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

// SalesMonthBanner 월별 캘린더 맨 위 띠. 날짜 칸에 넣지 않는다 (§39.7).
type SalesMonthBanner struct {
	SalesID   string
	Href      string
	Title     string
	Confirmed bool
	PlaceYM   string
}

// SalesPeriodFirstYM 정밀도 기간의 첫 달 (YYYY-MM). 분기·반기·연은 그 기간 첫 달에 놓는다.
func SalesPeriodFirstYM(ym, precision string) string {
	ym = NormalizeSalesYM(ym)
	if ym == "" {
		return ""
	}
	year, _ := strconv.Atoi(ym[:4])
	month, _ := strconv.Atoi(ym[5:7])
	switch NormalizeSalesPrecision(precision) {
	case SalesPrecisionQuarter:
		month = ((month-1)/3)*3 + 1
	case SalesPrecisionHalf:
		if month <= 6 {
			month = 1
		} else {
			month = 7
		}
	case SalesPrecisionYear:
		month = 1
	}
	return fmt.Sprintf("%04d-%02d", year, month)
}

// SalesPeriodBannerNote 띠 괄호 문구. 월은 「월 미정」, 분기는 「4분기 중」.
func SalesPeriodBannerNote(ym, precision string) string {
	ym = NormalizeSalesYM(ym)
	if ym == "" {
		return ""
	}
	month, _ := strconv.Atoi(ym[5:7])
	switch NormalizeSalesPrecision(precision) {
	case SalesPrecisionQuarter:
		return fmt.Sprintf("%d분기 중", (month-1)/3+1)
	case SalesPrecisionHalf:
		if month <= 6 {
			return "상반기 중"
		}
		return "하반기 중"
	case SalesPrecisionYear:
		return ym[:4] + "년 중"
	default:
		return "월 미정"
	}
}

// FormatSalesMonthBannerTitle 「9월 · 부여군도서관 제안 (월 미정)」.
func FormatSalesMonthBannerTitle(placeYM, name, precision string, confirmed bool) string {
	placeYM = NormalizeSalesYM(placeYM)
	if placeYM == "" {
		return strings.TrimSpace(name)
	}
	month, _ := strconv.Atoi(placeYM[5:7])
	name = strings.TrimSpace(name)
	if name == "" {
		name = "영업 사업"
	}
	prec := NormalizeSalesPrecision(precision)
	note := ""
	if prec == SalesPrecisionMonth {
		if !confirmed {
			note = "월 미정"
		}
	} else {
		note = SalesPeriodBannerNote(placeYM, prec)
	}
	if note == "" {
		return fmt.Sprintf("%d월 · %s", month, name)
	}
	return fmt.Sprintf("%d월 · %s (%s)", month, name, note)
}

func SalesMonthBannerFromProject(p SalesProject) (SalesMonthBanner, bool) {
	ym := NormalizeSalesYM(p.ExpectedYM)
	if ym == "" {
		return SalesMonthBanner{}, false
	}
	place := SalesPeriodFirstYM(ym, p.ExpectedPrecision)
	id := strings.TrimSpace(p.SalesID)
	return SalesMonthBanner{
		SalesID:   id,
		Href:      "/sales/" + id,
		Title:     FormatSalesMonthBannerTitle(place, p.Name, p.ExpectedPrecision, p.ExpectedYMConfirmed),
		Confirmed: p.ExpectedYMConfirmed,
		PlaceYM:   place,
	}, true
}

func IsSalesMonthOnly(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || NormalizeAppDate(s) != "" {
		return false
	}
	return NormalizeSalesYM(s) != ""
}

func RequireSalesDateOrYM(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if err := RequireAppDateYear(s); err == nil {
		return nil
	}
	if NormalizeSalesYM(s) != "" {
		return nil
	}
	return ErrAppDateYear
}

func SalesMonthBannerFromNextAction(salesID, salesName, nextAction, nextDate string) (SalesMonthBanner, bool) {
	if !IsSalesMonthOnly(nextDate) {
		return SalesMonthBanner{}, false
	}
	place := NormalizeSalesYM(nextDate)
	label := strings.TrimSpace(nextAction)
	if label == "" {
		label = strings.TrimSpace(salesName)
	}
	if label == "" {
		label = "다음 활동"
	}
	id := strings.TrimSpace(salesID)
	return SalesMonthBanner{
		SalesID: id,
		Href:    "/sales/" + id,
		Title:   FormatSalesMonthBannerTitle(place, label, SalesPrecisionMonth, false),
		PlaceYM: place,
	}, true
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
		{Code: SalesStage4Discover, Label: "발굴", Probability: 10, SortOrder: 1},
		{Code: SalesStage4Propose, Label: "제안", Probability: 20, SortOrder: 2},
		{Code: SalesStage4Bid, Label: "입찰", Probability: 0, SortOrder: 3},
		{Code: SalesStage4Closed, Label: "사업 종료", Probability: 0, SortOrder: 4},
	}
}

func DefaultSalesStageCode() string { return SalesStage4Discover }

const (
	SalesPartyOwn      = "own"
	SalesPartyPartner  = "partner"
	SalesPartyCustomer = "customer"
)

const (
	SalesChangeExpectedYM = "expected_ym"
	SalesChangeAmount     = "expected_amount"
	SalesChangeCustomer   = "customer"
	SalesChangeStage      = "stage"
	SalesChangeParty      = "party"
)

// SalesParty 사업별 관계자. 교체해도 행을 지우지 않는다 (§32.7).
type SalesParty struct {
	PartyID        string
	SalesID        string
	PartyType      string
	TypeLabel      string
	OrgName        string
	PersonName     string
	Title          string
	Phone          string
	Email          string
	PartyRole      string
	RoleLabel      string
	IsPrimary      bool
	UserID         string
	ContactID      string
	IsActive       bool
	ReplacedBy     string
	ReplacedByName string
	ReplacedReason string
	Note           string
	CreatedAt      string
	IsAuto         bool
}

func (p SalesParty) NeedsSupplement() bool {
	return p.IsAuto
}

func CustomerPartyHintNames(parties []SalesParty) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range parties {
		if p.PartyType != SalesPartyCustomer {
			continue
		}
		name := strings.TrimSpace(p.PersonName)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func (p SalesParty) DisplayName() string {
	name := strings.TrimSpace(p.PersonName)
	org := strings.TrimSpace(p.OrgName)
	switch {
	case name != "" && org != "":
		return org + " · " + name
	case name != "":
		return name
	case org != "":
		return org
	default:
		return p.PartyID
	}
}

func (p SalesParty) ChangeLabel() string {
	typ := p.TypeLabel
	if typ == "" {
		typ = SalesPartyTypeLabel(p.PartyType)
	}
	role := p.RoleLabel
	if role == "" {
		role = p.PartyRole
	}
	s := typ + " " + p.DisplayName()
	if role != "" {
		s += " (" + role + ")"
	}
	return strings.TrimSpace(s)
}

func SalesPartyTypeLabel(code string) string {
	switch strings.TrimSpace(code) {
	case SalesPartyOwn:
		return "당사"
	case SalesPartyPartner:
		return "협력사"
	case SalesPartyCustomer:
		return "고객"
	default:
		return code
	}
}

func SalesChangeFieldLabel(key string) string {
	switch strings.TrimSpace(key) {
	case SalesChangeExpectedYM:
		return "사업 시기"
	case SalesChangeAmount:
		return "예상 금액"
	case SalesChangeCustomer:
		return "고객"
	case SalesChangeStage:
		return "단계"
	case SalesChangeParty:
		return "관계자"
	default:
		return key
	}
}

// SalesChange 항목 변경 이력 (§32.6)
type SalesChange struct {
	ChangeID   string
	SalesID    string
	FieldKey   string
	FieldLabel string
	OldValue   string
	NewValue   string
	ChangedAt  string
	ChangedBy  string
	Note       string
}

func (c SalesChange) Title() string {
	label := c.FieldLabel
	if label == "" {
		label = SalesChangeFieldLabel(c.FieldKey)
	}
	oldV := strings.TrimSpace(c.OldValue)
	newV := strings.TrimSpace(c.NewValue)
	if oldV == "" {
		oldV = "(없음)"
	}
	if newV == "" {
		newV = "(없음)"
	}
	return label + ": " + oldV + " → " + newV
}

func SalesStageIsOpen(stage string) bool {
	switch strings.TrimSpace(stage) {
	case SalesStageWon, SalesStageContracted, SalesStageLost, SalesStageDelivered, SalesStageDropped:
		return false
	default:
		return true
	}
}

// SalesNextGap 다음행동이 월만 지정됐고 그 달이 되었는데 아직 업무가 없는 활동. §45.9
type SalesNextGap struct {
	ActivityID   string
	SalesID      string
	SalesName    string
	NextAction   string
	NextActionYM string
	OwnerName    string
	CustomerID   string
	CustomerName string
}

// SalesActivity 영업 활동 로그 (§32.8)
type SalesActivity struct {
	ActivityID     string
	SalesID        string
	ActivityDate   string
	StartTime      string
	DurationMin    int
	ActivityType   string
	TypeLabel      string
	Title          string
	Content        string
	Place          string
	OurMembers     string
	Counterparts   string
	NextAction     string
	NextActionDate string
	StageAtTime    string
	StageLabel     string
	CreatedBy      string
	CreatedAt      string

	SalesName    string
	SalesNo      string
	CustomerName string
}

// SalesMemo 핵심 전략 메모. 한 줄 = 한 행 (§46.4)
type SalesMemo struct {
	MemoID     string
	SalesID    string
	Content    string
	SortOrder  int
	AuthorID   string
	AuthorName string
	CreatedAt  string
}

func (a SalesActivity) CardDuration() string {
	return FormatMinutesAsCard(a.DurationMin)
}

func (a SalesActivity) NextChipLabel() string {
	l, _ := SalesNextActionChip(a.NextAction, a.NextActionDate, "")
	return l
}

func (a SalesActivity) NextChipClass() string {
	_, c := SalesNextActionChip(a.NextAction, a.NextActionDate, "")
	return c
}

func (a SalesActivity) TypeBadgeClass() string {
	return SalesActivityTypeBadgeClass(a.ActivityType)
}

func (a SalesActivity) OwnerLabel() string {
	if s := strings.TrimSpace(a.OurMembers); s != "" {
		return s
	}
	return strings.TrimSpace(a.CreatedBy)
}

func (a SalesActivity) ContentPreview() string {
	s := strings.ReplaceAll(strings.TrimSpace(a.Content), "\r\n", "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	var keep []string
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" && len(keep) == 0 {
			continue
		}
		keep = append(keep, ln)
		if len(keep) == 2 {
			break
		}
	}
	return strings.Join(keep, "\n")
}

func (a SalesActivity) HasFollowUp() bool {
	return strings.TrimSpace(a.NextAction) != "" || strings.TrimSpace(a.NextActionDate) != ""
}

func (a SalesActivity) OurMemberNames() []string {
	return SplitSalesPeople(a.OurMembers)
}

func SplitSalesPeople(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, "/", ",")
	s = strings.ReplaceAll(s, "\n", ",")
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func JoinSalesPeople(names []string) string {
	return strings.Join(SplitSalesPeople(strings.Join(names, ",")), ", ")
}

const (
	SalesTimelineActivity = "activity"
	SalesTimelineStage    = "stage"
	SalesTimelineChange   = "change"
)

// SalesTimelineEvent 사업 상세 활동 로그 — 활동·단계·항목 변경을 한 줄기로 섞는다 (§32.6·§32.8)
type SalesTimelineEvent struct {
	Kind           string
	SortAt         string
	Date           string
	Time           string
	Type           string
	TypeLabel      string
	Title          string
	Summary        string
	Members        string
	Counterparts   string
	Place          string
	NextAction     string
	NextActionDate string
	FromLabel      string
	ToLabel        string
	Reason         string
	By             string
	FieldKey       string
	DurationMin    int
	DurationLabel  string
	SalesID        string
	SalesName      string
	ActivityID     string
	NextChipLabel  string
	NextChipClass  string
}

func MergeSalesTimeline(acts []SalesActivity, hist []SalesStageHistory, changes []SalesChange) []SalesTimelineEvent {
	out := make([]SalesTimelineEvent, 0, len(acts)+len(hist)+len(changes))
	for _, a := range acts {
		tm := strings.TrimSpace(a.StartTime)
		if len(tm) > 5 {
			tm = tm[:5]
		}
		title := strings.TrimSpace(a.Title)
		if title == "" {
			title = a.TypeLabel
		}
		sortAt := strings.TrimSpace(a.ActivityDate)
		if tm != "" {
			sortAt += " " + tm
		} else {
			sortAt += " 00:00"
		}
		if a.CreatedAt != "" {
			sortAt += " " + a.CreatedAt
		}
		nextLabel, nextClass := SalesNextActionChip(a.NextAction, a.NextActionDate, "")
		out = append(out, SalesTimelineEvent{
			Kind:           SalesTimelineActivity,
			SortAt:         sortAt,
			Date:           a.ActivityDate,
			Time:           tm,
			Type:           a.ActivityType,
			TypeLabel:      a.TypeLabel,
			Title:          title,
			Summary:        a.Content,
			Members:        a.OurMembers,
			Counterparts:   a.Counterparts,
			Place:          a.Place,
			NextAction:     a.NextAction,
			NextActionDate: a.NextActionDate,
			By:             a.CreatedBy,
			DurationMin:    a.DurationMin,
			DurationLabel:  FormatMinutesAsCard(a.DurationMin),
			SalesID:        a.SalesID,
			SalesName:      a.SalesName,
			ActivityID:     a.ActivityID,
			NextChipLabel:  nextLabel,
			NextChipClass:  nextClass,
		})
	}
	for _, h := range hist {
		at := strings.TrimSpace(h.ChangedAt)
		date, tm := splitDateTime(at)
		title := h.ToLabel
		if h.FromLabel != "" {
			title = h.FromLabel + "→" + h.ToLabel
		}
		out = append(out, SalesTimelineEvent{
			Kind:      SalesTimelineStage,
			SortAt:    at,
			Date:      date,
			Time:      tm,
			TypeLabel: "단계",
			Title:     title,
			FromLabel: h.FromLabel,
			ToLabel:   h.ToLabel,
			Reason:    h.Reason,
			By:        h.ChangedBy,
			FieldKey:  SalesChangeStage,
		})
	}
	for _, ch := range changes {
		if strings.TrimSpace(ch.FieldKey) == SalesChangeStage {
			continue
		}
		at := strings.TrimSpace(ch.ChangedAt)
		date, tm := splitDateTime(at)
		out = append(out, SalesTimelineEvent{
			Kind:      SalesTimelineChange,
			SortAt:    at,
			Date:      date,
			Time:      tm,
			TypeLabel: SalesChangeFieldLabel(ch.FieldKey),
			Title:     ch.Title(),
			FromLabel: ch.OldValue,
			ToLabel:   ch.NewValue,
			Reason:    ch.Note,
			By:        ch.ChangedBy,
			FieldKey:  ch.FieldKey,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortAt == out[j].SortAt {
			return out[i].Kind > out[j].Kind
		}
		return out[i].SortAt > out[j].SortAt
	})
	return out
}

func (e SalesTimelineEvent) TypeBadgeClass() string {
	switch e.Kind {
	case SalesTimelineStage:
		return "bg-slate-100 text-slate-700"
	case SalesTimelineChange:
		return "bg-sky-100 text-sky-800"
	default:
		return SalesActivityTypeBadgeClass(e.Type)
	}
}

func (e SalesTimelineEvent) CardDuration() string {
	if e.Kind != SalesTimelineActivity {
		return ""
	}
	if e.DurationLabel != "" {
		return e.DurationLabel
	}
	return FormatMinutesAsCard(e.DurationMin)
}

func (e SalesTimelineEvent) ContentPreview() string {
	s := strings.ReplaceAll(strings.TrimSpace(e.Summary), "\r\n", "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	var keep []string
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" && len(keep) == 0 {
			continue
		}
		keep = append(keep, ln)
		if len(keep) == 2 {
			break
		}
	}
	return strings.Join(keep, "\n")
}

func splitDateTime(s string) (date, tm string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if len(s) >= 10 {
		date = s[:10]
	}
	if len(s) >= 16 {
		tm = strings.TrimSpace(s[11:16])
	}
	return date, tm
}

func NormalizeSalesBizType(s string) string {
	switch strings.TrimSpace(s) {
	case SalesBizBuild, SalesBizMaintenance, SalesBizGoods, SalesBizDevelop, SalesBizConstruction, SalesBizEtc:
		return strings.TrimSpace(s)
	default:
		return ""
	}
}

func NormalizeSalesBudgetStatus(s string) string {
	switch strings.TrimSpace(s) {
	case SalesBudgetUnknown, SalesBudgetNotReflected, SalesBudgetRequesting, SalesBudgetConfirmed, SalesBudgetNoPlan:
		return strings.TrimSpace(s)
	default:
		return ""
	}
}

func NormalizeDormantYM(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 7 && s[4] == '-' {
		return s[:7]
	}
	return ""
}

func DefaultDormantUntil(now time.Time, month int) string {
	if month < 1 || month > 12 {
		month = 6
	}
	y := now.Year() + 1
	return fmt.Sprintf("%04d-%02d", y, month)
}

func (p *SalesProject) IsDormant() bool {
	return p != nil && strings.TrimSpace(p.Status) == SalesStatusDormant
}

func SalesBizTypeLabel(s string) string {
	switch strings.TrimSpace(s) {
	case SalesBizBuild:
		return "구축"
	case SalesBizMaintenance:
		return "유지보수"
	case SalesBizGoods:
		return "물품 납품"
	case SalesBizDevelop:
		return "개발"
	case SalesBizConstruction:
		return "공사"
	case SalesBizEtc:
		return "기타"
	default:
		return ""
	}
}

func SalesBudgetStatusLabel(s string) string {
	switch strings.TrimSpace(s) {
	case SalesBudgetUnknown:
		return "미확인"
	case SalesBudgetNotReflected:
		return "미반영"
	case SalesBudgetRequesting:
		return "요청·협의 중"
	case SalesBudgetConfirmed:
		return "반영 확정"
	case SalesBudgetNoPlan:
		return "예산 계획 없음"
	default:
		return ""
	}
}

func DormantSummary(items []SalesProject, quotes map[string][]SalesQuote) (count, amount int) {
	for i := range items {
		p := items[i]
		if p.Status != SalesStatusDormant {
			continue
		}
		ApplySalesPipelineAmount(&p, quotes[p.SalesID])
		count++
		amount += p.PipeAmount
	}
	return
}
