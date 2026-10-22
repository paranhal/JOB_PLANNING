package model

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	ContractDirRevenue = "revenue"
	ContractDirCost    = "cost"

	ContractImportMaxBytes = 2 << 20
	ContractImportMaxRows  = 500
)

const (
	ContractActionNew      = "new"
	ContractActionSkip     = "skip"
	ContractActionPipeline = "pipeline"
	ContractActionErr      = "err"

	ContractErrOrgForbidden   = "org_forbidden"
	ContractErrCustomerMiss   = "customer_missing"
	ContractErrCustomerAmb    = "customer_ambiguous"
	ContractErrDateInvalid    = "date_invalid"
	ContractErrKindInvalid    = "kind_invalid"
	ContractErrMethodInvalid  = "method_invalid"
	ContractErrAmountInvalid  = "amount_invalid"
	ContractErrNameMissing    = "name_missing"
	ContractErrDupPipeline    = "duplicate_in_pipeline"
	ContractErrPipelineSkip   = "pipeline_skipped"
)

type ContractImportPreview struct {
	FileName    string
	NewN        int
	PipeN       int
	SkipN       int
	ErrN        int
	ErrMissingN int
	ErrDeniedN  int
	ErrOtherN   int
	Rows        []ContractImportRow
	Summary     ContractImportSummary
}

type ContractImportSummary struct {
	NewN           int                      `json:"new_n"`
	PipeN          int                      `json:"pipe_n"`
	SkipN          int                      `json:"skip_n"`
	ErrN           int                      `json:"err_n"`
	Total          int                      `json:"total"`
	RegisteredZero bool                     `json:"registered_zero"`
	Reasons        []ContractImportReasonN  `json:"reasons"`
	DeniedOrgs     []ContractImportOrgN     `json:"denied_orgs"`
	DeniedN        int                      `json:"denied_n"`
}

type ContractImportReasonN struct {
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	N        int      `json:"n"`
	Examples []string `json:"examples"`
	Extra    int      `json:"extra"`
}

type ContractImportOrgN struct {
	Label string `json:"label"`
	N     int    `json:"n"`
}

type ContractImportApplyOpt struct {
	Pipeline string // yes|no|""
	Stage    string
	Lines    map[int]bool
}

type ContractImportRow struct {
	Line         int
	Action       string // new|skip|pipeline|err
	Msg          string
	ErrKind      string
	SalesNo      string
	MissingName  string
	MissingRole  string // 발주처 | 최종수요처
	Name         string
	OrgID        string
	OrgLabel     string
	PartyName    string
	PartyID      string
	EndNames     []string
	EndIDs       []string
	Kind         string
	KindLabel    string
	ContractType string
	Direction    string
	StartDate    string
	EndDate      string
	Amount       int
	ContractNo   string
	SalesUser    string
	SalesID      string
	ExecUser     string
	ExecID       string
	Notes        string
	Years        []YearSlice
}

type YearSlice struct {
	YearNo    int
	Label     string
	StartDate string
	EndDate   string
	Amount    int
	Status    string
}

type ContractImportBatch struct {
	ID        string
	FileName  string
	RowsTotal int
	RowsOK    int
	RowsError int
	Status    string
	CreatedBy string
}

func NormalizePipelineStage(s string) string {
	switch strings.TrimSpace(s) {
	case SalesStage4Propose, SalesStage4Bid:
		return strings.TrimSpace(s)
	default:
		return SalesStage4Discover
	}
}

func PipelineStageLabel(s string) string {
	switch NormalizePipelineStage(s) {
	case SalesStage4Propose:
		return "제안"
	case SalesStage4Bid:
		return "입찰"
	default:
		return "발굴"
	}
}

func NormalizeContractMethod(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	key := strings.TrimSuffix(s, "계약")
	allowed := []string{"수의", "입찰", "제안입찰", "농공단지수의", "민간계약", "기타"}
	for _, a := range allowed {
		if s == a || key == a || key == strings.TrimSuffix(a, "계약") {
			return a, nil
		}
	}
	return "", fmt.Errorf("계약방식을 확인하세요")
}

func NormalizeContractDirection(s string) string {
	switch strings.TrimSpace(s) {
	case "", "매출", ContractDirRevenue:
		return ContractDirRevenue
	case "매입", ContractDirCost:
		return ContractDirCost
	default:
		return ""
	}
}

func ContractDirectionLabel(s string) string {
	if NormalizeContractDirection(s) == ContractDirCost {
		return "매입"
	}
	return "매출"
}

// MapContractKindLabel 양식 사업유형 → 기존 5종. 용역→개발, 구매→기타.
func MapContractKindLabel(s string) (string, bool) {
	switch strings.TrimSpace(s) {
	case "공사":
		return ProjectKindConstruction, true
	case "구축":
		return ProjectKindBuild, true
	case "용역", "개발":
		return ProjectKindDev, true
	case "유지보수":
		return ProjectKindMaintenance, true
	case "구매", "기타":
		return ProjectKindOther, true
	default:
		return "", false
	}
}

func InferContractKind(label, name string) (string, error) {
	if k, ok := MapContractKindLabel(label); ok {
		return k, nil
	}
	if strings.TrimSpace(label) != "" {
		return "", fmt.Errorf("사업유형을 확인하세요")
	}
	n := strings.TrimSpace(name)
	if strings.Contains(n, "유지관리") || strings.Contains(n, "유지보수") {
		return ProjectKindMaintenance, nil
	}
	if strings.Contains(n, "구축") {
		return ProjectKindBuild, nil
	}
	if strings.Contains(n, "공사") {
		return ProjectKindConstruction, nil
	}
	if strings.Contains(n, "용역") {
		return ProjectKindDev, nil
	}
	if strings.Contains(n, "구매") || strings.Contains(n, "구입") {
		return ProjectKindOther, nil
	}
	return ProjectKindOther, nil
}

func SplitEndCustomers(s string) []string {
	s = strings.ReplaceAll(s, ",", ";")
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func addYearsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	ny := y + n
	last := time.Date(ny, m+1, 0, 0, 0, 0, 0, loc).Day()
	if d > last {
		d = last
	}
	return time.Date(ny, m, d, 0, 0, 0, 0, loc)
}

func daysInclusive(a, b time.Time) int {
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours()/24) + 1
}

func yearStatus(start, end, today time.Time) string {
	ts := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	s := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	e := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case ts.Before(s):
		return YearPlanned
	case ts.After(e):
		return YearDone
	default:
		return YearActive
	}
}

// SplitContractYears 시작일 기준 12개월. 마지막 구간 31일 이하면 앞 연차에 합침.
// 나눈 뒤 1개면 연차를 만들지 않는다(nil).
func SplitContractYears(start, end time.Time, amount int, today time.Time) []YearSlice {
	if end.Before(start) {
		return nil
	}
	type bound struct{ s, e time.Time }
	var bounds []bound
	for k := 1; k <= 80; k++ {
		ys := addYearsClamped(start, k-1)
		if ys.After(end) {
			break
		}
		next := addYearsClamped(start, k)
		ye := next.AddDate(0, 0, -1)
		if ye.After(end) {
			ye = end
		}
		bounds = append(bounds, bound{ys, ye})
		if !ye.Before(end) {
			break
		}
	}
	if len(bounds) >= 2 {
		last := bounds[len(bounds)-1]
		if daysInclusive(last.s, last.e) <= 31 {
			bounds[len(bounds)-2].e = last.e
			bounds = bounds[:len(bounds)-1]
		}
	}
	if len(bounds) <= 1 {
		return nil
	}
	total := 0
	for _, b := range bounds {
		total += daysInclusive(b.s, b.e)
	}
	if total <= 0 {
		return nil
	}
	out := make([]YearSlice, 0, len(bounds))
	used := 0
	for i, b := range bounds {
		amt := 0
		if i == len(bounds)-1 {
			amt = amount - used
		} else {
			amt = int(math.Round(float64(amount) * float64(daysInclusive(b.s, b.e)) / float64(total)))
			used += amt
		}
		out = append(out, YearSlice{
			YearNo:    i + 1,
			Label:     fmt.Sprintf("%d년차", i+1),
			StartDate: b.s.Format("2006-01-02"),
			EndDate:   b.e.Format("2006-01-02"),
			Amount:    amt,
			Status:    yearStatus(b.s, b.e, today),
		})
	}
	return out
}
