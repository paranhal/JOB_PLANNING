package model

import "testing"

func TestNormalizeWorkPlace(t *testing.T) {
	cases := map[string]string{
		"office": WorkPlaceOffice,
		"내근":     WorkPlaceOffice,
		"field":  WorkPlaceField,
		"외근":     WorkPlaceField,
		"remote": "",
		"":       "",
	}
	for in, want := range cases {
		if got := NormalizeWorkPlace(in); got != want {
			t.Errorf("NormalizeWorkPlace(%q)=%q want %q", in, got, want)
		}
	}
}

func TestWorkPlaceLabel(t *testing.T) {
	if WorkPlaceLabel("office") != "내근" {
		t.Fatal("office → 내근")
	}
	if WorkPlaceLabel("field") != "외근" {
		t.Fatal("field → 외근")
	}
	if WorkPlaceLabel("") != "" {
		t.Fatal("빈 값은 표시하지 않는다")
	}
}
