package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 영업 단계 (§22.1.1 ②). 유지보수 사업은 비워 둔다.
const (
	SalesStageLead     = "lead"
	SalesStageProposal = "proposal"
	SalesStageQuote    = "quote"
	SalesStageBid      = "bid"
	SalesStageWon      = "won"
	SalesStageLost     = "lost"
	SalesStageDropped  = "dropped"
)

// 예정 시기 정밀도 (§22.1.1 ③). 저장은 항상 연-월.
const (
	ExpectedPrecisionMonth   = "month"
	ExpectedPrecisionQuarter = "quarter"
	ExpectedPrecisionHalf    = "half"
	ExpectedPrecisionYear    = "year"
)

func IsSalesKind(kind string) bool {
	switch NormalizeProjectKind(kind) {
	case ProjectKindBuild, ProjectKindSupply, ProjectKindConsumable, ProjectKindOther:
		return true
	default:
		return false
	}
}

func (p WorkProject) IsSales() bool { return IsSalesKind(p.ProjectKind) }

func (p WorkProject) IsProspect() bool {
	return strings.TrimSpace(p.CustomerID) == ""
}

// CustomerLabel 등록 고객명이 있으면 그 값, 없으면 가등록명.
func (p WorkProject) CustomerLabel() string {
	if s := strings.TrimSpace(p.CustomerName); s != "" {
		return s
	}
	return strings.TrimSpace(p.ProspectName)
}

func NormalizeSalesStage(stage string) string {
	switch strings.TrimSpace(stage) {
	case SalesStageLead, SalesStageProposal, SalesStageQuote, SalesStageBid,
		SalesStageWon, SalesStageLost, SalesStageDropped:
		return strings.TrimSpace(stage)
	default:
		return ""
	}
}

func SalesStageLabel(stage string) string {
	switch NormalizeSalesStage(stage) {
	case SalesStageLead:
		return "발굴"
	case SalesStageProposal:
		return "제안"
	case SalesStageQuote:
		return "견적"
	case SalesStageBid:
		return "입찰·협상"
	case SalesStageWon:
		return "수주"
	case SalesStageLost:
		return "실주"
	case SalesStageDropped:
		return "보류"
	default:
		return ""
	}
}

func DefaultWinProbability(stage string) int {
	switch NormalizeSalesStage(stage) {
	case SalesStageLead:
		return 10
	case SalesStageProposal:
		return 30
	case SalesStageQuote:
		return 50
	case SalesStageBid:
		return 70
	case SalesStageWon:
		return 100
	default:
		return 0
	}
}

func NormalizeExpectedPrecision(p string) string {
	switch strings.TrimSpace(p) {
	case ExpectedPrecisionQuarter, ExpectedPrecisionHalf, ExpectedPrecisionYear:
		return strings.TrimSpace(p)
	default:
		return ExpectedPrecisionMonth
	}
}

// NormalizeExpectedYM 연-월(YYYY-MM). 날짜가 들어오면 앞 7자리만 쓴다.
func NormalizeExpectedYM(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		s = s[:7]
	}
	if len(s) == 6 && !strings.Contains(s, "-") {
		s = s[:4] + "-" + s[4:]
	}
	if len(s) < 6 {
		return ""
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		t, err = time.Parse("2006-1", s)
	}
	if err != nil {
		return ""
	}
	return t.Format("2006-01")
}

// FormatExpectedPeriod 정밀도에 따른 표시. 축은 월이다.
func FormatExpectedPeriod(ym, precision string) string {
	ym = NormalizeExpectedYM(ym)
	if ym == "" {
		return ""
	}
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return ym
	}
	y, m := t.Year(), int(t.Month())
	switch NormalizeExpectedPrecision(precision) {
	case ExpectedPrecisionQuarter:
		q := (m-1)/3 + 1
		return fmt.Sprintf("%d년 %d분기", y, q)
	case ExpectedPrecisionHalf:
		if m <= 6 {
			return fmt.Sprintf("%d년 상반기", y)
		}
		return fmt.Sprintf("%d년 하반기", y)
	case ExpectedPrecisionYear:
		return fmt.Sprintf("%d년", y)
	default:
		return ym
	}
}

func (p WorkProject) ExpectedPeriodLabel() string {
	if s := FormatExpectedPeriod(p.ExpectedYM, p.ExpectedPrecision); s != "" {
		return s
	}
	if r := strings.TrimSpace(p.ExpectedUndatedReason); r != "" {
		return "미정 · " + NoDateReasonLabel(r)
	}
	return ""
}

func LeadSourceLabel(src string) string {
	switch strings.TrimSpace(src) {
	case "existing":
		return "기존 고객"
	case "bid":
		return "입찰공고"
	case "referral":
		return "소개"
	case "exhibition":
		return "전시회"
	case "other":
		return "기타"
	default:
		return strings.TrimSpace(src)
	}
}

func ProjectKindLabel(kind string) string {
	switch NormalizeProjectKind(kind) {
	case ProjectKindBuild:
		return "신규구축"
	case ProjectKindSupply:
		return "장비납품"
	case ProjectKindConsumable:
		return "소모품납품"
	case ProjectKindOther:
		return "기타"
	default:
		return "유지보수"
	}
}

func ParseWinProbability(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func ParseExpectedAmount(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	if n < 0 {
		return 0
	}
	return n
}

// ValidateWorkProject §22.1.1 ⑦. 유지보수는 사업명만(현행). 영업은 유형별 필수.
func ValidateWorkProject(p *WorkProject) error {
	if p == nil {
		return fmt.Errorf("사업 정보가 없습니다")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("사업명을 입력하세요.")
	}
	if !p.IsSales() {
		return nil
	}
	p.SalesStage = NormalizeSalesStage(p.SalesStage)
	if p.SalesStage == "" {
		p.SalesStage = SalesStageLead
	}
	p.ExpectedPrecision = NormalizeExpectedPrecision(p.ExpectedPrecision)
	p.ExpectedYM = NormalizeExpectedYM(p.ExpectedYM)
	if p.ExpectedYM == "" && strings.TrimSpace(p.ExpectedUndatedReason) == "" {
		return fmt.Errorf("예정 시기(연-월)를 넣거나 미정 사유를 남기세요.")
	}
	if strings.TrimSpace(p.SalesOwner) == "" && strings.TrimSpace(p.SalesOwnerID) == "" {
		return fmt.Errorf("영업담당을 지정하세요.")
	}
	if strings.TrimSpace(p.CustomerID) == "" && strings.TrimSpace(p.ProspectName) == "" {
		return fmt.Errorf("고객명(등록 고객 또는 가등록)을 입력하세요.")
	}
	if p.SalesStage != SalesStageWon {
		return nil
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return fmt.Errorf("수주(won)로 바꾸려면 고객을 등록하세요.")
	}
	if strings.TrimSpace(p.StartDate) == "" || strings.TrimSpace(p.EndDate) == "" {
		return fmt.Errorf("수주 시 계약기간을 입력하세요.")
	}
	if strings.TrimSpace(p.ContractType) == "" {
		return fmt.Errorf("수주 시 계약방식을 입력하세요.")
	}
	return nil
}

// CopySalesFields 유지보수 저장 때 영업 칸을 비우지 않는다.
func CopySalesFields(dst, src *WorkProject) {
	if dst == nil || src == nil {
		return
	}
	dst.SalesStage = src.SalesStage
	dst.ExpectedYM = src.ExpectedYM
	dst.ExpectedPrecision = src.ExpectedPrecision
	dst.ExpectedNote = src.ExpectedNote
	dst.ExpectedUndatedReason = src.ExpectedUndatedReason
	dst.ProspectName = src.ProspectName
	dst.ProspectRegion = src.ProspectRegion
	dst.ProspectContactName = src.ProspectContactName
	dst.ProspectContactTitle = src.ProspectContactTitle
	dst.ProspectContactPhone = src.ProspectContactPhone
	dst.ProspectContactEmail = src.ProspectContactEmail
	dst.SalesOwner = src.SalesOwner
	dst.SalesOwnerID = src.SalesOwnerID
	dst.ExpectedAmount = src.ExpectedAmount
	dst.WinProbability = src.WinProbability
	dst.Competitor = src.Competitor
	dst.LeadSource = src.LeadSource
}

const (
	ProjectTabAll    = "all"    // 전체
	ProjectTabSales  = "sales"  // 영업(수주 전)
	ProjectTabActive = "active" // 진행
	ProjectTabClosed = "closed" // 종료
)

func NormalizeProjectTab(tab string) string {
	switch strings.TrimSpace(tab) {
	case ProjectTabAll, ProjectTabSales, ProjectTabClosed:
		return strings.TrimSpace(tab)
	default:
		return ProjectTabActive
	}
}

func (p WorkProject) IsPreWon() bool {
	if !p.IsSales() {
		return false
	}
	switch NormalizeSalesStage(p.SalesStage) {
	case SalesStageWon, SalesStageLost, SalesStageDropped:
		return false
	default:
		return true
	}
}

func (p WorkProject) ExpectedYMOverdue(nowYM string) bool {
	if !p.IsPreWon() {
		return false
	}
	ym := NormalizeExpectedYM(p.ExpectedYM)
	nowYM = NormalizeExpectedYM(nowYM)
	return ym != "" && nowYM != "" && ym < nowYM
}

func FormatKRW(n int64) string {
	if n <= 0 {
		return ""
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + "원"
}

func (p WorkProject) ExpectedAmountLabel() string {
	return FormatKRW(p.ExpectedAmount)
}

// SalesTimelineMonths 가로축. 이번 달·다음 달은 항상 넣고, 목록의 예정월을 덮는다.
func SalesTimelineMonths(items []WorkProject, now time.Time) []string {
	cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	var months []string
	add := func(ym string) {
		ym = NormalizeExpectedYM(ym)
		if ym == "" || seen[ym] {
			return
		}
		seen[ym] = true
		months = append(months, ym)
	}
	add(cur.Format("2006-01"))
	add(cur.AddDate(0, 1, 0).Format("2006-01"))
	for _, p := range items {
		add(p.ExpectedYM)
	}
	sort.Strings(months)
	return months
}

type SalesKanbanColumn struct {
	Stage string
	Label string
}

func SalesKanbanColumns() []SalesKanbanColumn {
	return []SalesKanbanColumn{
		{Stage: SalesStageLead, Label: SalesStageLabel(SalesStageLead)},
		{Stage: SalesStageProposal, Label: SalesStageLabel(SalesStageProposal)},
		{Stage: SalesStageQuote, Label: SalesStageLabel(SalesStageQuote)},
		{Stage: SalesStageBid, Label: SalesStageLabel(SalesStageBid)},
		{Stage: SalesStageWon, Label: SalesStageLabel(SalesStageWon)},
	}
}
