package model

import (
	"reflect"
	"testing"
)

func TestParseIndustries95_219(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"도서관", []string{"도서관"}},
		{"school", []string{"학교"}},
		{"학교,도서관", []string{"도서관", "학교"}}, // 고정 순서
		{"도서관, 학교", []string{"도서관", "학교"}},
		{"학교/도서관", []string{"도서관", "학교"}},
		{"학교，도서관", []string{"도서관", "학교"}},
		{"library,school,private,public", []string{"도서관", "학교", "공공기관", "민간"}},
		{"도서관,도서관,library", []string{"도서관"}},
		{"교육청,도서관", []string{"도서관", "교육청"}}, // 목록 밖 값 보존(뒤에)
		{"교육청", []string{"교육청"}},
		{"학교;도서관", []string{"도서관", "학교"}},
	} {
		if got := ParseIndustries(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseIndustries(%q)=%v want %v", c.in, got, c.want)
		}
	}
	// 왕복: Join(Parse(x)) 는 같은 조합이면 같은 문자열
	if JoinIndustries([]string{"학교", "도서관"}) != "도서관,학교" || JoinIndustries(ParseIndustries("학교/도서관")) != "도서관,학교" {
		t.Error("조합이 같으면 같은 문자열이어야 한다")
	}
	if JoinIndustries(nil) != "" || JoinIndustries([]string{"도서관"}) != "도서관" {
		t.Error("빈 값·단일 값 모양이 기존과 같아야 한다")
	}
}

func TestHasIndustry95_219(t *testing.T) {
	for _, c := range []struct {
		raw, key string
		want     bool
	}{
		{"도서관,학교", "도서관", true},
		{"도서관,학교", "학교", true},
		{"도서관,학교", "공공기관", false},
		{"도서관", "library", true},
		{"학교", "공공", false}, // 「공공」은 「공공기관」이 아니다
		{"공공기관", "공공", false},
		{"", IndustryUnspecified, true},
		{"", "", true},
		{"도서관", IndustryUnspecified, false},
		{"교육청,도서관", "교육청", true},
	} {
		if got := HasIndustry(c.raw, c.key); got != c.want {
			t.Errorf("HasIndustry(%q,%q)=%v want %v", c.raw, c.key, got, c.want)
		}
	}
	if got := IndustryExtras("도서관,교육청,기타"); !reflect.DeepEqual(got, []string{"교육청", "기타"}) {
		t.Errorf("목록 밖 값 %v", got)
	}
}
