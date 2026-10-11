package handler

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

type rgbTriplet struct{ R, G, B float64 }

func parseTokenColors(t *testing.T, css string) map[string]rgbTriplet {
	t.Helper()
	re := regexp.MustCompile(`--c-([a-z0-9-]+):\s*(\d+)\s+(\d+)\s+(\d+)`)
	out := map[string]rgbTriplet{}
	for _, m := range re.FindAllStringSubmatch(css, -1) {
		r, _ := strconv.Atoi(m[2])
		g, _ := strconv.Atoi(m[3])
		b, _ := strconv.Atoi(m[4])
		out[m[1]] = rgbTriplet{float64(r), float64(g), float64(b)}
	}
	return out
}

func srgbLin(c float64) float64 {
	c = c / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func relativeLuminance(c rgbTriplet) float64 {
	return 0.2126*srgbLin(c.R) + 0.7152*srgbLin(c.G) + 0.0722*srgbLin(c.B)
}

func contrastRatio(a, b rgbTriplet) float64 {
	l1 := relativeLuminance(a)
	l2 := relativeLuminance(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestTokenContrastWCAG(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("web", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	colors := parseTokenColors(t, string(b))
	need := []string{
		"primary", "on-primary", "text", "text-muted", "surface", "canvas",
		"success", "success-soft", "warning", "warning-soft",
		"danger", "danger-soft", "info", "info-soft",
	}
	for _, n := range need {
		if _, ok := colors[n]; !ok {
			t.Fatalf("토큰 --c-%s 없음", n)
		}
	}

	assertMin := func(name string, a, b rgbTriplet, min float64) {
		t.Helper()
		got := contrastRatio(a, b)
		if got+1e-9 < min {
			t.Errorf("%s 대비 %.2f:1 < %.1f:1", name, got, min)
		} else {
			t.Logf("%s 대비 %.2f:1", name, got)
		}
	}

	assertMin("on-primary vs primary", colors["on-primary"], colors["primary"], 4.5)
	assertMin("text vs surface", colors["text"], colors["surface"], 7)
	assertMin("text vs canvas", colors["text"], colors["canvas"], 7)
	assertMin("text-muted vs surface", colors["text-muted"], colors["surface"], 4.5)
	assertMin("success vs success-soft", colors["success"], colors["success-soft"], 4.5)
	assertMin("warning vs warning-soft", colors["warning"], colors["warning-soft"], 4.5)
	assertMin("danger vs danger-soft", colors["danger"], colors["danger-soft"], 4.5)
	assertMin("info vs info-soft", colors["info"], colors["info-soft"], 4.5)
}
