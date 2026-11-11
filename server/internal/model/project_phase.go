package model

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	PhasePlanned = "planned"
	PhaseActive  = "active"
	PhaseDone    = "done"

	PhaseNameMax = 100

	PhaseTplBuild        = "phase_tpl_build"
	PhaseTplDevelop      = "phase_tpl_develop"
	PhaseTplConstruction = "phase_tpl_construction"
)

// ProjectPhase 사업의 수행 단계. phase_id 는 TEXT.
type ProjectPhase struct {
	PhaseID     string
	ProjectID   string
	OrgID       string
	Seq         int
	Name        string
	PlanStart   string
	PlanEnd     string
	ActualStart string
	ActualEnd   string
	Status      string
	Note        string
	IsActive    bool
	CreatedAt   string
	UpdatedAt   string
}

func ValidPhaseStatus(v string) bool {
	switch strings.TrimSpace(v) {
	case PhasePlanned, PhaseActive, PhaseDone:
		return true
	default:
		return false
	}
}

func PhaseStatusLabel(v string) string {
	switch strings.TrimSpace(v) {
	case PhaseActive:
		return "진행"
	case PhaseDone:
		return "완료"
	case PhasePlanned:
		return "예정"
	default:
		return "예정"
	}
}

func PhaseStatusRank(v string) int {
	switch strings.TrimSpace(v) {
	case PhaseDone:
		return 3
	case PhaseActive:
		return 2
	default:
		return 1
	}
}

// PhaseLate 계획 종료일이 오늘보다 이전이면 지연. 오늘과 같거나 비어 있으면 지연이 아니다.
func PhaseLate(planEnd, status, today string) bool {
	planEnd = strings.TrimSpace(planEnd)
	today = strings.TrimSpace(today)
	if planEnd == "" || today == "" || strings.TrimSpace(status) == PhaseDone {
		return false
	}
	return planEnd < today
}

// PhaseNameOK 공백을 뺀 이름. 100자 초과는 거절한다.
func PhaseNameOK(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("단계명을 입력하세요")
	}
	if utf8.RuneCountInString(name) > PhaseNameMax {
		return "", fmt.Errorf("단계명은 100자까지입니다")
	}
	return name, nil
}

// PhaseDatesOK 비어 있지 않은 날짜는 YYYY-MM-DD 이고 종료가 시작보다 빠르면 안 된다.
func PhaseDatesOK(planStart, planEnd, actualStart, actualEnd string) (string, string, string, string, error) {
	ps, err := phaseDate(planStart)
	if err != nil {
		return "", "", "", "", fmt.Errorf("계획 시작일: %w", err)
	}
	pe, err := phaseDate(planEnd)
	if err != nil {
		return "", "", "", "", fmt.Errorf("계획 종료일: %w", err)
	}
	as, err := phaseDate(actualStart)
	if err != nil {
		return "", "", "", "", fmt.Errorf("실제 시작일: %w", err)
	}
	ae, err := phaseDate(actualEnd)
	if err != nil {
		return "", "", "", "", fmt.Errorf("실제 종료일: %w", err)
	}
	if ps != "" && pe != "" && pe < ps {
		return "", "", "", "", fmt.Errorf("계획 종료일이 계획 시작일보다 빠릅니다")
	}
	if as != "" && ae != "" && ae < as {
		return "", "", "", "", fmt.Errorf("실제 종료일이 실제 시작일보다 빠릅니다")
	}
	return ps, pe, as, ae, nil
}

func phaseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	got := NormalizeAppDate(s)
	if got == "" {
		return "", fmt.Errorf("날짜 형식이 아닙니다")
	}
	return got, nil
}

// NormalizePhaseMergeName 합치기용. 공백과 괄호를 뺀다.
func NormalizePhaseMergeName(name string) string {
	name = strings.TrimSpace(name)
	repl := strings.NewReplacer(" ", "", "\t", "", "(", "", ")", "", "（", "", "）", "")
	return repl.Replace(name)
}

// PhaseTemplateGroup 사업 분류의 기본 단계 코드 그룹. 없으면 빈 문자열.
func PhaseTemplateGroup(kind string) string {
	switch NormalizeProjectKind(kind) {
	case ProjectKindBuild:
		return PhaseTplBuild
	case ProjectKindDev:
		return PhaseTplDevelop
	case ProjectKindConstruction:
		return PhaseTplConstruction
	default:
		return ""
	}
}

// PhaseSectionOpen 구축·개발·공사는 단계 구역을 펼친다.
func PhaseSectionOpen(kind string) bool {
	switch NormalizeProjectKind(kind) {
	case ProjectKindBuild, ProjectKindDev, ProjectKindConstruction:
		return true
	default:
		return false
	}
}

// PhaseEmptyHint 단계가 없을 때 안내. 유지보수는 사용 안 함, 기타·미분류는 설정 안 함.
func PhaseEmptyHint(kind string) string {
	switch NormalizeProjectKind(kind) {
	case ProjectKindMaintenance:
		return "사용 안 함"
	case ProjectKindOther, ProjectKindUnclassified, "":
		return "설정 안 함"
	default:
		return ""
	}
}

// PhaseDateSpan 한 단계에 제안하는 기간.
type PhaseDateSpan struct {
	Start string
	End   string
}

// SplitPhaseDates 계약 기간을 단계 수로 나눈다. 기간이 없으면 비운다.
// 기간이 단계 수보다 짧으면 앞쪽부터 하루씩 주고 나머지는 비운다.
func SplitPhaseDates(start, end string, n int) []PhaseDateSpan {
	out := make([]PhaseDateSpan, n)
	if n <= 0 {
		return nil
	}
	start = NormalizeAppDate(start)
	end = NormalizeAppDate(end)
	if start == "" || end == "" || end < start {
		return out
	}
	from, err1 := time.Parse("2006-01-02", start)
	to, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return out
	}
	days := int(to.Sub(from).Hours()/24) + 1
	if days <= 0 {
		return out
	}
	if days < n {
		for i := 0; i < days; i++ {
			d := from.AddDate(0, 0, i).Format("2006-01-02")
			out[i] = PhaseDateSpan{Start: d, End: d}
		}
		return out
	}
	base := days / n
	rem := days % n
	cursor := from
	for i := 0; i < n; i++ {
		span := base
		if i < rem {
			span++
		}
		last := cursor.AddDate(0, 0, span-1)
		out[i] = PhaseDateSpan{Start: cursor.Format("2006-01-02"), End: last.Format("2006-01-02")}
		cursor = last.AddDate(0, 0, 1)
	}
	return out
}

// PhaseListSummary 사업 목록의 현재 단계·진행.
type PhaseListSummary struct {
	Current string
	Late    bool
	Done    int
	Total   int
}

func (s PhaseListSummary) Label() string {
	if s.Total <= 0 {
		return "—"
	}
	return fmt.Sprintf("완료 %d / 전체 %d", s.Done, s.Total)
}

func (s PhaseListSummary) Pct() int {
	if s.Total <= 0 {
		return 0
	}
	return s.Done * 100 / s.Total
}

// SummarizePhases 활성 단계만 센다. 진행 중이면 그 이름, 없으면 다음 예정.
func SummarizePhases(list []ProjectPhase, today string) PhaseListSummary {
	var s PhaseListSummary
	var active []ProjectPhase
	for _, p := range list {
		if p.IsActive {
			active = append(active, p)
		}
	}
	s.Total = len(active)
	if s.Total == 0 {
		s.Current = "—"
		return s
	}
	var cur, next *ProjectPhase
	for i := range active {
		p := &active[i]
		if p.Status == PhaseDone {
			s.Done++
		}
		if PhaseLate(p.PlanEnd, p.Status, today) {
			s.Late = true
		}
		if cur == nil && p.Status == PhaseActive {
			cur = p
		}
		if next == nil && p.Status == PhasePlanned {
			next = p
		}
	}
	switch {
	case cur != nil:
		s.Current = cur.Name
	case next != nil:
		s.Current = "다음: " + next.Name
	default:
		s.Current = "—"
	}
	return s
}
