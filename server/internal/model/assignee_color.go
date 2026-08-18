package model

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// 제품군 기준색 — 담당자·AS·정기점검·행정 카드가 같은 계열을 쓰도록 공유한다.
var (
	ColorAnrobotics = colorTriple{"#16a34a", "#bbf7d0", "#14532d"} // 앤로보틱스(RFID)
	ColorKLAS       = colorTriple{"#2563eb", "#bfdbfe", "#1e3a8a"} // K-LAS
	ColorSejongKLAS = colorTriple{"#9333ea", "#e9d5ff", "#581c87"} // 세종 K-LAS
	ColorAdminAS    = colorTriple{"#ea580c", "#ffedd5", "#9a3412"} // 행정·기타
	ColorUnassigned = colorTriple{"#475569", "#e2e8f0", "#1e293b"}
)

type colorTriple struct {
	Border string
	Soft   string
	Text   string
}

// 담당자 → 제품군 계열 고정 매핑
var assigneeFixedColors = map[string]colorTriple{
	"양기헌": ColorAnrobotics, // 앤로보틱스(RFID자동화)
	"최혜영": ColorKLAS,       // KLAS
}

// 고정 색 외 담당자용 보조 팔레트(고정 초록/파랑과 겹치지 않게)
var assigneeColorPalette = []colorTriple{
	ColorAdminAS,                          // 주황 — 행정 계열
	ColorSejongKLAS,                       // 보라 — 세종 KLAS 계열
	{"#0e7490", "#a5f3fc", "#164e63"},     // 청록
	{"#be185d", "#fbcfe8", "#831843"},     // 분홍
	{"#a16207", "#fde68a", "#713f12"},     // 골드
	{"#7c2d12", "#fed7aa", "#431407"},     // 브라운
	{"#4d7c0f", "#d9f99d", "#365314"},     // 라임
	{"#701a75", "#f0abfc", "#4a044e"},     // 마젠타
	{"#1e3a5f", "#93c5fd", "#0f172a"},     // 네이비
	{"#9f1239", "#fda4af", "#881337"},     // 로즈
}

var (
	assigneeOrderMu sync.RWMutex
	assigneeOrder   map[string]int
)

// SetAssigneeColorOrder 배정 가능 담당자 순으로 고유 색 인덱스를 고정한다(해시 충돌 방지).
// 고정 매핑(양기헌·최혜영)은 순번과 무관하게 제품군 색을 유지한다.
func SetAssigneeColorOrder(names []string) {
	uniq := map[string]struct{}{}
	var list []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := uniq[n]; ok {
			continue
		}
		uniq[n] = struct{}{}
		list = append(list, n)
	}
	sort.Strings(list)
	m := make(map[string]int, len(list))
	idx := 0
	for _, n := range list {
		if _, fixed := assigneeFixedColors[n]; fixed {
			continue
		}
		m[n] = idx
		idx++
	}
	assigneeOrderMu.Lock()
	assigneeOrder = m
	assigneeOrderMu.Unlock()
}

// AssigneeColor 담당자명 → 테두리/배경/글자색.
func AssigneeColor(name string) (border, soft, text string) {
	p := assigneeColorTriple(name)
	return p.Border, p.Soft, p.Text
}

func assigneeColorTriple(name string) colorTriple {
	n := strings.TrimSpace(name)
	if n == "" {
		return ColorUnassigned
	}
	if p, ok := assigneeFixedColors[n]; ok {
		return p
	}
	assigneeOrderMu.RLock()
	idx, ok := assigneeOrder[n]
	assigneeOrderMu.RUnlock()
	if ok {
		return assigneeColorPalette[idx%len(assigneeColorPalette)]
	}
	h := uint32(2166136261)
	for i := 0; i < len(n); i++ {
		h ^= uint32(n[i])
		h *= 16777619
	}
	return assigneeColorPalette[int((h*2654435761)%uint32(len(assigneeColorPalette)))]
}

// AssigneeColorStyle 카드용 인라인 스타일(담당자 색).
func AssigneeColorStyle(name string) string {
	p := assigneeColorTriple(name)
	return fmt.Sprintf(
		"background-color:%s;border-color:%s;border-left:3px solid %s;color:%s",
		p.Soft, p.Border, p.Border, p.Text,
	)
}

// WorkCardColorStyle 업무 카드 색 — 담당자가 있으면 담당자(제품군) 색, 없으면 제품/구분 색.
func WorkCardColorStyle(assignee, productType, category string) string {
	if strings.TrimSpace(assignee) != "" {
		return AssigneeColorStyle(assignee)
	}
	switch {
	case strings.Contains(strings.ToUpper(productType), "세종"):
		p := ColorSejongKLAS
		return fmt.Sprintf("background-color:%s;border-color:%s;border-left:3px solid %s;color:%s", p.Soft, p.Border, p.Border, p.Text)
	case strings.Contains(strings.ToUpper(productType), "KLAS") || strings.Contains(productType, "케이라스"):
		p := ColorKLAS
		return fmt.Sprintf("background-color:%s;border-color:%s;border-left:3px solid %s;color:%s", p.Soft, p.Border, p.Border, p.Text)
	case strings.Contains(productType, "앤") || strings.Contains(strings.ToUpper(productType), "RFID") || strings.Contains(productType, "로보"):
		p := ColorAnrobotics
		return fmt.Sprintf("background-color:%s;border-color:%s;border-left:3px solid %s;color:%s", p.Soft, p.Border, p.Border, p.Text)
	case category == WBSourceAS:
		p := ColorAdminAS
		return fmt.Sprintf("background-color:%s;border-color:%s;border-left:3px solid %s;color:%s", p.Soft, p.Border, p.Border, p.Text)
	default:
		p := ColorAdminAS
		return fmt.Sprintf("background-color:%s;border-color:%s;border-left:3px solid %s;color:%s", p.Soft, p.Border, p.Border, p.Text)
	}
}
