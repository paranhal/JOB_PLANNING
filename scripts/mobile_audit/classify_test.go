package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogABCAndViewports(t *testing.T) {
	p := filepath.Join("screens.json")
	if _, err := os.Stat(p); err != nil {
		p = defaultCatalogPath()
	}
	cat, err := loadCatalog(p)
	if err != nil {
		t.Fatal(err)
	}
	if cat.MinTouch != 44 {
		t.Fatalf("min_touch=%d", cat.MinTouch)
	}
	if len(cat.Viewports) != 2 || cat.Viewports[0].Width != 390 || cat.Viewports[1].Width != 360 {
		t.Fatalf("뷰포트 %+v", cat.Viewports)
	}
	a, b, c := cat.screens("A"), cat.screens("B"), cat.screens("C")
	if len(a) == 0 || len(b) == 0 || len(c) == 0 {
		t.Fatal("A/B/C 가 비었다")
	}
	seen := map[string]string{}
	for _, g := range []string{"A", "B", "C"} {
		for _, s := range cat.screens(g) {
			if s.ID == "" || s.Path == "" || s.Title == "" {
				t.Fatalf("%s 항목 불완전 %+v", g, s)
			}
			if prev, ok := seen[s.ID]; ok {
				t.Fatalf("id 중복 %s (%s, %s)", s.ID, prev, g)
			}
			seen[s.ID] = g
		}
	}
	needA := []string{"login", "dashboard", "workboard_list", "as_list", "mnt_list", "customers"}
	for _, id := range needA {
		if seen[id] != "A" {
			t.Errorf("%s 는 A 여야 한다 (got %s)", id, seen[id])
		}
	}
	if seen["admin_data"] != "C" {
		t.Errorf("admin_data grade=%s", seen["admin_data"])
	}
}

func TestAuditJWTSecretMatchesServerHandler(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	h, err := os.ReadFile(filepath.Join(root, "server", "internal", "handler", "handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "scripts", "mobile_audit", "main.go"))
	if err != nil {
		src, err = os.ReadFile("main.go")
	}
	if err != nil {
		t.Fatal(err)
	}
	const secret = "cs-system-jwt-secret-2026"
	if !strings.Contains(string(h), secret) || !strings.Contains(string(src), secret) {
		t.Fatal("점검 JWT 비밀이 서버와 다르다")
	}
}

func TestFindIssuesHScrollTableTouchFixed(t *testing.T) {
	m := PageMetrics{
		InnerWidth: 390, InnerHeight: 844, ScrollWidth: 520,
		Tables: []TableMetric{
			{W: 500, ScrollParent: false, Clipped: true},
			{W: 800, ScrollParent: true, Clipped: false},
		},
		Targets: []TargetMetric{
			{Tag: "a", W: 40, H: 20, Short: 20},
			{Tag: "button", W: 48, H: 48, Short: 48},
		},
		Fixed: []FixedBox{
			{Pos: "fixed", T: 0, L: 0, B: 56, R: 390},
			{Pos: "fixed", T: 40, L: 0, B: 90, R: 390},
		},
	}
	iss := findIssues(m, 44)
	kinds := map[string]Issue{}
	for _, i := range iss {
		kinds[i.Kind] = i
	}
	if kinds["hscroll"].ExtraPx != 130 {
		t.Fatalf("hscroll %+v", kinds["hscroll"])
	}
	if kinds["table_clip"].Count != 1 {
		t.Fatalf("table %+v", kinds["table_clip"])
	}
	if kinds["touch"].Count != 1 {
		t.Fatalf("touch %+v", kinds["touch"])
	}
	if kinds["fixed_overlap"].Count < 1 {
		t.Fatalf("fixed %+v", kinds["fixed_overlap"])
	}

	clean := PageMetrics{InnerWidth: 390, InnerHeight: 844, ScrollWidth: 390,
		Targets: []TargetMetric{{Tag: "a", W: 44, H: 44, Short: 44}}}
	if n := findIssues(clean, 44); len(n) != 0 {
		t.Fatalf("깨끗한 화면 issues=%v", n)
	}
}
