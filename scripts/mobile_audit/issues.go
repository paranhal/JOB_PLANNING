package main

import "fmt"

type TableMetric struct {
	W            float64 `json:"w"`
	ScrollParent bool    `json:"scrollParent"`
	Clipped      bool    `json:"clipped"`
}

type TargetMetric struct {
	Tag   string  `json:"tag"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Short float64 `json:"short"`
}

type FixedBox struct {
	Pos string  `json:"pos"`
	T   float64 `json:"t"`
	L   float64 `json:"l"`
	B   float64 `json:"b"`
	R   float64 `json:"r"`
}

type PageMetrics struct {
	InnerWidth  float64        `json:"innerWidth"`
	InnerHeight float64        `json:"innerHeight"`
	ScrollWidth float64        `json:"scrollWidth"`
	Tables      []TableMetric  `json:"tables"`
	Targets     []TargetMetric `json:"targets"`
	Fixed       []FixedBox     `json:"fixed"`
}

type Issue struct {
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
	Count   int    `json:"count,omitempty"`
	ExtraPx int    `json:"extra_px,omitempty"`
}

func boxesOverlap(a, b FixedBox) bool {
	return a.L < b.R && b.L < a.R && a.T < b.B && b.T < a.B
}

func findIssues(m PageMetrics, minTouch int) []Issue {
	if minTouch <= 0 {
		minTouch = 44
	}
	var out []Issue
	extra := int(m.ScrollWidth - m.InnerWidth)
	if extra > 2 {
		out = append(out, Issue{Kind: "hscroll", Detail: "문서 가로 스크롤", ExtraPx: extra})
	}
	clipped := 0
	for _, t := range m.Tables {
		if t.Clipped {
			clipped++
		}
	}
	if clipped > 0 {
		out = append(out, Issue{Kind: "table_clip", Detail: "표가 뷰포트보다 넓고 가로 스크롤 부모 없음", Count: clipped})
	}
	short := 0
	for _, t := range m.Targets {
		if t.Short > 0 && t.Short < float64(minTouch) {
			short++
		}
	}
	if short > 0 {
		out = append(out, Issue{Kind: "touch", Detail: fmt.Sprintf("터치 대상 %dpx 미만", minTouch), Count: short})
	}
	overlap := 0
	for i := 0; i < len(m.Fixed); i++ {
		for j := i + 1; j < len(m.Fixed); j++ {
			a, b := m.Fixed[i], m.Fixed[j]
			if a.Pos != "fixed" && b.Pos != "fixed" {
				continue
			}
			aw := a.R - a.L
			ah := a.B - a.T
			bw := b.R - b.L
			bh := b.B - b.T
			if aw >= m.InnerWidth-8 && bw >= m.InnerWidth-8 {
				if boxesOverlap(a, b) {
					overlap++
				}
			}
			_ = ah
			_ = bh
		}
	}
	if overlap > 0 {
		out = append(out, Issue{Kind: "fixed_overlap", Detail: "고정 영역이 서로 겹침", Count: overlap})
	}
	return out
}
