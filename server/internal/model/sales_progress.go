package model

import (
	"sort"
	"strings"
	"time"
)

type SalesProgressStep struct {
	Code    string
	Label   string
	Done    bool
	Mark    string
}

type SalesStageProgress struct {
	Discover []SalesProgressStep
	Propose  []SalesProgressStep
}

func SalesActivityDone(a SalesActivity, today string) bool {
	d := strings.TrimSpace(a.ActivityDate)
	if d == "" || today == "" {
		return false
	}
	if len(d) > 10 {
		d = d[:10]
	}
	return d <= today
}

func BuildSalesStageProgress(p *SalesProject, acts []SalesActivity, quotes []SalesQuote, today string) SalesStageProgress {
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	has := map[string]bool{}
	for _, a := range acts {
		if !SalesActivityDone(a, today) {
			continue
		}
		has[strings.TrimSpace(a.ActivityType)] = true
	}
	sentQuote := false
	for _, q := range quotes {
		if NormalizeQuoteStatus(q.Status) == QuoteStatusSent {
			sentQuote = true
			break
		}
	}
	contact := has[SalesActTypeCall] || has[SalesActTypeVisit] || has[SalesActTypeOnline] || has[SalesActTypeMail]
	quoteSent := has[SalesActTypeQuote] || sentQuote
	rfp := p != nil && strings.TrimSpace(p.RFPReceivedAt) != ""
	eval := p != nil && strings.TrimSpace(p.BidEvalMethod) != ""
	step := func(code, label string, done bool) SalesProgressStep {
		m := "○"
		if done {
			m = "✔"
		}
		return SalesProgressStep{Code: code, Label: label, Done: done, Mark: m}
	}
	return SalesStageProgress{
		Discover: []SalesProgressStep{
			step("info", "정보 입수", has[SalesActTypeResearch]),
			step("contact", "담당자 접촉", contact),
			step("req", "요구사항 파악", has[SalesActTypeRequirement]),
		},
		Propose: []SalesProgressStep{
			step("proposal", "1차 제안", has[SalesActTypeProposal]),
			step("quote", "견적 발송", quoteSent),
			step("rfp", "RFP 접수", rfp),
			step("eval", "평가방법 확정", eval),
		},
	}
}

type SalesActivityProjectGroup struct {
	SalesID    string
	SalesNo    string
	Name       string
	Stage      string
	StageLabel string
	LastDate   string
	Count      int
	Items      []SalesActivity
}

func GroupSalesActivitiesByProject(acts []SalesActivity, projects []SalesProject) []SalesActivityProjectGroup {
	by := map[string]*SalesActivityProjectGroup{}
	order := []string{}
	for _, a := range acts {
		id := strings.TrimSpace(a.SalesID)
		if id == "" {
			continue
		}
		g, ok := by[id]
		if !ok {
			g = &SalesActivityProjectGroup{SalesID: id, Name: a.SalesName, SalesNo: a.SalesNo, Stage: a.StageAtTime}
			by[id] = g
			order = append(order, id)
		}
		g.Items = append(g.Items, a)
		g.Count++
		if a.ActivityDate > g.LastDate {
			g.LastDate = a.ActivityDate
		}
	}
	pmap := map[string]SalesProject{}
	for _, p := range projects {
		pmap[p.SalesID] = p
	}
	for id, g := range by {
		if p, ok := pmap[id]; ok {
			g.Name = p.Name
			g.SalesNo = p.SalesNo
			g.Stage = p.Stage
			g.StageLabel = p.StageLabel
		}
		sort.Slice(g.Items, func(i, j int) bool {
			if g.Items[i].ActivityDate == g.Items[j].ActivityDate {
				return g.Items[i].StartTime < g.Items[j].StartTime
			}
			return g.Items[i].ActivityDate < g.Items[j].ActivityDate
		})
	}
	sort.Slice(order, func(i, j int) bool {
		return by[order[i]].LastDate > by[order[j]].LastDate
	})
	out := make([]SalesActivityProjectGroup, 0, len(order))
	for _, id := range order {
		out = append(out, *by[id])
	}
	return out
}
