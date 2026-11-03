package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Viewport struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Screen struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Auth    *bool  `json:"auth,omitempty"`
	Resolve string `json:"resolve,omitempty"`
}

type GradeBlock struct {
	Label   string   `json:"label"`
	Note    string   `json:"note"`
	Screens []Screen `json:"screens"`
}

type Catalog struct {
	Viewports []Viewport            `json:"viewports"`
	MinTouch  int                   `json:"min_touch"`
	Grades    map[string]GradeBlock `json:"grades"`
}

func loadCatalog(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.MinTouch <= 0 {
		c.MinTouch = 44
	}
	return &c, nil
}

func (c *Catalog) screens(grade string) []Screen {
	g, ok := c.Grades[grade]
	if !ok {
		return nil
	}
	return g.Screens
}

func (c *Catalog) allIDs() []string {
	var ids []string
	for _, g := range c.Grades {
		for _, s := range g.Screens {
			ids = append(ids, s.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func defaultCatalogPath() string {
	if p := os.Getenv("MOBILE_AUDIT_SCREENS"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err == nil {
		cand := filepath.Join(filepath.Dir(exe), "screens.json")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	wd, _ := os.Getwd()
	for _, p := range []string{
		filepath.Join(wd, "screens.json"),
		filepath.Join(wd, "scripts", "mobile_audit", "screens.json"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "screens.json"
}

func needsAuth(s Screen) bool {
	if s.Auth != nil {
		return *s.Auth
	}
	return s.ID != "login"
}

func gradeSummary(c *Catalog) string {
	var out string
	for _, g := range []string{"A", "B", "C"} {
		block := c.Grades[g]
		out += fmt.Sprintf("%s %s: %d화면\n", g, block.Label, len(block.Screens))
	}
	return out
}
