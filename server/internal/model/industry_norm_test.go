package model

import "testing"

func TestIndustryKeySchoolAndLibraryAliases(t *testing.T) {
	if IndustryKey("school") != "학교" || IndustryKey("학교") != "학교" {
		t.Fatalf("school/학교 %q %q", IndustryKey("school"), IndustryKey("학교"))
	}
	if IndustryKey("library") != "도서관" || IndustryKey("도서관") != "도서관" {
		t.Fatalf("library/도서관")
	}
	if IndustryKey("public") != "공공기관" || IndustryKey("private") != "민간" {
		t.Fatalf("public/private")
	}
	if IndustryKey("") != IndustryUnspecified || IndustryKey("  ") != IndustryUnspecified {
		t.Fatalf("미지정 %q", IndustryKey(""))
	}
	raw := IndustryRawValues("학교")
	ok := false
	for _, v := range raw {
		if v == "school" || v == "학교" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("학교 원값 %v", raw)
	}
}
