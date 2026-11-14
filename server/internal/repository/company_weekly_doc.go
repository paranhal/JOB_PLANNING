package repository

import (
	"encoding/json"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type CompanyWeeklyLine struct {
	Text   string `json:"text"`
	Manual bool   `json:"manual"`
	Key    string `json:"key,omitempty"`
}

type CompanyWeeklyLog struct {
	At   string `json:"at"`
	By   string `json:"by"`
	Text string `json:"text"`
}

type CompanyWeeklyProgress struct {
	Done      *int               `json:"done"`
	Total     *int               `json:"total"`
	Pct       *float64           `json:"pct"`
	Cycle     string             `json:"cycle"`
	Memo      string             `json:"memo"`
	UpdatedBy string             `json:"updated_by"`
	UpdatedAt string             `json:"updated_at"`
	Log       []CompanyWeeklyLog `json:"log,omitempty"`
}

type CompanyWeeklyDoc struct {
	Progress *CompanyWeeklyProgress
	Rows     map[string]CompanyWeeklySavedRow
	// 95-192: 팀 전체 금주 추진내역·차주 추진계획(사업 행과 별개로 팀이 한 번 적는다).
	TeamDone []CompanyWeeklyLine
	TeamPlan []CompanyWeeklyLine
	// Extra 알 수 없는 최상위 키. 앞으로 필드가 늘어도 이 코드가 저장 때 지우지 않게 그대로 돌려 쓴다.
	Extra map[string]json.RawMessage
}

// companyWeeklyKnownKeys Parse/Encode 가 직접 다루는 최상위 키.
var companyWeeklyKnownKeys = map[string]bool{"progress": true, "rows": true, "team_done": true, "team_plan": true}

func ParseCompanyWeeklyDoc(raw string) CompanyWeeklyDoc {
	doc := CompanyWeeklyDoc{Rows: map[string]CompanyWeeklySavedRow{}}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return doc
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return doc
	}
	if rawRows, ok := top["rows"]; ok && len(rawRows) > 0 && rawRows[0] == '{' {
		_ = json.Unmarshal(rawRows, &doc.Rows)
		for _, k := range []string{"team_done", "team_plan"} {
			if v, ok := top[k]; ok && len(v) > 0 && v[0] == '[' {
				var lines []CompanyWeeklyLine
				if json.Unmarshal(v, &lines) == nil {
					if k == "team_done" {
						doc.TeamDone = lines
					} else {
						doc.TeamPlan = lines
					}
				}
			}
		}
		for k, v := range top {
			if !companyWeeklyKnownKeys[k] {
				if doc.Extra == nil {
					doc.Extra = map[string]json.RawMessage{}
				}
				doc.Extra[k] = v
			}
		}
		if p, ok := top["progress"]; ok && len(p) > 0 && p[0] == '{' {
			var pr CompanyWeeklyProgress
			if json.Unmarshal(p, &pr) == nil {
				doc.Progress = &pr
			}
		}
		if doc.Rows == nil {
			doc.Rows = map[string]CompanyWeeklySavedRow{}
		}
		return doc
	}
	_ = json.Unmarshal([]byte(raw), &doc.Rows)
	if doc.Rows == nil {
		doc.Rows = map[string]CompanyWeeklySavedRow{}
	}
	return doc
}

func EncodeCompanyWeeklyDoc(doc CompanyWeeklyDoc) string {
	if doc.Rows == nil {
		doc.Rows = map[string]CompanyWeeklySavedRow{}
	}
	payload := struct {
		Progress *CompanyWeeklyProgress           `json:"progress,omitempty"`
		Rows     map[string]CompanyWeeklySavedRow `json:"rows"`
		TeamDone []CompanyWeeklyLine              `json:"team_done,omitempty"`
		TeamPlan []CompanyWeeklyLine              `json:"team_plan,omitempty"`
	}{Progress: doc.Progress, Rows: doc.Rows, TeamDone: doc.TeamDone, TeamPlan: doc.TeamPlan}
	b, _ := json.Marshal(payload)
	if len(doc.Extra) == 0 {
		return string(b)
	}
	// 알 수 없는 키를 되돌려 넣는다(알려진 키는 위 payload 가 우선).
	var top map[string]json.RawMessage
	if json.Unmarshal(b, &top) != nil {
		return string(b)
	}
	for k, v := range doc.Extra {
		if _, ok := top[k]; !ok {
			top[k] = v
		}
	}
	out, err := json.Marshal(top)
	if err != nil {
		return string(b)
	}
	return string(out)
}

func SplitWeeklyLines(text string) []CompanyWeeklyLine {
	var out []CompanyWeeklyLine
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "·")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, CompanyWeeklyLine{Text: line})
	}
	return out
}

func JoinWeeklyLines(lines []CompanyWeeklyLine) string {
	var parts []string
	for _, l := range lines {
		t := strings.TrimSpace(l.Text)
		if t == "" {
			continue
		}
		parts = append(parts, "·"+t)
	}
	return strings.Join(parts, "\n")
}

func MergeWeeklyLines(draftText string, saved []CompanyWeeklyLine) []CompanyWeeklyLine {
	return MergeWeeklyLinesKeyed(SplitWeeklyLines(draftText), saved, nil)
}

func MergeWeeklyLinesKeyed(auto []CompanyWeeklyLine, saved []CompanyWeeklyLine, hidden []string) []CompanyWeeklyLine {
	hide := map[string]bool{}
	for _, k := range hidden {
		if k = strings.TrimSpace(k); k != "" {
			hide[k] = true
		}
	}
	savedByKey := map[string]CompanyWeeklyLine{}
	var extra []CompanyWeeklyLine
	for _, l := range saved {
		l.Text = strings.TrimSpace(l.Text)
		if l.Text == "" {
			continue
		}
		if k := strings.TrimSpace(l.Key); k != "" {
			savedByKey[k] = l
			continue
		}
		if l.Manual {
			extra = append(extra, CompanyWeeklyLine{Text: l.Text, Manual: true})
		}
	}
	if len(saved) > 0 && len(savedByKey) == 0 && len(extra) == 0 {
		var manual []CompanyWeeklyLine
		sawManual := false
		for _, l := range saved {
			if l.Manual {
				sawManual = true
				manual = append(manual, CompanyWeeklyLine{Text: strings.TrimSpace(l.Text), Manual: true})
			}
		}
		if !sawManual {
			if joined := JoinWeeklyLines(saved); joined != "" {
				return SplitWeeklyLines(joined)
			}
			return auto
		}
		return append(auto, manual...)
	}
	var out []CompanyWeeklyLine
	for _, a := range auto {
		a.Text = strings.TrimSpace(a.Text)
		if a.Text == "" {
			continue
		}
		if hide[a.Key] {
			continue
		}
		if s, ok := savedByKey[a.Key]; ok {
			if s.Manual || s.Text != a.Text {
				out = append(out, CompanyWeeklyLine{Text: s.Text, Manual: true, Key: a.Key})
				continue
			}
		}
		out = append(out, a)
	}
	return append(out, extra...)
}

func AutoToLines(in []model.CompanyWeeklyAutoLine) []CompanyWeeklyLine {
	var out []CompanyWeeklyLine
	for _, a := range in {
		t := strings.TrimSpace(a.Text)
		if t == "" {
			continue
		}
		out = append(out, CompanyWeeklyLine{Key: a.Key, Text: strings.TrimPrefix(t, "·")})
	}
	return out
}

func (p *CompanyWeeklyProgress) Display(cycleDefault string) (label string, empty bool, warn bool) {
	if p == nil {
		return "진행율 미입력", true, false
	}
	cyc := strings.TrimSpace(p.Cycle)
	if cyc == "" {
		cyc = cycleDefault
	}
	if p.Done != nil && p.Total != nil && *p.Total >= 0 && *p.Done > *p.Total {
		warn = true
	}
	switch {
	case p.Done != nil && p.Total != nil && p.Pct != nil:
		return fmt.Sprintf("%s · 누적 %d/%d · %s%%", cyc, *p.Done, *p.Total, formatPct(*p.Pct)), false, warn
	case p.Pct != nil:
		return fmt.Sprintf("%s · %s%%", cyc, formatPct(*p.Pct)), false, warn
	case p.Done != nil && p.Total != nil:
		return fmt.Sprintf("%s · 누적 %d/%d", cyc, *p.Done, *p.Total), false, warn
	default:
		return "진행율 미입력", true, warn
	}
}

func formatPct(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s
}

func rowToSaved(r model.CompanyWeeklyRow) CompanyWeeklySavedRow {
	s := CompanyWeeklySavedRow{
		Prev: r.PrevText, Plan: r.PlanText, Help: r.HelpText, Decision: r.DecisionText,
	}
	if len(SplitWeeklyLines(r.PrevText)) > 0 {
		s.DoneLines = SplitWeeklyLines(r.PrevText)
	}
	if len(SplitWeeklyLines(r.PlanText)) > 0 {
		s.PlanLines = SplitWeeklyLines(r.PlanText)
	}
	return s
}
