package model

import "testing"

func TestParseASPartsFromForm(t *testing.T) {
	rows, err := ParseASPartsFromForm(
		[]string{"메인보드", "팬"},
		[]string{"PT-M01", ""},
		[]string{"1", "2"},
		[]string{"", ""},
	)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %+v", err, rows)
	}
	if FormatPartsUsedSummary(rows) != "메인보드(PT-M01) ×1, 팬 ×2" {
		t.Fatalf("sum %s", FormatPartsUsedSummary(rows))
	}
	_, err = ParseASPartsFromForm([]string{""}, []string{"X"}, []string{"1"}, []string{""})
	if err == nil {
		t.Fatal("빈 품명")
	}
	names := make([]string, 31)
	for i := range names {
		names[i] = "a"
	}
	if _, err := ParseASPartsFromForm(names, nil, nil, nil); err == nil {
		t.Fatal("31행")
	}
}
