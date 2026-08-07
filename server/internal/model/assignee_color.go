package model

import "strings"

// 담당자별 고유 색(테두리·배경·글자). 이름은 해시로 팔레트에 매핑한다.
var assigneeColorPalette = []struct {
	Border string
	Soft   string
	Text   string
}{
	{"#2563eb", "#dbeafe", "#1e3a8a"}, // blue
	{"#dc2626", "#fee2e2", "#991b1b"}, // red
	{"#059669", "#d1fae5", "#065f46"}, // emerald
	{"#d97706", "#ffedd5", "#9a3412"}, // amber
	{"#7c3aed", "#ede9fe", "#5b21b6"}, // violet
	{"#0891b2", "#cffafe", "#155e75"}, // cyan
	{"#db2777", "#fce7f3", "#9d174d"}, // pink
	{"#4f46e5", "#e0e7ff", "#3730a3"}, // indigo
	{"#65a30d", "#ecfccb", "#3f6212"}, // lime
	{"#ea580c", "#ffedd5", "#9a3412"}, // orange
	{"#0d9488", "#ccfbf1", "#115e59"}, // teal
	{"#9333ea", "#f3e8ff", "#6b21a8"}, // purple
}

// AssigneeColor 담당자명 → 테두리/배경/글자색. 빈 담당자는 슬레이트.
func AssigneeColor(name string) (border, soft, text string) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "#64748b", "#f1f5f9", "#334155"
	}
	h := uint32(2166136261)
	for i := 0; i < len(n); i++ {
		h ^= uint32(n[i])
		h *= 16777619
	}
	p := assigneeColorPalette[int(h%uint32(len(assigneeColorPalette)))]
	return p.Border, p.Soft, p.Text
}
