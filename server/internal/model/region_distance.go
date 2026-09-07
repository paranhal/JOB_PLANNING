package model

import "strings"

// RegionDistanceOrder 당사 기준 가까운 시군구 순서. §34.4.3
// customers 에 위경도가 없어 실제 거리는 쓰지 않는다.
type RegionDistanceOrder struct {
	Sido      string
	Sigungu   string
	SortOrder int
}

// RegionKey 시·도 + 시군구 매칭 키.
func RegionKey(sido, sigungu string) string {
	return strings.TrimSpace(sido) + "\x1f" + strings.TrimSpace(sigungu)
}

// RegionVisitStat 하루(또는 묶음) 안의 지역별 방문·사이트 수.
type RegionVisitStat struct {
	Region   string
	Visits   int
	Sites    int
	siteSeen map[string]bool
}

// DominantRegion 같은 날 여러 지역이 섞이면 기준 지역을 고른다. §34.4.2
// 1) 방문 건수가 가장 많은 지역
// 2) 동수면 사이트(기관) 수가 많은 지역
// 3) 그래도 같으면 지역명 가나다순
func DominantRegion(items []RegionVisitStat) string {
	if len(items) == 0 {
		return ""
	}
	best := items[0]
	for i := 1; i < len(items); i++ {
		it := items[i]
		if it.Visits > best.Visits {
			best = it
			continue
		}
		if it.Visits < best.Visits {
			continue
		}
		if it.Sites > best.Sites {
			best = it
			continue
		}
		if it.Sites < best.Sites {
			continue
		}
		if it.Region < best.Region {
			best = it
		}
	}
	return best.Region
}

// CountRegionVisits 방문 목록에서 지역별 건수·사이트 수를 센다.
// regionOf(customerID) 가 빈 값이면 그 건은 건너뛴다.
func CountRegionVisits(customerIDs []string, regionOf func(string) string) []RegionVisitStat {
	by := map[string]*RegionVisitStat{}
	var order []string
	for _, id := range customerIDs {
		reg := strings.TrimSpace(regionOf(id))
		if reg == "" {
			continue
		}
		st, ok := by[reg]
		if !ok {
			st = &RegionVisitStat{Region: reg, siteSeen: map[string]bool{}}
			by[reg] = st
			order = append(order, reg)
		}
		st.Visits++
		if id != "" && !st.siteSeen[id] {
			st.siteSeen[id] = true
			st.Sites++
		}
	}
	out := make([]RegionVisitStat, 0, len(order))
	for _, reg := range order {
		out = append(out, *by[reg])
	}
	return out
}
