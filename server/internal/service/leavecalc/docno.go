package leavecalc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var docNoPattern = regexp.MustCompile(`^(.+)-근태-(\d{6})-(\d+)$`)

// FormatDocNo builds {prefix}-근태-{YYMMDD}-{NNN}.
func FormatDocNo(prefix string, day time.Time, seq int) string {
	if prefix == "" {
		prefix = DefaultDocPrefix
	}
	if seq < 1 {
		seq = 1
	}
	d := dateOnly(day)
	return fmt.Sprintf("%s-%s-%s-%03d", prefix, DocKindAttendance, d.Format("060102"), seq)
}

// ParseDocNo splits a document number. ok=false if the format does not match.
func ParseDocNo(docNo string) (prefix, yymmdd string, seq int, ok bool) {
	m := docNoPattern.FindStringSubmatch(strings.TrimSpace(docNo))
	if m == nil {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n < 1 {
		return "", "", 0, false
	}
	return m[1], m[2], n, true
}

// NextDocSeq returns max matching seq for prefix+day among existing, plus 1.
// Empty day list → 1. Concurrency/UNIQUE is a later DB concern.
func NextDocSeq(prefix string, day time.Time, existing []string) int {
	if prefix == "" {
		prefix = DefaultDocPrefix
	}
	want := dateOnly(day).Format("060102")
	max := 0
	for _, raw := range existing {
		p, ymd, seq, ok := ParseDocNo(raw)
		if !ok || p != prefix || ymd != want {
			continue
		}
		if seq > max {
			max = seq
		}
	}
	return max + 1
}

// NextDocNo is FormatDocNo(prefix, day, NextDocSeq(...)).
func NextDocNo(prefix string, day time.Time, existing []string) string {
	return FormatDocNo(prefix, day, NextDocSeq(prefix, day, existing))
}
