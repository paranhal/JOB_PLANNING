package model

import "strings"

const (
	ProjectKindConstruction  = "construction"
	ProjectKindBuild         = "build"
	ProjectKindDev           = "dev"
	ProjectKindMaintenance   = "maintenance"
	ProjectKindOther         = "other"
	ProjectKindUnclassified  = "unclassified"
	KindSourceConfirmed      = "confirmed"
	KindSourceInferred       = "inferred"
	KindSourceUnclassified   = "unclassified"
)

const (
	ProjectDutySales      = "sales"
	ProjectDutyExecution  = "execution"
	ProjectDutyManagement = "management"
	ProjectDutyAccounting = "accounting"
)

func ValidProjectKind(v string) bool {
	switch NormalizeProjectKind(v) {
	case ProjectKindConstruction, ProjectKindBuild, ProjectKindDev, ProjectKindMaintenance, ProjectKindOther:
		return true
	default:
		return false
	}
}

// NormalizeProjectKind 011·화면 잔여값을 5종으로 맞춘다.
// 신규구축·장비납품·supply·network → 구축, 소모품납품·consumable·기타 라벨 → 기타.
func NormalizeProjectKind(v string) string {
	switch strings.TrimSpace(v) {
	case "supply", "network", "신규구축", "장비납품":
		return ProjectKindBuild
	case "consumable", "소모품납품", "기타":
		return ProjectKindOther
	default:
		return strings.TrimSpace(v)
	}
}

func ProjectKindLabel(v string) string {
	switch NormalizeProjectKind(v) {
	case ProjectKindConstruction:
		return "공사"
	case ProjectKindBuild:
		return "구축"
	case ProjectKindDev:
		return "개발"
	case ProjectKindMaintenance:
		return "유지보수"
	case ProjectKindOther:
		return "기타"
	case ProjectKindUnclassified:
		return "미분류"
	default:
		return "—"
	}
}

func KindSourceLabel(src string) string {
	switch src {
	case KindSourceConfirmed:
		return "확정"
	case KindSourceInferred:
		return "추정"
	default:
		return "미분류"
	}
}

// MapSalesBizToProjectKind 영업 biz_type → 사업 유형. 빈 값은 매핑 불가.
func MapSalesBizToProjectKind(biz string) (kind string, ok bool) {
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return "", false
	}
	switch NormalizeSalesBizType(biz) {
	case SalesBizBuild:
		return ProjectKindBuild, true
	case SalesBizDevelop:
		return ProjectKindDev, true
	case SalesBizConstruction:
		return ProjectKindConstruction, true
	case SalesBizMaintenance:
		return ProjectKindMaintenance, true
	default:
		return ProjectKindOther, true
	}
}

type EffectiveKind struct {
	Kind   string
	Source string
}

func (e EffectiveKind) Label() string { return ProjectKindLabel(e.Kind) }
func (e EffectiveKind) SourceLabel() string { return KindSourceLabel(e.Source) }

func (p WorkProject) EffectiveKind() EffectiveKind {
	if p.ProjectKindSet == 1 && ValidProjectKind(p.ProjectKind) {
		return EffectiveKind{Kind: NormalizeProjectKind(p.ProjectKind), Source: KindSourceConfirmed}
	}
	if k, ok := MapSalesBizToProjectKind(p.SalesBizType); ok {
		return EffectiveKind{Kind: k, Source: KindSourceInferred}
	}
	return EffectiveKind{Kind: ProjectKindUnclassified, Source: KindSourceUnclassified}
}

func ProjectKindOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{ProjectKindConstruction, "공사"},
		{ProjectKindBuild, "구축"},
		{ProjectKindDev, "개발"},
		{ProjectKindMaintenance, "유지보수"},
		{ProjectKindOther, "기타"},
	}
}

func ValidProjectDuty(v string) bool {
	switch strings.TrimSpace(v) {
	case ProjectDutySales, ProjectDutyExecution, ProjectDutyManagement, ProjectDutyAccounting:
		return true
	default:
		return false
	}
}

func ProjectDutyLabel(v string) string {
	switch strings.TrimSpace(v) {
	case ProjectDutySales:
		return "영업 담당"
	case ProjectDutyExecution:
		return "수행 담당"
	case ProjectDutyManagement:
		return "사업관리 담당"
	case ProjectDutyAccounting:
		return "회계 담당"
	default:
		return v
	}
}

func ProjectDutyOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{ProjectDutySales, "영업 담당"},
		{ProjectDutyExecution, "수행 담당"},
		{ProjectDutyManagement, "사업관리 담당"},
		{ProjectDutyAccounting, "회계 담당"},
	}
}

type ProjectAssignment struct {
	ID        int64  `json:"id"`
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Duty      string `json:"duty"`
	UserName  string `json:"user_name,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

func AssignmentSelected(list []ProjectAssignment, duty, userID string) bool {
	for _, a := range list {
		if a.Duty == duty && a.UserID == userID {
			return true
		}
	}
	return false
}
