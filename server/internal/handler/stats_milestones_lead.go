package handler

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-219: 마일스톤 화면의 「리드타임 추이」. 95-217 이 저장한 일·주·월 스냅샷(lead_time_snapshots)을 읽어 보여 준다.
// 새 산식은 없다. 값이 없으면(표본 0) 0이 아니라 「—」로 표시한다(요구사항 v2 §4.3).

const leadSmallSample = 5 // 이 건수보다 적으면 「표본 적음」

type leadCellView struct {
	Has   bool
	Avg   string
	N     int
	Small bool
}

type leadRowView struct {
	Label  string
	Cells  [3]leadCellView // 접수→방문, 방문→조치완료, 접수→조치완료
	Legacy bool            // 지표 기준일 이전(이관 포함) — 참고용
	Final  bool            // 확정됨(끝난 지 7일 지남). 아니면 집계 중
}

type leadTab struct {
	Kind, Label, Href string
	Active            bool
}

type leadTrendView struct {
	Kind      string
	Tabs      []leadTab
	Rows      []leadRowView // 최근 기간이 먼저
	Spark     [3]string     // 오래된 → 최근 순 막대 문자
	SparkNote [3]string     // 최소·최대
	Names     [3]string
	OrgLabel  string
	Empty     bool
	HasLegacy bool
}

var leadMetricOrder = []string{repository.LeadMetricReceiptToVisit, repository.LeadMetricVisitToDone, repository.LeadMetricReceiptToDone}
var leadMetricNames = [3]string{"접수→방문", "방문→조치완료", "접수→조치완료"}

func leadWindow(kind string, now time.Time) (from, toEx string, limit int) {
	loc := model.KST()
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	toEx = today.AddDate(0, 0, 1).Format("2006-01-02")
	switch kind {
	case repository.LeadKindDay:
		return today.AddDate(0, 0, -29).Format("2006-01-02"), toEx, 30
	case repository.LeadKindMonth:
		first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
		return first.AddDate(0, -5, 0).Format("2006-01-02"), toEx, 6
	default:
		return repository.WeekMonday(today).AddDate(0, 0, -7*12).Format("2006-01-02"), toEx, 13
	}
}

func leadLabel(kind, start, endEx string) string {
	t, err := time.ParseInLocation("2006-01-02", start, model.KST())
	if err != nil {
		return start
	}
	switch kind {
	case repository.LeadKindDay:
		wd := []string{"일", "월", "화", "수", "목", "금", "토"}[t.Weekday()]
		return fmt.Sprintf("%02d/%02d(%s)", int(t.Month()), t.Day(), wd)
	case repository.LeadKindMonth:
		return t.Format("2006-01")
	default:
		e := t.AddDate(0, 0, 6)
		return fmt.Sprintf("%02d/%02d~%02d/%02d", int(t.Month()), t.Day(), int(e.Month()), e.Day())
	}
}

// leadSpark 값 목록(오래된 → 최근)을 ▁▂▃▄▅▆▇█ 로 줄인다. 값이 없는 자리는 「·」.
func leadSpark(vals []*float64) (string, string) {
	min, max := 0.0, 0.0
	first := true
	for _, v := range vals {
		if v == nil {
			continue
		}
		if first || *v < min {
			min = *v
		}
		if first || *v > max {
			max = *v
		}
		first = false
	}
	if first {
		return "", ""
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, v := range vals {
		if v == nil {
			b.WriteRune('·')
			continue
		}
		i := 0
		if max > min {
			i = int((*v - min) / (max - min) * float64(len(bars)-1))
		}
		b.WriteRune(bars[i])
	}
	return b.String(), fmt.Sprintf("최소 %.1f · 최대 %.1f", min, max)
}

func (h *StatsHandler) buildLeadTrend(org, kind string, q url.Values, now time.Time) leadTrendView {
	switch kind {
	case repository.LeadKindDay, repository.LeadKindMonth:
	default:
		kind = repository.LeadKindWeek
	}
	v := leadTrendView{Kind: kind, Names: leadMetricNames}
	for _, k := range []struct{ K, L string }{{repository.LeadKindDay, "일"}, {repository.LeadKindWeek, "주"}, {repository.LeadKindMonth, "월"}} {
		qq := url.Values{}
		for key, vals := range q {
			if key != "lead" && len(vals) > 0 {
				qq.Set(key, vals[0])
			}
		}
		qq.Set("lead", k.K)
		v.Tabs = append(v.Tabs, leadTab{Kind: k.K, Label: k.L, Href: "/stats/milestones?" + qq.Encode() + "#lead-trend", Active: k.K == kind})
	}
	orgID := strings.TrimSpace(org)
	v.OrgLabel = "전체"
	if orgID != "" && orgID != repository.OrgAll {
		v.OrgLabel = "이 조직"
	} else {
		orgID = repository.LeadOrgAll
	}
	if h == nil || h.repo == nil {
		v.Empty = true
		return v
	}
	from, toEx, limit := leadWindow(kind, now)
	rows, err := h.repo.ListLeadSnapshots(kind, orgID, from, toEx)
	if err != nil || len(rows) == 0 {
		v.Empty = true
		return v
	}
	byStart := map[string]*leadRowView{}
	var starts []string
	for _, r := range rows {
		rv := byStart[r.Start]
		if rv == nil {
			rv = &leadRowView{Label: leadLabel(kind, r.Start, r.EndEx), Legacy: r.Quality == repository.LeadQualityLegacy, Final: r.FinalAt != ""}
			byStart[r.Start] = rv
			starts = append(starts, r.Start)
		}
		for i, m := range leadMetricOrder {
			if r.Metric != m {
				continue
			}
			c := leadCellView{N: r.Sample, Small: r.Sample > 0 && r.Sample < leadSmallSample}
			if r.AvgDays != nil && r.Sample > 0 {
				c.Has = true
				c.Avg = fmt.Sprintf("%.1f", *r.AvgDays)
			}
			rv.Cells[i] = c
		}
	}
	// 일 단위는 표본이 하나도 없는 날(휴무일 등)을 빼고, 최근 limit 개만 쓴다.
	var keep []string
	for _, s := range starts {
		rv := byStart[s]
		any := rv.Cells[0].N+rv.Cells[1].N+rv.Cells[2].N > 0
		if kind == repository.LeadKindDay && !any {
			continue
		}
		keep = append(keep, s)
	}
	if len(keep) > limit {
		keep = keep[len(keep)-limit:]
	}
	for i := 0; i < 3; i++ {
		var vals []*float64
		for _, s := range keep {
			c := byStart[s].Cells[i]
			if c.Has {
				var f float64
				fmt.Sscanf(c.Avg, "%f", &f)
				vals = append(vals, &f)
			} else {
				vals = append(vals, nil)
			}
		}
		v.Spark[i], v.SparkNote[i] = leadSpark(vals)
	}
	for i := len(keep) - 1; i >= 0; i-- {
		rv := byStart[keep[i]]
		if rv.Legacy {
			v.HasLegacy = true
		}
		v.Rows = append(v.Rows, *rv)
	}
	v.Empty = len(v.Rows) == 0
	return v
}
