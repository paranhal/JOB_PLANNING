package model

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// RegionKeySep 월 대상 등록용 시·도|시·군·구 구분자. RegionKey 의 \x1f 는 폼에 쓰지 않는다.
	RegionKeySep = "|"
	// RegionSigunguUnsetQuery 시·군·구 미지정 필터 값.
	RegionSigunguUnsetQuery = "__unset__"
	// RegionSigunguUnsetLabel 시·군·구가 비어 있는 사이트 묶음.
	RegionSigunguUnsetLabel = "시·군·구 미지정"
)

// ShortRegionName 행정구역 정식명칭을 점검표 축약형으로 (충청남도 → 충남).
func ShortRegionName(sido string) string {
	s := strings.TrimSpace(sido)
	if s == "" {
		return ""
	}
	short := map[string]string{
		"서울특별시": "서울", "부산광역시": "부산", "대구광역시": "대구", "인천광역시": "인천",
		"광주광역시": "광주", "대전광역시": "대전", "울산광역시": "울산",
		"세종특별자치시": "세종", "경기도": "경기", "강원도": "강원", "강원특별자치도": "강원",
		"충청북도": "충북", "충청남도": "충남", "전라북도": "전북", "전북특별자치도": "전북",
		"전라남도": "전남", "경상북도": "경북", "경상남도": "경남",
		"제주도": "제주", "제주특별자치도": "제주",
	}
	if v, ok := short[s]; ok {
		return v
	}
	return s
}

func collapseRegionSpaces(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

// SiteRegion 월 대상·필터용 시·도 / 시·군·구. cfg.Region 을 바꾸지 않는다.
func SiteRegion(cfg MaintenanceSiteConfig) (sido, sigungu string) {
	sido = strings.TrimSpace(cfg.Region)
	if sido == "" {
		sido = ShortRegionName(strings.TrimSpace(cfg.AddrSido))
	}
	sigungu = collapseRegionSpaces(cfg.AddrSigungu)
	return sido, sigungu
}

// SiteRegionKey 사이트의 가장 구체적인 키(시·도만 / 시·도|시군구 / 시·도| / 지역 미지정).
func SiteRegionKey(cfg MaintenanceSiteConfig) string {
	sido, sigungu := SiteRegion(cfg)
	return FormatRegionKey(sido, sigungu)
}

// FormatRegionKey 시·도만이면 파이프 없음. 시·군·구 없으면 끝에 구분자만.
func FormatRegionKey(sido, sigungu string) string {
	sido = strings.TrimSpace(sido)
	sigungu = collapseRegionSpaces(sigungu)
	if sido == "" {
		return RegionUnspecified
	}
	if sigungu == "" {
		return sido + RegionKeySep
	}
	return sido + RegionKeySep + sigungu
}

// SidoRegionKey 시·도 전체 키. 시·도가 없으면 지역 미지정.
func SidoRegionKey(sido string) string {
	sido = strings.TrimSpace(sido)
	if sido == "" {
		return RegionUnspecified
	}
	return sido
}

// ParseRegionKey 시·도 키와 시·군·구(있으면), 파이프 여부.
func ParseRegionKey(key string) (sido, sigungu string, hasSigungu bool) {
	key = strings.TrimSpace(key)
	if key == "" || key == RegionUnspecified {
		return "", "", false
	}
	parent, child, ok := strings.Cut(key, RegionKeySep)
	if !ok {
		return strings.TrimSpace(parent), "", false
	}
	return strings.TrimSpace(parent), collapseRegionSpaces(child), true
}

// RegionKeyLabel 화면·결과 문구용.
func RegionKeyLabel(key string) string {
	sido, sigungu, has := ParseRegionKey(key)
	if sido == "" {
		return RegionUnspecified
	}
	if !has {
		return sido
	}
	if sigungu == "" {
		return sido + " (" + RegionSigunguUnsetLabel + ")"
	}
	return sido + " " + sigungu
}

// SiteRegionLabel 사이트 표 「충남 홍성군」. 시·군·구가 없으면 시·도만.
func SiteRegionLabel(cfg MaintenanceSiteConfig) string {
	sido, sigungu := SiteRegion(cfg)
	if sido == "" {
		return RegionUnspecified
	}
	if sigungu == "" {
		return sido
	}
	return sido + " " + sigungu
}

// MatchRegionKey 시·도 키면 시·도만, 시·도|시군구 면 둘 다.
func MatchRegionKey(cfg MaintenanceSiteConfig, key string) bool {
	sido, sigungu := SiteRegion(cfg)
	key = strings.TrimSpace(key)
	if key == "" {
		key = RegionUnspecified
	}
	wantSido, wantGu, has := ParseRegionKey(key)
	if !has {
		if wantSido == "" {
			return sido == ""
		}
		return sido == wantSido
	}
	return sido == wantSido && sigungu == wantGu
}

// MatchAnyRegionKey 선택된 지역 키 중 하나라도 맞으면 true. 키가 없으면 필터 없음.
func MatchAnyRegionKey(cfg MaintenanceSiteConfig, keys []string) bool {
	var nonempty []string
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		nonempty = append(nonempty, k)
	}
	if len(nonempty) == 0 {
		return true
	}
	for _, k := range nonempty {
		if MatchRegionKey(cfg, k) {
			return true
		}
	}
	return false
}

// CollapseMonthTargetRegionKeys 시·도 전체가 있으면 그 하위 시·군·구 키를 뺀다.
func CollapseMonthTargetRegionKeys(keys []string) []string {
	parents := map[string]bool{}
	var order []string
	seen := map[string]bool{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		order = append(order, k)
		sido, _, has := ParseRegionKey(k)
		if !has {
			if sido == "" {
				parents[RegionUnspecified] = true
			} else {
				parents[sido] = true
			}
		}
	}
	var out []string
	seenOut := map[string]bool{}
	for _, k := range order {
		sido, _, has := ParseRegionKey(k)
		if has {
			parent := sido
			if parent == "" {
				parent = RegionUnspecified
			}
			if parents[parent] {
				continue
			}
		}
		if seenOut[k] {
			continue
		}
		seenOut[k] = true
		out = append(out, k)
	}
	return out
}

func joinRegionRegisterLabels(keys []string) string {
	type grp struct {
		sido   string
		parent bool
		parts  []string
	}
	var groups []grp
	flushParent := func(sido string) {
		groups = append(groups, grp{sido: sido, parent: true})
	}
	for _, k := range keys {
		sido, gu, has := ParseRegionKey(k)
		if sido == "" {
			flushParent(RegionUnspecified)
			continue
		}
		if !has {
			flushParent(sido)
			continue
		}
		part := gu
		if part == "" {
			part = "(" + RegionSigunguUnsetLabel + ")"
		}
		if n := len(groups); n > 0 && !groups[n-1].parent && groups[n-1].sido == sido {
			groups[n-1].parts = append(groups[n-1].parts, part)
			continue
		}
		groups = append(groups, grp{sido: sido, parts: []string{part}})
	}
	var labels []string
	for _, g := range groups {
		if g.parent {
			labels = append(labels, g.sido)
			continue
		}
		labels = append(labels, g.sido+" "+strings.Join(g.parts, "·"))
	}
	return strings.Join(labels, "·")
}

// RegionRegisterSummary 「충남 홍성군·예산군 — 57곳 등록 / 3곳 건너뜀」
func RegionRegisterSummary(keys []string, added, skipped int) string {
	head := joinRegionRegisterLabels(CollapseMonthTargetRegionKeys(keys))
	body := fmt.Sprintf("%d곳 등록", added)
	if skipped > 0 {
		body += " / " + strconv.Itoa(skipped) + "곳 건너뜀"
	}
	if head == "" {
		return body
	}
	return head + " — " + body
}
