package leavecalc

import "testing"

func TestFormatDays(t *testing.T) {
	if FormatDays(0) != "0일" || FormatDays(480) != "1일" || FormatDays(240) != "0.5일" {
		t.Fatalf("%q %q %q", FormatDays(0), FormatDays(480), FormatDays(240))
	}
	if FormatDays(-960) != "-2일" {
		t.Fatalf("%q", FormatDays(-960))
	}
}
