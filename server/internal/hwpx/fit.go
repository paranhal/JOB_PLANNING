package hwpx

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FitASReportTables 한글에서 줄인 표 높이. 장애·조치 칸을 낮춰 사인 줄이 1페이지에 남게 한다.
func FitASReportTables(doc []byte) ([]byte, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("HWPX가 비어 있습니다")
	}
	parts, err := unzipEntries(doc)
	if err != nil {
		return nil, fmt.Errorf("HWPX를 열 수 없습니다: %w", err)
	}
	for i := range parts {
		n := filepath.ToSlash(parts[i].Name)
		if !strings.HasPrefix(n, "Contents/section") || !strings.HasSuffix(strings.ToLower(n), ".xml") {
			continue
		}
		parts[i].Body = []byte(fitASReportSection(string(parts[i].Body)))
	}
	return packHWPX(parts)
}

func fitASReportSection(s string) string {
	repl := [][2]string{
		{`height="17315"`, `height="14768"`}, // 장애사항 표
		{`height="8274"`, `height="6010"`},   // 장애사항 칸
		{`height="7376"`, `height="7093"`},   // 장애원인 칸
		{`height="17654"`, `height="12277"`}, // 조치사항 표
		{`height="16172"`, `height="10795"`}, // 조치내용 칸
		{`height="4193"`, `height="4828"`},   // 보고·사인 표 (칸 2414+2414)
	}
	for _, r := range repl {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return nudgeSignatureTable(s)
}

// nudgeSignatureTable 한글이 저장한 본(no_sig_사이즈 조정)의 마지막 표 자리. vertOffset 0 이면 사인 줄이 페이지 밖으로 밀린다.
func nudgeSignatureTable(s string) string {
	i := strings.LastIndex(s, "<hp:tbl")
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], "</hp:tbl>")
	if j < 0 {
		return s
	}
	chunk := s[i : i+j]
	n := strings.Replace(chunk, `vertOffset="0" horzOffset="0"`, `vertOffset="1977" horzOffset="0"`, 1)
	return s[:i] + n + s[i+j:]
}
