package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type legacyParityPair struct {
	Name   string
	Legacy string
	Modern []string
	Posts  []string
	Needles []string
}

func Test_LegacyParity_Calendar(t *testing.T) {
	root := findTemplateRoot(t)
	ex := loadParityExclusions(t)
	pairs := []legacyParityPair{
		{
			Name:   "일정캘린더",
			Legacy: "workboard/register.html",
			Modern: []string{"workboard/board_day.html", "workboard/board_week.html", "workboard/board_month.html", "workboard/_board_bar.html", "workboard/_board_scope.html", "workboard/_board_head.html"},
			Posts:  []string{"/workboard/schedule", "/workboard/unschedule", "/workboard/register/unlock-past"},
			Needles: []string{"assignee_from_column", "data-kind", "조치"},
		},
		{
			Name:   "리스트",
			Legacy: "work/all.html",
			Modern: []string{"workboard/list.html"},
			Posts:  []string{"/work/review/complete", "/work/review/hold"},
			Needles: []string{},
		},
		{
			Name:   "칸반",
			Legacy: "workboard/register.html",
			Modern: []string{"workboard/board_kanban.html"},
			Posts:  []string{"/workboard/register/kanban-move"},
			Needles: []string{},
		},
	}
	var missing []string
	for _, p := range pairs {
		legacy := readTpl(t, root, p.Legacy)
		mod := ""
		for _, f := range p.Modern {
			mod += readTpl(t, root, f)
		}
		js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "board_schedule.js")))
		mod += js
		for _, post := range p.Posts {
			if strings.Contains(legacy, post) && !strings.Contains(mod, post) && !ex[post] {
				missing = append(missing, p.Name+" 없음 "+post)
			}
		}
		for _, n := range p.Needles {
			if n == "" {
				continue
			}
			if strings.Contains(legacy, n) && !strings.Contains(mod, n) && !ex[n] {
				missing = append(missing, p.Name+" 없음 "+n)
			}
		}
	}
	if len(missing) > 0 {
		t.Fatalf("신화면 기능 이식 미완: %s", strings.Join(missing, "; "))
	}
}

func readTpl(t *testing.T, root, rel string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func loadParityExclusions(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	paths := []string{
		filepath.Join("testdata", "parity_exclusions.txt"),
		filepath.Join("..", "testdata", "parity_exclusions.txt"),
		filepath.Join("..", "..", "testdata", "parity_exclusions.txt"),
		filepath.Join("..", "..", "..", "testdata", "parity_exclusions.txt"),
	}
	var raw []byte
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			raw = b
			break
		}
	}
	if raw == nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := line
		if i := strings.Index(line, " "); i > 0 {
			key = line[:i]
		}
		out[strings.TrimSpace(key)] = true
	}
	return out
}
