package model

import (
	"strconv"
	"strings"
	"time"
)

const (
	ContractSourceWork  = "work"
	ContractSourceSales = "sales"
)

// ContractRow 계약 관리 합집합 행. 80-A. 73-E 계약 탭과 공유.
type ContractRow struct {
	Source         string
	ProjectID      string
	SalesID        string
	SalesNo        string
	Name           string
	Customer       string
	Amount         int
	ExpectedAmount int
	AmountMissing  bool
	ContractedAt   string
	EndDate        string
	StartDate      string
	Status         string
	OrgID          string
	Promoted       bool
	LinkNeed       string // 제안 영업번호. 80-B
	LinkSalesID    string
	LinkAmbiguous  bool
	AmountMismatch bool
	SalesAmount    int
	DupLinked      bool
	ProjectKind    string
	ProjectKindSet int
	SalesBizType   string
}

func (r ContractRow) EffectiveKind() EffectiveKind {
	p := WorkProject{ProjectKind: r.ProjectKind, ProjectKindSet: r.ProjectKindSet, SalesBizType: r.SalesBizType}
	if r.Source == ContractSourceSales {
		if k, ok := MapSalesBizToProjectKind(r.SalesBizType); ok {
			return EffectiveKind{Kind: k, Source: KindSourceInferred}
		}
		return EffectiveKind{Kind: ProjectKindUnclassified, Source: KindSourceUnclassified}
	}
	return p.EffectiveKind()
}

func (r ContractRow) Href() string {
	if r.Source == ContractSourceSales {
		return "/sales/" + r.SalesID
	}
	return "/contracts/" + r.ProjectID
}

func (r ContractRow) SourceLabel() string {
	if r.Source == ContractSourceSales {
		return "영업 계약"
	}
	return "사업관리"
}

func (r ContractRow) DisplayNo() string {
	if s := strings.TrimSpace(r.SalesNo); s != "" {
		return s
	}
	if r.Source == ContractSourceWork && strings.TrimSpace(r.ProjectID) != "" {
		return r.ProjectID
	}
	return strings.TrimSpace(r.SalesID)
}

func (r ContractRow) AmountText() string {
	if r.AmountMissing {
		return "금액 미입력"
	}
	if r.Amount > 0 {
		return FormatSalesMoney(int64(r.Amount))
	}
	return "—"
}

func (r ContractRow) ExpectedHint() string {
	if r.AmountMissing && r.ExpectedAmount > 0 {
		return "영업 예상 " + FormatSalesMoney(int64(r.ExpectedAmount))
	}
	return ""
}

func (r ContractRow) StatusChip() string {
	if r.Source != ContractSourceSales {
		return ""
	}
	if r.Promoted {
		return "전환 기록 있음, 연결 없음"
	}
	return "영업 계약 완료 · 계약 전환 필요"
}

func (r ContractRow) Year() int {
	s := strings.TrimSpace(r.ContractedAt)
	if s == "" {
		return 0
	}
	if len(s) >= 4 {
		y, _ := strconv.Atoi(s[:4])
		return y
	}
	return 0
}

type ContractUnionKPI struct {
	TotalN     int
	Amount     int64
	ValidN     int
	SoonN      int
	AvgDays    string
	Unpromoted int
	NeedLink   int
}

func BuildContractUnionKPI(rows []ContractRow, today string) ContractUnionKPI {
	k := ContractUnionKPI{AvgDays: "—"}
	k.TotalN = len(rows)
	var daySum, dayN int
	for i := range rows {
		r := rows[i]
		if r.Amount > 0 {
			k.Amount += int64(r.Amount)
		}
		if r.Source == ContractSourceSales {
			k.Unpromoted++
		}
		if r.LinkNeed != "" || r.LinkAmbiguous {
			k.NeedLink++
		}
		if r.Source != ContractSourceWork {
			continue
		}
		st := contractLife(r.StartDate, r.EndDate, today)
		if st == "유효" || st == "만료임박" {
			k.ValidN++
		}
		if st == "만료임박" {
			k.SoonN++
		}
		stt, err1 := time.ParseInLocation("2006-01-02", strings.TrimSpace(r.StartDate), time.Local)
		en, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(r.EndDate), time.Local)
		if err1 == nil && err2 == nil && !en.Before(stt) {
			daySum += int(en.Sub(stt).Hours()/24) + 1
			dayN++
		}
	}
	if dayN > 0 {
		k.AvgDays = strconv.Itoa(daySum/dayN) + "일"
	}
	return k
}

func contractLife(start, end, today string) string {
	p := WorkProject{StartDate: start, EndDate: end}
	return p.ContractLifeStatus(today)
}

func NormalizeProjectName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '(', ')', '[', ']', '{', '}':
			return -1
		}
		return r
	}, s)
	return s
}
