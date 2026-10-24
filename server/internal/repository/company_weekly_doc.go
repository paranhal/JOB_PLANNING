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
}

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
	}{Progress: doc.Progress, Rows: doc.Rows}
	b, _ := json.Marshal(payload)
	return string(b)
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
