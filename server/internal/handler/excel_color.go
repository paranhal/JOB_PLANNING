package handler

import "strings"

func isHex6(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// xlColor excelize 스타일용 6자리 RRGGBB. 8자리 ARGB의 앞 FF와 # 을 벗긴다.
func xlColor(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	s = strings.ToUpper(s)
	if len(s) == 8 && strings.HasPrefix(s, "FF") && isHex6(s[2:]) {
		return s[2:]
	}
	if isHex6(s) {
		return s
	}
	return s
}
